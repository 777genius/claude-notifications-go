---
description: Download notification binary for claude-notifications plugin
disable-model-invocation: true
allowed-tools: Bash
---

# 📥 Initialize Agent Notifications Binary

This command downloads the notification binary for your platform (macOS, Linux, or Windows).

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## Download Binary

Downloading the notification binary for your platform...

```bash
# Get plugin root directory
PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT}"
if [ -z "$PLUGIN_ROOT" ]; then
  INSTALLED_PATH="$HOME/.claude/plugins/marketplaces/claude-notifications-go"
  if [ -d "$INSTALLED_PATH" ]; then
    PLUGIN_ROOT="$INSTALLED_PATH"
  else
    PLUGIN_ROOT="$(pwd)"
  fi
fi

echo "Plugin root: $PLUGIN_ROOT"
echo ""

# Always download the latest install.sh from GitHub to ensure we have newest version
INSTALL_SCRIPT_URL="https://raw.githubusercontent.com/777genius/agent-notifications/main/bin/install.sh"
# Use portable temp directory (works on macOS, Linux, Windows Git Bash)
TEMP_DIR="${TMPDIR:-${TEMP:-/tmp}}"
TEMP_INSTALL_SCRIPT="${TEMP_DIR}/claude-notifications-install-$$.sh"

echo "📥 Fetching latest installer from GitHub..."
if curl -fsSL "$INSTALL_SCRIPT_URL" -o "$TEMP_INSTALL_SCRIPT" 2>/dev/null; then
  chmod +x "$TEMP_INSTALL_SCRIPT"
  echo "✓ Latest installer downloaded"
  echo ""

  # Run with --force to always update binaries
  # Set INSTALL_TARGET_DIR so install.sh knows where to put binaries
  INSTALL_TARGET_DIR="${PLUGIN_ROOT}/bin" bash "$TEMP_INSTALL_SCRIPT" --force
  RESULT=$?

  rm -f "$TEMP_INSTALL_SCRIPT"

  if [ $RESULT -ne 0 ]; then
    echo ""
    echo "❌ Error: Installation failed"
    exit 1
  fi
else
  echo "⚠ Could not download latest installer, using cached version..."
  if ! bash "${PLUGIN_ROOT}/bin/install.sh" --force; then
    echo ""
    echo "❌ Error: Failed to install notification binary"
    exit 1
  fi
fi

echo ""
echo "✅ Binary installed successfully!"
echo ""
echo "Next steps:"
echo "  Run /claude-notifications-go:settings to configure sounds and notifications"
```

This will automatically download the correct binary for your platform from GitHub Releases. Running this command again will update all binaries to the latest version.

**Supported platforms:**
- macOS (Intel & Apple Silicon)
- Linux (x64 & ARM64)
- Windows (x64)
