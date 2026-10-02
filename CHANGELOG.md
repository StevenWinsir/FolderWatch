# Changelog

## Unreleased — R1 / P0–P2 (2026-10-01)

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

### Not yet implemented
Live watcher, event debounce/coalescing, snapshot/baseline/reset, classifier/diff/ChangeStore, TUI, GUI, editor launching and file logging. Their reserved options must not be mistaken for working features. Gate A and Gate B have not been reached.
