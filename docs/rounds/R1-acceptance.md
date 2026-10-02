# R1 Acceptance — Foundation & Input (P0–P2)

Status: **PASS — local and GitHub Actions R1 integration acceptance**

Date: 2026-10-01

Owner: coding assistant acting on the repository owner's explicit P0–P2 request

Review: implementation self-review, regression tests, Git-oracle comparison, import-boundary test and local CLI QA. No independent human reviewer or Terminal/iTerm interactive acceptance is claimed.

Branch: `main` (initial empty-repository bootstrap; subsequent work follows feature PRs)

Start commit: `9fca76f3d72ef02fb656c70a4bddc7c2ff076186` (original Handoff preserved unchanged)

End checkpoint: `r1-complete` (resolve with `git rev-parse r1-complete^{commit}` after delivery)

Repository: https://github.com/StevenWinsir/FolderWatch (private)

## Scope and conclusion

P0, P1 and P2 are implemented. `app.Prepare` reliably produces validated normalized `Config`, reusable `ignore.Matcher` and metadata `scan.Result`, including structured partial-scan warnings. The program compiles and runs as a one-shot scan, not a monitoring session. P3–P25 remain unimplemented; **Gate A and Gate B are not passed**.

## Acceptance map

| Requirement | Evidence | Result |
|---|---|---|
| Go skeleton, locked dependencies, build tooling | go.mod/go.sum, Makefile, thin main, tools-tag dependency pins | PASS |
| CI definition and engineering conventions | .github/workflows/ci.yml, CONTRIBUTING.md, ADR-001–006 | PASS (definition and all 5 remote CI jobs; evidence below) |
| Help/version, defaults, invalid inputs | cli/config tests plus built-binary smoke | PASS |
| CLI > project > user > defaults; explicit false/empty arrays | config tests, smoke with isolated HOME/config | PASS |
| Nested directories, spaces, Unicode, all extensions | scan tests and smoke | PASS |
| Root-relative key, containment and no descendant link traversal | pathutil/ignore/scan tests, external/broken/cyclic links | PASS |
| .folderwatchignore, optional nested .gitignore, explicit file, CLI rules | matcher matrix, 16 Git-oracle rule sets × 22 paths, smoke | PASS |
| One shared initial/runtime policy | scan/runtime consistency test including newly created nested directories | PASS |
| Permissions/transient errors do not crash siblings | real mode-000 directory, injected permission/disappearance/Info failure, broken nested rules | PASS |
| No ordinary contents read or monitored-tree files written | FIFO metadata test, 5 GiB sparse-file smoke, reserved editor/log read-only tests | PASS |
| Cancellation / output errors / terminal controls | CLI and scan tests, including final-callback cancellation | PASS |
| Core independent of watcher/UI adapters | AST import boundary regression test | PASS |

## Commands verified locally

Environment: macOS 26.6.2 (25G83), darwin/arm64; Go 1.26.6; Git 2.54.0 (Apple Git-157). This is a correctness validation, not a P12 hardware/performance report.

| Command | Observed result |
|---|---|
| `go mod download`, `go mod tidy`, `go mod verify` | exit 0; all modules verified |
| `make fmt`, `go build ./...`, `make lint` | exit 0; gofmt and go vet pass |
| `go test -json -count=1 -coverprofile=coverage.out ./...` | exit 0; **127 tests/subtests pass**, 38 top-level tests/fuzz targets, no failed or skipped test cases |
| `go test -race -count=1 ./...` | exit 0, all tested packages pass |
| `make smoke` | binary build + **60 checks pass, 0 skipped** |
| `GOOS=darwin GOARCH=arm64 go build ./...` | PASS |
| `GOOS=darwin GOARCH=amd64 go build ./...` | PASS |
| `GOOS=linux GOARCH=amd64 go build ./...` | PASS |
| `GOOS=windows GOARCH=amd64 go build ./...` | PASS (compile only, not runtime support) |

`internal/model` has no executable statements/tests; its package-level skip is not a skipped test case. `cmd/folderwatch` is exercised by the external smoke harness rather than in-process unit coverage.

Coverage: total **88.4%** of statements; scan 96.1%, ignore 94.0%, CLI 92.7%, pathutil 88.9%, config 83.5%, fileutil 74.1%, app 71.4%. Generated local `coverage.out` and `artifacts/validation/go-test.jsonl` are ignored build/validation artifacts, not committed fixtures.

### Delivery addendum: bounded fuzz

- `go test ./internal/pathutil -run='^$' -fuzz=FuzzKeyContainment -fuzztime=5s -parallel=2`: PASS, 536,340 executions, 94 new interesting inputs.
- `go test ./internal/config -run='^$' -fuzz=FuzzSize -fuzztime=5s -parallel=2`: PASS, 453,329 executions, 192 new interesting inputs.

These are bounded bug-finding runs, not exhaustive proofs or performance benchmarks. Interesting inputs are kept in Go's local fuzz cache, not the monitored test trees.

### Delivery addendum: remote CI

Implementation commit **`8b59f7c92af293ec13fe1421983ff618abb7ec78`** passed [GitHub Actions run 36956805061](https://github.com/StevenWinsir/FolderWatch/actions/runs/36956805061). Observed workflow status: `completed`, conclusion: `success`; all **5 jobs succeeded**:

| Job | Result |
|---|---|
| ubuntu-latest / Go 1.23.x | PASS: lint, build, tests, race and CLI smoke |
| ubuntu-latest / Go 1.26.x | PASS: lint, build, tests, race and CLI smoke |
| macos-latest / Go 1.23.x | PASS: lint, build, tests, race and CLI smoke |
| macos-latest / Go 1.26.x | PASS: lint, build, tests, race and CLI smoke |
| cross-build | PASS: darwin arm64, darwin amd64 and windows amd64 |

The final documentation commit records this already-observed result without changing source, tests or workflow. `r1-complete` identifies the complete code-plus-handoff checkpoint. The [Actions page](https://github.com/StevenWinsir/FolderWatch/actions) is authoritative for subsequent runs. CI configuration alone is not counted as a passing run.

## Bugs found and fixed during this round

The first test run intentionally exposed real mismatches, then passed after source fixes and regression coverage: doublestar's trailing `/**` incorrectly matched its parent; nested rules were not preflighted before entering directories; a caller's incorrect directory hint could lead to reading a symlink target's rules. Review also tightened explicit empty root rejection, per-layer invalid-glob validation and final-callback cancellation. Historical failed runs are superseded by the final successful commands above, not omitted as though they never occurred.

## Behavior decisions frozen for R2

Default `.git/` exclusion, explicit `--include-git`, default-false `respect_gitignore`, rule-source ordering, ancestor-aware negation and cached-per-matcher policy are in ADR-005. The complete schema/overrides are in ADR-006. FileMeta contains only metadata, never SnapshotRef/content; baseline semantics are the **design** in ADR-003, not an implemented R1 feature. The 5 MiB diff budget is provisional and does not restrict inventory. The scanner includes `.` plus directories for future watcher registration, but excludes unreadable directories from that registration set.

## Known limitations and deferred work

There are no known unresolved blockers within the tested R1 scope. Ignore files do not hot-reload: existing scopes, missing files and errors are cached; restart/new Matcher is required. Full Git global/track-status semantics are deliberately unsupported. Path checks prevent ordinary symlink traversal but are not an atomic adversarial filesystem sandbox. Inventory memory scales with entry count; large-tree and sustained-watcher budgets are R5 work. Warning-bearing inventory is partial and R2 must preserve warnings. Snapshot storage, content classification and all ongoing observation are deferred. Reserved editor/log/debug/mouse/debounce/diff flags have no downstream effect in R1. Independent human review, signed distribution, live TUI tests and release gates remain future work.

## Next developer: R2 / P3–P5

Start with `internal/app.Prepare`, `Prepared.Config`, `Prepared.Matcher`, `Prepared.Inventory`; retain `model.FileMeta` keys and `ignore.Filter.Match`. Register only accepted directories, filter runtime-created paths with the same Matcher and correctly type final symlinks. Implement fsnotify watch management and new-directory enrollment, bounded event normalization/debounce/coalescing and reconciliation, then bounded snapshots with stable start/reset baseline semantics. Do not convert the R1 inventory into an implicit content baseline. Preserve explicit `--scan` and keep UI out of core. Any policy changes require ADR and regression updates.
