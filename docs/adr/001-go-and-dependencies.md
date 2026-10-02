# ADR-001: Go and pinned dependencies

Status: Accepted (R1, 2026-10-01)

## Context
FolderWatch needs one local backend reusable by Terminal and eventually Wails. R1 is foundation/input only, not a working watcher or TUI.

## Decision
Use Go, minimum language/toolchain 1.23.0, one module `github.com/StevenWinsir/FolderWatch`. macOS is the primary target. Pin dependencies and commit go.sum. The R1 runtime uses pflag 1.0.6 (interspersed CLI flags), go-toml/v2 2.2.3 (strict TOML), doublestar/v4 4.7.1 (glob primitive behind our ignore matcher). Planned Terminal adapters are pinned in the `tools` build-tag package: fsnotify 1.8.0, Bubble Tea 1.2.4, Lip Gloss 1.0.0, go-difflib 1.0.0. They do not enter the R1 executable. These are deliberate fixed versions, not a claim that they are the latest.

## Alternatives
Rust adds frontend/backend integration complexity here. A second TypeScript core duplicates filesystem semantics. Importing future adapters into app/main would violate layering.

## Consequences
Run `go mod tidy` after dependency changes and review go.mod/go.sum. Direct libraries use permissive licenses: MIT (doublestar, TOML, Bubble Tea, Lip Gloss), BSD-3-Clause (pflag, fsnotify and go-difflib). Transitive MIT/BSD licenses must be retained in future distribution notices; this is not legal clearance or a security audit. R5 must review licenses and vulnerabilities for the actual shipped graph. No project license is assigned without the owner's decision.

## Migration
Introduce real watcher/diff/TUI adapters only in their designated rounds, removing their placeholder imports then. Dependency upgrades require tests and an explicit version change, never an unreviewed latest upgrade.
