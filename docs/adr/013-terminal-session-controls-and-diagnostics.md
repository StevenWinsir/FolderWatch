# ADR-013: Terminal adapter, serialized controls and bounded diagnostics

Status: Accepted, implemented in R4 (2026-10-02)

## Context

R3 provides the sole semantic ChangeStore, generation/path-version fencing, cancellable GetDiff and atomic ResetBaseline. R4 must expose a usable Terminal experience without deriving a second filesystem truth or losing edits during pause. The merged R3 source tree was verified at main `1b52fcc23d199f6962206205e585c1d695cc0bae` (PR #2), identical to R3 head `c0f626b63735fba6ae43feaeb0c860089ffad0ea`. R4 develops on `feat/r4-terminal-tui` without rewriting history.

## Decision

### Mode and presentation boundary

With both stdin and stdout attached to terminals, the default invocation launches Bubble Tea. Redirected input/output retains the R1 one-shot scan, avoiding escape sequences in pipes and preserving scripts. `--tui` explicitly requires terminals and rejects `--scan`, `--watch` or `--json`; `--scan`/`--json` always scan once, and `--watch` remains semantic text/NDJSON. The initial configuration/metadata preparation precedes the UI; the longer watcher/baseline startup runs in one cancellable worker while the UI shows Scanning. Exit joins that worker and closes any acquired Session.

`internal/tui` imports the existing app facade. Core packages import no TUI, Lip Gloss or CLI adapter. The model owns only presentation state and a defensive projection of ChangeState. Each event retrieves the authoritative view; a generation/version watermark rejects older views. No UI stat/hash/classifier/watcher or semantic transition logic is added. A/M/D/R are kind markers; [+]/[-] are expansion markers. R remains reserved: the existing core still uses Deleted+Added rename fallback.

The interface uses a bounded-height file list and one selected unified-diff pane, not simultaneous retained diffs for every expanded file. Arrows/j/k select; Enter/Space expand/collapse; PgUp/PgDn or Ctrl+u/d page the diff; Home/End/g/G go to endpoints; arrows/h/l scroll horizontally. `/` opens a case-insensitive path filter (256-rune input limit); Enter applies, Esc cancels an edit or clears an applied filter. Help and diagnostics are scrollable overlays. Mouse is optional; --no-mouse actively disables reporting and rejects residual reports while preserving every keyboard action. Below 40×12 cells the model shows a resize/exit hint, never panics.

### Diff ownership and display safety

Only one diff command is in flight. Changing selection, generation, path version, resetting or collapsing cancels it and increments a presentation epoch. After the old command returns, at most one request for the latest selection is launched. Results must match epoch, path, generation and semantic path version. ErrStale/ErrNotChanged retry at most three times at 100ms; otherwise an explicit message permits manual retry. Commands capture immutable arguments; Update and View never read files or calculate a diff.

Formatting is part of that cancellable command. Core byte/line/work limits remain unchanged. The terminal adds a 16MiB/50,000-row preview limit and a 16KiB logical-line preview limit, with explicit clipping messages; these are not a process-memory ceiling. Only the selected preview is retained, and only visible rows are styled per frame. Text uses old/new line numbers, hunks, +/- and red/green. NO_COLOR or TERM=dumb disables styling. Binary/Unsupported/TooLarge/Unavailable remain selectable with sizes and honest reasons; missing before content is never invented. Empty added/deleted text has an explanatory no-differing-lines message.

Content and paths are sanitized before styling: control, bidi/format, newline and Unicode separator characters are quoted; tabs become four spaces. Grapheme/cell clipping avoids splitting visible Unicode clusters at viewport edges. The GUI may use a different presentation without changing the core result.

### Pause, resume, reset and errors

Session Pause/Resume/Reset are commands serialized by the same owner as event resolution. Pause freezes semantic resolution/publication and retains last-known state while the native watcher and bounded queues stay alive. The owner drains queued invalidations rather than building an unbounded pause log. Fatal watcher errors still terminate a paused session. Resume first reconciles the entire root against the SAME baseline; recoverable reads preserve last-known state, emit warnings and use the existing bounded retry timer. A cancelled/failed operation before commit retains its previous state; an already committed operation returns success rather than an ambiguous cancellation.

Reset requires `r` then `y`/Enter in the UI, calls only the existing atomic core operation and clears the presentation from its new authoritative generation. It is allowed while paused and stays paused. Failed Reset keeps the old baseline and list. Queued events reconcile in the new generation. Close racing native watcher shutdown must expose the facade's ErrSessionClosed, not a backend-specific watcher-closed error; a committed Reset is not changed into a reported failure.

Status is UI-independent: Idle/Scanning/Monitoring/Paused/Stopping/Error. Scanning is represented by asynchronous UI startup; the live Session exposes Monitoring/Paused and cleanup status. Recoverable errors appear as a banner plus `e` diagnostics; fatal errors restore the terminal and are printed after Run returns. q returns 0, Ctrl+C/context cancellation 130, runtime/startup errors 1, invalid CLI input 2. Automated PTY checks assert raw/canonical/echo/control settings, cursor and alternate-screen restoration; the Darwin kernel-maintained PENDIN bit alone is masked in the attribute comparison.

### Logging and privacy

`internal/logging` keeps at most 200 entries, with at most 2048 message bytes plus a truncation suffix. --debug adds metadata diagnostics; callers never log file or diff content. No implicit file is opened. An explicit --log-file in live modes must be a NEW file outside the monitored root, including resolved symlink and case-insensitive identity aliases. O_EXCL prevents overwrite/symlink targeting and permissions are 0600. Parents are not created. Existing destinations fail clearly rather than append to or truncate someone else's file.

JSONL output stops at 4MiB or the first I/O error, retains the bounded ring and reports the failure without stopping monitoring. TUI never writes debug to stdout/stderr; --watch keeps diagnostics out of its NDJSON stream. The log path and cache protections are not an adversarial atomic filesystem sandbox against concurrent directory replacement. A slow explicit filesystem log destination is not a guaranteed latency bound; normal memory-only logging avoids that I/O.

## Alternatives

Pausing only drawing would silently advance the displayed state behind the user; stopping the watcher would require reconstructing subscriptions. Replaying every paused event would consume unbounded memory and still be inferior to reconciliation. Per-selection goroutines without a single-flight lane accumulate cancelled work. UI-owned change inference or clearing the list independently of Reset breaks core authority. Multi-file retained previews amplify memory usage; the selected-pane design gives predictable ownership. Appending to existing user files or logging inside the root risks data damage and feedback loops.

## Consequences

R4 provides the Terminal workflow, not Gate A or a macOS release. PTY automation is not actual Terminal.app/iTerm2 visual QA. P12 profiling/10k-file/long-run budgets, clean-machine installation, release packaging and human Terminal/iTerm2 acceptance belong to R5. Ignore hot reload, reliable rename association, editor execution, side-by-side TUI and GUI are not introduced. Dependencies remain pinned; x/term and uniseg are promoted from existing indirect requirements, with no version changes. The R2 vendor patch and all previous regression suites remain mandatory.

## Migration

Existing pipe callers keep one-shot default behavior, but terminal users who relied on a one-shot default must explicitly use --scan. Scripts can require --tui and receive input error 2 without a TTY. Existing log destinations must be changed to a new path for each live run. `Session.Pause(ctx)`, `Resume(ctx)` and `Status()` augment existing APIs; Event.status is an additive metadata field. GUI adapters must reuse these semantics after Gate A, not copy the TUI model. See R4 acceptance and Handoff §28 for actual evidence and integration status.
