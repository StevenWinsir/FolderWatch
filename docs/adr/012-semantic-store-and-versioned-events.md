# ADR-012: Single semantic store, bounded resolution and versioned delivery

Status: Accepted, implemented in R3 (2026-10-02)

## Context
R2 events are invalidations, not final Added/Modified/Deleted states. Duplicate saves, restored content, directory replacement, unreadable paths, slow consumers and Reset must not cause duplicate or stale visible changes. The user requested R3 using the verified R2 endpoint. During this round, remote inspection confirmed R2 had merged via PR #1; its merge tree is identical to the starting implementation.

## Decision
`internal/changes.Store` is the sole semantic owner. ResolveBatch serializes through a cancellable token, stages successful reads and publishes one map delta under a short lock. View returns sorted defensive copies. Current regular files are compared to immutable baseline hashes, not the last save; metadata-only chmod/mtime differences do not create content modifications. Added then deleted disappears; deleted then recreated with original contents disappears; different recreation is Modified. Directory entries are not changed-file rows. Symlink link-string changes are metadata-only rows. Rename always uses the documented safe Deleted(old)+Added(new) fallback in this round: no content-similarity guess or unproven file-identity correlation.

The resolver uses one metadata-only scratch SnapshotStore (no retained current contents), deleting each captured reference immediately. GetDiff owns a separate single-reference scratch store and token. Mixed root/config/matcher/baseline namespaces are rejected. No duplicated file reader, watcher or baseline engine is introduced. `max_snapshot_files` also bounds the reconciliation inventory acceptance and semantic map; exceeding it is explicit. Scan allocation itself remains the R1 inventory behavior, not a strict process-memory cap.

Full reconciliation uses the union of the current inventory, baseline non-directory paths and existing changed paths. Unreadable subtrees preserve their last known state plus warnings, rather than being mass-deleted. Files disappearing during stable read preserve prior state until a subsequent successful resolve. Excluded-but-present entries are not mistaken for deletions. Directory replacement can make old descendants absent within the monitored namespace without following a new symlink target.

Session registers watchers and captures the baseline, then explicitly reconciles the startup window. Runtime errors schedule one retry timer with 100 ms to 2 s bounded backoff until resolved or cancelled; idle successful sessions do not periodically rescan. Each successful Reset commits the new baseline and empty ChangeStore before a reset marker. Failed/cancelled Reset preserves both. Pending filesystem invalidations are resolved in the new generation. Snapshot-wide point-in-time atomic filesystem capture remains outside the guarantee.

Batches contain Generation, Version, Upserts and Removed; Removed means no longer changed, not filesystem deletion. A deletion is an Upsert with Kind=Deleted. Each path carries its semantic Version and FirstSeen/LastSeen. Empty duplicate resolutions do not advance versions. Session output saturation discards obsolete delivery detail only with a Reload marker; consumers fetch ChangeState and reject older generation/version batches. CLI implements that watermark itself, strips compatibility Paths, and includes authoritative state on ready/reset/reload. No event contains content, diff text, or private cache names. Warning messages remain visible. Plain output shows A/M/D and quoted paths/classifications.

## Alternatives
A UI-owned map that independently stats/hashes or guesses change kinds creates a second core. Keeping current content for every event exhausts retention budgets. Treating partial scans as deletions destroys truth. Force-merging historical R2 work or rewriting published tags would not satisfy source/acceptance provenance.

## Consequences
Three private cache directories exist in a running R3 session: baseline, metadata-only resolver scratch, diff scratch. Reset can momentarily hold two bounded baseline generations plus one bounded diff snapshot and caller buffers/matrix; this is not a claim of a fixed total RAM ceiling. Changes/View return values are read-only copies. GetDiff can return ErrStale/ErrNotChanged, and clients must handle no retained content. Slow clients may lose intermediate transitions but recover the current authoritative state. Error-bearing state is last-known, not proof the unreadable file is unchanged.

## Migration
R2 Go invalidation metadata remains on Event.Paths for compatibility/testing, but only Batch/View are semantic APIs and the CLI omits Paths. `--watch` switches from path diagnostics to semantic text/NDJSON; default/--scan stays metadata-only. Public Go entrypoints are Session.Changes, ChangeState, GetDiff and existing ResetBaseline. R4 adds TUI adapters without copying core. R3 starts from R2's final commit 32529a7 and fast-forwards its identical-tree merge record 1dbdb5b before committing new work. R3 main integration is tracked separately from automated round acceptance.
