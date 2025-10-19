# Claude Notifications (Go)

[![Ubuntu CI](https://github.com/777genius/claude-notifications-go/workflows/Ubuntu%20CI/badge.svg)](https://github.com/777genius/claude-notifications-go/actions)
[![macOS CI](https://github.com/777genius/claude-notifications-go/workflows/macOS%20CI/badge.svg)](https://github.com/777genius/claude-notifications-go/actions)
[![Windows CI](https://github.com/777genius/claude-notifications-go/workflows/Windows%20CI/badge.svg)](https://github.com/777genius/claude-notifications-go/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/777genius/claude-notifications-go)](https://goreportcard.com/report/github.com/777genius/claude-notifications-go)
[![codecov](https://codecov.io/gh/777genius/claude-notifications-go/branch/master/graph/badge.svg)](https://codecov.io/gh/777genius/claude-notifications-go)

**Professional rewrite of the claude-notifications plugin in Go.**

Smart notifications for Claude Code task statuses with cross-platform support, webhook integrations, and intelligent deduplication.

## Features

- ✅ **Cross-platform**: macOS, Linux, Windows (Git Bash/WSL)
- ✅ **Smart status detection**: State machine-based analysis with temporal locality
- ✅ **PreToolUse integration**: Instant notifications for ExitPlanMode and AskUserQuestion
- ✅ **Deduplication**: Two-phase lock mechanism prevents duplicate notifications
- ✅ **Cooldown system**: Suppress noisy back-to-back alerts
- ✅ **Desktop notifications**: Native OS notifications with custom sounds/icons
- ✅ **Native sound playback**: Using gopxl/beep (no external commands)
- ✅ **Multi-format support**: MP3, WAV, FLAC, OGG, AIFF
- ✅ **Interactive setup**: `/setup-notifications` command with sound preview
- ✅ **Volume control**: Customizable notification volume (0-100%) for all environments
- ✅ **Session names**: Friendly names like "[bold-cat]" for easy session identification
- ✅ **Webhook support**: Slack, Discord, Telegram, and custom endpoints with enterprise reliability patterns
  - Retry with exponential backoff
  - Circuit breaker for fault tolerance
  - Rate limiting with token bucket
  - Rich platform-specific formatting
  - Request tracing and metrics
  - **→ [Complete Webhook Documentation](docs/webhooks/README.md)**
- ✅ **JSONL parsing**: Efficient streaming parser for large transcripts
- ✅ **Comprehensive testing**: Unit tests with race detection

## Architecture

```
cmd/
  claude-notifications/     # CLI entry point
  sound-preview/            # Sound preview utility
internal/
  config/                   # Configuration loading and validation
  logging/                  # Structured logging to notification-debug.log
  platform/                 # Cross-platform utilities (temp dirs, mtime, etc.)
  analyzer/                 # JSONL parsing and state machine
  state/                    # Per-session state and cooldown management
  dedup/                    # Two-phase lock deduplication
  notifier/                 # Desktop notifications and native sound playback
  webhook/                  # Webhook integrations (Slack/Discord/Telegram/Custom)
  hooks/                    # Hook routing (PreToolUse/Stop/SubagentStop/Notification)
  summary/                  # Message summarization and markdown cleanup
  sessionname/              # Friendly session name generation ([bold-cat], etc.)
pkg/
  jsonl/                    # JSONL streaming parser
commands/
  setup-notifications.md    # Interactive setup wizard
sounds/                     # Custom notification sounds (MP3)
claude_icon.png             # Plugin icon for desktop notifications
```

## Installation

### Prerequisites

- Claude Code v2.0.15+
- Windows 10+ (for Toast notifications), macOS, or Linux
- **No additional software required** - pre-built binaries included for all platforms

### Install from GitHub

```bash
# Add marketplace
/plugin marketplace add 777genius/claude-notifications-go

# Install plugin
/plugin install claude-notifications-go@claude-notifications-go

# Restart Claude Code
```

That's it! The plugin will automatically select the correct binary for your platform (macOS, Linux, or Windows).

## Platform Support

**Supported platforms:**
- macOS (Intel & Apple Silicon)
- Linux (x64 & ARM64)
- Windows 10+ (x64)

**No additional dependencies:**
- ✅ Pre-built binaries included
- ✅ Pure Go - no C compiler needed
- ✅ All libraries bundled

**Windows-specific features:**
- Native Toast notifications (Windows 10+)
- Works in PowerShell, CMD, Git Bash, or WSL
- MP3/WAV/OGG/FLAC audio playback via native Windows APIs
- System sounds not accessible - use built-in MP3s or custom files

## Quick Start

### Interactive Setup (Recommended)

Run the interactive setup wizard to configure notification sounds:

```
/setup-notifications
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
    "suppressQuestionAfterTaskCompleteSeconds": 7
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

# 2. Verify binaries (optional)
./setup.sh

# 3. Add as local marketplace
/plugin marketplace add .

# 4. Install plugin
/plugin install claude-notifications-go@local-dev

# 5. Restart Claude Code for hooks to take effect
```

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
  - **[Custom Webhooks](docs/webhooks/custom.md)** - Any webhook-compatible service
  - **[Configuration](docs/webhooks/configuration.md)** - Retry, circuit breaker, rate limiting
  - **[Monitoring](docs/webhooks/monitoring.md)** - Metrics and debugging
  - **[Troubleshooting](docs/webhooks/troubleshooting.md)** - Common issues and solutions

## License

MIT
