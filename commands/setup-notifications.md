---
description: Interactive setup wizard for claude-notifications plugin
allowed-tools: Bash, AskUserQuestion, Write, Read
---

# 🎵 Claude Notifications Setup Wizard

Welcome! This interactive setup will help you configure notification sounds for Claude Code.

Let's make your Claude experience more delightful with custom audio notifications!

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 🎯 KEY FEATURE: Interactive Sound Preview

**IMPORTANT FOR CLAUDE:**

This setup wizard is INTERACTIVE. Users can preview sounds at ANY time by saying:
- "play [sound_name]"
- "preview [sound_name]"
- "прослушать [sound_name]" (Russian)
- "проиграть [sound_name]" (Russian)

**Your job:**
1. Detect when user wants to preview a sound (keywords: play, preview, прослушать, проиграть)
2. Extract the sound name from their message
3. Run `bin/sound-preview <path>` to play it
4. Ask if they want to hear more sounds
5. When they're ready, proceed with AskUserQuestion selections

**Flow:**
- Step 1-2: Detect system and list available sounds
- Step 3: **INTERACTIVE PREVIEW PHASE** - let user explore sounds freely
- Step 4: Ask 4 questions (Task/Review/Question/Plan) - remind about preview before each
- Step 5: Webhook configuration
- Step 6: Generate config.json
- Step 7: Summary & test

**Be patient and encouraging** - sound selection is personal!

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## Step 1: Determine Plugin Root

First, let me find where the plugin is installed:

```bash
# Get plugin root directory
PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT}"
if [ -z "$PLUGIN_ROOT" ]; then
  echo "Warning: CLAUDE_PLUGIN_ROOT not set, using current directory"
  PLUGIN_ROOT="$(pwd)"
fi
echo "Plugin root: $PLUGIN_ROOT"
```

## Step 1.5: Download Binary (First-Time Setup)

Now let me check if the notification binary is installed, and download it if needed:

```bash
# Run the installer to download the binary for your platform
echo ""
echo "Checking for notification binary..."
if ! "${PLUGIN_ROOT}/bin/install.sh"; then
  echo ""
  echo "Error: Failed to install notification binary"
  echo "Please check your internet connection and try again"
  exit 1
fi
echo ""
```

This will automatically download the correct binary for your platform (macOS, Linux, or Windows) from GitHub Releases on first run. Subsequent runs will skip this step if the binary is already installed.

## Step 2: Discover Available Sounds

Now let me detect what sound options are available on your system!

### Detect Operating System

```bash
OS_TYPE=$(uname -s)
case "$OS_TYPE" in
  Darwin*)
    echo "Operating System: macOS"
    HAS_SYSTEM_SOUNDS="true"
    SYSTEM_SOUNDS_DIR="/System/Library/Sounds"
    ;;
  Linux*)
    echo "Operating System: Linux"
    # Check common Linux sound directories
    if [ -d "/usr/share/sounds" ]; then
      HAS_SYSTEM_SOUNDS="true"
      SYSTEM_SOUNDS_DIR="/usr/share/sounds"
    else
      HAS_SYSTEM_SOUNDS="false"
    fi
    ;;
  MINGW*|MSYS*|CYGWIN*)
    echo "Operating System: Windows"
    HAS_SYSTEM_SOUNDS="false"
    ;;
  *)
    echo "Operating System: Unknown"
    HAS_SYSTEM_SOUNDS="false"
    ;;
esac
```

### Built-in Sounds (Always Available)

The plugin includes these pre-configured sounds that work on all platforms:

```bash
echo "Built-in sounds (included with plugin):"
if [ -d "${PLUGIN_ROOT}/sounds" ]; then
  ls -1 "${PLUGIN_ROOT}/sounds/"*.mp3 2>/dev/null | while read file; do
    name=$(basename "$file" .mp3)
    echo "  ✓ $name.mp3"
  done
else
  echo "  Warning: sounds/ directory not found!"
fi
```

**Always available:**
- ✅ **task-complete.mp3** - Triumphant completion chime
- ✅ **review-complete.mp3** - Gentle notification tone
- ✅ **question.mp3** - Attention-grabbing sound
- ✅ **plan-ready.mp3** - Professional planning tone

### System Sounds (Platform-dependent)

Now checking for system sounds:

```bash
if [ "$HAS_SYSTEM_SOUNDS" = "true" ]; then
  echo ""
  echo "System sounds detected at: $SYSTEM_SOUNDS_DIR"

  if [ "$OS_TYPE" = "Darwin"* ]; then
    # macOS system sounds
    echo "Available macOS system sounds:"
    ls -1 /System/Library/Sounds/*.aiff 2>/dev/null | while read file; do
      name=$(basename "$file" .aiff)
      echo "  • $name"
    done
  elif [ "$OS_TYPE" = "Linux"* ]; then
    # Linux system sounds (varies by distribution)
    echo "Available Linux system sounds (sample):"
    find /usr/share/sounds -type f \( -name "*.ogg" -o -name "*.wav" \) 2>/dev/null | head -10 | while read file; do
      name=$(basename "$file")
      echo "  • $name"
    done
  fi
else
  echo ""
  echo "⚠️  No system sounds detected on this platform."
  echo "   Don't worry! You can use the built-in MP3 sounds included with the plugin."
  echo "   They work perfectly on all platforms!"
fi
```

**macOS system sounds** (if detected):
- **Glass** - Crisp, clean chime ✨
- **Ping** - Subtle ping sound 🏓
- **Pop** - Quick pop sound 🎈
- **Purr** - Gentle purr 🐱
- **Funk** - Distinctive funk groove 🎵
- **Hero** - Triumphant fanfare 🦸
- **Sosumi** - Pleasant notification 🔔
- **Basso** - Deep bass sound 🎻
- **Blow** - Breeze-like whoosh 💨
- **Frog** - Unique ribbit sound 🐸
- **Submarine** - Sonar-like ping 🌊
- **Bottle** - Cork pop sound 🍾
- **Morse** - Morse code beeps ⚡
- **Tink** - Light metallic sound ✨

**Linux system sounds** (if detected):
- Location varies by distribution (Ubuntu, Fedora, etc.)
- Typically in `/usr/share/sounds/`
- Formats: .ogg, .wav files

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## Step 3: Interactive Sound Preview 🔊

**CRITICAL INSTRUCTION FOR CLAUDE:**

Before asking the user to make final choices, you MUST offer to play sounds for them.

Tell the user:

> 🎵 **Want to hear sounds before choosing?**
> I can play any sound for you! Just say:
> - `"play task-complete"` - Built-in task-complete sound
> - `"play Glass"` - macOS Glass system sound
> - `"preview Hero"` - Preview any available sound
>
> Try as many as you like! When you're ready, I'll ask you to select sounds for each notification type.

**How to handle preview requests:**

When user says "play [sound_name]", "preview [sound_name]", "прослушать [sound_name]", or "проиграть [sound_name]":

1. **Extract sound name** from user message (ignore the command word, keep only the sound name)

2. **Determine the full path** to the sound file:
   ```bash
   # For built-in sounds (no extension needed)
   if [[ "$sound_name" == "task-complete" ]] || [[ "$sound_name" == "review-complete" ]] || [[ "$sound_name" == "question" ]] || [[ "$sound_name" == "plan-ready" ]]; then
     SOUND_PATH="${PLUGIN_ROOT}/sounds/${sound_name}.mp3"

   # For macOS system sounds
   elif [[ -f "/System/Library/Sounds/${sound_name}.aiff" ]]; then
     SOUND_PATH="/System/Library/Sounds/${sound_name}.aiff"

   # Try common variations
   elif [[ -f "/System/Library/Sounds/${sound_name}.mp3" ]]; then
     SOUND_PATH="/System/Library/Sounds/${sound_name}.mp3"

   else
     echo "❌ Sound '${sound_name}' not found. Available options listed above."
     exit 1
   fi
   ```

2. **Play the sound** using the sound-preview utility with reduced volume:
   ```bash
   echo "🔊 Playing: ${sound_name}... (volume: 30%)"
   bin/sound-preview --volume 0.3 "$SOUND_PATH"
   echo "✓ Playback complete!"
   ```

   **IMPORTANT:** Always use `--volume 0.3` (30% volume) when previewing sounds during setup to avoid disturbing the user with loud sounds.

3. **Ask if they want to hear more**:
   > Would you like to:
   > - Hear another sound? (just type "play [name]")
   > - Ready to make your selections? (type "ready")

**Examples of user interactions:**

```
User: play Glass
Claude: [runs bin/sound-preview --volume 0.3 /System/Library/Sounds/Glass.aiff]
Claude: "🔊 Playing: Glass... (volume: 30%) ✓ Playback complete! Would you like to hear another sound, or ready to choose?"

User: preview task-complete
Claude: [runs bin/sound-preview --volume 0.3 sounds/task-complete.mp3]
Claude: "🔊 Playing: task-complete... (volume: 30%) ✓ Playback complete!"

User: прослушать Hero
Claude: [runs bin/sound-preview --volume 0.3 /System/Library/Sounds/Hero.aiff]
Claude: "🔊 Playing: Hero... (volume: 30%) ✓ Playback complete!"

User: проиграть Ping
Claude: [runs bin/sound-preview --volume 0.3 /System/Library/Sounds/Ping.aiff]
Claude: "🔊 Playing: Ping... (volume: 30%) ✓ Playback complete!"

User: ready
Claude: "Great! Let's configure your notification sounds..."
[proceeds to Questions 1-4]
```

**Edge cases:**

```
User: play unknown-sound
Claude: "❌ Sound 'unknown-sound' not found. Available sounds are:
  Built-in: task-complete, review-complete, question, plan-ready
  System (macOS): Glass, Hero, Funk, Sosumi, Ping, Purr, Basso, etc.
Try: 'play Glass' or 'preview task-complete'"

User: I want Glass for everything
Claude: "Great choice! Let me confirm - you want Glass for all notification types?
Or would you like to choose different sounds for each type?
(You can still preview other sounds if you'd like)"
```

**IMPORTANT:**
- Allow users to preview AS MANY sounds as they want before making selections
- Be patient and encouraging - sound selection is personal!
- If a sound name isn't recognized, show the available sounds list again

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## Step 4: Interactive Configuration

Now let's configure your notification sounds! I'll ask you 4 questions - one for each notification type.

**IMPORTANT:** Build the options list dynamically based on what's available:

```bash
# Build options array based on OS and available sounds
OPTIONS=""

# Always include built-in sounds (available on all platforms)
OPTIONS="${OPTIONS}Built-in: task-complete.mp3|Triumphant completion chime (recommended)\n"
OPTIONS="${OPTIONS}Built-in: review-complete.mp3|Gentle notification tone\n"
OPTIONS="${OPTIONS}Built-in: question.mp3|Attention sound\n"
OPTIONS="${OPTIONS}Built-in: plan-ready.mp3|Professional tone\n"

# Add system sounds if available
if [ "$HAS_SYSTEM_SOUNDS" = "true" ] && [ "$OS_TYPE" = "Darwin"* ]; then
  # macOS system sounds
  OPTIONS="${OPTIONS}System: Glass|Crisp macOS Glass sound\n"
  OPTIONS="${OPTIONS}System: Hero|Triumphant fanfare\n"
  OPTIONS="${OPTIONS}System: Funk|Distinctive funk groove\n"
  OPTIONS="${OPTIONS}System: Sosumi|Pleasant macOS notification\n"
  OPTIONS="${OPTIONS}System: Ping|Subtle ping sound\n"
  OPTIONS="${OPTIONS}System: Purr|Gentle purr\n"
fi

echo "Available sound options built: $(echo -e "$OPTIONS" | wc -l) options"
```

### Question 1: Task Complete Sound ✅

**Before presenting the question**, remind the user:

> 🎵 **Reminder:** You can still preview sounds! Just say "play [sound_name]" before making your choice.

When Claude finishes a task, which sound would you like to hear?

Use AskUserQuestion with dynamically generated options:

**If macOS with system sounds:**
- question: "Which sound for Task Complete notifications?"
- header: "✅ Task Complete"
- multiSelect: false
- options:
  1. **Built-in: task-complete.mp3** - "Triumphant completion chime (recommended)"
  2. **Built-in: review-complete.mp3** - "Gentle notification tone"
  3. **Built-in: question.mp3** - "Attention sound"
  4. **Built-in: plan-ready.mp3** - "Professional tone"
  5. **System: Glass** - "Crisp macOS Glass sound"
  6. **System: Hero** - "Triumphant fanfare"
  7. **System: Funk** - "Distinctive funk groove"
  8. **System: Sosumi** - "Pleasant macOS notification"

**If Linux/Windows (no system sounds):**
- question: "Which sound for Task Complete notifications?"
- header: "✅ Task Complete"
- multiSelect: false
- options:
  1. **task-complete.mp3** - "Triumphant completion chime (recommended)"
  2. **review-complete.mp3** - "Gentle notification tone"
  3. **question.mp3** - "Attention sound"
  4. **plan-ready.mp3** - "Professional tone"

**Note:** System sounds are only available on macOS. On other platforms, use the built-in MP3 sounds which work perfectly everywhere!

**CRITICAL:** If user says "play [sound]" instead of choosing, DO NOT call AskUserQuestion yet. First play the sound, then re-ask the question.

### Question 2: Review Complete Sound 🔍

**Before presenting the question**, remind the user:

> 🎵 **Reminder:** You can preview sounds! Just say "play [sound_name]" before choosing.

When Claude completes a code review or analysis, which sound?

Use AskUserQuestion with the same dynamically generated options as Question 1.

### Question 3: Question Sound ❓

**Before presenting the question**, remind the user:

> 🎵 **Reminder:** You can preview sounds! Just say "play [sound_name]" before choosing.

When Claude has a question or needs clarification?

Use AskUserQuestion with the same dynamically generated options as Question 1.

### Question 4: Plan Ready Sound 📋

**Before presenting the question**, remind the user:

> 🎵 **Reminder:** You can preview sounds! Just say "play [sound_name]" before choosing.

When Claude finishes planning and is ready for your review?

Use AskUserQuestion with the same dynamically generated options as Question 1.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## Step 5: Notification Volume Configuration

Now let's configure the volume for your notification sounds.

Use AskUserQuestion with:
- question: "What volume level do you want for notification sounds?"
- header: "🔊 Volume"
- multiSelect: false
- options:
  1. **Full volume (100%)** - "Maximum volume (default)"
  2. **High volume (70%)** - "Loud but not maximum"
  3. **Medium volume (50%)** - "Balanced volume"
  4. **Low volume (30%)** - "Quiet, good for offices"
  5. **Very low (10%)** - "Very quiet, minimal distraction"

**Volume mapping:**
- "Full volume (100%)" → `1.0`
- "High volume (70%)" → `0.7`
- "Medium volume (50%)" → `0.5`
- "Low volume (30%)" → `0.3`
- "Very low (10%)" → `0.1`

**Important:** Parse the user's choice and extract the numeric value (e.g., "70%" → 0.7).

**Note:** You can offer to preview a sound at the selected volume:
```bash
echo "Let me play a quick test at your selected volume..."
bin/sound-preview --volume <selected_volume> sounds/task-complete.mp3
echo "How does that sound? (If you want to adjust, just let me know)"
```

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## Step 6: Webhook Configuration (Optional)

Do you want to send notifications to a webhook (Slack, Discord, Telegram)?

Use AskUserQuestion with:
- question: "Enable webhook notifications?"
- header: "🔗 Webhooks"
- multiSelect: false
- options:
  1. **No webhooks** - "Desktop notifications only (recommended)"
  2. **Slack** - "Send to Slack webhook (JSON format)"
  3. **Discord** - "Send to Discord webhook (embed format)"
  4. **Telegram** - "Send to Telegram bot (requires chat_id)"
  5. **Custom** - "Custom webhook endpoint (JSON)"

If webhook is enabled, I'll create a placeholder configuration that you can edit later.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## Step 7: Generate Configuration File

Based on your answers, I'll create `config/config.json`:

**Sound Path Construction (Important!):**

Parse the user's choice and construct the correct path:

```bash
# Function to convert user choice to file path
get_sound_path() {
  local choice="$1"

  # Check if it's a built-in sound
  if [[ "$choice" == "Built-in:"* ]] || [[ "$choice" == *".mp3" ]]; then
    # Extract filename
    filename=$(echo "$choice" | sed 's/Built-in: //' | sed 's/^[^:]*: //')
    echo "\${CLAUDE_PLUGIN_ROOT}/sounds/${filename}"

  # Check if it's a system sound (macOS)
  elif [[ "$choice" == "System:"* ]]; then
    # Extract sound name (e.g., "Glass" from "System: Glass")
    soundname=$(echo "$choice" | sed 's/System: //' | awk '{print $1}')
    echo "/System/Library/Sounds/${soundname}.aiff"

  # Fallback to built-in if parsing fails
  else
    echo "\${CLAUDE_PLUGIN_ROOT}/sounds/task-complete.mp3"
  fi
}

# Example usage:
TASK_COMPLETE_PATH=$(get_sound_path "$user_answer_1")
REVIEW_COMPLETE_PATH=$(get_sound_path "$user_answer_2")
QUESTION_PATH=$(get_sound_path "$user_answer_3")
PLAN_READY_PATH=$(get_sound_path "$user_answer_4")
```

**Examples:**
- Built-in: `${CLAUDE_PLUGIN_ROOT}/sounds/task-complete.mp3`
- System (macOS): `/System/Library/Sounds/Glass.aiff`
- Fallback (if parsing fails): Always use built-in MP3

**Configuration Template:**

```json
{
  "notifications": {
    "desktop": {
      "enabled": true,
      "sound": true,
      "volume": <user's selected volume>,
      "appIcon": "${CLAUDE_PLUGIN_ROOT}/claude_icon.png"
    },
    "webhook": {
      "enabled": <true if webhook selected>,
      "preset": "<slack|discord|telegram|custom>",
      "url": "<placeholder - user must edit>",
      "chat_id": "<for telegram only>",
      "format": "json",
      "headers": {}
    },
    "suppressQuestionAfterTaskCompleteSeconds": 7
  },
  "statuses": {
    "task_complete": {
      "title": "✅ Task Completed",
      "sound": "<user's choice>",
      "keywords": ["completed", "done", "finished", "успешно", "завершен"]
    },
    "review_complete": {
      "title": "🔍 Review Completed",
      "sound": "<user's choice>",
      "keywords": ["review", "ревью", "analyzed", "проверка", "analysis"]
    },
    "question": {
      "title": "❓ Claude Has Questions",
      "sound": "<user's choice>",
      "keywords": ["question", "вопрос", "clarify"]
    },
    "plan_ready": {
      "title": "📋 Plan Ready for Review",
      "sound": "<user's choice>",
      "keywords": ["plan", "план", "strategy"]
    }
  }
}
```

Write this to: `${PLUGIN_ROOT}/config/config.json`

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## Step 8: Summary & Test

After creating the configuration, show the user:

```
🎉 Configuration Saved Successfully!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📝 Summary:
  ✅ Task Complete    → <chosen sound>
  🔍 Review Complete  → <chosen sound>
  ❓ Question         → <chosen sound>
  📋 Plan Ready       → <chosen sound>

  🔊 Desktop notifications: ENABLED
  🔊 Volume: <selected volume>%
  🔗 Webhooks: <ENABLED/DISABLED>

Configuration file: config/config.json
```

### Test Your Setup

Ask user: "Would you like to test your task-complete notification now?"

If yes:
```bash
echo "Testing task-complete sound at your configured volume (<selected_volume>%)..."
bin/sound-preview --volume <selected_volume> "<path-to-chosen-sound>"
echo "✓ Sound test complete!"
```

**Note:** This test uses your configured volume level. The actual notifications will use this same volume.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## Additional Notes

**Editing Later:**
- You can re-run `/setup-notifications` anytime to reconfigure
- Or manually edit `config/config.json`

**Webhook Configuration:**
If you enabled webhooks, you'll need to manually edit `config/config.json` to add:
- **Slack:** Your webhook URL from Slack integrations
- **Discord:** Your webhook URL from Discord server settings
- **Telegram:** Bot token in URL + chat_id field
- **Custom:** Your endpoint URL and any required headers

**Sound Formats Supported:**
- MP3, WAV, FLAC, OGG/Vorbis, AIFF
- Cross-platform playback via gopxl/beep library

**System Sounds:**
- macOS: `/System/Library/Sounds/*.aiff`
- Linux: `/usr/share/sounds/**/*.ogg` (varies by distribution)
- Windows: Use custom sounds (system sounds not easily accessible)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## Tips for Best Experience

✨ **Sound Selection Tips:**
- Use distinct sounds for different notification types
- Choose sounds that won't be disruptive in your workspace
- Test sounds at your typical volume before finalizing

🎯 **Recommended Combinations:**

**Minimal Setup:**
- Task Complete: Glass (crisp, professional)
- Review Complete: Tink (subtle)
- Question: Sosumi (attention-grabbing)
- Plan Ready: Ping (gentle reminder)

**Power User Setup:**
- Task Complete: Hero (celebration!)
- Review Complete: Purr (satisfaction)
- Question: Funk (stand out)
- Plan Ready: Submarine (unique)

**Built-in Sounds:**
- Use the included MP3s if you want consistent cross-platform experience
- Plugin sounds work on all operating systems

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

**Ready to begin?** Let's start by choosing your sound source! 🎵
