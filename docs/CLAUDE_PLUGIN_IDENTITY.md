# Claude Plugin Identity Compatibility

The public product and repository are named **Agent Notifications** and
`agent-notifications`. The following Claude Code identifiers intentionally retain the legacy
name and must not be changed as part of branding or routine cleanup:

- `.claude-plugin/plugin.json` -> `name`
- `.claude-plugin/marketplace.json` -> top-level `name`
- `.claude-plugin/marketplace.json` -> `plugins[].name`
- the resulting `/claude-notifications-go:*` command namespace

Use `displayName: "Agent Notifications"`, descriptions, and the current repository URL for
user-facing branding.

## Why the identifiers are frozen

Claude Code keys installations and updates by `plugin-name@marketplace-name`. This was also
verified end to end on Claude Code 2.1.265 on 2026-09-10 using an isolated config and a
throwaway project:

- Renaming the marketplace, marketplace entry, and plugin manifest left the existing install
  enabled but unresolved: `Plugin claude-notifications-go not found in marketplace
  claude-notifications-go`.
- Renaming only the plugin manifest allowed an update, but operations using the new internal
  name were inconsistent: disable reported `already disabled` while the installed plugin was
  enabled.
- Keeping all three legacy `name` values and changing only `displayName` updated cleanly and
  rendered as `Agent Notifications (claude-notifications-go)`.

There is no documented alias or atomic rename mechanism that migrates existing marketplace
registrations, installed-plugin records, cache paths, enabled state, and command namespaces.
The GitHub repository URL may change independently and does not require changing these IDs.

## Rule for a future migration

Do not change these identifiers unless a dedicated migration ships with disposable-profile
E2E coverage for existing installs, update/rollback, enable/disable/uninstall, duplicate-hook
prevention, and both old and new command namespaces. A new identifier must be treated as a
new plugin identity, not as a routine rename.
