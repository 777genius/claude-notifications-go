package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/777genius/claude-notifications/internal/analyzer"
	"github.com/777genius/claude-notifications/internal/config"
	"github.com/777genius/claude-notifications/internal/logging"
	"github.com/google/uuid"
)

// Sender sends webhook notifications with professional patterns
type Sender struct {
	cfg            *config.Config
	client         *http.Client
	retry          *Retryer
	circuitBreaker *CircuitBreaker
	rateLimiter    *RateLimiter
	metrics        *Metrics
	formatters     map[string]Formatter

	// Graceful shutdown
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new professional webhook sender
func New(cfg *config.Config) *Sender {
	// Create base HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Parse retry config
	retryConfig := parseRetryConfig(cfg.Notifications.Webhook.Retry)
	retry := NewRetryer(retryConfig)

	// Parse circuit breaker config
	cbCfg := cfg.Notifications.Webhook.CircuitBreaker
	var circuitBreaker *CircuitBreaker
	if cbCfg.Enabled {
		timeout, _ := time.ParseDuration(cbCfg.Timeout)
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		circuitBreaker = NewCircuitBreaker(cbCfg.FailureThreshold, cbCfg.SuccessThreshold, timeout)
	}

	// Create rate limiter
	var rateLimiter *RateLimiter
	if cfg.Notifications.Webhook.RateLimit.Enabled {
		rateLimiter = NewRateLimiter(cfg.Notifications.Webhook.RateLimit.RequestsPerMinute)
	}

	// Create formatters
	formatters := map[string]Formatter{
		"slack":    &SlackFormatter{},
		"discord":  &DiscordFormatter{},
		"telegram": &TelegramFormatter{ChatID: cfg.Notifications.Webhook.ChatID},
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	return &Sender{
		cfg:            cfg,
		client:         client,
		retry:          retry,
		circuitBreaker: circuitBreaker,
		rateLimiter:    rateLimiter,
		metrics:        NewMetrics(),
		formatters:     formatters,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Send sends a webhook notification with full professional stack
func (s *Sender) Send(status analyzer.Status, message, sessionID string) error {
	if !s.cfg.IsWebhookEnabled() {
		logging.Debug("Webhooks disabled, skipping")
		return nil
	}

	// Check rate limit (non-blocking check)
	if s.rateLimiter != nil && !s.rateLimiter.Allow() {
		s.metrics.RecordRateLimited()
		logging.Warn("Rate limit exceeded, dropping webhook")
		return ErrRateLimitExceeded
	}

	// Check circuit breaker
	if s.circuitBreaker != nil && s.circuitBreaker.GetState() == StateOpen {
		s.metrics.RecordCircuitOpen()
		logging.Warn("Circuit breaker is open, skipping webhook")
		return ErrCircuitOpen
	}

	// Generate request ID for tracing
	requestID := uuid.New().String()

	// Record metrics
	s.metrics.RecordRequest()
	start := time.Now()

	// Execute with retry and circuit breaker
	err := s.sendWithRetryAndCircuitBreaker(requestID, status, message, sessionID)

	// Record result
	latency := time.Since(start)
	if err != nil {
		s.metrics.RecordFailure()
		logging.Error("[%s] Webhook failed after retries: %v (latency: %v)", requestID, err, latency)
	} else {
		s.metrics.RecordSuccess(status, latency)
		logging.Info("[%s] Webhook sent successfully (latency: %v)", requestID, latency)
	}

	// Update circuit breaker state in metrics
	if s.circuitBreaker != nil {
		s.metrics.UpdateCircuitBreakerState(s.circuitBreaker.GetState())
	}

	return err
}

// sendWithRetryAndCircuitBreaker executes the webhook with retry and circuit breaker
func (s *Sender) sendWithRetryAndCircuitBreaker(requestID string, status analyzer.Status, message, sessionID string) error {
	webhookCfg := s.cfg.Notifications.Webhook

	// Build payload
	payload, contentType, err := s.buildPayload(status, message, sessionID)
	if err != nil {
		return fmt.Errorf("failed to build payload: %w", err)
	}

	// Validate URL
	if err := validateURL(webhookCfg.URL); err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	// Create request function for retry
	sendFn := func(ctx context.Context) error {
		return s.sendHTTPRequest(ctx, requestID, webhookCfg.URL, payload, contentType, webhookCfg.Headers)
	}

	// Execute with circuit breaker and retry
	var executeErr error
	if s.circuitBreaker != nil {
		// Wrap with circuit breaker
		executeErr = s.circuitBreaker.Execute(s.ctx, func() error {
			// Execute with retry
			return s.retry.Do(s.ctx, sendFn)
		})
	} else {
		// Just retry without circuit breaker
		executeErr = s.retry.Do(s.ctx, sendFn)
	}

	return executeErr
}

// buildPayload builds the webhook payload based on preset
func (s *Sender) buildPayload(status analyzer.Status, message, sessionID string) ([]byte, string, error) {
	webhookCfg := s.cfg.Notifications.Webhook
	statusInfo, _ := s.cfg.GetStatusInfo(string(status))

	// Use formatter if available
	if formatter, ok := s.formatters[webhookCfg.Preset]; ok {
		payload, err := formatter.Format(status, message, sessionID, statusInfo)
		if err != nil {
			return nil, "", err
		}
		data, err := json.Marshal(payload)
		return data, "application/json", err
	}

	// Fallback to custom format
	return s.buildCustomPayload(status, message, sessionID, webhookCfg.Format, statusInfo)
}

// buildCustomPayload builds a custom webhook payload
func (s *Sender) buildCustomPayload(status analyzer.Status, message, sessionID, format string, statusInfo config.StatusInfo) ([]byte, string, error) {
	if format == "text" {
		text := fmt.Sprintf("[%s] %s", status, message)
		return []byte(text), "text/plain", nil
	}

	// JSON format
	payload := map[string]interface{}{
		"status":     string(status),
		"message":    message,
		"timestamp":  time.Now().Format(time.RFC3339),
		"session_id": sessionID,
		"source":     "claude-notifications",
		"title":      statusInfo.Title,
	}

	data, err := json.Marshal(payload)
	return data, "application/json", err
}

// sendHTTPRequest sends the actual HTTP request
func (s *Sender) sendHTTPRequest(ctx context.Context, requestID, url string, payload []byte, contentType string, headers map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "claude-notifications/1.0")
	req.Header.Set("X-Request-ID", requestID)

	// Set custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body (limited to 1MB)
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return NewHTTPError(resp, string(body))
	}

	return nil
}

// SendAsync sends a webhook asynchronously with graceful shutdown support
func (s *Sender) SendAsync(status analyzer.Status, message, sessionID string) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		if err := s.Send(status, message, sessionID); err != nil {
			logging.Error("Async webhook send failed: %v", err)
		}
	}()
}

// Shutdown gracefully shuts down the webhook sender
// Waits for in-flight requests to complete (with timeout)
func (s *Sender) Shutdown(timeout time.Duration) error {
	logging.Info("Shutting down webhook sender...")

	// Cancel context
	s.cancel()

	// Wait for in-flight requests with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logging.Info("All webhook requests completed")
		return nil
	case <-time.After(timeout):
		logging.Warn("Webhook shutdown timeout, some requests may be incomplete")
		return fmt.Errorf("shutdown timeout after %v", timeout)
	}
}

// GetMetrics returns current metrics
func (s *Sender) GetMetrics() Stats {
	return s.metrics.GetStats()
}

// Helper functions

// parseRetryConfig converts config.RetryConfig to webhook.RetryConfig
func parseRetryConfig(cfg config.RetryConfig) RetryConfig {
	initialBackoff, _ := time.ParseDuration(cfg.InitialBackoff)
	if initialBackoff == 0 {
		initialBackoff = 1 * time.Second
	}

	maxBackoff, _ := time.ParseDuration(cfg.MaxBackoff)
	if maxBackoff == 0 {
		maxBackoff = 10 * time.Second
	}

	return RetryConfig{
		Enabled:        cfg.Enabled,
		MaxAttempts:    cfg.MaxAttempts,
		InitialBackoff: initialBackoff,
		MaxBackoff:     maxBackoff,
		Multiplier:     2.0,
	}
}

// validateURL validates the webhook URL
func validateURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL is empty")
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("URL must use http or https scheme")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}
