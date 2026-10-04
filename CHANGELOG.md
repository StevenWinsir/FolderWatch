# Changelog

## Unreleased — large-folder reliability (2026-10-04)

- Use recursive FSEvents for native macOS and joined, visible polling fallback when native budgets/descriptors are exhausted; survive disappearing/unreadable child paths during fallback enrollment.
- Remove the default inventory-count cap while retaining explicit positive caps, bounded content/diff budgets and bounded event payloads. Store metadata/scan queues/polling inventories in private disk indexes (pinned bbolt 1.3.11; reviewed fsnotify patch unchanged).
- Preserve dirty subtree scopes, avoid full-index copies for ordinary file events, and conservatively reuse verified hashes only with strong local metadata signatures. Untrusted/nested filesystems do not inherit hash-reuse trust.
- Permit explicitly unknown initial baseline scopes without fabricating Added/Deleted or before text; keep Reset complete and atomic. Expose metadata progress and safe cancellation instead of a fixed 30-second reset limit.
- Make every GUI change reachable with bounded 500-row pages and full-index search, single-flight refresh/diff and stale-response fences. Fix long-root active-layout overflow, unknown markers, missing workspace error banners and offline font loading.
- Align native macOS CLI releases/performance with the FSEvents build; retain separately tested no-CGO builds. Extend CI with native/no-CGO 9000-directory/100005-file tests, 64-descriptor smoke and real Wails IPC pagination/large-folder coverage.
- Acceptance, measurements, historical failure notes and remaining platform/release boundaries: `docs/rounds/large-folder-acceptance.md` and ADR-016. This entry does not claim signing/notarization or Gate B.

## Unreleased — R6 / P15–P16 desktop foundation (2026-10-02)

### Added / fixed
- Wails v2.10.1 / Svelte / TypeScript desktop shell with native menus, version metadata, explicit root input, Start/Stop and truthful connection/session/error states.
- Headless Core Facade and native host lifecycle, exclusively reusing internal/app; random client/session capabilities, scan cancellation, joined cleanup, heartbeat/lease and stale-request rejection.
- Versioned IPC v1 DTOs, metadata-only bounded events, decimal-string counters, paginated summaries, on-demand single-flight Diff with version/path fences and conservative wire budget.
- Go/TypeScript shared contract, lifecycle/resource/path/serialization tests, 11 frontend tests and two real Wails IPC browser E2E workflows; native production build and Node22/24/macOS GUI CI targets.
- Wails binding build-tag/bootstrap compatibility, browser entry resolution for Svelte tests, Node E2E types, favicon HTTP-error regression and application-context shutdown handling.
- Retained the exact reviewed fsnotify v1.8.0 patch while pinning Wails dependencies and preserving generated bindings/lockfiles; reran Terminal regression, full race x10, fuzz and cross-builds.

### Scope / acceptance
- Gate A and P0–P14 are already accepted (PR #6 / 88b3541). R6 implementation/local automation passed; integration remains IN_REVIEW until review/merge. Actual evidence and CI links are in docs/rounds/R6-acceptance.md.
- Native WKWebView button/menu interaction was not revalidated because macOS Accessibility/Screen Recording permissions are unavailable. Browser IPC E2E is not production WebView or independent human acceptance.
- Picker/list/Monaco are R7; settings/system integration/sleep UX are R8; signing/notarization/Gate B remain R9. The generated app is a development artifact, not a GUI release.

## R5 checkpoint — P12–P14 engineering (2026-10-02)

### Added / fixed
- Generated 1k/10k/burst/large-text/binary/deep-tree/lifecycle measurements, CPU/heap/goroutine profiles and real-binary PTY latency evidence with explicit measurement boundaries.
- Fixed the kqueue descriptor leak caused by Close setting done before a now-no-op Remove. The vendor patch now fences fd ownership, joins the reader, releases every watch and synchronizes concurrent Close; native opens are close-on-exec. Existing no-follow and reconciliation patches remain.
- Removed the redundant small-text snapshot copy while preserving capture ownership, bounded retention and defensive ReadContent copies.
- Added native/session cleanup, burst overflow, ownership and event-normalization fuzz regressions; retained all R1–R4 tests and included R4 native fixture synchronization.
- Hardened macOS/Linux × Go1.23/1.26 CI, six-target bounded fuzzing, retained test/coverage logs, cross-builds, Core/UI dependency checks and native candidate installation tests.
- Added deterministic darwin arm64/amd64 archives, exact version/commit/build-date, checksums, provenance/license files, checksum-specific Homebrew formula and private candidate workflow. No public release/tag/tap or signing/notarization is implied.
- Added R5 performance/acceptance, ADR-014 and explicit Gate A records. Gate A was subsequently signed PASS and recorded through PR #6; the historical candidate and its human acceptance remain the authoritative Terminal baseline.

## R4 checkpoint — P9–P11 (2026-10-02)

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
