# Changelog

## Unreleased — R2 / P3–P5 (2026-10-02)

### Added
- Recursive fsnotify adapter, directory identity/registration reconciliation, shared runtime Ignore and explicit fatal directory limits.
- OS-neutral path normalization and bounded trailing-edge aggregation with one ticker, metadata-only hints and maximum-wait delivery.
- Backpressure/overflow folding into root invalidation rather than silently losing final state.
- Immutable memory/disk snapshots, streaming SHA-256, bounded content retention, private temporary cache, atomic baseline generations and cancellation-safe reset/close.
- Application Session API and opt-in `--watch` text/NDJSON diagnostics; default/explicit `--scan` retained.
- Shared Config/CLI limits, true filesystem integration, native no-follow regression, repeated lifecycle/race tests and real headless Vim saves.
- Vendored pinned dependencies with a reproducible small fsnotify kqueue patch, license preservation, SHA-256/reversibility/build-source guard and ADR-007–009.

### Fixed
- Retired native descriptors no longer become empty-path/cwd events; directory invalidations cover registration-window children and symlink replacement.
- kqueue's internal directory enrollment no longer follows descendant symlink targets.
- Directory recreation reloads its new identity's Ignore scope; retired scope caches are removed, without silently adding hot reload.
- Reset preserves old baseline on cancellation, missing/unreadable files and capacity failures; waiting captures/resets can be cancelled.

### Still deferred
R3 classifier/diff/ChangeStore, R4 TUI and interactive reset, pause/resume, editor integration, file logging and all GUI/release gates. VS Code save-pattern models are tested; an actual VS Code GUI run is not claimed. See R2 acceptance.

## R1 checkpoint — P0–P2 (2026-10-01)

### Added
- Go module, pinned dependencies, thin executable entrypoint, Make targets and macOS/Linux CI matrix with cross-build checks.
- Strict shared TOML configuration, explicit CLI override presence, path/duration/byte-budget validation and help/version.
- One-shot metadata inventory via default invocation or `--scan`, with optional `--json`.
- Canonical root-relative paths, no descendant-symlink traversal, metadata-only handling of all file extensions/sizes and special files.
- Shared concurrent-safe initial/runtime Ignore Matcher: default `.git/`, opt-in nested `.gitignore`, `.folderwatchignore`, explicit rule files, config/CLI rules and ordered negation.
- Structured recoverable warnings, startup/output/cancellation exit codes and terminal-safe path rendering.
- Unit, integration, Git-oracle, race, fuzz and real-binary smoke tests; ADR-001–006 and R1 handoff/acceptance.

### Corrected during R1 validation
- Trailing `/**` must match descendants but not the parent itself, consistent with Git.
- Kept directories preflight nested rules before descent; unreadable/broken rule files cannot silently widen the inventory.
- Directory hints cannot authorize following final symlinks.
- Explicit empty CLI roots are rejected, malformed lower-priority config globs are not hidden by overrides, and cancellation during the final walk callback is preserved.

### Scope at the R1 checkpoint
At R1, watcher/debounce/snapshot were deferred; R2 above now implements those layers. Classifier/diff/ChangeStore, TUI, GUI, editor launching and file logging remain deferred. Gate A and Gate B have not been reached.
