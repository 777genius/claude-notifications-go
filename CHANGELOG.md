# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0] - 2025-10-25

### Added
- **New notification type: API Error 401** 🔴
  - Detects authentication errors when OAuth token expires
  - Shows "🔴 API Error: 401" with message "Please run /login"
  - Triggered when both "API Error: 401" and "Please run /login" appear in assistant messages
  - Priority detection (checks before tool-based detection)
  - Added comprehensive tests for API error detection

### Improved
- **Binary size optimization** - 30% smaller release binaries
  - Production builds now use `-ldflags="-s -w" -trimpath` flags
  - Binary size reduced from ~10 MB to ~7 MB per platform
  - Faster download times for users (5 seconds instead of 8 seconds)
  - Better privacy (no developer paths in binaries)
  - Deterministic builds across different machines
  - Development builds unchanged (still include debug symbols)

### Changed
- Updated notification count from 5 to 6 types in README
- All tests passing with new features

## [1.0.3] - 2025-10-24

### Fixed
- Critical bug in duration calculation ("Took" time in notifications)
  - User text messages were not being detected in transcript parsing
  - `GetLastUserTimestamp` now correctly parses string content format
  - Duration now shows accurate time (e.g., "Took 5m" instead of "Took 2h 30m")
  - Tool counting now accurate (prevents showing inflated counts like "Edited 32 files")
- Added custom JSON marshaling/unmarshaling for `MessageContent` to handle both string and array content formats

### Technical Details
- Fixed `pkg/jsonl/jsonl.go`: Added `ContentString` field and custom `UnmarshalJSON`/`MarshalJSON` methods
- User messages with `"content": "text"` format now properly parsed (previously only array format worked)
- All existing tests pass + added new tests for string content parsing

## [1.0.2] - 2025-10-23

### Added
- Linux ARM64 support for Raspberry Pi and other ARM64 Linux systems (#2)
  - Native ARM64 runner (`ubuntu-24.04-arm`) for reliable builds
  - Full audio and notification support via CGO
  - Automatic binary download via `/notifications-init` command

### Fixed
- Webhook configuration validation now only runs when webhooks are enabled (#1)
  - Previously caused "invalid webhook preset: none" error even with webhooks disabled
  - Preset and format validation now conditional on `webhook.enabled` flag

### Changed
- Documentation updates for clarity and platform-specific instructions

## [1.0.1] - 2025-10-22

### Added
- Windows ARM64 binary support
- Windows CMD and PowerShell compatibility improvements

### Fixed
- Plugin installation and hook integration issues
- Plugin manifest command paths
- POSIX-compliant OS detection for better cross-platform support

## [1.0.0] - 2025-10-20

### Added
- Initial release of Claude Notifications plugin
- Cross-platform desktop notifications (macOS, Linux, Windows)
- Smart notification system with 5 types:
  - Task Complete
  - Review Complete
  - Question
  - Plan Ready
  - Session Limit Reached
- State machine analysis for accurate notification detection
- Webhook integrations (Slack, Discord, Telegram, Custom)
- Enterprise-grade webhook features:
  - Retry logic with exponential backoff
  - Circuit breaker for fault tolerance
  - Rate limiting with token bucket algorithm
- Audio notification support (MP3, WAV, FLAC, OGG, AIFF)
- Volume control (0-100%)
- Interactive setup wizards
- Two-phase lock deduplication system
- Friendly session names
- Pre-built binaries for all platforms
- GitHub Releases distribution

### Fixed
- Error handling improvements across webhook and notifier packages
- Data race in error handler
- Question notification cooldown system
- Cross-platform path normalization

[1.0.2]: https://github.com/777genius/claude-notifications-go/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/777genius/claude-notifications-go/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/777genius/claude-notifications-go/releases/tag/v1.0.0
