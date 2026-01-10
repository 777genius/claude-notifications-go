# Claude Notifications (plugin)

[![Ubuntu CI](https://github.com/777genius/claude-notifications-go/workflows/Ubuntu%20CI/badge.svg)](https://github.com/777genius/claude-notifications-go/actions)
[![macOS CI](https://github.com/777genius/claude-notifications-go/workflows/macOS%20CI/badge.svg)](https://github.com/777genius/claude-notifications-go/actions)
[![Windows CI](https://github.com/777genius/claude-notifications-go/workflows/Windows%20CI/badge.svg)](https://github.com/777genius/claude-notifications-go/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/777genius/claude-notifications-go)](https://goreportcard.com/report/github.com/777genius/claude-notifications-go)
[![codecov](https://codecov.io/gh/777genius/claude-notifications-go/branch/main/graph/badge.svg)](https://codecov.io/gh/777genius/claude-notifications-go)

<img width="250" height="350" alt="image" src="https://github.com/user-attachments/assets/e7aa6d8e-5d28-48f7-bafe-ad696857b938" />
<img width="350" height="239" alt="image" src="https://github.com/user-attachments/assets/42b7a306-f56f-4499-94cf-f3d573416b6d" />
<img width="220" alt="image" src="https://github.com/user-attachments/assets/4b5929d8-1a51-4a15-a3d5-dda5482554cc" />


Smart notifications for Claude Code with click-to-focus, git branch display, and webhook integrations.

## Table of Contents

  - [Supported Notification Types](#supported-notification-types)
  - [Installation](#installation)
    - [Prerequisites](#prerequisites)
    - [Install from GitHub](#install-from-github)
    - [Updating](#updating)
  - [Features](#features)
    - [🖥️ Cross-Platform Support](#️-cross-platform-support)
    - [🧠 Smart Detection](#-smart-detection)
    - [🔔 Flexible Notifications](#-flexible-notifications)
    - [🔊 Audio Customization](#-audio-customization)
    - [🌐 Enterprise-Grade Webhooks](#-enterprise-grade-webhooks)
    - [🛠️ Developer Experience](#️-developer-experience)
    - [🤝 Plugin Compatibility](#-plugin-compatibility)
  - [Platform Support](#platform-support)
    - [macOS Click-to-Focus](#macos-click-to-focus)
  - [Quick Start](#quick-start)
    - [Interactive Setup (Recommended)](#interactive-setup-recommended)
    - [Manual Configuration](#manual-configuration)
    - [Sound Options](#sound-options)
    - [Test Sound Playback](#test-sound-playback)
  - [Architecture](#architecture)
  - [Usage](#usage)
  - [Development](#development)
    - [Local installation for development](#local-installation-for-development)
    - [Building binaries](#building-binaries)
  - [Testing](#testing)
  - [Documentation](#documentation)
  - [License](#license)

## Supported Notification Types

| Status | Icon | Description | Trigger |
|--------|------|-------------|---------|
| Task Complete | ✅ | Main task completed | Stop/SubagentStop hooks (state machine detects active tools like Write/Edit/Bash, or ExitPlanMode followed by tool usage) |
| Review Complete | 🔍 | Code review finished | Stop/SubagentStop hooks (state machine detects only read-like tools: Read/Grep/Glob with no active tools, plus long text response >200 chars) |
| Question | ❓ | Claude has a question | PreToolUse hook (AskUserQuestion) OR Notification hook |
| Plan Ready | 📋 | Plan ready for approval | PreToolUse hook (ExitPlanMode) |
| Session Limit Reached | ⏱️ | Session limit reached | Stop/SubagentStop hooks (state machine detects "Session limit reached" text in last 3 assistant messages) |
| API Error: 401 | 🔴 | Authentication expired | Stop/SubagentStop hooks (state machine detects "API Error: 401" and "Please run /login" in last 3 assistant messages) |


## Installation

### Prerequisites

- Claude Code (tested on v2.0.15)
- **Windows users:** Git Bash (included with [Git for Windows](https://git-scm.com/download/win)) or WSL
- **macOS/Linux users:** No additional software required

### Install from GitHub

```bash
# 1) Add marketplace
/plugin marketplace add 777genius/claude-notifications-go
# 2) Install plugin
/plugin install claude-notifications-go@claude-notifications-go
# 3) Restart Claude Code
# 4) Init
/claude-notifications-go:notifications-init

# Optional
# Configure sounds and settings
/claude-notifications-go:notifications-settings
```

**That's it!**

1. `/claude-notifications-go:notifications-init` downloads the correct binary for your platform (macOS/Linux/Windows) from GitHub Releases
2. `/claude-notifications-go:notifications-settings` guides you through sound configuration with an interactive wizard

The binary is downloaded once and cached locally. You can re-run `/claude-notifications-go:notifications-settings` anytime to reconfigure.

### Updating

To update to the latest version:

1. Run `/plugin`, select **Marketplaces**, choose `claude-notifications-go`, then select **Update marketplace**
2. Select **Installed**, choose `claude-notifications-go`, then select **Update now**
3. Run `/claude-notifications-go:notifications-init` to download new binaries
4. Restart Claude Code to apply hook changes

Your `config.json` settings will be preserved during the update.


## Features

### 🖥️ Cross-Platform Support
- **macOS** (Intel & Apple Silicon), **Linux** (x64 & ARM64), **Windows 10+** (x64)
- Works in PowerShell, CMD, Git Bash, or WSL
- Pre-built binaries included - no compilation needed

### 🧠 Smart Detection
- **Operations count** File edits, file creates, ran commans + total time
- **State machine analysis** with temporal locality for accurate status detection
- **6 notification types**: Task Complete, Review Complete, Question, Plan Ready, Session Limit, API Error
- **PreToolUse integration** for instant alerts when Claude asks questions or creates plans
- Analyzes conversation context to avoid false positives

### 🔔 Flexible Notifications
- **Desktop notifications** with custom icons and sounds
- **Click-to-focus** (macOS): Click notification to activate your terminal window
- **Git branch in title**: See current branch like `✅ Completed [bold-cat] main`
- **Webhook integrations**: Slack, Discord, Telegram, Lark/Feishu, and custom endpoints
- **Session names**: Friendly identifiers like `[bold-cat]` for multi-session tracking
- **Cooldown system** to prevent notification spam

### 🔊 Audio Customization
- **Multi-format support**: MP3, WAV, FLAC, OGG, AIFF
- **Volume control**: 0-100% customizable volume
- **Audio device selection**: Route notifications to a specific output device (e.g., "MacBook Pro-Lautsprecher")
- **Built-in sounds**: Professional notification sounds included
- **System sounds**: Use macOS/Linux system sounds (optional)
- **Sound preview**: Test sounds before choosing with `/claude-notifications-go:notifications-settings`

### 🌐 Enterprise-Grade Webhooks
- **Retry logic** with exponential backoff
- **Circuit breaker** for fault tolerance
- **Rate limiting** with token bucket algorithm
- **Rich formatting** with platform-specific embeds/attachments
- **Request tracing** and performance metrics
- **→ [Complete Webhook Documentation](docs/webhooks/README.md)**

### 🛠️ Developer Experience
- **Interactive setup wizards**: `/claude-notifications-go:notifications-init` for binary setup, `/claude-notifications-go:notifications-settings` for configuration
- **JSONL streaming parser** for efficient large file processing
- **Comprehensive testing**: Unit tests with race detection
- **Two-phase lock deduplication** prevents duplicate notifications
- **Structured logging** to `notification-debug.log` for troubleshooting

**Notes:**
- **PreToolUse hooks** trigger instantly when Claude is about to use ExitPlanMode or AskUserQuestion tools
- **Stop/SubagentStop hooks** analyze the conversation transcript using a state machine to determine the task status
- **Notification hook** is triggered when Claude needs user input (permission dialogs, questions)
- The state machine uses temporal locality (last 15 messages) and tool analysis to accurately detect task completion

### 🤝 Plugin Compatibility

Compatible with other Claude Code plugins that spawn background Claude instances:

- **[double-shot-latte](https://github.com/obra/double-shot-latte)** - Auto-continue plugin that uses a background Claude instance for context evaluation. Notifications are automatically suppressed for the background judge process (via `CLAUDE_HOOK_JUDGE_MODE=true` environment variable).

If you're developing a plugin that spawns background Claude instances and want to suppress notifications, set `CLAUDE_HOOK_JUDGE_MODE=true` in the environment before invoking Claude.

## Platform Support

**Supported platforms:**
- macOS (Intel & Apple Silicon)
- Linux (x64 & ARM64)
- Windows 10+ (x64)

**No additional dependencies:**
- ✅ Binaries auto-download from GitHub Releases
- ✅ Pure Go - no C compiler needed
- ✅ All libraries bundled
- ✅ Works offline after first setup

**Windows-specific features:**
- Native Toast notifications (Windows 10+)
- Works in PowerShell, CMD, Git Bash, or WSL
- MP3/WAV/OGG/FLAC audio playback via native Windows APIs
- System sounds not accessible - use built-in MP3s or custom files

### macOS Click-to-Focus

On macOS, clicking a notification will activate your terminal window - no more hunting for the right window!

**How it works:**
- Automatically detects your terminal (iTerm2, Warp, Terminal.app, kitty, Ghostty, WezTerm, Alacritty)
- Uses `terminal-notifier` (auto-installed via `/claude-notifications-go:notifications-init`)
- Falls back to standard notifications if terminal-notifier is unavailable

**Configuration** (in `config/config.json`):
```json
{
  "notifications": {
    "desktop": {
      "clickToFocus": true,
      "terminalBundleId": ""
    }
  }
}
```

| Option | Default | Description |
|--------|---------|-------------|
| `clickToFocus` | `true` | Enable click-to-focus on macOS |
| `terminalBundleId` | `""` | Override auto-detected terminal. Use bundle ID like `com.googlecode.iterm2` |

**Supported terminals (auto-detected):**
- Terminal.app, iTerm2, Warp, kitty, Ghostty, WezTerm, Alacritty, Hyper, VS Code

To find your terminal's bundle ID: `osascript -e 'id of app "YourTerminal"'`

## Quick Start

### Interactive Setup (Recommended)

First, download the notification binary:

```
/claude-notifications-go:notifications-init
```

Then configure your notification sounds:

```
/claude-notifications-go:notifications-settings
```

This will:
- ✅ Show available built-in and system sounds
- 🔊 Let you preview sounds before choosing
- 📝 Create config.json with your preferences
- ✅ Test your setup when complete

**Features:**
- Preview sounds: Type `"play Glass"` or `"preview task-complete"`
- Choose from built-in MP3s or system sounds (macOS/Linux)
- Configure webhooks (optional)
- Interactive questions with AskUserQuestion tool

### Manual Configuration

Alternatively, edit `config/config.json` directly:

```json
{
  "notifications": {
    "desktop": {
      "enabled": true,
      "sound": true,
      "volume": 1.0,
      "audioDevice": "",
      "appIcon": "${CLAUDE_PLUGIN_ROOT}/claude_icon.png"
    },
    "webhook": {
      "enabled": false,
      "preset": "slack",
      "url": "https://hooks.slack.com/services/YOUR/WEBHOOK/URL",
      "chat_id": "",
      "format": "json",
      "headers": {}
    },
    "suppressQuestionAfterTaskCompleteSeconds": 12
  },
  "statuses": {
    "task_complete": {
      "title": "✅ Task Completed",
      "sound": "${CLAUDE_PLUGIN_ROOT}/sounds/task-complete.mp3",
      "keywords": ["completed", "done", "finished"]
    },
    "plan_ready": {
      "title": "📋 Plan Ready for Review",
      "sound": "${CLAUDE_PLUGIN_ROOT}/sounds/plan-ready.mp3",
      "keywords": ["plan", "strategy"]
    },
    "question": {
      "title": "❓ Claude Has Questions",
      "sound": "${CLAUDE_PLUGIN_ROOT}/sounds/question.mp3",
      "keywords": ["question", "clarify"]
    },
    "session_limit_reached": {
      "title": "⏱️ Session Limit Reached",
      "sound": "${CLAUDE_PLUGIN_ROOT}/sounds/question.mp3"
    }
  }
}
```

### Sound Options

**Built-in sounds** (included):
- `${CLAUDE_PLUGIN_ROOT}/sounds/task-complete.mp3`
- `${CLAUDE_PLUGIN_ROOT}/sounds/review-complete.mp3`
- `${CLAUDE_PLUGIN_ROOT}/sounds/question.mp3`
- `${CLAUDE_PLUGIN_ROOT}/sounds/plan-ready.mp3`

**System sounds:**
- macOS: `/System/Library/Sounds/Glass.aiff`, `/System/Library/Sounds/Hero.aiff`, etc.
- Linux: `/usr/share/sounds/**/*.ogg` (varies by distribution)
- Windows: Use built-in MP3s (system sounds not easily accessible)

**Supported formats:** MP3, WAV, FLAC, OGG/Vorbis, AIFF

### Audio Device Selection

Route notification sounds to a specific audio output device instead of the system default:

```bash
# List available audio devices
bin/list-devices

# Output:
#   0: MacBook Pro-Lautsprecher
#   1: Babyface (23314790) (default)
#   2: Immersed
```

Then add the device name to your `config.json`:

```json
{
  "notifications": {
    "desktop": {
      "audioDevice": "MacBook Pro-Lautsprecher"
    }
  }
}
```

Leave `audioDevice` empty or omit it to use the system default device.

### Test Sound Playback

Preview any sound file with optional volume control:

```bash
# Test built-in sound (full volume)
bin/sound-preview sounds/task-complete.mp3

# Test with reduced volume (30% - recommended for testing)
bin/sound-preview --volume 0.3 sounds/task-complete.mp3

# Test macOS system sound at 30% volume
bin/sound-preview --volume 0.3 /System/Library/Sounds/Glass.aiff

# Test custom sound at 50% volume
bin/sound-preview --volume 0.5 /path/to/your/sound.wav

# Show all options
bin/sound-preview --help
```

**Volume flag:** Use `--volume` to control playback volume (0.0 to 1.0). Default is 1.0 (full volume).


## Architecture

```text
cmd/
  claude-notifications/     # CLI entry point
  sound-preview/            # Sound preview utility
  list-devices/             # List available audio output devices
internal/
  audio/                    # Audio playback with device selection (malgo)
  config/                   # Configuration loading and validation
  logging/                  # Structured logging to notification-debug.log
  platform/                 # Cross-platform utilities (temp dirs, mtime, etc.)
  analyzer/                 # JSONL parsing and state machine
  state/                    # Per-session state and cooldown management
  dedup/                    # Two-phase lock deduplication
  notifier/                 # Desktop notifications and sound playback
  webhook/                  # Webhook integrations (Slack/Discord/Telegram/Custom)
  hooks/                    # Hook routing (PreToolUse/Stop/SubagentStop/Notification)
  summary/                  # Message summarization and markdown cleanup
  sessionname/              # Friendly session name generation ([bold-cat], etc.)
pkg/
  jsonl/                    # JSONL streaming parser
commands/
  notifications-init.md     # Binary download wizard
  notifications-settings.md # Interactive settings configuration wizard
sounds/                     # Custom notification sounds (MP3)
claude_icon.png             # Plugin icon for desktop notifications
```

## Usage

The plugin is invoked automatically by Claude Code hooks. You can also test manually:

```bash
# Test PreToolUse hook
echo '{"session_id":"test","transcript_path":"/path/to/transcript.jsonl","tool_name":"ExitPlanMode"}' | \
  claude-notifications handle-hook PreToolUse

# Test Stop hook
echo '{"session_id":"test","transcript_path":"/path/to/transcript.jsonl"}' | \
  claude-notifications handle-hook Stop
```

## Development

### Local installation for development

```bash
# 1. Clone repository
git clone https://github.com/777genius/claude-notifications-go
cd claude-notifications-go

# 2. Build binary for your platform
make build

# 3. Add as local marketplace
/plugin marketplace add .

# 4. Install plugin
/plugin install claude-notifications-go@local-dev

# 5. Restart Claude Code for hooks to take effect

# 6. Download binary and configure settings
/claude-notifications-go:notifications-init
/claude-notifications-go:notifications-settings
```

**Note:** For local development, build the binary with `make build` first. The `/claude-notifications-go:notifications-init` command will use your locally built binary if it exists, otherwise it will download from GitHub Releases.

### Building binaries

```bash
# Run tests
make test

# Run tests with race detection
make test-race

# Generate coverage report
make test-coverage

# Build for all platforms
make build-all

# Rebuild and prepare for commit
make rebuild-and-commit

# Lint
make lint
```

**Note:** GitHub Actions automatically rebuilds binaries when Go code changes are pushed.

## Testing

```bash
# Unit tests
go test ./internal/config -v
go test ./internal/analyzer -v
go test ./internal/dedup -v -race

# Integration tests
go test ./test -v

# Specific test
go test -run TestStateMachine ./internal/analyzer -v
```

## Documentation

- **[Volume Control Guide](docs/volume-control.md)** - Customize notification volume
  - Configure volume from 0% to 100%
  - Logarithmic scaling for natural sound
  - Per-environment recommendations

- **[Interactive Sound Preview](docs/interactive-sound-preview.md)** - Preview sounds during setup
  - Interactive sound selection
  - Preview before choosing

- **[Webhook Integration Guide](docs/webhooks/README.md)** - Complete guide for webhook setup
  - **[Slack](docs/webhooks/slack.md)** - Slack integration with color-coded attachments
  - **[Discord](docs/webhooks/discord.md)** - Discord integration with rich embeds
  - **[Telegram](docs/webhooks/telegram.md)** - Telegram bot integration
  - **[Lark/Feishu](docs/webhooks/lark.md)** - Lark/Feishu integration with interactive cards
  - **[Custom Webhooks](docs/webhooks/custom.md)** - Any webhook-compatible service
  - **[Configuration](docs/webhooks/configuration.md)** - Retry, circuit breaker, rate limiting
  - **[Monitoring](docs/webhooks/monitoring.md)** - Metrics and debugging
  - **[Troubleshooting](docs/webhooks/troubleshooting.md)** - Common issues and solutions

## License

GPL-3.0 - See [LICENSE](LICENSE) file for details.
