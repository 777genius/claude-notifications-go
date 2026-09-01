# Multi-agent + Codex support: authoritative implementation contract

> [!IMPORTANT]
> This is the only normative implementation plan in this directory. Documents `01` through `06`
> preserve the investigation trail and are historical. If they conflict with this file, this file wins.
> Do not copy historical installer, config, CLI, trust, Go-version, or release-gate decisions into code.

The external Codex contract in this plan is pinned to `openai/codex` tag `rust-v0.152.0`: annotated
tag object `7f6bee13af649d0da23ac0c2bf5c83f571fcd611`, commit
`316795b3cf2a45e90d121d9f46499d4658b2645c`. Verified sources are
`codex-rs/config/src/hook_config.rs`, `codex-rs/hooks/src/schema.rs`,
`codex-rs/hooks/src/{lib.rs,declarations.rs,engine/discovery.rs}`,
`codex-rs/plugin/src/{manifest.rs,plugin_id.rs}`,
`codex-rs/core-plugins/src/{manifest.rs,marketplace.rs}`,
`codex-rs/exec-server-protocol/src/protocol.rs`, and `codex-rs/login/src/auth/storage.rs`. Before
implementation or release, re-check these locations against the target Codex version and record any
schema delta.

## 1. Scope and non-goals

This milestone keeps the existing Claude path behavior-compatible and adds Codex notifications
through the same Go binary.

In scope:

- Codex `Stop` and `PermissionRequest` notification delivery.
- Typed SDK decode for `Stop`, `PermissionRequest`, and `SubagentStop`. `SubagentStop` is decoded
  losslessly for SDK completeness, but product notification UX remains disabled until separately
  accepted.
- Native Codex plugin/marketplace discovery, not mutation of the user's `~/.codex/hooks.json`.
- One source-neutral product event contract and one existing notification pipeline.
- Go 1.22 minimum, required by `plugin-kit-ai/sdk`.

Not in scope:

- Cursor, OpenCode, and CodeBuddy runtime support. Add one only after its own verified host contract
  and accepted product scope; do not ship stub files or empty connectors now.
- A cross-product `status`/`doctor` command.
- Full parsing of Codex `rollout-*.jsonl` transcripts.
- Legacy Codex `notify = [...]` installation or `config.toml` surgery in this milestone.
- A new universal plugin governance or installer platform.

## 2. Repository boundary

Dependencies point inward toward product policy:

| Responsibility | Owner | Contract |
|---|---|---|
| Host payload decode and host signals | `plugin-kit-ai/sdk` | Platform DTOs only |
| Source normalization, classification, dedup, config, webhook formatting | `notification_plugin_go` | Product `Event`/`EventSource` |
| Desktop delivery, click-to-focus, CLI, plugin artifacts | `notification_plugin_go` | Framework/driver layer |

The SDK must never import or return the product's `Event`. It returns Codex/Claude DTOs; product-side
adapters map those DTOs to the product contract. This keeps decode reusable and prevents an
SDK-to-product dependency cycle.

The uncommitted Codex hooks SDK work mentioned in the historical documents was not recoverable.
Implementation starts from the verified schemas, not from an assumed local branch or worktree.

## 3. `sdk/hostdetect`

The selected location remains a public `sdk/hostdetect` package, but the MVP exposes only real
Claude and Codex signals.

```go
type Platform string

const (
    PlatformUnknown Platform = ""
    PlatformClaude  Platform = "claude"
    PlatformCodex   Platform = "codex"
)

type Env interface {
    LookupEnv(string) (string, bool)
}

type Signal struct {
    Platform      Platform
    EnvMarkers   []string
    PayloadSniff func(map[string]any) bool
}

type Registry []Signal

func DefaultRegistry() Registry
func Detect(registry Registry, override string, env Env, payload []byte) (Platform, error)
```

Invariants:

- A valid explicit override wins. An unknown override is an error, not an arbitrary `Platform`.
- `DefaultRegistry()` returns a fresh slice. Do not expose mutable package-global registry state.
- Detection fails closed with `PlatformUnknown`; it does not silently interpret an unknown host as
  Claude.
- Payload sniffing inspects bounded top-level JSON only and does not import platform decoder
  packages.
- Existing Claude hooks remain identifiable through the legacy `handle-hook <ClaudeEvent>` command
  and `CLAUDE_PLUGIN_ROOT`. Codex uses the same public event names with an explicit
  `--product codex`; only the internal SDK invocation names are prefixed.

Tests cover explicit override, invalid override, env markers, bounded payload sniffing, ambiguous
signals, unknown input, and registry isolation.

## 4. Codex trust and plugin identity

Codex persists a plugin hook state key derived from:

```text
<marketplace-plugin-name>@<marketplace-name>:<relative-hooks-path>:<snake_case-event>:<group-index>:<handler-index>
```

That stable key is not the trust decision by itself. Handler position belongs to the state key, not
the hash. Codex first selects the platform-effective command (`commandWindows` on Windows when set,
otherwise `command`), then hashes the normalized event, matcher, and single-handler config with that
effective command, timeout, async, status message, and additional-context limit. It does not hash both
command variants at once. The saved `trusted_hash` must equal the current platform hash; otherwise
the hook is `Modified` and requires review again.

Therefore keep all of these stable after the first release unless a migration with explicit
re-trust is accepted:

- marketplace plugin entry name and marketplace name;
- relative hooks path;
- event/group/handler ordering;
- matcher and command strings, including argv;
- timeout, async, and other hashed hook metadata.

`PLUGIN_ROOT` is expanded after the normalized config identity is built. Marketplace cache
relocation and changes to the contents of a wrapper/binary are safe when the configured
`${PLUGIN_ROOT}/...` command string remains unchanged. Changing that configured path is not safe.

### Manifest contract

`plugin_id` is not a Codex manifest field. The Codex-specific manifest must be a valid manifest,
declare the custom hooks file, and keep its version synchronized with the existing plugin and
marketplace metadata:

```json
{
  "name": "claude-notifications-go",
  "version": "<release-version>",
  "hooks": "./hooks/hooks-codex.json"
}
```

The actual plugin identity is `claude-notifications-go@claude-notifications-go`, derived from the
matching `plugins[].name` entry and top-level marketplace `name`, not directly from manifest `name`.
The Codex manifest name must still match the selected plugin entry. Marketplace plugin name,
marketplace name, and manifest name are therefore frozen compatibility fields.

### First-release hook identity

The first-release `hooks/hooks-codex.json` is exactly this two-handler contract; `SubagentStop`
remains SDK-only and is intentionally absent:

```json
{
  "description": "Desktop notifications for Codex Stop and PermissionRequest events.",
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "sh \"${PLUGIN_ROOT}/bin/hook-wrapper.sh\" handle-hook Stop --product codex",
            "commandWindows": "cmd.exe /d /s /c call \"${PLUGIN_ROOT}\\bin\\hook-wrapper.cmd\" handle-hook Stop --product codex",
            "timeout": 30,
            "async": true
          }
        ]
      }
    ],
    "PermissionRequest": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "sh \"${PLUGIN_ROOT}/bin/hook-wrapper.sh\" handle-hook PermissionRequest --product codex",
            "commandWindows": "cmd.exe /d /s /c call \"${PLUGIN_ROOT}\\bin\\hook-wrapper.cmd\" handle-hook PermissionRequest --product codex",
            "timeout": 30,
            "async": true
          }
        ]
      }
    ]
  }
}
```

There is one matcher group and one handler per event, with matcher, `statusMessage`, and
`additionalContextLimit` omitted. The launcher paths are `bin/hook-wrapper.sh` and
`bin/hook-wrapper.cmd`; both accept the exact argv shown above. This is the candidate identity until
the disposable macOS/Linux/Windows proof passes. It may be corrected before the first public Codex
release; after that release, changing any hashed field, group/handler order, launcher path, or argv
requires an explicit re-trust migration. A golden test serializes the normalized identity for both
handlers on Unix and Windows separately and detects drift in either effective command.

Before implementation proceeds past the plugin-artifact stage, a disposable Codex home must prove:

1. the manifest loads;
2. `hooks/hooks-codex.json` is discovered;
3. `${PLUGIN_ROOT}` expands to the installed plugin root;
4. the hook is initially untrusted, becomes trusted through `/hooks`, and stays trusted after a
   plugin cache relocation with unchanged handler config;
5. an intentional command change becomes `Modified` and is surfaced as a migration.

## 5. SDK Codex event contract

Keep the existing legacy `sdk/codex/Notify` API unchanged. Add prefixed invocation names to avoid
the current flat-resolver collision with Claude:

| Invocation | SDK event | Carrier |
|---|---|---|
| `CodexStop` | `Stop` | stdin JSON |
| `CodexSubagentStop` | `SubagentStop` | stdin JSON |
| `CodexPermissionRequest` | `PermissionRequest` | stdin JSON |

Required DTO coverage:

- Common: `session_id`, `turn_id`, nullable `transcript_path`, `cwd`, `hook_event_name`, `model`,
  `permission_mode`.
- Stop: `stop_hook_active`, nullable `last_assistant_message`.
- SubagentStop: all Stop fields plus `agent_id`, `agent_type`, nullable
  `agent_transcript_path`.
- PermissionRequest: `tool_name`, arbitrary `tool_input`, optional `agent_id` and `agent_type`.

Decoder rules:

- The SDK's 1 MiB payload limit remains the single wire limit and is exported as
  `pluginkitai.MaxPayloadBytes`; the product must not duplicate the magic number.
- Empty or malformed payloads return typed errors.
- UTF-8/non-ASCII input is covered, including the known Windows risk.
- Observation handlers encode empty stdout on success.
- A regression test proves raw `Stop` still resolves to Claude while `CodexStop` resolves to Codex.
- Generated artifacts and the public stability/support documentation are updated together.

## 6. Product runtime contract

### 6.1 Source-neutral event

The product owns the normalized input contract:

```go
type EventKind string

const (
    EventStop              EventKind = "stop"
    EventSubagentStop      EventKind = "subagent_stop"
    EventPermissionRequest EventKind = "permission_request"
    // Existing Claude-only kinds remain represented without lossy mapping.
)

type Event struct {
    Product             string
    Kind                EventKind
    PayloadEventName    string
    Raw                 json.RawMessage
    SessionID           string
    TurnID              string
    CWD                 string
    TranscriptPath      string
    Message             string
    LastAssistantMessage string
    ToolName            string
    ToolUseID           string
    ToolInput           json.RawMessage
    Model               string
    PermissionMode      string
    TeamName            string
    TeammateName        string
    AgentID             string
    AgentType           string
    AgentTranscriptPath string
    ParentSessionID     string
    ParentToolUseID     string
    StopHookActive      bool
}

type EventSource interface {
    Decode(context.Context, string, io.Reader) (Event, error)
}
```

`Status` is derived product policy and is not a decoder field.

Mapping invariants:

- `ClaudeSource` preserves every current `HookData` field and current raw-event behavior.
- The legacy Claude entrypoint is never converted to `io.ReadAll`. `ClaudeSource` keeps BOM skipping,
  decodes exactly the first JSON value from the reader without waiting for EOF, stores that value in
  `Raw`, and ignores trailing data exactly as the current decoder does.
- `Kind` comes from the validated outer argv; `PayloadEventName` is diagnostic and never selects a
  route. A mismatch is logged but cannot redirect execution.
- `Raw` preserves unknown future fields and the difference between absent and `null`; it must be
  treated as sensitive and never logged wholesale.
- `CodexSource` maps public `Stop` to internal SDK invocation `CodexStop`, then to `EventStop`.
- Codex Stop classification uses `last_assistant_message`; it never parses Codex rollout JSONL with
  the Claude analyzer.
- Codex `Stop`/`SubagentStop` with `stop_hook_active=true` does not emit a notification, preventing
  recursive/continuation duplicates.
- Claude Stop continues to analyze `TranscriptPath` exactly as before.
- PermissionRequest adds `StatusPermissionRequest = "permission_request"` across analyzer status,
  config defaults/validation, summary body, notifier urgency, and webhook formatting. It is
  time-sensitive and uses the tool identity in the body.
- `ToolInput` stays raw for typed consumers but is never logged or displayed wholesale; any body
  projection uses an allowlist plus redaction and truncation.
- If SubagentStop product delivery is enabled later, dedup includes product, session, turn, and
  agent identity so parallel subagents cannot collapse into one notification.
- Claude continues to pass the original byte-for-byte `sessionID` and original case-sensitive hook
  event to the existing state/dedup managers, preserving all pre-upgrade filenames and cooldown
  state. Codex uses filename-safe SHA-256 identities over length-prefixed fields: product+session for
  session state, and product+session+turn+agent/tool identity for event dedup. Never concatenate raw
  ids with `:` or path separators.

`NewHandler(pluginRoot string)` keeps its signature and Claude behavior. Add an explicit
source-injected constructor for the composition root; existing Claude tests must pass unchanged.

### 6.2 Exact SDK adapter wiring

The existing no-flag Claude invocation remains the Claude route and keeps its current input
semantics. The explicit Codex route reads stdin once with a 1 MiB + 1 byte bounded read before host
detection, maps the public event to the prefixed SDK invocation, and constructs the SDK with
synthetic argv and in-memory IO:

```go
app := pluginkitai.New(pluginkitai.Config{
    Args: []string{argv0, sdkInvocation}, // e.g. CodexStop
    IO:   newBufferedSDKIO(rawPayload),   // ReadStdin returns the saved bytes
    Env:  env,
})
```

The IO adapter returns a copy of the saved payload and captures SDK stdout/stderr; it never forwards
them directly to the host. The registered callback stores the typed DTO and maps it to `Event`. A
non-zero SDK result, missing
callback result, decode failure, or panic is logged to `notification-debug.log`, then the outer
observation hook returns exit 0 with empty stdout and stderr so notification failures cannot block
Codex.

Any existing permission-guidance `systemMessage` output must use an injected sink: the Claude route
keeps its current stdout behavior, while the Codex observation route uses `io.Discard`. No current
`fmt.Printf` path may leak into Codex stdout.

An integration test must execute the exact public form:

```text
<binary> handle-hook Stop --product codex
```

with stdin JSON, and prove the outer parser rejected duplicate/unknown flags, the SDK received
synthetic `[binary, CodexStop]`, consumed the payload once, invoked the Codex callback once, emitted
no process output, and produced the normalized event. Malformed, oversized, panic, and SDK-error
cases still return outer exit 0 with empty process stdout/stderr and never call the notifier.
Once `--product codex` is recognized, unsupported events, duplicate/unknown flags, missing values,
decode errors, and initialization failures all use that same fail-open process contract and are
file-logged only. Ordinary non-Codex human CLI usage errors keep the existing non-zero/stderr UX.

## 7. Configuration and resources

For this milestone, keep the existing canonical config file for every product:

```text
~/.claude/claude-notifications-go/config.json
```

Do not add `~/.codex/claude-notifications-go/config.json`; two writable sources would create
split-brain precedence and migration problems. Codex-only installation creates the existing stable
directory when needed. Shared notification settings apply to both products. A product-specific
override section is a future additive change only after a real differing setting is required.

`getPluginRootForProduct()` preserves today's Claude precedence (`CLAUDE_PLUGIN_ROOT`, then current
fallbacks). The Codex route prefers native `PLUGIN_ROOT`, then uses `CLAUDE_PLUGIN_ROOT` only as a
compatibility fallback. Resource paths and sounds resolve from that explicit root; no mutable
pointer-file installer protocol is introduced for Codex.

## 8. Dependency-safe implementation stages

1. **SDK host detection**: implement only Claude/Codex signals and the immutable registry API.
2. **SDK Codex events**: DTOs, wrappers, descriptors, generation, collision tests, docs.
3. **SDK release**: merge/tag a real `sdk` version. No local `replace` reaches product CI.
4. **Product Go 1.22 floor**: update `go.mod` and all OS CI matrices before importing the SDK.
5. **Product event boundary**: add lossless `Event`/`EventSource`, preserve Claude behavior.
6. **Codex adapter and CLI wiring**: bounded read, host detection, synthetic SDK args/IO, normalized
   event routing, output containment.
7. **Codex plugin artifacts**: valid manifest, stable marketplace identity, stable hook config, and
   platform launchers. Windows is not declared supported until `command_windows`/launcher behavior
   is proven in a disposable Windows environment.
8. **Tests and release docs**: focused unit/integration/E2E, README limitations, troubleshooting,
   changelog, and release gate.

Deliver these as dependency-safe PRs near the repository's review budget. Each PR starts from main
or its declared stacked predecessor, passes focused gates, and is independently revertible. Do not
combine the SDK, product refactor, and cross-platform installer into one mega-PR.

## 9. Hermetic verification

### 9.1 Unit and integration tests

- SDK: host detection, all DTO fields, `limit-1`/`limit`/`limit+1`, multibyte UTF-8, reader error,
  resolver collision, and empty response.
- Product: Claude regression suite unchanged; Codex mapping/classification; exact outer CLI wiring;
  dedup product/turn/agent identity; config remains single-source. Claude compatibility fixtures
  cover BOM, trailing JSON/data, a reader that supplies one complete value without EOF, and unchanged
  legacy state/dedup filenames across upgrade.
- Manifest: parse the real `.codex-plugin/plugin.json`, resolve the custom hooks path, assert version
  equality across `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json` metadata and plugin
  entry, `.codex-plugin/plugin.json`, and the Go binary version, and freeze the normalized handler
  config in a golden test.
- Release-selector test: changes to any Codex-owned path must activate the Codex release gate.

### 9.2 Disposable Codex E2E

Never run hook/install/runtime tests against the user's real project, real `HOME`, or real Codex
configuration. The harness builds a fail-closed environment from an allowlist instead of inheriting
the user's shell environment. Every automated/live E2E uses a newly created test project and:

- a temporary `HOME`;
- `USERPROFILE`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `XDG_STATE_HOME`, `XDG_DATA_HOME`, `APPDATA`,
  `LOCALAPPDATA`, `TMPDIR`, `TEMP`, and `TMP` redirected to dedicated directories under the same
  sandbox (with the platform-irrelevant variables harmlessly set there too);
- a temporary `CODEX_HOME` inside that home;
- only the minimum auth seed needed for one explicitly planned Codex invocation; the source seed is
  mounted/read read-only, while its sandbox `auth.json` copy is writable mode `0600` on POSIX and
  protected by a sandbox-user-only ACL on Windows so token refresh cannot touch the real profile;
- `cli_auth_credentials_store = "file"` pinned in the exact-version sandbox config, with keyring and
  secrets-store modes rejected and a platform assertion that no keychain API is accessed;
- no copied `config.toml`, hooks state, history, transcripts, or user plugin cache;
- injected desktop and webhook sinks that record locally and cannot contact real destinations;
- process/runner-level default-deny egress with a narrowly audited allowlist for the one explicitly
  planned Codex provider connection; marketplace/cache inputs are local-only, update checks,
  telemetry, MCP, Git network access, proxies, and inherited network credentials are disabled;
- a synthetic notification message and a disposable plugin marketplace/cache;
- one bounded attempt per scenario, tracked by a durable control-plane ledger outside the disposable
  runner, keyed by repository + candidate SHA + scenario;
- a post-run assertion that no file was written outside the sandbox roots.

Before launch, materialize a clean plugin bundle from the exact candidate commit inside the sandbox;
the native marketplace source and `${PLUGIN_ROOT}` must point only to that copy, never this checkout
or another real project. Materialize the built exact-head binary inside the bundle. Resolve every
symlink and reject outside-root targets, reject hardlinks to files outside the materialized tree, and
reject hooks/launchers whose resolved targets escape the bundle. Canonicalize the complete bundle
tree, test project, every home/config/cache/temp path,
plugin cache, config path, log, socket, state file, and recording sink and assert that each is a
descendant of the sandbox root. Clear inherited `CLAUDE_CONFIG_DIR`, `CLAUDE_PLUGIN_ROOT`,
`PLUGIN_ROOT`, webhook URLs, notification credentials, and product-specific config overrides, then
inject only the sandbox values required by the scenario. Any unresolved or outside-root path aborts
before Codex starts.
Run live E2E only on a runner where the egress policy is enforceable; otherwise run the offline
fixtures and report the live gate as not proven. After the run, the network recorder must show zero
non-provider connections. Delete the writable auth copy, destroy the ephemeral workspace/runner, and
assert that auth contents never appear in logs or retained artifacts. The phase ledger transitions
`not_started -> provider_started -> completed|uncertain` using atomic compare-and-set, and commits
`provider_started` before the request. A new workflow run id cannot claim the same SHA/scenario again.
A failed pre-provider phase may be retried; after `provider_started`, automatic, manual, and workflow
retries are forbidden until provider effects/spend are inspected. An uncertain result stops the gate
rather than rerunning it; only a documented owner resolution with proof that no provider effect
occurred may clear the attempt key.

Required scenarios:

1. plugin add/discovery and valid manifest;
2. initial `/hooks` trust and trusted execution;
3. Stop decode to normalized event and captured notification sink;
4. PermissionRequest through an interactive test mode when it can be forced deterministically;
5. cache relocation with unchanged trust hash;
6. intentional handler-config mutation becomes `Modified`;
7. malformed/oversized/non-ASCII payload behavior;
8. uninstall/removal performed by the native plugin manager without touching unrelated config;
   upstream-retained `hooks.state` for this plugin is accepted and documented, reinstalling the same
   identity/hash remains trusted, while a changed handler becomes `Modified`; unrelated state/cache
   canaries remain byte- or semantic-identical;
9. macOS/Linux launchers and a separately proven Windows launcher before claiming Windows support.

Manual GUI smoke for sound/click-to-focus is separate, explicit, and uses only the synthetic test
session, a unique test-only bundle id, and a disposable OS user/VM. Production bundle ids are
forbidden. Automated tests do not send real desktop notifications or register production identities.

## 10. Release gate

The Codex gate is required when the diff touches any of:

- `go.mod`, `go.sum`, or the Go-version matrix in `.github/workflows/ci-*.yml`;
- `internal/codexsource/**`, `internal/hooks/**`, or host-detection/CLI wiring in
  `cmd/claude-notifications/**`;
- `.codex-plugin/**`, `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`,
  `hooks/hooks-codex.json`, `config/config.json`, `setup.sh`, `bin/hook-wrapper.sh`,
  `bin/hook-wrapper.cmd`, `bin/install.sh`, or `bin/bootstrap.sh`;
- shared product behavior under `internal/analyzer/**`, `internal/audio/**`, `internal/config/**`,
  `internal/daemon/**`, `internal/dedup/**`, `internal/errorhandler/**`, `internal/hooks/**`,
  `internal/logging/**`, `internal/notifier/**`, `internal/platform/**`, `internal/sessionname/**`,
  `internal/sounds/**`, `internal/state/**`, `internal/summary/**`, `internal/webhook/**`, or
  `internal/winfocus/**`;
- delivery assets under `sounds/**`, `swift-notifier/**`, or `claude_icon.png`;
- `.github/workflows/release.yml` or the selector helper/tests themselves.

Implement the selector as a tested script/helper, not a prose-only grep. A protected pre-tag
`workflow_dispatch` accepts only a full commit SHA from this repository and proves that it equals the
head of the allowed protected release branch, descends from the previous reachable release tag, and
still equals that branch head immediately before tag creation. It runs exact-head CI and any selected
Codex gate, then stores Codex version, sandbox identity, scenarios, and result in an immutable
out-of-tree check/attestation keyed to that SHA. The pre-tag workflow verifies the attestation and
creates only the tag; humans do not push tags directly. A post-run evidence commit is invalid because
it changes the head. The existing tag-triggered workflow must verify that tag target and attested SHA
are identical before it alone builds assets and creates the GitHub Release. Unrelated releases may
skip live Codex E2E only when the tested selector proves no relevant behavior changed.

A synchronized version-only release bump may skip live Codex E2E only when a semantic parser proves
that, among Codex-owned trigger paths, the only behavioral-file changes are all five required version
occurrences (ordinary changelog/docs updates remain allowed): the Go binary `const version`,
`.codex-plugin/plugin.json.version`, `.claude-plugin/plugin.json.version`,
`.claude-plugin/marketplace.json.metadata.version`, and the matching marketplace plugin's `version`.
All five must be identical. Any other code, manifest, or marketplace delta triggers the normal
selector. This carve-out has positive, negative, missing-occurrence, and mixed-change fixtures.

The pre-tag workflow compares the exact candidate commit with the previous reachable release tag and
fetches full history (`fetch-depth: 0`). Positive/negative fixture coverage includes every ownership
path above and the first release with no previous tag. The SDK repository owns a separate release
gate; the product gate does not prove SDK E2E.

## 11. Residual risks that remain explicit

1. Codex-only bootstrap UX and the exact marketplace command must be proven with the target Codex
   version before public installation docs are finalized.
2. PermissionRequest cannot fire when Codex never asks for approval; bypass/never modes therefore
   cannot provide permission notifications.
3. Windows launcher/trust behavior is unsupported until the disposable Windows scenario passes.
4. Codex status classification is intentionally thinner than Claude transcript analysis.
5. Codex v0.152.0 exposes 12 hook events; this milestone delivers 2/12 (`Stop`,
   `PermissionRequest`) and the SDK decodes 3/12 (those two plus `SubagentStop`).
