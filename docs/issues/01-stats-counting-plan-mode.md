# Issue #01: Stats Counting in Plan Mode

**Status:** Open
**Priority:** Medium
**Affects:** Summary generation (Edited X files, Ran Y commands)
**Date:** 2025-10-19

---

## Problem Description

Notification summaries show incorrect tool counts when Claude is in Plan Mode or when user doesn't send text messages for extended periods.

### Example

User complaint:
> "Почему-то только что написало Edited 32 files. Ran 36 commands - слишком много!"

**Expected:** Show stats for current response only
**Actual:** Shows stats accumulated over 2.5 hours

### Root Cause

`GetLastUserTimestamp()` in `pkg/jsonl/jsonl.go:192` only finds user messages with `type=="text"`:

```go
func GetLastUserTimestamp(messages []Message) string {
    for i := len(messages) - 1; i >= 0; i-- {
        if msg.Type == "user" && msg.Message.Content[0].Type == "text" {
            return msg.Timestamp  // ← Returns OLD timestamp
        }
    }
}
```

**In Plan Mode:**
- User enters plan mode (no text messages sent)
- User asks questions via UI (creates tool_result, not text)
- Last text message may be hours old
- `countToolsByType()` counts ALL tools since last text → wrong stats

**Example timeline:**
```
18:02 - User: "Продолжай" (text message)
18:05 - [Plan Mode activated]
18:10 - User clicks button → tool_result (NOT text)
18:15 - User clicks button → tool_result (NOT text)
20:30 - Stop hook called
```

At 20:30:
- `GetLastUserTimestamp()` returns `18:02` (2.5 hours ago)
- Counts ALL tools from 18:02 to 20:30
- Shows "Edited 32 files, Ran 36 commands" ❌

---

## Investigated Solutions

### ❌ Variant 1: Gap Detection (Rejected)

**Idea:** Detect Stop hook by finding gaps between messages > 3 seconds

```go
// Find gap > 3s between consecutive messages
for i := len(messages)-1; i > 0; i-- {
    gap := messages[i].Time - messages[i-1].Time
    if gap > 3 {
        return messages[i].Timestamp  // After gap
    }
}
```

**Problems:**
- Magic number (3 seconds) - unreliable
- Depends on network latency, processing time
- False positives: slow tools (git, npm install)
- Not deterministic

**Verdict:** Too brittle, rejected

---

### ❌ Variant 2: State-based with Fixed TTL (Issue Found)

**Idea:** Save hook timestamp in SessionState with 60s TTL

**Problems:**
1. **TTL too short:** State cleaned up after 60s, but Plan Mode sessions can last 10+ minutes
2. **User message priority issue:** Old user message (11 min) takes priority over fresh hook (2 min)

**Example failure:**
```
20:00 - User text message
20:05 - Stop hook (saved to state)
20:11 - State cleaned up (60s TTL expired)
20:12 - Next Stop hook:
        - userTS = 20:00 (11 min old)
        - hookTS = missing (cleaned up)
        - Uses 20:00 → counts 12 minutes of tools ❌
```

**Verdict:** Unreliable due to cleanup timing

---

### ⚠️ Variant 3: Hook Timestamp in State (Current Approach)

**Idea:**
1. Increase state TTL to 10 minutes
2. Save hook timestamp when Stop/Notification called
3. Use as fallback when user text not found

**Implementation:**

```go
// state/state.go
type SessionState struct {
    // ... existing fields ...
    LastHookTimestamp int64  `json:"last_hook_ts,omitempty"`
    LastHookType      string `json:"last_hook_type,omitempty"`
}

// hooks/hooks.go
func (h *Handler) HandleHook() {
    // Save hook timestamp IMMEDIATELY
    h.stateMgr.UpdateHookTimestamp(sessionID, hookEvent)

    // Use as fallback in summary
    fallbackTS := getHookTimestampFromState(sessionID)
    message := summary.GenerateFromTranscript(path, status, cfg, fallbackTS)
}

// summary/summary.go
func countToolsByType(messages, fallbackTS) {
    userTS := GetLastUserTimestamp(messages)
    if userTS == "" {
        userTS = fallbackTS  // Fallback to hook timestamp
    }
    // Count tools after userTS
}
```

**Pros:**
✅ Persistent (10 min TTL)
✅ Per-session (via SessionState)
✅ No magic numbers

**Cons:**
❌ Still has priority issues (see Variant 4)
❌ Doesn't handle Notification → Stop sequences well

---

### ⚠️ Variant 4: Max(user, hook) Timestamp (Edge Cases)

**Idea:** Take the MOST RECENT timestamp (user OR hook)

```go
func countToolsByType(messages, fallbackTS) {
    userTS := GetLastUserTimestamp(messages)

    var userTime, hookTime time.Time
    if userTS != "" {
        userTime = parse(userTS)
    }
    if fallbackTS != "" {
        hookTime = parse(fallbackTS)
    }

    // Take MAXIMUM (most recent)
    sinceTime := max(userTime, hookTime)

    // Count tools after sinceTime
}
```

**Pros:**
✅ Always uses freshest marker
✅ Handles old user messages correctly

**Edge Cases Found:**

#### Edge Case 1: hookTimestamp is CURRENT hook
```
20:00 - User message
20:01 - Edit tool
20:02 - Bash tool
20:03 - Stop hook called → hookTS = time.Now() = 20:03

Summary:
- userTS = 20:00
- hookTS = 20:03 (just recorded!)
- max(20:00, 20:03) = 20:03
- Counts from 20:03 → MISSES Edit and Bash! ❌
```

**Issue:** We need PREVIOUS hook timestamp, not CURRENT!

#### Edge Case 2: Notification → Stop sequence
```
20:00 - Notification hook → hookTS = 20:00
20:01 - User answer (tool_result, not text)
20:02 - Claude tools
20:03 - Stop hook

Stop summary:
- userTS = "" (no text after Notification)
- hookTS = 20:00 (Notification timestamp)
- Uses 20:00 → counts from Notification → WRONG window! ❌
```

**Issue:** Need to differentiate hook types and track sequences

#### Edge Case 3: Multiple Stops in session
```
20:00 - User text
20:05 - Stop #1 → hookTS = 20:05
20:10 - Stop #2
      - userTS = 20:00
      - hookTS = 20:05
      - max = 20:05
      - Counts from Stop #1 → Stop #2 ✅ (accidentally correct!)
```

**Verdict:** Works for simple cases (~70% confidence), but has edge cases

---

## Recommended Solution (TBD)

**Option A: Track Previous + Current Hook**

Save two timestamps in state:
```go
type SessionState struct {
    PreviousHookTimestamp int64
    CurrentHookTimestamp  int64
    CurrentHookType       string
}
```

Count tools between PreviousHook and CurrentHook.

**Option B: Include tool_result in user messages**

Modify `GetLastUserTimestamp()` to accept ANY user message:
```go
func GetLastUserTimestamp(messages) {
    for i := len(messages) - 1; i >= 0; i-- {
        if msg.Type == "user" {  // ANY user message
            return msg.Timestamp
        }
    }
}
```

**Pros:** Simpler, no state needed
**Cons:** May count from tool_result instead of actual user input

---

## TODO

- [ ] Decide on recommended solution
- [ ] Implement chosen approach
- [ ] Add tests for edge cases:
  - [ ] Plan Mode (no text messages)
  - [ ] Notification → Stop sequence
  - [ ] Multiple Stops in one session
  - [ ] Long pauses (10+ minutes)
- [ ] Test with real usage scenarios
- [ ] Update ARCHITECTURE.md if needed

---

## Related Code

- `pkg/jsonl/jsonl.go:192` - GetLastUserTimestamp()
- `internal/summary/summary.go:361` - countToolsByType()
- `internal/state/state.go` - SessionState
- `internal/hooks/hooks.go:274` - generateMessage()

---

## Discussion Notes

**2025-10-19:** Initial investigation revealed Plan Mode issue. Explored multiple solutions but all have edge cases. Need to validate approach with comprehensive testing before implementation.
