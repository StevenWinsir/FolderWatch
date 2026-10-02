# R6 acceptance — P15–P16 GUI shell and IPC/Core Facade

Date: 2026-10-02. Branch: `feat/r6-gui-shell-ipc`. Base: `88b3541bf4d4b9bc237202276db63613a9ca2aac` (merged PR #6, Gate A approval).

## Status

**Implementation and local automated acceptance: PASS. Round integration: IN_REVIEW.** P0–P14/Gate A remain accepted. P17–P25 and Gate B are not part of this delivery. The branch is submitted for review, not automatically merged. Independent human review and actual native WKWebView button/menu/keyboard interaction are not claimed.

The root `Handoff_Rounds.md` has been updated in place: current banner, round table, R6 checklist, next-developer instructions and a new §30. Earlier round evidence remains historical rather than being overwritten. README, CHANGELOG and CONTRIBUTING now reflect R6 and the already-passed Gate A.

## Delivered contract and implementation

| Stage | Delivered | Evidence |
|---|---|---|
| P15 | Wails v2.10.1 app, embedded Svelte/TypeScript assets, version metadata, native menus, 1040×720 / minimum 680×480 window, path/Start/Stop/reconnect/status/error UI | `gui/main.go`, `gui/wails.json`, `gui/frontend/src/`, production build |
| P15 | Headless lifecycle host, one event pump, concurrent Start/Close safety, application-context cancellation and join | `gui/host/host.go`, 3 host tests, race x10 |
| P16 | The only bound object is `backend.API`, forwarding to the existing `internal/app` session rather than duplicating core logic | `gui/backend/`, core dependency guard, full Terminal regression |
| P16 | Start/Stop/Pause/Resume/Reset/status, attachment/detachment/heartbeat, versioned paginated changes and on-demand Diff | Go facade tests and real Wails IPC E2E |
| P16 | Random client/session capabilities, scan cancellation, joined teardown, expired lease revocation and stale RPC rejection | Stop-during-scan, reload/restart/concurrent-close/heartbeat tests |
| P16 | Metadata-only events, bounded queue, explicit reload, counter strings, independent DTOs and shared Go/TS fixture | Contract/precision/reflection/backpressure tests |
| P16 | Canonical root-relative keys, no ancestor symlink traversal, metadata-only final symlinks, Diff final version fence | Traversal/symlink/old-generation/pagination tests |

The frontend owns view/connection state only. The core remains the sole watcher, baseline, classifier, change store and diff engine. `changes.updated` is an invalidation, not a full file list. `GetChanges` defaults to 200/max 500 summaries; pagination must retain generation and global version. `GetDiff` uses generation and the selected path version, one active request, a 15-second context and a conservative 16 MiB JSON budget. Counters and byte sizes are decimal strings. Over-budget Diff returns `too-large` without hunks.

Frontend heartbeat is every 2 seconds, with a 15-second lease. A new attachment revokes the old client and joins its owner before returning. Reload does not resume monitoring. Suspension/starvation can expire the lease; this is a documented R8 UX follow-up, not an unattended watcher leak. Full details: [IPC v1](../gui-ipc-v1.md), [ADR-015](../adr/015-gui-shell-and-ipc-facade.md), [GUI build guide](../../gui/README.md).

## Local validation environment

macOS 26.6.2, arm64 Apple M4; Go 1.26.6; Node 24.19.0; Wails CLI 2.10.1; repository vendor mode and `GOWORK=off`. Xcode is installed. Frontend lockfile is committed. Local Node22 and non-Mac GUI runtime execution are not claimed; the CI matrix covers Node22/24 frontend checks separately.

Final full regression ran **17:03:11–17:05:26 UTC**. Machine-readable evidence is [R6-validation.json](R6-validation.json); raw logs/coverage remain in ignored `artifacts/r6-final/`. Counts below are from executed results, not from configured CI jobs.

| Validation | Actual result |
|---|---|
| `make lint` | PASS: gofmt, vendor SHA/reversible patch/actual compiler path, Core/UI boundary, go vet |
| `make scripts-test` | 8 Python tests PASS |
| `go build ./...` | PASS; no native GUI dependency required for headless build |
| `go test -count=1 -json -coverprofile=... ./...` | 158 top-level / 320 test and subtest pass events; 0 failed, 0 skipped |
| Statement coverage | 80.4% across measured Go packages; not native WebView or frontend coverage |
| `go test -race -shuffle=on -count=10 ./...` | All packages, ten randomized-order runs PASS |
| `go test -race -count=1 -v ./gui/backend ./gui/host` | 18 backend + 3 host tests PASS |
| `make smoke` | CLI 60, watcher 17, semantic 16, TUI PTY 21 checks PASS; real headless Vim direct/atomic saves included |
| `make fuzz` | Six bounded targets PASS: classifier, reconstruction, canonical containment, normalization, size parsing, text chunk boundaries |
| `go mod verify`; `go mod tidy -diff` | PASS, no module-graph drift |
| Headless cross-build | darwin/arm64, darwin/amd64, linux/amd64, windows/amd64 PASS; not native GUI runtime evidence |
| `make gui-check` | Svelte/TS 0 errors/0 warnings; Vitest 3 files / 11 tests PASS; Vite production build PASS |
| `make gui-build VERSION=0.2.0-dev` | Binding generation, frontend, native darwin/arm64 compilation and `.app` packaging PASS |

### Resource and lifecycle observations

`TestFacadeLifecycleReleasesDescriptorsAndDiskSnapshots` exercises a real disk-backed baseline and 12 alternating Stop/reload cycles. Descriptors **6 → 6**, goroutines **2 → 2**, temporary cache empty after every teardown. The suite also runs 30 complete Start/Stop cycles and 50 groups of concurrent host Start/Close/context callbacks. It covers old-client detach after a successor attaches, old-session Stop/Reset/Diff, failed startup cleanup, root-loss error recovery, scan cancellation and lease expiry.

These are bounded regression measurements, not proof of hour/day-long GUI endurance or hard real-time cancellation of slow OS I/O. The R5 Gate A candidate and its separate performance/human evidence are not replaced by this development build.

## Rendered UI / real IPC QA

**Flow:** load shell → enter a fixture root → Start → observe real core change/Diff → Pause/Resume/Reset → Stop/Start → reload → reject old capability → recover from invalid root.

Environment: `http://127.0.0.1:34115`, actual Wails development server and Go API, installed Chrome via Playwright. **Browser plugin not available**, so the existing repository Playwright workflow was used without installing another browser. No mock backend or test-only production RPC was introduced.

Command:

```sh
FW_GUI_START_SERVER=1 FW_BROWSER_CHANNEL=chrome \
  FW_GUI_QA_DIR=/tmp/folderwatch-r6-final-gui-qa make gui-e2e
```

**2/2 workflows PASS in 10.2 seconds**, zero retries. The owned Wails server shut down at the end.

| Check | Result |
|---|---|
| Page identity and nonblank content | FolderWatch title, real GetAppInfo protocol/heartbeat/lease, visible heading/path input/status PASS |
| Error overlay | No Vite error overlay |
| Console/network health | No console warning/error, uncaught page error or HTTP >=400 response in either workflow |
| Visible controls | Start/Stop/Start produces Monitoring/Idle from the backend; failed relative root remains Idle and recovers |
| Filesystem/Core integration | Unicode/space/quote filenames, modified summary, before/after hunks, Pause/Resume/Reset and new generation PASS |
| Privacy/path behavior | Events contain no fixture content sentinel; traversal returns INVALID_PATH |
| Reload | Reload returns Idle; an old client's Stop returns STALE_CLIENT and cannot affect a successor |
| Layout | 1040×720 and 680×480, plus 380×800 CSS stress: content present, no horizontal overflow |
| Screenshots | desktop-idle, desktop-monitoring, minimum-window, narrow-css-stress and input-error captured outside tracked source |

Screenshot paths are under `/tmp/folderwatch-r6-final-gui-qa/`; local inspection copies are in ignored `artifacts/r6-final/browser/`. The 380px test is a CSS stress case, not a supported mobile application. Browser screenshots are not labeled as production WKWebView screenshots.

### Native process verification and explicit remaining UI gap

The **production** `.app` executable was launched directly, remained alive, had no TCP listeners at the observation point, then accepted `NSRunningApplication.terminate` (`true`, caller exit 0) and exited with code 0. The confirmed check ran at **17:07:59 UTC**; local evidence: `artifacts/r6-final/native-confirmed.json` and its empty output log. This verifies native process launch/shutdown, not visual correctness. An earlier AppleScript bundle-id quit returned an error despite the process exiting 0; it was replaced with the acknowledged process-targeted check rather than treated as proof of successful AppleScript delivery.

The runner reports **Accessibility trusted=false** and **Screen Recording permission denied**. Therefore native window screenshots and actual WKWebView button/menu/keyboard interaction could not be verified in this run. No system permissions were changed or worked around. Before native acceptance, an authorized reviewer should open the built app, exercise Start/Stop/restart, cancel an initial scan, inspect About/Edit/Quit and minimum-window behavior, and confirm shutdown cleanup. This remains an explicit integration QA item.

The built app initially embeds base commit `88b3541` because the source was not yet committed. It is not a provenance-verified release candidate; final Git source and CI build identity belong in the integration section below. Developer ID signing, notarization, clean-Mac installation, Intel/older-macOS GUI support, accessibility conformance and extended background/sleep testing remain R8/R9 work. Do not infer support from Wails' template minimum-OS field.

## Failures corrected during R6

The first Wails CLI download exceeded its initial budget; the resumed install completed and the pinned tool is now usable. Svelte component tests initially resolved the server entry and rejected mount; Vite browser resolution fixed this. E2E initially lacked Node type declarations and contained an extra parenthesis; pinned types and the syntax fix restored checking. Wails binding generation needed `desktop || bindings` and an ignored bootstrap asset; normal builds preserve vendor using the documented flags. The first real IPC UI test caught a favicon 404 through the strict console/network assertion; an actual favicon and link fixed it without weakening assertions. Native host application-context cancellation received a dedicated regression test before the final full test/race run.

## Review, scope and integration

Self-review covered facade ownership/lock ordering, teardown joins, random capabilities, stale-response fences, root/path checks, bounded events/DTOs/diff payload, generated RPC surface, frontend subscription/timer disposal and preservation of core/vendor behavior. This is not independent human review. No pre-existing user changes were discarded. No second module/watcher/diff implementation, file-content broadcast, editor command execution or signing credentials were added.

Files changed in this round include `gui/`, root module/vendor/Makefile/CI, ADR-015, IPC v1, README/CHANGELOG/CONTRIBUTING and the original Handoff sections plus §30. Generated bindings and lockfiles are source; node_modules, dist, .app, Wails install stamps and raw test artifacts are excluded from the PR.

**GitHub:** Implementation commit **`265e19891cfb119e09f02121910be991030eba48`** is pushed on `feat/r6-gui-shell-ipc` and submitted as **[PR #7](https://github.com/StevenWinsir/FolderWatch/pull/7)** against main. PR state is OPEN; it was not automatically merged. [Source push CI 37039054920](https://github.com/StevenWinsir/FolderWatch/actions/runs/37039054920) and [source PR CI 37039099850](https://github.com/StevenWinsir/FolderWatch/actions/runs/37039099850) both completed successfully, **10/10 jobs each, 20/20 total**. This includes macOS/Linux × Go1.23/1.26, Node22/24, native Wails build/binding drift check, cross-build, fuzz and Terminal candidate. Frozen exact job IDs, timestamps and URLs are in [R6-ci-source.json](R6-ci-source.json). These results belong to the implementation commit; follow the PR checks for later documentation-only head runs rather than relabeling the frozen source runs.

**Clean local build:** After committing, `make gui-build VERSION=0.2.0-dev` passed again from a clean `265e198` checkout. Binding generation and build left tracked module/vendor/bindings unchanged (`git diff --exit-code`). Go build info confirms embedded commit/version/build-date. The local darwin/arm64 executable SHA-256 is `d1cdb6983f32ecb1bda2d82f6bf05fbf6ebb416a40b4c682b658ed446cb1b3a7`; it does not describe a different-toolchain CI artifact. At 17:16:05 UTC this clean build passed native process launch, no TCP listener observation and acknowledged graceful termination with exit 0. [R6-build.json](R6-build.json) records the exact source/artifact and scope. codesign reports only an ad-hoc linker signature, not Developer ID, a sealed bundle or notarization. Native visual/button/menu acceptance remains unverified as explained above.

## R7 handoff

After R6 review/merge, implement native Folder Picker, changes list/filter and read-only Monaco against generated IPC v1. Preserve client/session and selection generation/path-version checks; restart stale pagination rather than mixing generations. Do not interpret core Removed as Deleted, compute filesystem semantics in JavaScript, prefetch every Diff or silently restart an expired session. R8 handles settings/editor/reveal and background/sleep UX; R9 handles GUI release/notice packaging and Gate B.
