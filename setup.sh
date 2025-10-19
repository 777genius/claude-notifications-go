#!/bin/bash
# setup.sh - Verify notification plugin installation

set -e

echo "=========================================="
echo " Claude Notifications Plugin - Setup"
echo "=========================================="
echo ""

echo "Checking pre-built binaries..."
echo ""

# Check if wrapper exists
if [ ! -f "bin/claude-notifications" ]; then
    echo "❌ Error: bin/claude-notifications wrapper not found"
    echo ""
    echo "This file should be included in the repository."
    echo "If missing, run 'make build-all' to rebuild binaries."
    exit 1
fi

# Make wrapper executable
chmod +x bin/claude-notifications

# Check platform-specific binaries
PLATFORMS=(
    "darwin-amd64"
    "darwin-arm64"
    "linux-amd64"
    "linux-arm64"
    "windows-amd64.exe"
)

MISSING=0
FOUND=0

for platform in "${PLATFORMS[@]}"; do
    if [ -f "bin/claude-notifications-${platform}" ]; then
        FOUND=$((FOUND + 1))
        echo "  ✓ $platform"
    else
        MISSING=$((MISSING + 1))
        echo "  ⚠  $platform (missing)"
    fi
done

echo ""

if [ $MISSING -gt 0 ]; then
    echo "⚠️  Warning: $MISSING platform binary(ies) missing"
    echo ""
    echo "For development: Run 'make build-all' to rebuild (requires Go 1.21+)"
    echo "Or trigger GitHub Actions to build all platforms automatically"
    echo ""
fi

if [ $FOUND -eq 0 ]; then
    echo "❌ Error: No platform binaries found!"
    echo ""
    echo "Please run 'make build-all' or check GitHub Actions builds"
    exit 1
fi

echo "✓ Setup verified! Found $FOUND/$((FOUND + MISSING)) platform binaries"

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
