---
description: Download notification binary for claude-notifications plugin
allowed-tools: Bash
---

# 📥 Initialize Claude Notifications Binary

This command downloads the notification binary for your platform (macOS, Linux, or Windows).

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## Download Binary

Downloading the notification binary for your platform...

```bash
# Get plugin root directory
# Priority: 1) CLAUDE_PLUGIN_ROOT env var, 2) installed plugin location, 3) current directory
PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT}"
if [ -z "$PLUGIN_ROOT" ]; then
  # Try the standard installed plugin location
  INSTALLED_PATH="$HOME/.claude/plugins/marketplaces/claude-notifications-go"
  if [ -d "$INSTALLED_PATH" ]; then
    PLUGIN_ROOT="$INSTALLED_PATH"
  else
    # Fallback to current directory (for development)
    PLUGIN_ROOT="$(pwd)"
  fi
fi

echo "Plugin root: $PLUGIN_ROOT"
echo ""

# Run the installer to download the binary for your platform
echo "Downloading notification binary from GitHub Releases..."
if ! "${PLUGIN_ROOT}/bin/install.sh"; then
  echo ""
  echo "❌ Error: Failed to install notification binary"
  echo ""
  echo "Possible causes:"
  echo "  • No internet connection"
  echo "  • GitHub is unreachable"
  echo "  • Unsupported platform"
  echo ""
  echo "Please check your connection and try again."
  exit 1
fi

echo ""
echo "✅ Binary installed successfully!"
echo ""
echo "Next steps:"
echo "  Run /claude-notifications-go:notifications-settings to configure sounds and notifications"
```

This will automatically download the correct binary for your platform from GitHub Releases with a progress bar. The binary is cached locally - subsequent runs will skip the download if already installed.

**Supported platforms:**
- macOS (Intel & Apple Silicon)
- Linux (x64)
- Windows (x64)
