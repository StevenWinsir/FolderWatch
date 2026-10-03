# Round R8 Acceptance — P19–P21

Date: 2026-10-02. Scope: GUI Session controls, shared Settings, macOS system integration, accessibility and lifecycle robustness. The implementation is on the current `codex/r7-gui-main-diff` checkout and remains IN_REVIEW until the pull request is reviewed and merged. This record does not claim Gate B, signing, notarization or native accessibility approval.

## Delivered

| Area | Delivered | Evidence |
|---|---|---|
| P19 Session | Pause, Resume, Reset Baseline with confirmation, Stop and Change folder | `gui/backend/api.go`, `gui/frontend/src/controller.ts`, `gui/frontend/src/App.svelte` |
| P19 Settings | Debounce, ignore patterns, `.gitignore` mode, max diff bytes and editor; values are loaded through `config.Load` and sent as the existing `config.Overlay` model | `GUISettings`, `SettingsRequest`, `GetSettings`, `StartOptions` |
| P21 theme | System, Light and Dark with persisted preference and explicit CSS overrides | `App.svelte`, `style.css` |
| P20 editor | Default macOS editor and configurable editor command parsed into an argument array; no `sh -c` or shell string concatenation | `gui/backend/system.go` |
| P20 Finder/clipboard | Reveal in Finder, copy relative path and editor action from the selected change; all paths are checked against the active canonical root | `PathRequest`, `EditorRequest`, `CopyPathRequest`, `safeKey` |
| P21 lifecycle | Visibility wake calls heartbeat/reconnect and change refresh; pagehide and component destruction remove listeners, timers, subscriptions and Monaco models | `App.svelte`, `controller.ts`, `workspace.ts`, `DiffEditor.svelte` |
| P21 resilience | Empty settings arrays remain JSON arrays, explicit error/empty/loading states stay available, and status text/letters supplement color | backend DTO tests and UI shell |

## Security and behavior decisions

- GUI and CLI use the same defaults and TOML project/user configuration. `GetSettings` reads the config without scanning, and `StartSession` still validates the complete config in `app.Prepare`.
- `Reset Baseline` is never silent: the UI asks for confirmation that the current state becomes the new baseline and the existing list is cleared.
- Root-relative keys are canonical, reject traversal, absolute paths, backslashes and symlinked ancestors, and are converted to absolute paths only after validation. Unicode, spaces and quotes remain ordinary arguments.
- Custom editor commands support quoted arguments but reject NUL, newlines, shell operators, unmatched quotes and escapes. The executable and each argument are passed directly to `exec.CommandContext`.
- Sleep/wake does not silently restart a session that the 15-second lease already stopped. Wake reconnects and refreshes the authoritative status/list, so the UI cannot display a phantom monitoring state.

## Automated validation

Environment: macOS arm64, Go 1.26.6, Node 26.10.0, Wails 2.10.1, Chrome.

| Command | Result |
|---|---|
| `go test ./...` | PASS |
| `go test -race ./...` | PASS |
| `make lint` | PASS: vendor guard, core boundary, `go vet` |
| `make gui-check` | PASS: Svelte/TypeScript 0 errors/0 warnings, Vitest 3 files/11 tests, Vite production build |
| `make gui-build VERSION=0.2.0-dev` | PASS: unsigned development `gui/build/bin/FolderWatch.app` |
| `FW_GUI_START_SERVER=1 FW_BROWSER_CHANNEL=chrome make gui-e2e` | PASS: 2/2 real Wails IPC tests, 13.2s |
| `go test -race ./gui/backend ./gui/host` | PASS; backend system-command/path/settings regression coverage included |

The real Wails tests cover the existing Start/Stop/change/diff/Pause/Resume/Reset/reload paths and remained green after adding Settings and lifecycle handling. One first E2E run exposed a `null` JSON ignore list on settings hydration; `guiSettings` now guarantees an empty array and the complete E2E suite was rerun to 2/2 PASS.

### Follow-up regression fix — 2026-10-02

The Monaco diff view previously rebuilt each returned hunk as a short standalone model, so its left and right line labels restarted at 1 even when the hunk began later in the file. The frontend now maps each displayed model line to the backend `oldLine`/`newLine` value, preserving source line numbers while still omitting the opposite-side lines. `gui/frontend/tests/diff-lines.test.ts` covers the mapping and the real Wails E2E checks labels 7–12 for a change at line 10.

| Command | Result |
|---|---|
| `npm test -- --run` | PASS: 4 files, 12 tests |
| `npm run check` | PASS: 0 diagnostics |
| `npm run build` | PASS |
| `go test ./internal/diff ./gui/backend ./internal/app ./internal/changes` | PASS |
| `FW_GUI_START_SERVER=1 FW_BROWSER_CHANNEL=chrome make gui-e2e` | PASS: 2/2 real Wails IPC tests |

## Known limits

- Native WKWebView button/menu/accessibility interaction still needs a reviewer with macOS Accessibility and Screen Recording permission. Playwright Wails IPC is not a substitute for that manual check.
- The generated `.app` is unsigned development output. Developer ID signing, notarization, clean-machine installation and Gate B belong to R9/P23–P24.
- Default editor, Finder and `pbcopy` integrations are macOS-specific. Unsupported platforms return a user-visible error rather than falling back to a shell.
- The wake path reconciles the authoritative status after reconnect; it intentionally does not silently recreate a session whose lease expired during sleep.
- Hour-scale GUI soak has not been claimed. Existing core resource tests and repeated start/stop/reload tests remain the resource boundary for this round.

## Handoff

R9 may focus on E2E expansion, packaging, signing/notarization and Gate B. It should preserve the single Core Facade and the direct-command path validation introduced here. Do not move editor execution into the frontend or reimplement watcher/config semantics there.
