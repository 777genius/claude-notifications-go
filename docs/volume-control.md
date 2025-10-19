# Volume Control for Notifications

## Overview

The plugin now supports **customizable volume control** for notification sounds. Users can configure notification volume from 0% (silent) to 100% (full volume) through the setup wizard or by editing `config.json`.

## Configuration

### Via Setup Wizard (Recommended)

Run the interactive setup command:

```bash
/setup-notifications
```

The wizard will ask you to choose a volume level:
- **Full volume (100%)** - Maximum volume (default)
- **High volume (70%)** - Loud but not maximum
- **Medium volume (50%)** - Balanced volume
- **Low volume (30%)** - Quiet, good for offices
- **Very low (10%)** - Very quiet, minimal distraction

The wizard will let you preview a sound at your selected volume before saving.

### Manual Configuration

Edit `config/config.json` and set the `volume` field:

```json
{
  "notifications": {
    "desktop": {
      "enabled": true,
      "sound": true,
      "volume": 0.5,
      "appIcon": "${CLAUDE_PLUGIN_ROOT}/claude_icon.png"
    },
    ...
  },
  ...
}
```

**Volume values:**
- `1.0` - Full volume (100%, default)
- `0.7` - High volume (70%)
- `0.5` - Medium volume (50%)
- `0.3` - Low volume (30%)
- `0.1` - Very low volume (10%)
- Valid range: `0.0` to `1.0`

## How It Works

### Logarithmic Volume Scaling

The plugin uses **logarithmic volume scaling** to match human hearing perception:

```
Linear Volume → Logarithmic Units (log₂)
1.0 (100%)    → 0.0   (full volume, no change)
0.7 (70%)     → -0.5  (~-3dB)
0.5 (50%)     → -1.0  (~-6dB, half perceived volume)
0.3 (30%)     → -1.7  (~-10dB)
0.1 (10%)     → -3.3  (~-20dB)
```

This means:
- `0.5` sounds like "half as loud" to human ears
- `0.3` is noticeably quieter but still audible
- `0.1` is very quiet, suitable for very quiet environments

### Technical Implementation

Volume control is implemented using `gopxl/beep/effects.Volume`:

```go
volumeStreamer := &effects.Volume{
    Streamer: audioStream,
    Base:     2,           // Exponential base
    Volume:   log₂(volume), // Logarithmic conversion
    Silent:   false,
}
```

The same algorithm is used in:
- `internal/notifier/notifier.go` - For actual notifications
- `cmd/sound-preview/main.go` - For sound preview utility

## Usage Examples

### Example 1: Office Environment (30% volume)

```json
{
  "notifications": {
    "desktop": {
      "enabled": true,
      "sound": true,
      "volume": 0.3,
      "appIcon": "${CLAUDE_PLUGIN_ROOT}/claude_icon.png"
    }
  }
}
```

**Use case:** You work in an office and don't want to disturb colleagues.

### Example 2: Home Office (50% volume)

```json
{
  "notifications": {
    "desktop": {
      "enabled": true,
      "sound": true,
      "volume": 0.5,
      "appIcon": "${CLAUDE_PLUGIN_ROOT}/claude_icon.png"
    }
  }
}
```

**Use case:** Balanced volume for home office environment.

### Example 3: Loud Environment (100% volume)

```json
{
  "notifications": {
    "desktop": {
      "enabled": true,
      "sound": true,
      "volume": 1.0,
      "appIcon": "${CLAUDE_PLUGIN_ROOT}/claude_icon.png"
    }
  }
}
```

**Use case:** You work in a noisy environment and need maximum volume.

### Example 4: Silent Notifications

```json
{
  "notifications": {
    "desktop": {
      "enabled": true,
      "sound": false,
      "volume": 1.0,
      "appIcon": "${CLAUDE_PLUGIN_ROOT}/claude_icon.png"
    }
  }
}
```

**Use case:** You only want visual notifications, no sound.

## Testing Volume

### Test with sound-preview utility

```bash
# Test 30% volume
bin/sound-preview --volume 0.3 sounds/task-complete.mp3

# Test 50% volume
bin/sound-preview --volume 0.5 sounds/task-complete.mp3

# Test full volume
bin/sound-preview sounds/task-complete.mp3
```

### Test with actual notifications

After configuring volume in `config/config.json`, trigger a test notification:

```bash
# Manually trigger a test hook
echo '{"session_id":"test","transcript_path":"","tool_name":"ExitPlanMode"}' | \
  bin/claude-notifications handle-hook PreToolUse
```

Or just wait for the next real notification from Claude Code.

## Validation

The plugin validates volume values:

```bash
# Valid volumes
volume: 0.0   # Silent (technically valid)
volume: 0.3   # 30% ✓
volume: 0.5   # 50% ✓
volume: 1.0   # 100% ✓

# Invalid volumes (will cause error)
volume: -0.1  # Negative ✗
volume: 1.5   # Above 1.0 ✗
volume: "50%" # String ✗ (must be float)
```

**Error message:**
```
Error: desktop volume must be between 0.0 and 1.0 (got 1.5)
```

## Default Behavior

### New Installations

- Default volume: `1.0` (full volume)
- Applied automatically when config is generated

### Existing Installations

If you have an existing `config.json` without the `volume` field:

1. The plugin will automatically add `volume: 1.0` on first load (via `ApplyDefaults()`)
2. Your notifications will continue at full volume (backward compatible)
3. You can manually add `"volume": 0.5` to change it

## Migration Guide

### For Users Upgrading from Older Versions

**No action required!** The plugin automatically defaults to full volume (1.0) if the field is missing.

If you want to change the volume:

1. **Option A:** Run `/setup-notifications` and reconfigure
2. **Option B:** Manually add `"volume": 0.5` to your `config.json`:

```json
{
  "notifications": {
    "desktop": {
      "enabled": true,
      "sound": true,
      "volume": 0.5,  // ← Add this line
      "appIcon": "..."
    }
  }
}
```

## Troubleshooting

### Volume too loud

Lower the volume value in config:

```json
"volume": 0.3  // Try 30%
```

### Volume too quiet

Increase the volume value:

```json
"volume": 0.7  // Try 70%
```

### Volume has no effect

Check:

1. Is sound enabled? `"sound": true`
2. Is volume in valid range? `0.0` to `1.0`
3. Is the sound file path correct?
4. Check logs: `notification-debug.log` should show:
   ```
   Applying volume control: 30%
   Sound played successfully: sounds/task-complete.mp3 (volume: 30%)
   ```

### Want different volume for different notification types

Currently not supported - volume is global for all notifications. This feature may be added in the future.

**Workaround:** Use different sound files with varying loudness.

## Related Files

- **Config structure:** `internal/config/config.go`
- **Notifier implementation:** `internal/notifier/notifier.go`
- **Sound preview:** `cmd/sound-preview/main.go`
- **Setup wizard:** `commands/setup-notifications.md`
- **Example config:** `config/config.json`

## Future Enhancements

Potential improvements:

- [ ] Per-status volume control (different volume for each notification type)
- [ ] Time-based volume (quieter at night, louder during day)
- [ ] Adaptive volume based on system volume
- [ ] Fade-in/fade-out effects
- [ ] Volume presets ("office", "home", "loud")

## See Also

- [Interactive Sound Preview](interactive-sound-preview.md) - Preview sounds before choosing
- [Webhook Documentation](webhooks/README.md) - Send notifications to external services
- [Architecture](../ARCHITECTURE.md) - Plugin architecture overview
