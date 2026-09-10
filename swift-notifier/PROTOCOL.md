# Native protocol v1 (PR1, inactive by default)

This is the native foundation only. No installer, hook or feature enablement is
changed. New actions support only `none`; legacy CLI and persisted ClickAction
activate/execute/combined data continue to use their existing decoder. PR3 must
add its typed action and capability together; it must not repurpose legacy shell
actions or claim Codex navigation in PR1.

`--capabilities-json` must be the entire argv. It emits one JSON object plus LF,
without creating NSApplication, UNUserNotificationCenter, a delegate, permission
probe, sound or callback executor. It declares versions `[1]`, actions `["none"]`,
receipt support and backend `macos.usernotifications`; this is protocol support,
not authorization or a claim that a banner will appear. Future caller must verify
the offline managed artifact fingerprint/protocol floor first, then run a private
direct probe with a one-second deadline. Never probe an unknown/old binary.

Structured invocation (through LaunchServices, fresh instance):

```
--send-json --request-file /private/.../<correlation UUID>/<nonce UUID>.request --receipt-file /private/.../<correlation UUID>/<nonce UUID>.receipt -launchedViaLaunchServices
```

Paths must be absolute and free of symlink components (on macOS canonicalize
`/tmp` to `/private/tmp`, `/var` to `/private/var`). Resolve the trusted spool
root using `realpath` / Go `filepath.EvalSymlinks` before creating the attempt;
Foundation `resolvingSymlinksInPath` can retain the `/var` alias. The native
reader still rejects symlink components instead of resolving request paths.
The caller exclusively owns a fresh 0700 directory and
0600 regular request file, both owned by effective UID. Names are never reused.
Files are bounded to 16 KiB before allocation and decoding, locked nonblocking,
read through pinned directory descriptors, and consumed once before any effect.
An exclusive empty `<nonce>.claim` marker gives only one native instance receipt
ownership, including after body consumption. A competing or abandoned claim
exits without publishing a false rejection while another instance may submit.
The marker stays with the attempt for caller cleanup; it is not a replay journal.
Symlinks, hardlinks, special files, permissive modes and existing receipts fail
closed. File ownership isolates other users; it does not authenticate a caller
against malicious same-UID processes. The copied body is removed after reading;
invalid oversized/unowned files are left to the caller, never submitted.

Required request keys: schemaVersion (integer 1), correlationID (directory UUID),
nonce (filename UUID), bootID (`kern.bootsessionuuid`), notAfter (seconds from
`mach_continuous_time` converted with mach_timebase_info), title, body, category
(`info|attention|progress`), action (`none`), silent (boolean). Optional subtitle
is a single-line string or null. No unknown or duplicate keys, invalid UTF-8 or
unpaired surrogate escapes. UTF-8 limits: title/subtitle 256 bytes, body 4096;
Unicode control scalars (general category Cc, including NUL/ESC) are rejected
except LF/TAB in body. Format scalars (Cf), including ZWJ/ZWNJ and emoji tags,
are preserved without stripping or normalization. The Go producer must use
the same policy (`unicode.IsControl`, not the broader Cf category).
Text is never truncated.
Title/subtitle reject Unicode line separators as well. All text is literal.
Category does not enable time-sensitive interruption.

The sender inherits the original admission deadline, including validation,
locks, launches and sleep, with at most 15 seconds remaining. Boot mismatch,
expiry or a larger budget rejects before permission lookup. Permission is read
without requesting authorization; undetermined means activation_required, denied
means permission_denied. Deadline is rechecked after lookup and completion. A
serialized owner ignores repeated/late callbacks. Only positive UN `add`
completion yields submitted; OS error yields rejected, deadline after handoff
unknown. Timeout before handoff is rejected/expired. No retry, fallback or bell.

Receipt keys: schemaVersion=1, correlationID, nonce, notificationID (same as
correlationID), status, reason, retrySafe=false. Atomic exclusive publication
never overwrites another receipt or recreates a removed directory. Receipt has
no title/body. Structured send stdout is empty; exit 0 means a receipt was
written, not that delivery succeeded. Missing/invalid/mismatched receipt after
handoff means unknown. Invalid argv or unsafe directory cannot safely receive a
receipt and exit 1. The consumer must validate version, correlation, nonce and
status/reason, not just exit code. Suppressed is reserved for the future policy
layer and not emitted by native.

| Reason | Native status | Meaning |
| --- | --- | --- |
| malformed_request | rejected | JSON, fields, limits or correlation invalid |
| unsupported_version | rejected | unsupported schema |
| unsupported_action | rejected | action unavailable (including PR3 target) |
| invalid_file | rejected | unsafe/missing/bounded file read failed |
| expired | rejected | boot/deadline invalid or elapsed before handoff |
| activation_required | rejected | permission undetermined; setup needed |
| permission_denied | rejected | permission denied |
| unsupported_notifier | rejected | bundle/LaunchServices/boot clock unavailable |
| os_rejected | rejected | negative OS completion |
| os_accepted | submitted | positive OS completion, not banner/read/click |
| timeout | unknown | handoff could have occurred; never automatically retry |

The caller's future spool/admission owner must impose total byte/count limits,
retain attempts while LaunchServices may still launch, clean its expired orphans
with a bounded scan on notify/status/setup, and remove directories only after
terminal receipt plus sender exit, or original deadline under its ownership
protocol. PR1 creates no spool or request files and implements no journal,
installer, global cleanup scan or daemon. This per-attempt native reader neither
recreates a missing attempt nor depends on the caller's cwd. There is no promise
of cleanup at a fixed time after every owner has crashed. These caller gates are
required before activating a producer in later PRs.

Tests are pure codecs/parser/state-machine spies and isolated filesystem tests.
Eight legacy action fixtures characterize current source behavior, including
its intentional ignoring of extra schemaVersion; they are reconstructed here,
not copied from inaccessible historical spike artifacts. Run the complete Swift
suite on macOS. Linux source review cannot qualify AppKit/UN or macOS filesystem
APIs. Native permission, delayed LaunchServices, sleep, atomic receipt, old
artifact rejection and installed UI behavior remain macOS integration gates.
