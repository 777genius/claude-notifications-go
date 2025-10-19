#!/bin/bash
# setup.sh - Build notification plugin binary

set -e

echo "=========================================="
echo " Claude Notifications Plugin - Setup"
echo "=========================================="
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed."
    echo ""
    echo "Please install Go 1.21 or later from:"
    echo "  https://golang.org/dl/"
    echo ""
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "✓ Go version: $GO_VERSION"

# Build binary
echo ""
echo "Building claude-notifications binary..."
mkdir -p bin

if go build -o bin/claude-notifications ./cmd/claude-notifications; then
    echo "✓ Build successful!"
else
    echo "❌ Build failed"
    exit 1
fi

# Make binary executable
chmod +x bin/claude-notifications

# Show binary info
BINARY_SIZE=$(ls -lh bin/claude-notifications | awk '{print $5}')
echo "✓ Binary created: bin/claude-notifications ($BINARY_SIZE)"

echo ""
echo "=========================================="
echo " Setup Complete!"
echo "=========================================="
echo ""
echo "Next steps:"
echo ""
echo "1. Add marketplace to Claude Code:"
echo "   /plugin marketplace add $(pwd)"
echo ""
echo "2. Install plugin:"
echo "   /plugin install claude-notifications-go@local-dev"
echo ""
echo "3. Restart Claude Code for hooks to take effect"
echo ""
echo "4. (Optional) Test notification:"
echo "   echo '{\"session_id\":\"test\",\"tool_name\":\"ExitPlanMode\"}' | \\"
echo "     bin/claude-notifications handle-hook PreToolUse"
echo ""
