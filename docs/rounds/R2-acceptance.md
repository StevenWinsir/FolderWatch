# R2 Acceptance — Watcher / Events / Baseline（P3–P5）

Status: **PASS — 本地集成验收**。远端 CI/PR 信息在观察实际结果后补记。

Date: 2026-10-01 至 2026-10-02（本机 CDT）

Owner / Review: 编码助手依据仓库所有者本轮授权实施；完成实现自评审、真实文件系统回归、race、资源与 CLI 验证。没有独立人工 reviewer，不冒充实际 VS Code GUI 或 Terminal/iTerm TUI 验收。

Start commit: `720ecdb06044145a0f9f8a15b0220a0b4be45113`（R1 checkpoint）

Branch: `feat/r2-watch-baseline`。交付 checkpoint：`r2-complete`，完成推送后用 `git rev-parse r2-complete^{commit}` 解析精确提交。

Repository: https://github.com/StevenWinsir/FolderWatch （private）

## Scope / Delivered

P3：独立 `watcher.Watcher` interface + fsnotify adapter，accepted directories 递归登记、新建/移入目录 enrollment、删除/重命名清理、identity/native WatchList 校准、root 丢失及目录限额的 fatal 处理、context/Close。raw events 与 errors 分离；容量不足变成明确 root invalidation。

P4：`eventnorm.Normalize` 与 canonical path、CHMOD-only hint；`debounce.Start` 单 owner / 单 ticker / bounded map / sorted batches；默认 trailing window 150ms、繁忙路径最多 4 倍窗口。慢消费者和队列溢出合并为 root reconciliation，不增加无限 timer/goroutine，也不把 fsnotify flags 误当最终语义变化。

P5：`snapshot.Store` 提供 SHA-256、不可变 owned refs、小文本内存、中型文本私有 temp cache、binary/size/budget 降级、稳定读取与取消、原子 baseline generations。`app.Session` 注册 watcher 后 fresh scan/capture，普通保存不改变 before，Reset 原子换代；取消/缺失/权限错误保留旧代，旧引用在成功 Reset 后失效。退出清理 owned cache。

应用/输入：`app.StartSession`、Events/Baseline/ReadBaseline/ResetBaseline/Close，`--watch` 文本/NDJSON 诊断与共享资源 flags/TOML。默认与 `--scan` 继续单次扫描。R1 path/ignore 基础复用；新增目录 identity 的缓存退役，不等于已有目录规则热更新。

## 验收映射

| 要求 | 自动化证据 | 结论 |
|---|---|---|
| create/write/remove/rename 与递归目录 | watcher/fsnotify_test.go，app/session_test.go | PASS |
| 新建/移入目录的立即内容与后续独立写入 | TestMovedTreeImmediateAndLaterContentsRemainObservable、TestMovedTreeNestedIgnoreAndSymlink | PASS |
| 同名目录删除重建/旧规则不复用 | TestDeletedRecreatedDirectoryGetsNewRegistration、ignore/lifecycle_test.go | PASS |
| atomic-save 目标路径不丢失 | TestSessionStableBaselineAndAtomicSave、watch_smoke.py | PASS |
| 同窗口去重、CHMOD hint、最大等待 | debounce/coalescer_test.go、eventnorm/normalize_test.go | PASS |
| 队列/消费者背压不静默丢最终状态 | TestPendingOverflowAndSlowConsumer、TestPublicationBackpressureIsExplicit | PASS |
| 普通写入不推进 baseline、恢复 hash | snapshot/store_test.go、app/session_test.go | PASS |
| reset success/rollback/取消/并发与旧 refs | snapshot/store_test.go、queued_test.go、app/session_test.go | PASS |
| 不可读文件不能导致部分 reset | TestUnreadableResetDoesNotSilentlyEraseBaseline | PASS |
| bounded memory/disk、private permissions | TestCaptureStorageBudgetsAndHashes | PASS |
| symlink/FIFO/二进制与巨大稀疏文件取消 | snapshot/unix_test.go、store_test.go | PASS |
| native 库本身不跟随链接，不仅是过滤输出 | TestNativeDirectoryWatchDoesNotFollowSymlinks | PASS（vendor 模式） |
| Start/reset/Close 多轮不持续增长 | TestConcurrentResetAndStopResourceCleanup（每次12 sessions×4 resets） | PASS |
| Ctrl+C / 错误输出 / 清理 | CLI tests、watch_smoke.py | PASS |
| R1 配置/扫描/ignore 行为 | 原全部 Go tests + 60 项 scan smoke | PASS |

同一 path 的后续独立窗口可以再次产生请求；最终 changed-file 状态唯一性在 R3 ChangeStore 验收，R2 没有假的语义列表。迁移窗口内允许 root/subtree invalidation 代替某个单文件原始通知，消费者必须实际 reconcile。

## 已执行的验证

环境：macOS 26.6.2（25G83）、darwin/arm64、Go 1.26.6、Git 2.54.0。以下不是 P12 性能基准，也没有宣称全部 OS 的运行支持。

| 命令 | 观察结果 |
|---|---|
| `go test -count=1 -coverprofile=coverage.out ./...` | exit 0，全部测试包通过 |
| `go test -race -count=1 ./...` | exit 0，无 race 报告 |
| `go test -race -count=10 -coverprofile=coverage.out ./...` | 最终源码全量10轮通过，含发布锁等待期间取消的确定性回归 |
| `go test -count=10 ./internal/watcher ./internal/debounce ./internal/snapshot ./internal/app` | 四包各10次均通过 |
| `make smoke` | 原 scan **60 checks / 0 skipped**；R2 watch **17 checks** 通过 |
| `make lint` | gofmt、go vet、vendor SHA-256/patch可逆/实际构建源校验通过 |
| `go build ./...`、`go mod verify` | exit 0；all modules verified |
| `GOOS=darwin GOARCH=arm64/amd64 go build ./...` | 两个架构分别编译通过 |
| `GOOS=linux GOARCH=amd64 go build ./...` | 编译通过 |
| `GOOS=windows GOARCH=amd64 go build ./...` | 编译通过，非 Windows 运行验收 |
| `go test ./internal/snapshot -run='^$' -fuzz=FuzzTextProbeChunkBoundaries -fuzztime=5s -parallel=2` | PASS，910724 次执行、38 个新增 interesting inputs |

最终10轮语句覆盖率：总计 **84.3%**；watcher 80.3%、eventnorm 100.0%、debounce 90.6%、snapshot 84.9%、app 72.6%、CLI 85.5%、config 85.1%、ignore 93.3%、scan 96.1%。main 由真实二进制 smoke 验证，不属于进程内单元覆盖；model 无可执行语句。coverage.out 和 Go fuzz cache 属本地验证产物，不作为用户数据或发布资产提交。

### 编辑器验证边界

补丁负对照：在完全隔离的临时目录中显式运行 `go test -mod=mod -count=1 -run='^TestNativeDirectoryWatchDoesNotFollowSymlinks$' ./internal/watcher`，原始 fsnotify v1.8.0 返回 exit 1，确实收到 symlink 外部目标文件的 CHMOD 事件。这是预期失败，用于证明原问题；正常 vendor 构建的同一测试通过。最终补丁还退役无独立 fd 的 skipped-link seen 状态，并测试符号链接删除/同名重建。

真实执行 `/usr/bin/vim -Nu NONE -n -es`，分别设置 `backupcopy=yes` 与 `backupcopy=no`，验证保存内容与持续 invalidation。另以隔离 fixture 验证直接覆盖、连续30次写入、临时文件 rename/replace、删除/新增及含空格/Unicode路径。

本机没有发现 VS Code 可执行文件或应用，故只验证对应直接写入/atomic replacement 保存模型。实际 VS Code GUI、TextEdit、JetBrains、Terminal.app/iTerm2 交互未执行，不记录为已通过；后续环境验收继续补齐。

## 实现过程中发现的问题与修复

首次测试真实暴露空 native path 和目录迁移窗口问题，之后补回归并复跑。kqueue 内部可能跟随子项链接，因此仅在 application filter 中拒绝路径不足以保证边界；vendor patch 在 native 层阻止这种行为，并把不可识别描述符和目录变动变成明确的恢复请求。目录 identity/native watch 状态校准与 Ignore scope 生命周期避免同名重建保留旧状态。

快照增加有界读/whole-stream storage probe、可取消 ownership 等待、取消失败回滚（含等待发布锁时再次检查取消）、基线引用所有权、临时目录权限与清理；Reset 发布与事件 generation 串行化。仅修复报告过的断言，不删除测试来假装一次通过。

## 依赖变动与可复现性

没有升级第三方版本。标准 vendor 包含 go.mod/go.sum 已锁定图及各自许可证；fsnotify v1.8.0 的 kqueue 后端有 125 行 unified patch，详见 ADR-009、`patches/fsnotify-v1.8.0.patch` 与 JSON SHA-256 manifest。`scripts/vendor_guard.py` 验证当前构建实际使用 vendor，不允许误用未打补丁的 module cache。

重新生成：`go mod vendor` → `git apply patches/fsnotify-v1.8.0.patch` → `make lint test race smoke`。不要单独 vendor 后提交覆盖补丁，也不要把外部 `go install ...@version` 视为相同构建。

## Known Issues / Accepted Limits

| ID / Owner | 边界与后续动作 |
|---|---|
| R2-L1 / core | watch 启动和 Reset 要求完整可读基线；失败不发布部分结果。后续若允许 partial baseline，必须增加 unknown-path 状态及 ADR，不能静默跳过。 |
| R2-L2 / R3–R5 | 无 Ignore 热更新；已有目录规则更改需新 session，目录删除/替换是 scope 生命周期，不是热更新。 |
| R2-L3 / R5 | 强杀可能残留私有 cache；没有自动过期清扫。正常 Close 清理已验证。 |
| R2-L4 / R5 | inventory 内存随条目数增长；kqueue 描述符随文件数增长，OS资源错误显式上报。未执行小时级运行或P12 CPU/内存/P95预算。 |
| R2-L5 / QA | 实际 VS Code GUI/Terminal应用交互/独立人工评审未执行；保存模型和Vim证据不替代它们。 |
| R2-L6 / core | Unix path保护不是原子恶意文件系统沙箱；Windows仅编译验证。Reset不是整个FS同一时刻快照，捕获窗口内事件需reconcile。 |

没有在已测 R2 范围内发现未解决的阻塞缺陷。上述边界是明确接受/延后项，不应被解读为没有资源、安全或兼容性限制。

## 下一轮依赖 / 禁止假设

R3 可依赖 Config、Matcher、Watcher、path requests、Session generation、Snapshot Ref 与 baseline/reset。必须处理 root reconciliation、过期引用、错误/取消、无保留内容的 ref，以及不同窗口重复路径请求。实现 Classifier → Diff → 唯一 ChangeStore，并验证恢复基线、Added→Deleted、Deleted→Recreated 迁移。

R2 未实现文本 diff、Changed List、rename语义识别、Pause/Resume、TUI、交互reset、日志/编辑器或GUI。Gate A/B仍未通过。不要将 storage probe 当成正式 Classifier，也不要直接把 raw event 显示成 Added/Deleted。

## GitHub 集成记录

本地验收已完成；提交/PR/远端 CI 的精确链接与结果将在观察后补入本节。CI 配置存在不作为远端成功证据。
