# ADR-016: Large-folder coverage, disk metadata, and bounded GUI pages

Status: Accepted for implementation; integration acceptance is recorded separately.
Date: 2026-10-04
Supersedes the directory-count fatal/default inventory-count policy in ADR-007/008 and the initial all-or-nothing startup policy. Existing strict Reset semantics and path confinement remain.

## Context

`watch directory limit exceeded: 8192` was emitted by FolderWatch's recursive fsnotify directory registry, not a queried operating-system quota. Raising that number does not address kqueue file descriptors, the separate 100000 snapshot cap, full-tree metadata copies, repeated whole-root hashing, or the GUI fetching only its first 500 summaries. Native capacity exhaustion and a child disappearing during replacement-backend enrollment must not terminate an otherwise healthy session.

## Decision

### Coverage backend and handover

macOS builds with CGO use a single recursive FSEvents stream. The C serial dispatch queue is stopped, invalidated and drained before deleting the Go callback handle. Native callbacks never wait for the UI. Dropped/coalesced events and bounded-queue overflow retain a root invalidation. The original reviewed fsnotify kqueue backend remains vendored unchanged and is still exercised directly by native regression tests.

No-CGO/non-macOS builds use fsnotify through Adaptive. Native registration budget exhaustion, EMFILE, ENFILE, ENOSPC and unexpected stream closure close/join the original backend before a metadata polling backend takes over. A retained root invalidation covers the handover. A visible warning distinguishes this mode and its latency; recovered resource conditions do not escape as fatal ErrDirectoryLimit. Root disappearance/replacement remains fatal.

Polling keeps private disk-backed previous/next metadata catalogs, not one open handle or goroutine per path. A disappearing or unreadable descendant protects that scope's old metadata while allowing healthy siblings to progress, including the initial fallback scan. Partial scans never imply deletion. Scans do not overlap and wait at least 2 seconds, or five times the previous sweep duration, after finishing. Core retries after failed reconciliation also account for the preceding scan duration. These are work scheduling policies, not hard CPU or latency guarantees on a stalled filesystem.

### Metadata storage and scanning

Pin `go.etcd.io/bbolt v1.3.11` (MIT license; license included by release packaging) as an internal temporary metadata index. Catalogs are 0600 files in 0700 session directories outside the selected root. They are reconstructible, NoSync temporary state; no catalog from a previous process is trusted as a new baseline. No monitored file content is stored in a metadata record.

Snapshot references, baseline membership, unknown startup scopes, changed summaries and polling inventories move to ordered disk catalogs. Catalog reads/writes are bounded to 128 records per batch; GUI windows decode at most 500 summaries. A streaming scanner reads 128 directory entries at a time and uses a private disk work queue rather than an in-memory full inventory or one descriptor per directory depth. The explicit diagnostic `Scan`, `Baseline` and `ChangeState` compatibility APIs still materialize caller-owned results; callers must not use them for GUI polling. Legacy Terminal/NDJSON full-list consumers retain their existing output semantics and can allocate with the returned list size. Optional gitignore scope caching is also not a whole-process constant-memory guarantee.

The default inventory count cap becomes 0 (no count cap); explicit positive caps are still enforced. Memory/disk **content** budgets remain 32MiB/256MiB per baseline generation. At most 4096 nonempty small-file contents are retained in the RAM map, bounding tiny-object overhead as well as bytes; remaining eligible content uses the existing disk spool or metadata-only fallback. Metadata files, filesystem allocation overhead, mmap/page cache, OS queues, UI state and caller copies are additional resources. Available disk, OS path limits, permissions and hardware remain real constraints; this design does not promise infinite-size directories in fixed RAM/disk.

### Event scope and content verification

Keep dirty subtree paths through normalization and debounce. A directory notification reconciles its subtree; overflow/resume/startup repair use the root. A small file-only batch stages at most 128 mutations and commits one transaction without cloning the entire change index. Large reconciliation stages disk catalogs and swaps the semantic owner only after successful completion. Large semantic deltas become a Reload invalidation rather than an unlimited event payload.

Native file invalidations always re-read/hash the file. Recovery scans may reuse a known hash only with matching identity, size, mode, nanosecond mtime and ctime on recognized local filesystems. Unknown/network filesystems and nested mount devices do not inherit this optimization; unsupported platforms re-read. This is not the incorrect mtime+size-only shortcut. Initial baseline hashes still read every regular file; large-file hash cost is not hidden behind the diff-size limit.

### Partial initial baseline without fabricated history

An initial unreadable or unstable descendant no longer rejects a healthy root. Successfully read baseline entries are retained and failed scopes are explicitly indexed as unknown. The semantic kind `unknown` (GUI marker `?`, label `Baseline unknown`) is not Added/Modified/Deleted and does not assert a change happened. Restoring access never substitutes today's bytes for missing startup bytes. Unknown paths return an Unavailable diff until the user requests a complete Reset. Existing readable paths continue monitoring.

Reset remains all-or-nothing: stage a complete new baseline, then publish its generation and clear changes; failure/cancellation preserves the old baseline and list. A completed explicit Reset clears unknown coverage and begins truthful future comparisons. Snapshot reference lookup and generation are captured together so a diff cannot pair an old summary with the next generation's baseline during publication.

### GUI query and operation contract

GetChanges accepts optional case-insensitive path `filter`; filtering covers the entire disk index. It returns global `total`, filtered `matched`, a maximum-500-row page, `nextOffset`, generation and version. Continuations require the same filter/head; stale pages restart from zero instead of merging generations. The workspace retains a single page, provides accessible Previous/Next controls, and never hides files after row 500. A bounded page is used instead of retaining an entire million-row DOM or list in JavaScript.

Frontend refresh bursts coalesce to one in-flight list RPC plus one pending invalidation. Diff selection similarly has one in-flight RPC and an epoch/path/client/session/generation/version fence. Backend list/control admission also rejects duplicate work with BUSY. Head/selected-path checks do not clone the whole change list.

Baseline/reset/reconciliation progress reports metadata counts at most every 250ms from existing work owners, without new goroutines/timers. Reset and Resume no longer use a fixed 30-second deadline. Stop/reload/lease cancellation owns work lifetime; on-demand Diff retains its independent computation/time/wire limits. Filesystem calls themselves are not forcibly interruptible OS operations.

### Build/release consistency

Native macOS CLI release builds enable CGO for both arm64 and amd64 and record `watch_backend=fsevents-with-polling-fallback`. No-CGO builds remain separately compiled and tested. The performance script measures the native default unless `--no-cgo` is requested. Both variants have low-descriptor and large-folder coverage. GUI bundles remain development artifacts until the separate signing/notarization/installation gate completes.

## Alternatives considered

Raising 8192 alone leaves descriptor and inventory ceilings. Removing all budgets leaves an OOM path. Silently ignoring unregistered/unreadable directories claims coverage that does not exist. Re-reading every file for every directory event preserves correctness but does not scale. Loading all GUI pages into an unbounded JS array merely moves the resource problem. Switching to mtime/size-only comparisons loses same-size edits with restored timestamps. A durable user database was rejected: session baselines are ephemeral and must not be resurrected after a crash.

## Consequences and migration

The change is wider than the original error-string fix because loading, baseline storage, recovery and presentation must agree. Temporary disk usage increases; memory pressure and native handles no longer track the entire tree in the old way. Finite index pages still involve O(N) scan/search I/O in the worst case, and deep-path checks cost more than shallow paths. Network volumes, remote writers, coarse timestamp filesystems and hostile concurrent path replacement are not newly certified by local APFS tests. Ignore-file hot reload and forced-crash cache cleanup remain separate work.

Existing TOML files with a positive `max_snapshot_files` continue limiting their sessions; users may explicitly set 0. `max_watch_dirs` now describes a native registration budget, not a monitored-tree maximum. GUI wire additions are additive within protocol v1; unknown kinds must be rendered explicitly, not interpreted as deletion. Historical gate signatures are not rewritten; this core change requires the regression evidence in the large-folder acceptance record and its exact PR-head CI.
