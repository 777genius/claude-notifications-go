#!/bin/bash
# setup.sh - Verify notification plugin installation

set -e

echo "=========================================="
echo " Claude Notifications Plugin - Setup"
echo "=========================================="
echo ""

# Check if wrapper script exists
if [ ! -f "bin/claude-notifications" ]; then
    echo "❌ Error: bin/claude-notifications wrapper not found"
    echo ""
    echo "This file should be included in the repository."
    exit 1
fi

# Check if installer exists
if [ ! -f "bin/install.sh" ]; then
    echo "❌ Error: bin/install.sh installer not found"
    echo ""
    echo "This file should be included in the repository."
    exit 1
fi

# Make scripts executable
chmod +x bin/claude-notifications
chmod +x bin/install.sh

echo "✓ Plugin scripts verified"
echo ""
echo "=========================================="
echo " Setup Complete!"
echo "=========================================="
echo ""
echo "Next steps:"
echo ""
echo "1. Add marketplace to Claude Code:"
echo "   /plugin marketplace add 777genius/claude-notifications-go"
echo ""
echo "2. Install plugin:"
echo "   /plugin install claude-notifications-go@claude-notifications-go"
echo ""
echo "3. Restart Claude Code for hooks to take effect"
echo ""
echo "4. Run the interactive setup wizard:"
echo "   /setup-notifications"
echo ""
echo "   This will:"
echo "   - Download the binary for your platform (first time only)"
echo "   - Configure notification sounds"
echo "   - Set up webhooks (optional)"
echo ""
echo "Note: The binary will be downloaded automatically when you"
echo "      run /setup-notifications for the first time."
echo ""
