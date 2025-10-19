.PHONY: build test test-race lint clean install help

# Binary name
BINARY=claude-notifications
BINARY_PATH=bin/$(BINARY)

# Build targets
build: ## Build the binary
	@echo "Building $(BINARY)..."
	@go build -o $(BINARY_PATH) ./cmd/claude-notifications

build-all: ## Build for all platforms
	@echo "Building for all platforms..."
	@mkdir -p dist
	@GOOS=darwin GOARCH=amd64 go build -o dist/$(BINARY)-darwin-amd64 ./cmd/claude-notifications
	@GOOS=darwin GOARCH=arm64 go build -o dist/$(BINARY)-darwin-arm64 ./cmd/claude-notifications
	@GOOS=linux GOARCH=amd64 go build -o dist/$(BINARY)-linux-amd64 ./cmd/claude-notifications
	@GOOS=linux GOARCH=arm64 go build -o dist/$(BINARY)-linux-arm64 ./cmd/claude-notifications
	@GOOS=windows GOARCH=amd64 go build -o dist/$(BINARY)-windows-amd64.exe ./cmd/claude-notifications
	@echo "Build complete! Binaries in dist/"

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

# Help
help: ## Show this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
