# ADR-011: Bounded cancellable line diff

Status: Accepted, implemented in R3 (2026-10-02)

## Context
P7 requires line-level additions/removals/context, hunks, line numbers and exact newline semantics. Calling an uncancellable library in a goroutine and abandoning it after a timeout would not bound CPU, memory or goroutines. The pinned go-difflib dependency has not yet been adopted by a production adapter.

## Decision
`internal/diff.Engine` accepts context, two byte slices and validated Options; it returns UI-independent Result/Hunk/Line structures. `BoundedLCS` checks byte/line limits and both inputs' safe-text status, trims identical edges, interns line strings to integer IDs, then computes a capped LCS matrix. Deletion wins deterministic ties. Cooperative cancellation checks occur during classification, edge scanning, matrix construction and edit generation. There is no detached computation or per-request worker goroutine.

Defaults: **5 MiB per side, 20,000 lines per side, 2,000,000 matrix cells, 3 context lines**. The Go Options validation ceilings are 64 MiB, 200,000 lines, 4,000,000 cells and 1,000 context lines. Cells use uint32 (about 8 MB at the default), plus bounded input/line/hunk storage. Oversize bytes, line count or matrix work returns structured `too-large` with a reason, not an OOM or silently inaccurate approximate patch. Complex inputs can hit the work budget even below the byte/line cap; equal-edge trimming makes small edits in large otherwise-identical files cheap. This is a deliberate resource tradeoff, not a shortest-edit performance claim for all input sizes.

Text lines preserve CRLF and missing final newline; each output line has Added/Removed/Context, old/new line number and NoNewline. Unified produces plain text, not ANSI styling. Binary and recognized unsupported text return status-only results; unavailable baseline/current retained content is represented by the ChangeStore as `unavailable`, never substituted with live content as the old side.

On-demand GetDiff serializes diff requests through one cancellable token and owns one temporary current snapshot. It does not cache all file contents/results, and releases the current reference after every request. It validates current hash against the resolved state and validates path version plus baseline generation after calculation. A stale result returns ErrStale and must be requested again; it cannot silently overwrite a newer file or reset generation. Diff computation runs independently of the resolver ownership token.

## Alternatives
An unbounded matrix is unacceptable. The pinned go-difflib adapter was not used because this round needs explicit cancellation/work accounting inside the algorithm. A more scalable Myers/patience implementation or bounded cache can be added later behind Engine after its own correctness/resource tests; blindly changing the algorithm or caching without ownership is not accepted.

## Consequences
Dense changes spanning a long document may return TooLarge due to work rather than bytes; clients must display the reason and retain the changed-file entry. There is no word-level or side-by-side rendering in R3. The engine does not color terminal output. Nine golden fixtures, cancellation injection, a 10k-line single-edit case and a reconstruction fuzz target test the adapter. Limits are provisional pending P12 measurement.

## Migration
The unused pinned difflib remains in the tools-tag dependency inventory, not the R3 executable; removing unused future pins can be a separately reviewed dependency change. R4 consumes Result directly and must not recalculate diff during rendering. Any cache must be bounded and key on both hashes, generation and options, and preserve stale-result/cancellation checks.
