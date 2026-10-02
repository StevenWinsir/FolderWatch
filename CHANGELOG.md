# Changelog

## Unreleased — R4 / P9–P11 (2026-10-02)

### Added
- Interactive-terminal default and explicit --tui mode, preserving non-TTY default/--scan/--json and --watch text/NDJSON.
- Bubble Tea/Lip Gloss changed-file list, keyboard/mouse navigation, filtering, bounded resize/scroll layouts, help and diagnostics.
- Single-flight cancellable selected unified Diff, epoch/path/generation/version fencing, bounded stale retries, line numbers, newline markers, +/- and NO_COLOR support.
- Explicit Binary/Unsupported/TooLarge/Unavailable states; safe Unicode cell clipping, terminal-control escaping and bounded preview memory.
- Serialized Session Pause/Resume/Status, watcher-alive paused semantics, same-baseline full reconciliation and confirmation-protected atomic Reset.
- Bounded diagnostics ring, metadata-only --debug and explicit new root-external 0600 JSONL capped at 4MiB; no overwrite, symlink alias or root feedback.
- Model/golden/control/logging tests, real binary PTY lifecycle and keyboard/mouse smoke, and CI integration of all R1–R4 regressions.

### Fixed / compatibility
- Concurrent Close and control commands normalize native shutdown errors at the Session facade without changing committed Reset success.
- --no-mouse also ignores preexisting terminal mouse reports; log containment checks directory identity on case-insensitive filesystems.
- R3 PR #2 is now merged (main 1b52fcc, 2026-10-02 07:08:58 UTC); the old pending-merge handoff was updated from observed remote facts.
- No dependency versions upgraded; existing x/term and uniseg promoted to direct requirements, fsnotify vendor patch retained.
- PTY automation is not Terminal.app/iTerm2 human QA. Gate A, P12 performance/long-running tests, packaging, editor execution and GUI remain deferred to their planned rounds.

## R3 checkpoint — P6–P8 (2026-10-02)

### Added
- Shared bounded Classifier with immutable snapshot classification, UTF-8/control-byte validation, UTF-16/32 BOM recognition and no-read oversized classification.
- Independent 8 MiB classification/retention and 5 MiB diff defaults, configurable line limits and validated hard ceilings.
- UI-independent cancellable bounded-LCS Diff with hunks, line numbers, exact newline semantics, explicit resource/status fallbacks, nine golden fixtures and reconstruction fuzzing.
- One versioned ChangeStore/Resolver for Added/Modified/Deleted, restoration/removal state transitions and deterministic rename fallback.
- Session Changes/ChangeState/GetDiff APIs, atomic reset/list semantics, on-demand current reads, stale diff rejection, partial-scan protection and bounded recovery retry.
- Semantic --watch text/NDJSON, reload snapshots and watermark suppression of queued obsolete deltas; default scan remains unchanged.
- State-transition, concurrent diff/reset, cancellation, permission/disappearance, mixed-root, live filesystem and CLI regression tests; ADR-010–012 and R3 handoff.

### Behavior changes / scope
- --max-diff-bytes now limits actual diff; --max-snapshot-bytes independently controls classification/retention. Ordinary events do not retain all current contents.
- --watch JSON uses semantic batches rather than raw path diagnostics; events never include file contents. Diff is a Go API, not an interactive terminal viewer yet.
- R2 has since merged through PR #1; its previously announced r2-complete.1 tag was not created. Use actual commit 32529a7 or merge 1dbdb5b for the R2 endpoint.
- At the R3 checkpoint TUI, interactive diff/reset, pause/resume and logging remained later work; R4 above implements them. Editor execution, GUI and release gates remain deferred. R3 PR #2 has since merged.

## R2 checkpoint — P3–P5 (2026-10-02)

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
At the R2 checkpoint, classifier/diff/ChangeStore were deferred; R3 above implements them. R4 above implements TUI/interactive reset, pause/resume and logging. Editor execution and GUI/release gates remain deferred. Actual VS Code GUI testing is still not claimed.

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
At R1, watcher/debounce/snapshot were deferred; R2 implements those layers, R3 adds classifier/diff/ChangeStore, and R4 adds TUI/controls/logging. GUI and editor launching remain deferred. Gate A and Gate B have not been reached.
