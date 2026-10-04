# Large-folder reliability acceptance

Date: 2026-10-04
Branch: `fix/large-folder-reliable-monitoring`
Start: `60e5f492ca9a47457f2ce00dcb329f3b5fe89da2` (main, PR #13)
Scope: preserve and complete the user's 29-path partial large-folder fix; no direct merge.
Architecture and migration: [ADR-016](../adr/016-large-folder-coverage-and-disk-indexes.md).

## Delivered

Native macOS FSEvents and joined adaptive polling cover trees beyond the former 8192-directory and 100000-entry defaults. Polling enrollment survives disappearing/unreadable descendants, protects failed scopes, and continues healthy siblings. The reviewed fsnotify vendor patch is unchanged.

Private disk catalogs/streaming queues replace full-tree metadata maps on monitoring hot paths. Batches/pages are bounded, small file events avoid cloning the whole change index, directory events keep their subtree scope, and large deltas become Reload. Hash reuse requires strong local identity/size/mode/mtime/ctime; native file events force verification. Untrusted/coarse/nested-device metadata does not authorize reuse.

Initial unavailable baseline scopes are explicitly Unknown, not invented Added/Deleted/before text. Reset stays complete and atomic. Diff rejects the interval between snapshot and semantic generation publication. GUI pages/search access every result with a bounded 500-row DOM, single-flight requests and stale fences. Scan/reset progress and Stop cancellation replace the arbitrary 30-second tree deadline. Native CLI releases now use CGO/FSEvents on both Mac architectures; no-CGO remains separately tested.

## Local verification

Environment: Apple M4 / 10 logical CPUs / 32GiB / APFS SSD / macOS 26.6.2 / Go 1.26.6 / Node 24.19.0 / Wails 2.10.1 / Chrome. All monitored fixtures are owned temporary directories. Logs and screenshots are under ignored `artifacts/large-folder-fix-20261004/`; selected machine-readable evidence is retained beside the committed benchmark report.

| Check | Observed result |
|---|---|
| Formatter, vendor patch/reversibility, Core boundary, go vet | PASS |
| `go test -count=1 -json ./...` | 350 tests/subtests PASS, 0 fail; one deliberate opt-in large-fixture skip in normal runs |
| `go test -race -shuffle=on -count=10 ./...` | PASS; final frozen-source rerun recorded separately |
| Native and no-CGO large integration | Both PASS: 9000 directories + 100005 files, 109006 refs, exact edit hashes, pause/resume/reset, new Unicode subtree, delete and cleanup |
| 64-descriptor CLI limit | Both builds PASS ready/edit/atomic-save/new subtree/delete/cleanup; no-CGO visibly falls back after real EMFILE |
| Frontend | Svelte/TypeScript 0 errors/0 warnings; 5 Vitest files / 17 tests; production build PASS |
| Native development GUI | Wails `.app` build PASS; no Developer ID signing/notarization |
| Real Wails IPC E2E | 4/4 PASS: existing two workflows, 9000 directories, 1001 changes across pages/search/Monaco/active resize |
| Legacy CLI/TUI and scripts | All four smoke suites and Python release-script tests PASS; actual Vim save modes retained |
| Cross-build | No-CGO darwin arm64/amd64, linux amd64, windows amd64 PASS (compilation is not runtime certification) |
| Native CLI release rehearsal | Dual-architecture packaging/provenance/checksums plus isolated HOME/PATH arm64 install/watch/hash/SIGINT/cache cleanup PASS |
| Fuzz/dependencies | Six fuzz targets, `go mod verify`, `go mod tidy -diff` PASS |

Reproduce large fixtures with `FOLDERWATCH_LARGE_TESTS=1 CGO_ENABLED=1 go test ./internal/app -run '^TestLargeTreePastBothFormerLimits$' -count=1 -v -timeout=10m`, then repeat with `CGO_ENABLED=0`. Build each CLI and run `python3 scripts/large_tree_smoke.py NATIVE_BINARY NOCGO_BINARY`.

`scale-v2` recorded native ready 32.668s, HeapAlloc 35,820,776 B, fd 6→29→6; no-CGO ready 33.444s, HeapAlloc 27,496,328 B, fd 6→18→6. Independent exact edit hashes appeared after 8.040s / 7.429s. These runs overlapped other validation, were warm synthetic fixtures with nine baseline bytes per file, and are not a terabyte-content benchmark. HeapAlloc is sampled Go heap, not peak RSS. Earlier faster development runs are not substituted for these retained observations.

## Performance and resources

The native default performance script ran all seven workloads with 10 seconds idle and 20 samples each. Raw JSON/profiles: `performance-v2/results/`.

| Workload | Ready | Idle CPU, one core | Core P95 | fd after / cache after |
|---|---:|---:|---:|---|
| 1k × 1KiB | 269.80ms | 0.091% | 223.54ms | 6 / 0 |
| 10k × 1KiB | 1731.68ms | 0.111% | 223.93ms | 6 / 0 |
| 5MiB text | 145.97ms | 0.110% | 196.25ms | 6 / 0 |
| 10MiB text | 47.07ms | 0.099% | 170.27ms | 6 / 0 |
| 64MiB binary | 109.66ms | 0.128% | 242.28ms | 6 / 0 |
| Depth 32 | 123.38ms | 0.124% | 249.59ms | 6 / 0 |
| 50 extra lifecycle cycles | 81.57ms | 0.082% | 228.16ms | 6 / 0 |

10k settled heap: 9,896,504 B; peak RSS: 66,174,976 B (~63.1MiB); ready+settled: 2330.50ms. P95 includes the 150ms debounce and ends at authoritative Core state, not rendered pixels. Heap/RSS includes fixture allocation where applicable, especially the 64MiB buffer. Goroutines were 1 before / 5–6 active / 2 after, stable in the lifecycle workload; no assertion is made that runtime helpers return to the pre-framework count of 1. These measurements are not a universal latency/RSS SLA.

## Development failures retained

The review's two fallback-unlink cases and 501-row GUI probe failed before the fixes. Their production regressions are now committed. A new 257-entry test was updated to expect bounded Reload while retaining exact full-state/reset assertions. The new 1001-entry API test initially misused a helper comparing a default-200 page length with 1001; it now waits for total and checks every ordered row across all pages. The real pagination E2E found 686px scroll width at a 680px viewport; flex-child/minmax sizing was fixed without dropping the assertion. Missing Monaco source-map and Vite chunk-size warnings are non-blocking build diagnostics, distinct from the retained browser console/page/HTTP error assertions.

## CI and integration

The workflow adds macOS native/no-CGO large-folder + low-fd coverage and real IPC E2E to the GUI build, alongside the existing Go/Node matrices, fuzz, cross-build and release checks. PR, exact source commit and observed CI are recorded in the delivery section after creation; workflow configuration alone is not remote success. No auto-merge is requested.

### First remote run and test timing correction

[PR #14](https://github.com/StevenWinsir/FolderWatch/pull/14) was opened at implementation commit `9d224e1`. Its first push run [37220181452](https://github.com/StevenWinsir/FolderWatch/actions/runs/37220181452) failed the macOS/Go1.23.12 randomized race job because `TestAllChangePagesAndServerSearchAreAccessible` reused a generic five-second small-fixture convergence helper; the log reported that timeout, not a data-race detector report. The same test passed the independent PR run's Go1.23 matrix, and a local Go1.23.12 single-core race rerun passed ten times (whole test 2.24–3.34s). These passing observations do not erase the first CI failure.

The follow-up changes only that test's wait: retain real live-watcher delivery with no forced reconciliation, use a bounded 30-second functional coverage deadline, renew the frontend lease, poll at 25ms rather than 5ms, reject unexpected API errors immediately, and report last count/status on failure. Every 1001-row ordering, all-page, final-row, full-index-search and cleanup assertion remains. This is not a production timeout relaxation or removal of performance benchmarks. The exact follow-up validation and final PR-head CI are recorded in the PR delivery evidence.

Before pushing this test-only correction: Go1.23.12 `-race -cpu=1 -count=20` passed the exact live pagination case (70.686s total; observed post-write convergence 1.847–2.958s); Go1.26.6 `go test -count=1 ./...` passed again, and GUI backend `-race -shuffle=on -count=10` passed (101.863s). Formatter output was empty. Production Go/C/TypeScript code and generated bindings were unchanged by this correction.

## Explicit boundaries

No fixed inventory maximum does not mean infinite storage or constant total RSS/time. Metadata disk/mmap/OS cache, optional ignore caches, legacy diagnostic/Terminal/NDJSON full-list consumers, path length, available disk and filesystem I/O remain scale constraints. Initial regular-file hashing still reads the file. Unknown startup bytes remain unknown until a complete explicit Reset. Polling reconciles final state, not every transient event between scans.

This work does not certify network-volume/remote-writer behavior, all macOS versions, native WKWebView/VoiceOver manual interactions, multi-hour GUI sleep/wake endurance, Developer ID signing/notarization or Gate B. Historical gate signatures and the original vendor patch are not rewritten.

## Frozen local completion

The final frozen Go source rerun passed 350 test/subtest events (0 failures, one opt-in large fixture skipped only in the ordinary suite), followed by full `-race -shuffle=on -count=10` PASS. Both opt-in large variants were run separately. The final GUI build, 17 frontend tests, all 4 real IPC cases, and workflow YAML parse passed. These commands/results are retained in [local Go evidence](../benchmarks/large-folder/local-go-validation.json) and [GUI evidence](../benchmarks/large-folder/local-gui-validation.json).

The longer native resource run used 1000 files, 100-file bursts × 100 samples, 120.000488 seconds idle and 100 additional complete start/stop cycles. Actual execution lasted 164.549 seconds. Core P95 was 223.728ms, idle CPU 0.2085% of one core, idle events 0, peak RSS 30,130,176 bytes, fd 6→28→6 and cache entries 0. Goroutines were 1→5→2 with the native runtime helper remaining stable. Exact samples and execution bounds are in [soak.json](../benchmarks/large-folder/soak.json) and [soak execution](../benchmarks/large-folder/soak-execution.json); this is not a multi-hour endurance claim.

Selected raw evidence and SHA-256 manifest are committed under `docs/benchmarks/large-folder/`. Platform packaging records describe the explicitly dirty development rehearsal, not a clean release candidate. Exact source-commit packaging and GitHub PR/check results are recorded in the PR delivery comment and retained local delivery JSON; the PR Checks tab is authoritative for the latest head.
