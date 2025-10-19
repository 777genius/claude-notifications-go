# Custom Webhook Integration

Integrate Claude Code notifications with any webhook-compatible service.

## Overview

Custom webhooks allow you to send notifications to any HTTP endpoint that accepts JSON payloads. This is useful for:
- Custom internal APIs
- Webhook services (Zapier, n8n, Make)
- Monitoring systems (PagerDuty, Datadog)
- Chat platforms not supported by presets
- Custom notification routing

## Basic Setup

### Configuration

Edit `config/config.json`:

```json
{
  "notifications": {
    "webhook": {
      "enabled": true,
      "preset": "",
      "url": "https://your-webhook-endpoint.com/notifications",
      "format": "json"
    }
  }
}
```

**Key fields:**
- `preset: ""` - Empty string for custom webhooks
- `url` - Your webhook endpoint
- `format: "json"` - Payload format (currently only JSON supported)

### Payload Format

Custom webhooks receive a JSON payload:

```json
{
  "status": "task_complete",
  "message": "[bold-cat] Created new authentication system with JWT tokens",
  "session_id": "abc-123",
  "timestamp": 1729353045
}
```

**Fields:**
- `status` (string) - One of: `task_complete`, `review_complete`, `question`, `plan_ready`, `session_limit_reached`
- `message` (string) - Notification message with session name
- `session_id` (string) - Unique session identifier
- `timestamp` (integer) - Unix timestamp (seconds since epoch)

## Authentication

### Bearer Token

```json
{
  "notifications": {
    "webhook": {
      "enabled": true,
      "preset": "",
      "url": "https://api.yourservice.com/webhooks/claude",
      "format": "json",
      "headers": {
        "Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
      }
    }
  }
}
```

### API Key

```json
{
  "notifications": {
    "webhook": {
      "enabled": true,
      "preset": "",
      "url": "https://api.yourservice.com/webhooks",
      "format": "json",
      "headers": {
        "X-API-Key": "your-api-key-here",
        "X-Service-Name": "claude-notifications"
      }
    }
  }
}
```

### Basic Auth

Encode credentials as `base64(username:password)`:

```bash
echo -n "username:password" | base64
# Output: dXNlcm5hbWU6cGFzc3dvcmQ=
```

```json
{
  "notifications": {
    "webhook": {
      "enabled": true,
      "preset": "",
      "url": "https://api.yourservice.com/webhooks",
      "format": "json",
      "headers": {
        "Authorization": "Basic dXNlcm5hbWU6cGFzc3dvcmQ="
      }
    }
  }
}
```

### Multiple Headers

You can include multiple custom headers:

```json
{
  "headers": {
    "Authorization": "Bearer YOUR_TOKEN",
    "X-API-Key": "your-api-key",
    "X-Service-Name": "claude-notifications",
    "X-Environment": "production",
    "Content-Type": "application/json"
  }
}
```

**Note:** `Content-Type: application/json` is automatically added, you don't need to include it.

## Configuration Examples

### Minimal Configuration

```json
{
  "notifications": {
    "webhook": {
      "enabled": true,
      "preset": "",
      "url": "https://webhook.site/unique-url-here",
      "format": "json"
    }
  }
}
```

### With Authentication

```json
{
  "notifications": {
    "webhook": {
      "enabled": true,
      "preset": "",
      "url": "https://api.yourservice.com/webhooks/claude",
      "format": "json",
      "headers": {
        "Authorization": "Bearer YOUR_TOKEN",
        "X-API-Key": "your-api-key-here"
      }
    }
  }
}
```

### High-Throughput Configuration

For environments with frequent notifications:

```json
{
  "notifications": {
    "webhook": {
      "enabled": true,
      "preset": "",
      "url": "https://api.yourservice.com/webhooks/claude",
      "format": "json",
      "headers": {
        "Authorization": "Bearer YOUR_TOKEN"
      },
      "retry": {
        "enabled": true,
        "maxAttempts": 2,
        "initialBackoff": "500ms",
        "maxBackoff": "2s"
      },
      "circuitBreaker": {
        "enabled": true,
        "failureThreshold": 10,
        "successThreshold": 5,
        "timeout": "10s"
      },
      "rateLimit": {
        "enabled": true,
        "requestsPerMinute": 120
      }
    }
  }
}
```

### Production Configuration

Balanced for reliability and performance:

```json
{
  "notifications": {
    "webhook": {
      "enabled": true,
      "preset": "",
      "url": "https://api.yourservice.com/webhooks/claude",
      "format": "json",
      "headers": {
        "Authorization": "Bearer YOUR_TOKEN",
        "X-Service-Name": "claude-notifications",
        "X-Environment": "production"
      },
      "retry": {
        "enabled": true,
        "maxAttempts": 5,
        "initialBackoff": "2s",
        "maxBackoff": "30s"
      },
      "circuitBreaker": {
        "enabled": true,
        "failureThreshold": 10,
        "successThreshold": 3,
        "timeout": "60s"
      },
      "rateLimit": {
        "enabled": true,
        "requestsPerMinute": 60
      }
    }
  }
}
```

## Common Integrations

### Zapier

1. Create a **Webhook by Zapier** trigger
2. Copy the webhook URL
3. Configure Claude Notifications:

```json
{
  "webhook": {
    "enabled": true,
    "preset": "",
    "url": "https://hooks.zapier.com/hooks/catch/YOUR_HOOK_ID/",
    "format": "json"
  }
}
```

### n8n

1. Add **Webhook** node to workflow
2. Set method to `POST`
3. Copy webhook URL
4. Configure Claude Notifications with the URL

### Make (formerly Integromat)

1. Create scenario with **Webhooks** module
2. Add **Custom webhook**
3. Copy webhook URL
4. Configure Claude Notifications with the URL

### PagerDuty

Use PagerDuty's Events API v2:

```json
{
  "webhook": {
    "enabled": true,
    "preset": "",
    "url": "https://events.pagerduty.com/v2/enqueue",
    "format": "json",
    "headers": {
      "Authorization": "Token token=YOUR_INTEGRATION_KEY"
    }
  }
}
```

**Note:** You'll need to transform the payload format. Consider using Zapier/n8n as middleware.

### Microsoft Teams

Teams uses a different JSON structure. Consider using the webhook as a bridge:

```json
{
  "webhook": {
    "enabled": true,
    "preset": "",
    "url": "https://your-middleware.com/teams-adapter",
    "format": "json"
  }
}
```

Your middleware should transform the payload to Teams' format.

## Testing

### webhook.site

Perfect for testing webhook payloads:

1. Go to https://webhook.site/
2. Copy your unique URL
3. Configure Claude Notifications:

```json
{
  "webhook": {
    "url": "https://webhook.site/your-unique-url"
  }
}
```

4. Trigger a notification
5. Check webhook.site for the payload

### RequestBin

Alternative to webhook.site:

1. Go to https://requestbin.com/
2. Create a request bin
3. Use the endpoint URL in your config
4. View captured requests

### curl Testing

Test your endpoint manually:

```bash
curl -X POST https://your-webhook-endpoint.com/notifications \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "status": "task_complete",
    "message": "[test-session] Test notification",
    "session_id": "test-123",
    "timestamp": 1729353045
  }'
```

## Response Handling

### Success (2xx)

Any HTTP status code in the 2xx range is considered successful:
- `200 OK`
- `201 Created`
- `202 Accepted`
- `204 No Content`

### Retry (5xx, 429)

These status codes trigger automatic retry:
- `500 Internal Server Error`
- `502 Bad Gateway`
- `503 Service Unavailable`
- `504 Gateway Timeout`
- `429 Too Many Requests`

### No Retry (4xx)

Client errors don't trigger retry (except 429):
- `400 Bad Request` - Check payload format
- `401 Unauthorized` - Check authentication
- `403 Forbidden` - Check permissions
- `404 Not Found` - Check URL

## Troubleshooting

### Webhooks Not Arriving

1. **Test endpoint with curl** (see Testing section)
2. **Check logs:**
   ```bash
   tail -f notification-debug.log | grep webhook
   ```
3. **Verify URL** - Check for typos
4. **Test with webhook.site** - Isolate issue

### Authentication Failing

1. **Check header format:**
   - Bearer: `Authorization: Bearer TOKEN`
   - API Key: `X-API-Key: KEY`
   - Basic: `Authorization: Basic BASE64`

2. **Verify credentials** - Test with curl

3. **Check token expiration** - Refresh if needed

### Timeout Errors

1. **Check endpoint response time** - Should respond < 10s
2. **Increase retry backoff:**
   ```json
   {
     "retry": {
       "initialBackoff": "5s",
       "maxBackoff": "30s"
     }
   }
   ```

### Rate Limiting

If you're hitting rate limits:

```json
{
  "webhook": {
    "rateLimit": {
      "enabled": true,
      "requestsPerMinute": 30
    }
  }
}
```

## Best Practices

1. **Always use HTTPS** - Never use plain HTTP for webhooks
2. **Implement authentication** - Use API keys or bearer tokens
3. **Enable retry** - Handle transient failures
4. **Monitor webhook health** - Track success rates
5. **Use circuit breaker** - Prevent cascading failures
6. **Rate limit appropriately** - Match your endpoint's capacity
7. **Test thoroughly** - Use webhook.site before production
8. **Rotate credentials** - Update tokens/keys regularly
9. **Log webhook activity** - Track all requests/responses
10. **Handle all status codes** - Return appropriate HTTP codes

## Security Considerations

- **Use HTTPS only** - Encrypt data in transit
- **Authenticate requests** - Verify sender identity
- **Validate payloads** - Check structure and content
- **Rotate credentials** - Change tokens periodically
- **Limit IP access** - Whitelist if possible
- **Monitor for abuse** - Watch for unusual patterns
- **Use short-lived tokens** - JWT with expiration
- **Log all requests** - Audit trail for security

## Learn More

- [Configuration Options](configuration.md) - Retry, circuit breaker, rate limiting
- [Monitoring](monitoring.md) - Metrics and debugging
- [Troubleshooting](troubleshooting.md) - Common issues

---

[← Back to Webhook Overview](README.md)
