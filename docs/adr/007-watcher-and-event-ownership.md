# ADR-007: Recursive watcher and bounded path invalidations

Status: Accepted for R2 (2026-10-01)

## Context
Directory notifications are neither recursive by default nor a transaction log. Saves can include replacement/rename noise; OS queues and consumers can overflow. R1's paths and Ignore policy must stay shared.

## Decision
`internal/watcher.Watcher` owns Start/Reconcile/Close and separate bounded event/error channels. The fsnotify adapter watches accepted directories before descending, registers newly created/moved trees, removes stale registrations and compares directory identity/native registrations during reconciliation. It never follows descendant directory symlinks. Root disappearance/replacement is fatal; registration failure is explicit rather than claiming complete coverage. Empty native event names are unknowable-scope invalidations, not valid file keys.

One watcher owner maintains registration state. One coalescer owns a bounded pending map and a single ticker. It combines each path's trailing-edge events (default 150 ms), with maximum settling wait 4x debounce for continuously busy files. CHMOD-only requests carry `MetadataOnly`; R3 must inspect metadata/hash before deciding whether content work is needed. Atomic replacement triggers re-reading the final path. A directory invalidation means root reconciliation, including children that existed before registration. R2 deliberately publishes neither rename guesses nor Added/Modified/Deleted semantics.

Default raw/pending capacity is 4096 (configurable), output batch capacity 1, watcher error capacity 16, application output capacity 32. Overflow/backpressure folds work into an explicit root reconciliation instead of silently dropping final state or creating unbounded queues. Error details can coalesce under overload; reconciliation is retained. Directory registrations default to at most 8192; exhausting that configured cap is fatal rather than claiming full coverage. There is no periodic full-tree polling; the application reconciles registrations on subtree/overflow requests. An unsuccessful recovery remains visible as a warning and is retried on later invalidation/reconciliation.

Context cancellation and idempotent Close terminate owners/timers and close output channels. Application commands serialize baseline resets with event publication. Every public event carries a monotonic sequence and baseline generation. A slow reader may lose detail but receives `Reconcile=true`, so R3 must never interpret an incomplete event stream as complete state.

## Alternatives
Watching individual files loses atomic replacements. Per-event goroutines and per-path timers complicate cancellation and can grow without bounds. Dropping saturated events silently makes the eventual changed-file state incorrect. Deriving semantic changes here would duplicate R3's hash/baseline resolver.

## Consequences
Large directory changes can trigger full registration/inventory reconciliation. Directory and snapshot-reference indexes have configured caps; the initial scan's inventory memory is still proportional to entry count. Ignore files retain cached-per-directory-identity behavior, including missing rule files; deleted/replaced directory scopes are retired, but existing rule-file edits do not hot reload. Native kqueue no-follow and skipped-entry cleanup are covered by the reproducible patch in ADR-009. OS-specific editor acceptance and long-running performance remain distinct from deterministic save-pattern tests.

## Migration
R3 consumes settled paths/root invalidations and resolves actual current state against the baseline. It must keep bounds and cancellation, not wire fsnotify flags directly to UI. R4 will supply TUI interaction; R2 adds opt-in `--watch` text/NDJSON diagnostics while preserving default/explicit `--scan`.
