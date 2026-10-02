# ADR-009: Reproducible fsnotify v1.8.0 kqueue patch

Status: Accepted for R2 (2026-10-02)

## Context
Real macOS directory-migration testing and inspection of the pinned fsnotify v1.8.0 source exposed two incompatible behaviors: its kqueue backend follows symlinks internally when adding a parent directory, and an already-retired descriptor can yield an unknown path. Filtering returned paths alone cannot prevent the library from opening an out-of-root symlink target. Directory WRITE notifications were also consumed internally, leaving no fallback invalidation for skipped symlink deletion or registration-window races.

## Decision
Retain pinned dependency versions and commit standard `go mod vendor` output, including upstream licenses and modules.txt. Apply only `patches/fsnotify-v1.8.0.patch` to `vendor/github.com/fsnotify/fsnotify/backend_kqueue.go`. The source diff and SHA-256 manifest are reproducible against the Go module cache's original v1.8.0 source.

The patch skips internal symlink targets and adds O_NOFOLLOW to native opens; reports unknown retired descriptors as overflow rather than allowing an empty name to become cwd; propagates directory-change errors and emits directory WRITE invalidations for reconciliation; retires seen-state for skipped/unreadable children that disappear or whose containing directory is removed, so link churn cannot accumulate historical names. The project adapter still owns directory identity checks, shared Ignore, queues, cancellation and root/path boundaries. No public third-party API is added and no dependency version is claimed to be current/latest.

## Alternatives
Ignoring returned out-of-root events does not satisfy no-follow. Silently upgrading the dependency without verification would not prove this policy. Reimplementing a complete kqueue backend in core adds a larger maintenance surface. The small vendored patch is explicit and testable until an evaluated upstream release can replace it.

## Consequences
Default Go builds from this repository use vendor mode. Build from a checkout with `make build`; do not use `-mod=mod` or assume `go install ...@version` includes the patch. `make lint` verifies the patched file checksum and its reversible patch. Running `go mod vendor` alone would erase the patch; regenerate with `go mod vendor`, then `git apply patches/fsnotify-v1.8.0.patch`, then `make lint test race smoke`.

`TestNativeDirectoryWatchDoesNotFollowSymlinks` tests the library before the wrapper filter, so hiding external events cannot fake compliance. Directory migration/recreation, root deletion, symlink cycles, raw overflow and later nested writes have real-filesystem tests. Normal post-migration events can be folded into subtree/root invalidation during enrollment; a later independent write is separately checked.

All other vendored files remain generated copies of the pinned graph. The fsnotify BSD-3-Clause license and other dependency licenses remain in vendor. Distribution license/vulnerability review is still required at R5; vendoring is neither legal clearance nor a security audit. kqueue can internally allocate one descriptor per file; OS descriptor exhaustion is surfaced, not a promise of constant descriptor use per directory.

## Migration
For a dependency upgrade, regenerate the graph, review upstream changes and evaluate whether the patch is still required. Update the patch and manifest together only after tests pass; do not remove the native no-follow regression. Recheck macOS and Linux integration/race tests and CI.
