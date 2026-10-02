# ADR-008: Immutable snapshots, atomic generations and reset failure semantics

Status: Accepted for R2 (2026-10-01)

## Context
The baseline must not move on ordinary saves. Capturing every file permanently in memory, keeping stale generation caches, or publishing a half-reset baseline would violate this contract. Files can disappear or change during reads.

## Decision
`internal/snapshot.Store` owns immutable opaque-ID references, SHA-256 hashes and a published baseline generation. Small eligible contents use memory; larger eligible contents use private 0600 files under a private 0700 OS-temporary directory outside the watched root. Defaults: 64 KiB per in-memory file, 32 MiB total memory, 256 MiB total disk, 100000 references. All budgets except the small-file cutoff are exposed via shared Config/CLI (and all are adjustable through the core Options). At R2 max_diff_bytes (5MiB) capped retained contents; R3 separates max_snapshot_bytes (8MiB) for classification/retention from max_diff_bytes (5MiB) for actual diff (ADR-010). Neither filters discovery or hashing. Budget/size exclusions still retain metadata plus streaming hash.

R3 replaces the old private storage-only probe with the shared filetype.Probe and persists its immutable Result in Ref.Class. Rendering still consumes classified structured data, not unexamined raw bytes. Binary/invalid-text contents are not retained. Symlinks retain link metadata/target-string hash, never target contents; special files are metadata-only. Unix opens use NOFOLLOW and NONBLOCK plus Lstat/opened/final identity checks to reject final symlinks and avoid a raced FIFO blocking. Ancestor checks keep R1's policy, not an atomic adversarial filesystem sandbox. Other OS builds are compile-checked, not equivalent runtime-sandbox claims.

Reads stream in 32 KiB chunks with cancellation and an initial-size boundary; a growing file cannot extend the read indefinitely. Identity/size/mtime/mode changes cause a transient failure with up to three attempts. SHA-256 may still require reading an entire huge regular file, but memory does not scale with its size and cancellation is checked between chunks.

Reset builds a separate complete generation, then publishes it under a lock. Any requested-path capture failure or cancellation before commit preserves the old generation/content. Old references become stale after commit; baseline-owned references cannot be individually deleted. Ordinary Capture never advances the baseline. Reset can transiently retain two bounded generations plus bounded I/O buffers; returned content copies belong to the caller. Cleanup is retried before permitting another retired generation to accumulate, and persistent cleanup errors are reported by Close.

The application registers the watcher, rescans, then captures baseline; startup-window notifications remain queued. Successful reset drains old-generation public events, increments the generation and publishes a reset/root-reconciliation marker. Queued filesystem invalidations are then processed against the new baseline. A filesystem-wide instantaneous transaction is impossible: each captured file is checked for a stable read during the capture window; queued events/reconciliation reveal edits across that window. There is no implicit prior-save baseline advancement.

A diagnostic scan remains best-effort, as in R1. Watch startup and Reset require a complete readable baseline, fail explicitly on scan warnings/read failures, and do not silently omit inaccessible old paths. This is a deliberate fail-closed R2 policy, not a claim that partial baselines are complete. A later partial-baseline policy would require explicit unknown-path states and ADR/tests.

## Alternatives
In-place per-file reset can expose mixed generations and erase inaccessible paths. Copying content into every event leaks data and bloats UI messages. Keeping every generation indefinitely violates resource bounds. A metadata-only R1 inventory is not an adequate content baseline.

## Consequences
Closed sessions remove owned cache contents. Abnormal process termination can leave a private bounded cache; automatic crash-residue scavenging is deferred, not implemented by deleting arbitrary temporary directories. The snapshot layer exposes before contents/hash/classification; R3's separate changes/diff packages now provide semantic state and diff. `ResetBaseline(ctx)` is a Go application API; interactive reset UI belongs to R4. Once a reset command is admitted, the caller receives its definitive commit/failure result rather than an ambiguous cancellation that encourages a duplicate reset.

## Migration
R3 has added safe classification and current-vs-baseline change resolution using these references (ADR-010–012), with separate resolver and on-demand Diff scratch stores. Capture limits are provisional until R5/P12 measurement. New generations, partial-baseline handling, hot reload and snapshot storage changes require regression tests and an ADR update.
