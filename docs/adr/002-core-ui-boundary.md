# ADR-002: Core and UI boundary

Status: Accepted (R1, 2026-10-01)

## Context
Both future TUI and GUI must consume the same scan, watcher, baseline and diff logic.

## Decision
`cmd/folderwatch/main.go` only assembles process context/version/streams and exits. `internal/cli` owns flags and rendering. `internal/config`, `pathutil`, `ignore`, `scan`, `model` contain UI-independent logic. `internal/app.Prepare` composes validated config, a reusable matcher and an initial inventory. Context cancellation is propagated. No fsnotify/TUI/Wails imports in these core packages.

## Alternatives
A large main or UI-owned filesystem logic is initially shorter but makes reuse and tests unreliable.

## Consequences
R1 offers a one-shot scan (`--scan`, optionally `--json`); the default also scans and exits until the R4 interface exists. It must never claim to be monitoring or to have captured baseline content. Scan DTOs carry paths/metadata, never file contents. R1 does not create state/log/snapshot files in the monitored tree.

## Migration
R2 should use Prepared.Config, Prepared.Matcher and Prepared.Inventory to register directories, then add watcher/debounce/snapshot in their own packages. R4 can change the default to TUI while retaining explicit `--scan` for diagnostics. GUI remains prohibited before Gate A.
