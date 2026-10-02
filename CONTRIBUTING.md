# Contributing

Read `Handoff_Rounds.md`, then `docs/rounds/R2-acceptance.md` and the ADRs. Go 1.23+ and Python 3 (CLI smoke tests) are required. Run `go mod download`, `make build`, `make test`, `make race`, `make lint`, `make smoke`.

Use small `feat/<topic>`, `fix/<topic>` or `docs/<topic>` branches and PR review for subsequent work. Commit convention: `feat(scope): ...`, `fix(scope): ...`, `test(scope): ...`, `docs(scope): ...`, `chore(scope): ...`. Initial R1 bootstrap may land directly on the empty repository's main branch after its implementation review and local acceptance. Never force-push shared history.

Build from the vendored checkout. fsnotify's kqueue patch is documented in ADR-009 and guarded by `make lint`. After deliberate `go mod vendor` regeneration, reapply `git apply patches/fsnotify-v1.8.0.patch` and rerun tests/race/smoke. Do not use `-mod=mod` for normal builds or omit dependency licenses. The pinned upstream negative-control test is an explicit exception used only to reproduce the original bug in isolated fixtures.

Keep main.go thin; core must not import UI adapters. Fixes need regression tests. New dependencies need pinned versions, rationale and license review. Commit go.sum; do not commit build artifacts, credentials or monitored content. Update the existing Handoff sections in place and add a dated Round record. Do not mark remote CI, independent human review, Terminal/iTerm interaction or release gates passed without evidence. R2–R5 must complete before GUI work starts.
