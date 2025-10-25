.PHONY: build test test-race lint clean install help

# Binary name
BINARY=claude-notifications
BINARY_PATH=bin/$(BINARY)

# Build flags
# Development build: includes debug symbols for debugging
# Production build: optimized for size and deployment
RELEASE_FLAGS=-ldflags="-s -w" -trimpath

# Build targets
build: ## Build the binary (development mode with debug symbols)
	@echo "Building $(BINARY) (development mode)..."
	@go build -o $(BINARY_PATH) ./cmd/claude-notifications

build-all: ## Build optimized binaries for all platforms
	@echo "Building optimized release binaries for all platforms..."
	@mkdir -p dist
	@GOOS=darwin GOARCH=amd64 go build $(RELEASE_FLAGS) -o dist/$(BINARY)-darwin-amd64 ./cmd/claude-notifications
	@GOOS=darwin GOARCH=arm64 go build $(RELEASE_FLAGS) -o dist/$(BINARY)-darwin-arm64 ./cmd/claude-notifications
	@GOOS=linux GOARCH=amd64 go build $(RELEASE_FLAGS) -o dist/$(BINARY)-linux-amd64 ./cmd/claude-notifications
	@GOOS=linux GOARCH=arm64 go build $(RELEASE_FLAGS) -o dist/$(BINARY)-linux-arm64 ./cmd/claude-notifications
	@GOOS=windows GOARCH=amd64 go build $(RELEASE_FLAGS) -o dist/$(BINARY)-windows-amd64.exe ./cmd/claude-notifications
	@echo "Build complete! Optimized binaries in dist/"

# Test targets
test: ## Run tests
	@echo "Running tests..."
	@go test -v -cover ./...

test-race: ## Run tests with race detection
	@echo "Running tests with race detection..."
	@go test -v -race -cover ./...

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.txt -covermode=atomic ./...
	@go tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Linting
lint: ## Run linter
	@echo "Running linter..."
	@go vet ./...
	@go fmt ./...

# Installation
install: build ## Install binary to /usr/local/bin
	@echo "Installing $(BINARY) to /usr/local/bin..."
	@cp $(BINARY_PATH) /usr/local/bin/$(BINARY)
	@echo "Installation complete!"

# Cleanup
clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf bin/ dist/ coverage.* *.log
	@echo "Clean complete!"

# Rebuild and prepare for commit
rebuild-and-commit: build-all ## Rebuild optimized binaries and prepare for commit
	@echo "Moving optimized binaries to bin/..."
	@cp dist/* bin/ 2>/dev/null || true
	@rm -rf dist
	@echo "✓ Optimized binaries ready in bin/"
	@echo ""
	@echo "Platform binaries updated:"
	@ls -1 bin/claude-notifications-* 2>/dev/null || echo "  (none found)"
	@echo ""
	@echo "To commit: git add bin/claude-notifications-* && git commit -m 'chore: rebuild binaries'"

# Help
help: ## Show this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
