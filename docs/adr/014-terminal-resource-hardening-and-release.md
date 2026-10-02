# ADR-014: Measured resource ownership and private Terminal candidates

Status: Implemented for R5; release/Gate A approval is separate (2026-10-02).

## Context

P12–P14 require reproducible directory-scale performance evidence, bounded lifecycle resources, a cross-platform test matrix and installable macOS Terminal binaries. A fast snapshot microbenchmark alone does not establish TUI latency, retained memory, or descriptor cleanup. The repository retains the reviewed fsnotify v1.8.0 kqueue patch and has not selected a public project license.

R5's first successful content/correctness probe could not enumerate Darwin fdescfs descriptors. Replacing that unsupported measurement with numeric descriptors from `/usr/sbin/lsof -p <self>` exposed a real bug: a 1,000-file Session went from 6 descriptors to 1,010 active and 1,007 after Close. The upstream-shaped Close implementation marked `done` closed and then called Remove, whose closed guard returned without releasing any watched descriptor. Directory cache cleanup and goroutine counts did not detect this.

## Decision

Extend the existing reproducible vendor patch, without changing dependency versions. Each native registration/removal owns its descriptor under `doneMu`; directory recursion occurs outside that lock. Close marks the backend closed, wakes and joins its reader, directly closes every remaining owned descriptor, clears its maps and publishes `closeDone`. Concurrent Close callers await the same completed cleanup and receive the first cleanup error. New registration cannot publish an fd after the close fence. Native opens use O_CLOEXEC/O_NOFOLLOW, and the kqueue itself is close-on-exec. A failed re-registration does not close an already-owned descriptor while leaving its map entry alive. Preserve the existing no-follow, unknown-fd overflow, directory invalidation and seen-state fixes.

Regenerate `patches/fsnotify-v1.8.0.patch` and its manifest against the hash-verified v1.8.0 module-cache source using `scripts/refresh_vendor_patch.py`. The ordinary vendor guard remains read-only and rejects unreviewed source/patch hashes or a build that bypasses vendor. Keep the dependency's Go 1.17 language syntax, while FolderWatch itself requires Go 1.23+.

For retained small text, pre-size a capture-owned bytes.Buffer within the existing retention decision and transfer ownership instead of copying it a second time. ReadContent continues to return defensive copies. No reusable pool, asynchronous hashing, mtime shortcut, baseline advancement or additional retained current-content cache is introduced.

Use developer-only `fwbench` for generated disposable 1k/10k, burst, large-text, binary, deep-tree and start/stop workloads; report Core latency and resource counts explicitly. Use a separate real-binary PTY probe for write-to-TUI-output latency. Record raw samples and nearest-rank P95; subtracting the nominal debounce is labeled an estimate, not an internal phase measurement. CPU uses process user+system time divided by elapsed time (100% = one core); heap, cumulative allocations, content budgets and peak RSS are distinct. Profile files and fixture writes remain outside the monitored root or in private synthetic roots. Unsupported FD measurement fails a benchmark rather than proving absence of leaks. Test helpers are not linked into the shipped executable.

Keep all existing unit/integration/golden/model/CLI/PTY regressions. Add native fd cleanup and concurrent Close/enrollment tests, Session lifecycle/reset and bounded-overflow convergence tests, capture ownership tests and event-normalization fuzzing. CI retains macOS/Linux × Go1.23/1.26 and Windows compile-only, adds six bounded fuzz targets, retained test/coverage logs, Python packaging tests and a native macOS candidate install check. `go list -deps` enforces the transitive Core/UI boundary.

Build private evaluation candidates for darwin/arm64 and darwin/amd64 from a clean checkout using vendor, CGO disabled, trimpath, explicit version/commit/commit-date and deterministic tar/gzip metadata. Include dependency/Go licenses, manifest, SHA256SUMS and a Homebrew formula with actual archive checksums. Verify archive paths/types, executable architecture, metadata and installation in isolated HOME/PATH. SHA-256 detects corruption against trusted expected checksums; it is not publisher authentication or notarization. Manual workflow dispatch uploads candidate artifacts with contents:read permission only, never publishes a public release or signs Gate A.

## Alternatives

Ignoring unavailable FD metrics or treating deleted cache directories as proof of cleanup would conceal the observed leak. Closing watches only in the FolderWatch adapter before native Close races native enrollment and does not repair the library's lifecycle contract. Weakening test counts/deadlines or removing final-state hash assertions was rejected. A second native watcher, broad dependency upgrade, buffer pool, hash-skipping cache, public license selection or GUI work would enlarge the scope or change established semantics without evidence.

## Consequences

Descriptor use while active is still proportional to kqueue's watched files; it is not a constant-per-directory guarantee. Permission/OS descriptor limits remain visible startup failures. Native Close now waits for its reader and cleanup instead of returning early. Tests exercise this contract under race instrumentation and without a public event consumer. Memory content budgets remain per-generation logical retention limits, not whole-process RSS promises.

Performance data is conditional on the recorded hardware, Go version, fixtures and observation window; a minutes-scale stress run is not an hours/days endurance claim. PTY output checks are not Terminal.app/iTerm2 human visual acceptance. Clean HOME/PATH on a developer Mac is not a fresh Mac. Gate A remains FAIL/pending until those manual criteria are signed; public release also needs the owner's project-license decision. No tag, tap publication, Developer ID signature, notarization or public v1 is implied.

## Migration

Preserve ADR-009 plus this lifecycle extension during future dependency upgrades. Recreate the patch, run its hash/reversibility guard, all native/resource tests and macOS/Linux race/PTY CI before dropping any local fix. Re-measure the same fixtures after performance changes, retaining failed diagnostic evidence separately. Resolve the manual entries in `docs/gates/Gate-A.md` before R6; publish exactly the approved, checked artifact, never silently rebuild it under the same version with different provenance.
