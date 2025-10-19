---
description: Interactive setup wizard for claude-notifications plugin
allowed-tools: Bash, AskUserQuestion, Write, Read
---

# Setup Notifications Plugin

You are helping the user configure the claude-notifications plugin interactively. Follow these steps carefully:

## Step 1: Detect System Sounds

First, detect the operating system and find available system sounds:

**For macOS:**
- List sounds in `/System/Library/Sounds/`
- Common sounds: Glass.aiff, Ping.aiff, Pop.aiff, Purr.aiff, Funk.aiff, Hero.aiff, Sosumi.aiff, Basso.aiff, Blow.aiff, Frog.aiff, Submarine.aiff, Bottle.aiff, Morse.aiff, Tink.aiff

**For Linux:**
- Check `/usr/share/sounds/` and subdirectories
- Look for .ogg, .wav files

## Step 2: Present Available Sounds

Show the user a formatted list of all available sounds with descriptions. For example:

**Available System Sounds:**
- **Glass** - Crisp, clean chime
- **Ping** - Subtle ping sound
- **Hero** - Triumphant fanfare
- **Funk** - Distinctive funk sound
- **Pop** - Quick pop sound
- **Purr** - Gentle purr
- **Basso** - Deep bass sound
- **Sosumi** - Pleasant notification
- **Tink** - Light metallic sound
- **Frog** - Unique ribbit sound
- **Submarine** - Sonar-like ping
- (etc.)

Tell the user: "You can ask me to play any sound before making your choice. For example, say 'play Glass' or 'прослушать Hero'. When you're ready, I'll ask you to select sounds for each notification type."

## Step 3: Interactive Sound Selection

For EACH notification type (Task Complete, Review Complete, Question, Plan Ready):

1. **Announce the notification type** you're configuring
2. **Remind the user** they can request to play any sound (e.g., "play Funk", "прослушать Ping")
3. **Wait for user to explore sounds** - If user requests to play a sound:
   - Play it using bash: `afplay /System/Library/Sounds/[SoundName].aiff` (macOS) or `paplay /usr/share/sounds/[file]` (Linux)
   - Ask if they want to hear more sounds or are ready to choose
4. **When user is ready**, use AskUserQuestion to gather their final choice

## Step 4: Ask Confirmation Questions

After the user has explored sounds for each notification type, use AskUserQuestion to confirm their selections. Structure the questions as follows:

**For each notification type (Task Complete, Review Complete, Question, Plan Ready):**
- **question**: "Which sound would you like for '[Type]' notifications?"
- **header**: "[Type]"
- **multiSelect**: false
- **options**: List all available system sounds with brief descriptions

**Example options format:**
- Glass - "Crisp, clean chime"
- Ping - "Subtle ping sound"
- Hero - "Triumphant fanfare"
- Funk - "Distinctive funk sound"
- Pop - "Quick pop sound"
- Purr - "Gentle purr"
- Basso - "Deep bass sound"
- Sosumi - "Pleasant notification"
- (include all detected sounds)

### Webhook Configuration
**question**: "Do you want to enable webhook notifications?"
**header**: "Webhook"
**multiSelect**: false
**options**:
- "No" - "Desktop notifications only"
- "Yes, JSON format" - "Send structured JSON to webhook"
- "Yes, text format" - "Send plain text to webhook"

**If webhook is enabled**, inform the user:
"Please edit `config/config.json` manually to add your webhook URL and any custom headers."

## Step 5: Create Configuration File

Based on the user's answers, create the `config/config.json` file. Use the following template:

```json
{
  "notifications": {
    "desktop": {
      "enabled": true,
      "sound": true
    },
    "webhook": {
      "enabled": <true/false based on answer>,
      "url": "",
      "format": "<json/text based on answer>",
      "headers": {}
    }
  },
  "statuses": {
    "task_complete": {
      "title": "✅ Task Completed",
      "sound": "/System/Library/Sounds/<user's choice>",
      "keywords": ["completed", "done", "finished", "успешно", "завершен"]
    },
    "review_complete": {
      "title": "🔍 Review Completed",
      "sound": "/System/Library/Sounds/<user's choice>",
      "keywords": ["review", "ревью", "analyzed", "проверка", "analysis"]
    },
    "question": {
      "title": "❓ Question",
      "sound": "/System/Library/Sounds/<user's choice>",
      "keywords": ["question", "вопрос", "clarify"]
    },
    "plan_ready": {
      "title": "📋 Plan Ready",
      "sound": "/System/Library/Sounds/<user's choice>",
      "keywords": ["plan", "план", "strategy"]
    }
  }
}
```

**IMPORTANT**:
- Get the plugin directory path using: `cd "$(dirname "$(readlink -f "$0" || echo "$0")")"`
- Write the config to `config/config.json` relative to the plugin root directory
- For Linux, adjust sound paths to use `/usr/share/sounds/` instead

## Step 6: Confirm Success

After creating the config file:
1. Show the user the generated configuration
2. Confirm setup is complete
3. Suggest they can re-run `/setup-notifications` anytime to reconfigure
4. If webhook was enabled, remind them to edit the URL in `config/config.json`

## Step 7: Test Notification

Offer to test the notification by playing the selected "task_complete" sound to confirm it works.

---

**Remember**: Be friendly, clear, and help the user understand each step. Make this setup experience smooth and enjoyable!
