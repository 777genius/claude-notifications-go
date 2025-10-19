# Custom Notification Volume - Feature Implementation

## Summary

Added **customizable volume control** for notification sounds. Users can now configure notification volume from 0% (silent) to 100% (full volume) through the interactive setup wizard or by editing `config.json`.

## Motivation

Users requested the ability to control notification volume for different environments:
- **Office:** Quiet notifications (30%) to avoid disturbing colleagues
- **Home:** Medium volume (50%) for balanced sound
- **Loud environment:** Full volume (100%) to ensure notifications are heard

## Changes

### 1. Config Structure (`internal/config/config.go`)

**Added:**
- `Volume float64` field to `DesktopConfig` struct
- Default value: `1.0` (full volume, 100%)
- Validation: Must be between `0.0` and `1.0`
- `ApplyDefaults()` sets volume to `1.0` if missing

**Example:**
```go
type DesktopConfig struct {
    Enabled bool    `json:"enabled"`
    Sound   bool    `json:"sound"`
    Volume  float64 `json:"volume"` // 0.0-1.0
    AppIcon string  `json:"appIcon"`
}
```

### 2. Notifier (`internal/notifier/notifier.go`)

**Added:**
- Import `math` and `gopxl/beep/effects`
- `volumeToBase()` function for logarithmic volume conversion
- Volume control in `playSound()` using `effects.Volume`
- Debug logging for volume application

**Volume scaling:**
```go
volumeStreamer := &effects.Volume{
    Streamer: resampled,
    Base:     2,
    Volume:   volumeToBase(volume), // log₂(volume)
    Silent:   false,
}
```

**Logarithmic conversion:**
- `1.0` → `0.0` (full volume, no change)
- `0.5` → `-1.0` (~-6dB, half perceived volume)
- `0.3` → `-1.7` (~-10dB, office-friendly)
- `0.1` → `-3.3` (~-20dB, very quiet)

### 3. Setup Wizard (`commands/setup-notifications.md`)

**Added Step 5: Notification Volume Configuration**

New question in setup wizard:
- **Question:** "What volume level do you want for notification sounds?"
- **Options:**
  - Full volume (100%) - Maximum volume (default)
  - High volume (70%) - Loud but not maximum
  - Medium volume (50%) - Balanced volume
  - Low volume (30%) - Quiet, good for offices
  - Very low (10%) - Very quiet, minimal distraction

**Volume preview:**
```bash
bin/sound-preview --volume <selected_volume> sounds/task-complete.mp3
```

**Configuration generation:**
```json
{
  "notifications": {
    "desktop": {
      "volume": <user's selected volume>
    }
  }
}
```

### 4. Configuration Files

**Updated:**
- `config/config.json` - Added `"volume": 1.0` to desktop settings
- `README.md` - Added volume control to features list and documentation section
- All example configs now include volume field

### 5. Documentation

**New files:**
- `docs/volume-control.md` - Complete volume control guide
  - Configuration examples
  - Technical implementation details
  - Testing instructions
  - Troubleshooting

**Updated:**
- `README.md` - Added volume control feature
- `commands/setup-notifications.md` - Added volume question (Step 5)

## Usage

### Via Setup Wizard (Recommended)

```bash
/setup-notifications
```

Follow the interactive prompts to configure volume.

### Manual Configuration

Edit `config/config.json`:

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

**Volume values:**
- `1.0` = 100% (full volume, default)
- `0.7` = 70% (high)
- `0.5` = 50% (medium)
- `0.3` = 30% (low, office-friendly)
- `0.1` = 10% (very low)

### Testing

```bash
# Test with sound-preview
bin/sound-preview --volume 0.3 sounds/task-complete.mp3

# Trigger test notification
echo '{"session_id":"test","tool_name":"ExitPlanMode"}' | \
  bin/claude-notifications handle-hook PreToolUse
```

## Technical Details

### Logarithmic Volume Scaling

Human hearing perceives volume logarithmically, not linearly. The implementation uses `log₂` conversion:

```
Linear (config) → Logarithmic (beep) → Perceived Volume
1.0             → 0.0                → Full volume
0.7             → -0.5               → High volume
0.5             → -1.0               → Half volume
0.3             → -1.7               → Office-friendly
0.1             → -3.3               → Very quiet
```

**Why logarithmic?**
- Linear: `0.5` would sound like 75% volume (too loud)
- Logarithmic: `0.5` sounds like 50% volume (natural)

### Code Implementation

**Config validation:**
```go
if c.Notifications.Desktop.Volume < 0.0 || c.Notifications.Desktop.Volume > 1.0 {
    return fmt.Errorf("desktop volume must be between 0.0 and 1.0")
}
```

**Volume conversion:**
```go
func volumeToBase(volume float64) float64 {
    if volume <= 0 { return -10 }  // Almost silent
    if volume >= 1.0 { return 0 }  // Full volume
    return math.Log(volume) / math.Log(2)  // log₂
}
```

**Sound playback with volume:**
```go
volume := n.cfg.Notifications.Desktop.Volume
if volume < 1.0 {
    volumeStreamer = &effects.Volume{
        Streamer: resampled,
        Base:     2,
        Volume:   volumeToBase(volume),
    }
}
speaker.Play(beep.Seq(volumeStreamer, callback))
```

## Backward Compatibility

**Existing installations:**
- Config without `volume` field → defaults to `1.0` (full volume)
- No breaking changes
- Notifications continue at full volume until user changes config

**Migration:**
- No action required for existing users
- Plugin automatically applies defaults via `ApplyDefaults()`

## Testing Results

### Test 1: 30% Volume
```bash
go run test-volume.go
# Result: ✓ Quiet, audible notification sound
```

### Test 2: 100% Volume
```bash
go run test-volume-full.go
# Result: ✓ Loud, clear notification sound
```

### Test 3: Config Validation
```bash
# Invalid volume → Error
{"volume": 1.5}  # ✗ Error: must be 0.0-1.0
{"volume": -0.1} # ✗ Error: must be 0.0-1.0
```

## Files Modified

```
modified:   internal/config/config.go
modified:   internal/notifier/notifier.go
modified:   commands/setup-notifications.md
modified:   config/config.json
modified:   README.md
new file:   docs/volume-control.md
new file:   CHANGELOG_CUSTOM_VOLUME.md
```

## Comparison with sound-preview

Both implementations use the same algorithm:

| Feature | sound-preview | notifier |
|---------|--------------|----------|
| Volume control | ✅ `--volume` flag | ✅ `config.volume` |
| Range | 0.0 - 1.0 | 0.0 - 1.0 |
| Scaling | Logarithmic (log₂) | Logarithmic (log₂) |
| Implementation | `effects.Volume` | `effects.Volume` |
| Default | 1.0 (full) | 1.0 (full) |

**Consistency:** Same volume value produces same loudness in both preview and actual notifications.

## Use Cases

### Office Environment
```json
{"volume": 0.3}  // Quiet, won't disturb colleagues
```

### Home Office
```json
{"volume": 0.5}  // Balanced, comfortable
```

### Noisy Environment
```json
{"volume": 1.0}  // Maximum, ensures you hear it
```

### Late Night Coding
```json
{"volume": 0.1}  // Very quiet, minimal distraction
```

### Silent Mode
```json
{"sound": false}  // No sound, visual only
```

## Future Enhancements

Potential improvements:

- [ ] **Per-status volume:** Different volume for each notification type
  - Example: `task_complete: 1.0`, `question: 0.5`

- [ ] **Time-based volume:** Adjust volume based on time of day
  - Example: Quiet at night (9pm-7am), normal during day

- [ ] **System integration:** Respect system volume settings
  - Example: Multiply config volume by system volume

- [ ] **Volume presets:** Named presets for common scenarios
  - Example: `"preset": "office"` → `volume: 0.3`

- [ ] **Fade effects:** Gradual volume fade-in/fade-out
  - Example: Fade in over 0.5s, fade out over 0.5s

## Related PRs / Issues

- Addresses user request: "Add volume control for notifications"
- Complements: Interactive sound preview feature
- Related: sound-preview `--volume` flag implementation

## See Also

- [Volume Control Guide](docs/volume-control.md) - Complete documentation
- [Interactive Sound Preview](docs/interactive-sound-preview.md) - Preview sounds
- [CHANGELOG_VOLUME.md](CHANGELOG_VOLUME.md) - sound-preview volume feature

---

**Version:** Added in commit `<commit-hash>`
**Date:** 2025-10-19
**Author:** 777genius
