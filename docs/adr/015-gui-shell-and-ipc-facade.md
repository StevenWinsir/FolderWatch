# ADR-015 — Wails shell and versioned IPC facade

- Status: Accepted for implementation in R6; pull-request review is required before merge.
- Scope: P15–P16 only. P0–P14 and Gate A remain the accepted Terminal baseline.

## Context

The desktop adapter must reuse the tested Go application core without introducing another watcher, snapshot store, classifier or diff engine. A WebView document can reload while startup, reset or diff RPCs are still running. Wails events are notifications rather than a durable stream. JavaScript numbers cannot represent every Go uint64 exactly. The R5 fsnotify v1.8.0 vendor patch is part of the accepted core, not an optional development override.

## Decision

1. Keep one Go module and the reviewed vendor tree. Put a headless, testable facade in `gui/backend`; compile the native Wails entry point with `desktop || bindings` build tags (Wails substitutes `bindings` during generation). Default CLI/test/cross-build targets remain free of native GUI dependencies.
2. Bind only `backend.API`. Lifecycle methods, native handles and core structs are not exported to JavaScript. Each RPC returns a typed DTO envelope with a stable problem code. App metadata advertises protocol version 1.
3. Use random frontend and session capability IDs. Every session operation identifies both owners. A new frontend attachment first revokes the previous client, cancels its session, and waits for cleanup before returning. Start admits no second owner while startup or teardown is in progress. Failed startup also joins its owner before returning.
4. Use best-effort pagehide/unmount detachment plus a 2-second heartbeat and 15-second backend lease. An expired lease cannot be revived. Lease expiry begins cancellation; completion still depends on cooperative core/OS I/O. Suspension or severe event-loop starvation can intentionally end monitoring rather than leave an unattended owner. Background/sleep UX refinement belongs to R8.
5. Emit only bounded metadata invalidations on `session.status`, `changes.updated`, `session.warning`, and `session.error`. A 32-slot queue coalesces overflow to the latest full status. A global sequence orders notifications across starts; core generation/version identify the authoritative data. Serialize counters and byte sizes as decimal strings.
6. Serve change lists in pages of at most 500 summaries. Continuation requires generation/version. Return diffs only on demand with the selected path version and generation, one active request, session cancellation, a 15-second deadline and a conservative 16 MiB JSON upper bound. Oversize previews become metadata-only `too-large` results. The core computes all classifications, changes and hunks.
7. Require an explicit absolute or `~/` root at startup. Diff accepts only canonical root-relative keys, rejects traversal/absolute/NUL/backslash paths, checks root/ancestors and relies on the core's no-follow content reads. Final symlinks remain metadata-only. This is confinement within the selected session, not an atomic sandbox against a malicious process replacing ancestors or a compromised WebView selecting another root.
8. Keep R6 UI intentionally small: path input, status, Start/Stop, reconnect, native menus and version metadata. No fake picker, settings, file list or Monaco view. R7 and R8 implement those surfaces against this contract.

## Alternatives considered

- A separate GUI Go module: rejected because a module replacement of the root would not automatically inherit its reviewed vendor patch.
- Directly binding `app.Session`: rejected because it exposes internal structures and makes lifetime/serialization part of the public contract.
- Only handling pagehide or Wails shutdown: insufficient for document crashes and delayed RPCs.
- Broadcasting file text or sending complete lists for every filesystem event: rejected due to memory and WebView message pressure.
- JavaScript numeric generation/version fields: rejected due to precision loss beyond 2^53.

## Consequences

There is one core owner and a deterministic cleanup boundary. Consumers must treat notifications as invalidations, ignore stale client/sequence values, and reload versioned data after gaps. Monitoring is intentionally not restored after reload or lease expiry. The root module gains pinned Wails dependencies, while default binaries still link no Wails code. npm dependencies and generated Wails bindings are committed through lockfiles/generated source; node_modules and compiled apps are excluded. Development transport must be loopback-only and is not part of the production application.

This round does not claim a signed, notarized or Gate B-approved GUI release. Native picker, full selection/diff presentation, settings persistence, system integrations and long-running background UX retain their later milestones.

## Dependency and build review

Wails is pinned to v2.10.1 to provide the native window/menu/runtime and generated Go bindings rather than introducing another transport stack. Svelte/TypeScript provide the small reactive UI; Vite is the asset toolchain, Vitest/jsdom/testing-library cover controller/components, and Playwright exercises the real dev transport. Exact frontend versions and transitive integrity are in package-lock.json. The CLI build does not import Wails; test-only frontend dependencies are not loaded by the packaged application.

The one-time root module/vendor refresh brought Wails transitive updates to go-runewidth, termenv and golang.org/x packages. The reviewed fsnotify v1.8.0 patched file is byte-identical to the R5 base, verified by the guard and regression suite. Vendor retains upstream license files; npm sources retain their packaged notices. This records dependency/source review, not a security-audit claim or permission to publicly relicense/distribute FolderWatch. Formal GUI third-party-notice packaging and distribution approval remain R9 work.

Do not check in node_modules, dist, built apps or Wails' package.json.md5 install stamp. A generated development app is not a release candidate; its embedded commit is the Git base when built from a dirty tree. Native minimum-OS/Intel compatibility and signing must be established separately at the release gate.

## Migration

R7 should use generated bindings, keep `clientId`/`sessionId` and selection generation/version on each asynchronous request, and retry stale list pagination from the beginning. `Batch.Removed` in the core means a path is no longer changed, never a new deletion. R8 can extend the DTO contract additively; incompatible field or lifecycle changes require a protocol increment and new shared fixtures. Dependency refreshes must reapply and verify the fsnotify patch and rerun Terminal regression tests.
