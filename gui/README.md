# FolderWatch desktop shell (R6–R8)

This is the P15–P21 Wails/Svelte adapter, not the final desktop release. The Terminal's watcher, snapshots, classifier, ChangeStore and diff engine remain the only implementation of file semantics.

## Build and run

Run all Make targets from the repository root. Requirements for the supported local target are macOS, Go compatible with the root `go.mod`, Node 22 or 24 with npm, and Xcode Command Line Tools. The Wails CLI is pinned to v2.10.1 and installed into ignored `bin/`; the frontend uses exact package versions and `package-lock.json`.

```sh
make gui-setup
make gui-build VERSION=0.2.0-dev
open gui/build/bin/FolderWatch.app

# Development: Wails RPC/assets bind to loopback only.
make gui-dev
```

`make gui-check` runs Svelte/TypeScript checks, Vitest controller/component/contract tests and the Vite production build. `make gui-bindings` regenerates the committed Wails API/models/runtime. `make test`, `make race`, `make lint` and `make smoke` still run from the root. GUI backend tests are included in the ordinary Go test tree without requiring a WebView or GTK.

The native entry point uses `desktop || bindings` build tags. Wails removes `desktop` during binding generation and supplies `bindings`, so both are intentional. Keep the root Go module: a separate GUI module would bypass the reviewed fsnotify vendor patch. `gui-build` explicitly skips Wails module synchronization/tidy after generation and checks the active patched source path. Do not run an unreviewed vendor refresh; see ADR-009/014 and the root contribution instructions.

For a first binding-generation pass only, the Make target creates an ignored minimal `frontend/dist/index.html` when no frontend build exists. That bootstrap is never the application build: `gui-build` must run and pass the real production frontend build before packaging.

## R6–R8 scope

The window has native menus, version metadata, a native folder picker plus a root path input, Start/Stop/Pause/Resume/Reset, shared Settings, session status, reconnect/error states, a filtered changed-files list, a read-only Monaco diff viewer, external editor/Finder/copy actions and System/Light/Dark themes. Start requires an absolute path or `~/path`; it respects shared user/project config. Stop can cancel the initial scan. Files are never edited by this shell.

The facade supports pause/resume/reset, versioned summary pagination and on-demand structured diff for R7 consumers, even though the R6 screen deliberately does not pretend to be a completed diff workspace. Binary, unsupported text, huge files and symlinks stay metadata-only when the core says they cannot be diffed. Additional wire-budget fallback is documented in `docs/gui-ipc-v1.md`.

Page reload intentionally closes monitoring. Listener cleanup plus a 15-second backend lease handle lost documents. Visibility wake renews the lease when possible and refreshes authoritative state; if sleep already expired the lease, reconnecting reports Idle and does not silently restart monitoring.

## Tests and later work

The committed browser smoke workflow (`npm run test:e2e`) targets an already-started Wails dev server at `http://127.0.0.1:34115`. Alternatively, `FW_GUI_START_SERVER=1 make gui-e2e` starts, waits for and tears down its own Wails dev process, refusing to reuse an occupied port. Set `FW_BROWSER_CHANNEL=chrome` to use installed Chrome, or install Playwright Chromium with `cd gui/frontend && npx playwright install chromium`. It exercises the real Go bindings/core through Wails' development WebSocket transport; it is not a browser mock or proof of production WKWebView rendering. The production `.app` must be checked separately. Browser tests use temporary fixture directories only.

See `docs/rounds/R6-acceptance.md`, `docs/rounds/R7-acceptance.md` and `docs/rounds/R8-acceptance.md` for the actual executed commands, environments, screenshots, failures/fixes and remaining validation limits. R9/Gate B owns formal packaging, signing/notarization and release acceptance. The generated local `.app` is unsigned development output and must not be represented as Gate B-approved.
