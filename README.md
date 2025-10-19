# Claude Notifications (Go)

**Professional rewrite of the claude-notifications plugin in Go.**

Smart notifications for Claude Code task statuses with cross-platform support, webhook integrations, and intelligent deduplication.

## Features

- ✅ **Cross-platform**: macOS, Linux, Windows (Git Bash/WSL)
- ✅ **Smart status detection**: State machine-based analysis with temporal locality
- ✅ **PreToolUse integration**: Instant notifications for ExitPlanMode and AskUserQuestion
- ✅ **Deduplication**: Two-phase lock mechanism prevents duplicate notifications
- ✅ **Cooldown system**: Suppress noisy back-to-back alerts
- ✅ **Desktop notifications**: Native OS notifications with custom sounds/icons
- ✅ **Session names**: Friendly names like "[bold-cat]" for easy session identification
- ✅ **Webhook support**: Slack, Discord, Telegram, and custom endpoints
- ✅ **JSONL parsing**: Efficient streaming parser for large transcripts
- ✅ **Comprehensive testing**: Unit tests with race detection

## Architecture

```
cmd/
  claude-notifications/     # CLI entry point
internal/
  config/                   # Configuration loading and validation
  logging/                  # Structured logging to notification-debug.log
  platform/                 # Cross-platform utilities (temp dirs, mtime, etc.)
  analyzer/                 # JSONL parsing and state machine
  state/                    # Per-session state and cooldown management
  dedup/                    # Two-phase lock deduplication
  notifier/                 # Desktop notifications and sound
  webhook/                  # Webhook integrations (Slack/Discord/Telegram/Custom)
  hooks/                    # Hook routing (PreToolUse/Stop/SubagentStop/Notification)
  summary/                  # Message summarization and markdown cleanup
  sessionname/              # Friendly session name generation ([bold-cat], etc.)
pkg/
  jsonl/                    # JSONL streaming parser
sounds/                     # Custom notification sounds (MP3)
claude_icon.png             # Plugin icon for desktop notifications
```

## Installation

### Prerequisites

- Go 1.21 or later
- Claude Code v2.0.15+

### Install from source

```bash
# 1. Clone repository
git clone https://github.com/belief/claude-notifications-go
cd claude-notifications-go

# 2. Run setup script (builds binary)
chmod +x setup.sh
./setup.sh

# 3. Add marketplace to Claude Code
/plugin marketplace add $(pwd)

# 4. Install plugin
/plugin install claude-notifications-go@local-dev

# 5. Restart Claude Code for hooks to take effect
```

### Verify installation

```bash
# List installed plugins
/plugin list

# (Optional) Test notification manually
echo '{"session_id":"test","tool_name":"ExitPlanMode"}' | \
  bin/claude-notifications handle-hook PreToolUse
```

## Configuration

Edit `config/config.json`:

```json
{
  "notifications": {
    "desktop": {
      "enabled": true,
      "sound": true,
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
    }
  }
}
```

**Note**: The plugin includes custom sound files (`sounds/*.mp3`). You can also use system sounds:
- macOS: `/System/Library/Sounds/Glass.aiff`, `/System/Library/Sounds/Hero.aiff`, etc.
- Linux: `/usr/share/sounds/...`
- Windows: `C:\Windows\Media\...`

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

```bash
# Run tests
make test

# Run tests with race detection
make test-race

# Generate coverage report
make test-coverage

# Build for all platforms
make build-all

# Lint
make lint
```

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

## License

MIT
