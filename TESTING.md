# Testing Guide

## Running Tests

### All tests
```bash
go test ./...
```

### With race detection
```bash
go test ./... -race
```

### With coverage
```bash
go test -coverprofile=coverage.txt ./...
go tool cover -html=coverage.txt
```

### Specific package
```bash
go test ./internal/config -v
go test ./internal/dedup -v -race
go test ./pkg/jsonl -v
```

## Test Results

### ✅ All tests passing (with race detection)

```
ok  	github.com/belief/claude-notifications/internal/analyzer	1.402s
ok  	github.com/belief/claude-notifications/internal/config	1.204s
ok  	github.com/belief/claude-notifications/internal/dedup	4.782s
ok  	github.com/belief/claude-notifications/internal/platform	1.576s
ok  	github.com/belief/claude-notifications/internal/sessionname	1.124s
ok  	github.com/belief/claude-notifications/pkg/jsonl	1.914s
```

## Test Coverage

### internal/config
- ✅ Default config
- ✅ Load from file
- ✅ Load non-existent (returns defaults)
- ✅ Validation (presets, URLs, chat_id)
- ✅ Status info lookup
- ✅ Notification enabled checks

### pkg/jsonl
- ✅ Parse JSONL (tolerant to invalid lines)
- ✅ Get last N assistant messages
- ✅ Extract tools with positions
- ✅ Find tool by name
- ✅ Count tools after position
- ✅ Extract text from messages

### internal/analyzer
- ✅ PreToolUse status detection
- ✅ Notification status (always question)
- ✅ Tool category checks (contains)

### internal/platform
- ✅ OS detection
- ✅ Temp directory (no trailing slash)
- ✅ File exists check
- ✅ File mtime and age
- ✅ Current timestamp
- ✅ Atomic file creation
- ✅ Cleanup old files
- ✅ Path normalization
- ✅ Environment variable expansion
- ✅ Platform checks (macOS/Linux/Windows)

### internal/dedup
- ✅ Early duplicate check
- ✅ Acquire lock (atomic)
- ✅ **Concurrent lock acquisition** (race detection)
- ✅ Release lock
- ✅ Cleanup old locks
- ✅ Cleanup for session

### internal/sessionname
- ✅ Generate session name from UUID
- ✅ Deterministic name generation
- ✅ Handle empty/invalid session IDs
- ✅ Name format validation (adjective-noun)
- ✅ Hex to int conversion

## Race Detection

All dedup tests pass with `-race` flag, confirming thread-safety:
- No data races detected
- Concurrent lock acquisition works correctly (only 1 succeeds)
- Atomic file operations are safe

## Manual Testing

### Test PreToolUse hook
```bash
echo '{"session_id":"test","tool_name":"ExitPlanMode","cwd":"/tmp"}' | \
  ./bin/claude-notifications handle-hook PreToolUse
```

Should trigger "Plan Ready" notification.

### Test Stop hook
```bash
echo '{"session_id":"test","transcript_path":"","cwd":"/tmp"}' | \
  ./bin/claude-notifications handle-hook Stop
```

Should trigger notification (uses default status without transcript).

### Test with config
```bash
export CLAUDE_PLUGIN_ROOT=/Users/belief/dev/projects/claude/notification_plugin_go
echo '{"session_id":"test","tool_name":"ExitPlanMode"}' | \
  ./bin/claude-notifications handle-hook PreToolUse
```

Check `notification-debug.log` for detailed logs.

## Future Tests

### TODO
- [ ] State manager tests (cooldown, session state)
- [ ] Summary generator tests (markdown cleanup)
- [ ] Webhook sender tests (mock HTTP)
- [ ] Notifier tests (mock beeep)
- [ ] Hooks integration tests (end-to-end)
- [ ] Analyzer tests with real transcript fixtures

### Integration Tests
- [ ] Full hook flow (PreToolUse → Stop)
- [ ] Deduplication in real scenario
- [ ] State persistence between hooks
- [ ] Cooldown behavior

### Performance Tests
- [ ] Large transcript parsing (10k+ lines)
- [ ] Concurrent hook invocations
- [ ] Memory usage benchmarks
