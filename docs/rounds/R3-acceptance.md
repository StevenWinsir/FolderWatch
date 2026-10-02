# R3 Acceptance — Classifier / Diff / ChangeStore（P6–P8）

Status: **实现与本地自动化验收 PASS；Round 集成状态 IN_REVIEW**。远端 CI、PR 与最终提交仅在实际观察后补记。

Date: 2026-10-02（本机 CDT）

Owner / Review: 编码助手依据仓库所有者本轮授权实施；实现自评审、结构/边界复核、单元/集成/黄金/race/fuzz/真实 CLI 验证。未声称独立人工 reviewer、实际 VS Code GUI 或 TUI 人工验收。

Initial source: `32529a7dad7014c5cb54f2aff8791a8969e66bc7`（R2 最终修复）。

Integration base: `1dbdb5bfdb99f1f1e2f04a6f366e3e78fc640627`。R3 期间核对 [R2 PR #1](https://github.com/StevenWinsir/FolderWatch/pull/1) 已于 2026-10-02 06:00:44 UTC 合并；其完整源码树与 32529a7 相同。本分支 fast-forward 同步该合并历史，保留所有 R3 修改和测试证据。

Branch: `feat/r3-classify-diff-changes`。最终 checkpoint 使用 `r3-complete`；以交付补记确认的提交/推送为准，不将名称预告当成已创建标签。

Repository: https://github.com/StevenWinsir/FolderWatch （private）

## Delivered

**P6**：`internal/filetype` 的 bounded Classifier / shared Probe；UTF-8、Binary、UTF-16/32 UnsupportedText、TooLarge 及非 regular Unsupported；Ref 保存不可变分类，替换 R2 的私有探针；oversize 分类零读取，snapshot 哈希仍有界流式；分类/保留与 Diff 字节上限独立。

**P7**：`internal/diff.Engine` / BoundedLCS；公共首尾裁剪、行 ID、显式矩阵预算及循环取消；UI 无关 hunk/行号/Added/Removed/Context/NoNewline，plain Unified；九个黄金文件与 reconstruction fuzz。字节、行数、计算预算超限结构化降级，不遗留不可取消 goroutine。不引入新的第三方依赖。

**P8**：单一 `changes.Store`，可取消串行 resolve/reset、短锁发布、排序防御副本、baseline-relative 状态迁移、rename 删除+新增降级、部分扫描/瞬时读取错误保护、版本化 Batch、独立按需 Diff 令牌与 scratch、原子 Reset 清表与 stale 防护。

**集成**：Session Changes/ChangeState/GetDiff；启动窗口及失败退避重查；CLI A/M/D / semantic NDJSON、reload state / 水位抑制旧批次；默认扫描保持只读元数据。R1/R2 全部回归继续运行。

## Acceptance map

| 要求 | 证据 | 结果 |
|---|---|---|
| 扩展名无关分类、UTF-8/Binary/Unsupported/TooLarge | classifier matrix、BOM/控制字节、零读取 oversized、chunk invariance | PASS |
| 有界读取与读取中消失处理 | existing safe snapshot tests、size mismatch、cancel、transient recovery | PASS |
| 统一 diff 结构与 newline/Unicode/hunk | 9 golden files、10k-line small edit、status/limit tests | PASS |
| 计算中取消、无遗留计算 | deterministic cancellation inside matrix、queued diff cancellation | PASS |
| Added/Modified/Deleted 与恢复消失 | state transition tests、live Session、semantic CLI smoke | PASS |
| Added→Deleted、Deleted→same/different recreation | store + real FS/CLI cases | PASS |
| 每 path 唯一，排序、副本、批次、时间/版本 | copy isolation、duplicate/chmod suppression、capacity tests | PASS |
| 原基线不会因普通保存推进 | before content across saves、restore/reset tests | PASS |
| 不可读子树不 mass-delete、忽略不误删 | Unix permissions、transient capture、excluded-present recovery | PASS |
| Diff 的旧 path version / generation 拒绝 | gated engine + concurrent Resolve/Reset, ErrStale assertions | PASS |
| Slow consumer / backpressure | event queue saturation、Reload + current View、CLI watermark | PASS |
| 不跨 root 混用对象 | Config/Matcher/Store namespace regression tests | PASS |
| 无文本内容泄露、链接/FIFO 不乱读 | snapshot/native R2 tests、CLI escaped paths / no contents / cleanup | PASS |
| rename 无虚假关联 | deterministic Deleted(old)+Added(new) tests | PASS（降级，不是 Renamed 识别） |
| Core 不依赖 TUI/GUI/CLI adapter | AST import boundary regression, expanded to new packages | PASS |

## Final local validation

Environment: macOS 26.6.2 / darwin-arm64 / Go 1.26.6。本机 Git 为 2.54.0。不是 P12 性能报告，也不是 GUI/Windows 运行支持声明。

| Command | Observed result |
|---|---|
| `make fmt` | 完成；最终 gofmt 检查通过 |
| `make build test race lint smoke` | PASS；含 vendor SHA/source/reversibility guard、go vet |
| `go test -json -count=1 ./...` | **248 个 Go 测试/子测试通过**，101 个顶层测试/模糊目标；失败0，测试级跳过0 |
| `go test -race -count=10 -coverprofile=coverage.out ./...` | **全包10轮PASS** |
| `scripts/smoke.py`（make smoke） | R1 扫描 **60 checks，0 skipped** |
| `scripts/watch_smoke.py`（make smoke） | R2 监听 **17 checks**；实际 Vim backupcopy=yes/no 保存通过 |
| `scripts/semantic_smoke.py`（make smoke） | R3 语义 **16 checks**，状态恢复/分类/rename/退出与清理 |
| `GOOS=darwin GOARCH=arm64 go build ./...` | PASS |
| `GOOS=darwin GOARCH=amd64 go build ./...` | PASS |
| `GOOS=linux GOARCH=amd64 go build ./...` | PASS |
| `GOOS=windows GOARCH=amd64 go build ./...` | PASS（compile only） |
| `go mod verify` | all modules verified |

Coverage（上述全量10轮带覆盖率）：总 **86.0%**；filetype 93.5%、diff 94.2%、changes 84.7%、app 80.6%、CLI 86.6%、config 86.2%、snapshot 84.2%、watcher 80.5%。cmd/main 由真实二进制 smoke 覆盖，不计入进程内 main 函数覆盖；model 没有可执行逻辑。测试父项与子项计数均包含在248内，不是248个互不重叠的场景。

### Bounded fuzz evidence

- `go test ./internal/filetype -run='^$' -fuzz=FuzzClassifier -fuzztime=5s -parallel=2`：PASS，**1,061,660** 次执行，31 个新增 interesting inputs。
- `go test ./internal/diff -run='^$' -fuzz=FuzzDiffReconstruct -fuzztime=5s -parallel=2`：PASS，**911,178** 次执行，52 个新增 interesting inputs。

不是穷尽证明。测试数据仅生成于隔离临时目录/Go fuzz cache；本地 JSON 证据 `artifacts/validation/r3-tests.jsonl` 和 coverage.out 是被忽略的验证产物，不提交为业务数据。

## Behavior decisions / review corrections

分类结果与 captured bytes 绑定，不事后重新判别 before。默认 max_snapshot_bytes=8MiB、max_diff_bytes=5MiB、max_diff_lines=20000，均有独立配置校验。Oversized 文件哈希仍读完整流，但不做文本探测或按文件大小分配内存；snapshot probe 不能绕开 classification。

状态只比较 baseline/current；重复同内容和纯 chmod 不推进版本。恢复内容、Added→Deleted 自动移除。目录不作为 changed-file row；链接变更只比链接字符串。Unreadable / unstable 保护最后已知状态并发 warning；R3 不把 unknown 冒充 unchanged。失败重查只有一个退避计时器，空闲成功后不轮询。

审查补强：不同 root 的 Prepared/Matcher/Baseline 拒绝混用；目录被文件/链接替换后，旧子文件可安全返回 deleted Diff，不访问链接外部；被过滤但实际存在的路径不误判删除；排队 Diff 可取消，旧结果不能越过 Reset/新的 path version；Reload 取得更晚版本时，CLI 水位拒绝排队旧 delta；Current snapshots 每次及时删除，不随保存次数增长。

Architecture: [ADR-010](../adr/010-classification-and-safe-content.md)、[ADR-011](../adr/011-bounded-cancellable-diff.md)、[ADR-012](../adr/012-semantic-store-and-versioned-events.md)。R2 fsnotify vendor patch 原样保留并校验，未重新下载升级依赖来掩盖问题。

## Known limits / deferred

| ID | 范围 / 优先级 | 边界、处理方式、后续责任 |
|---|---|---|
| R3-L1 | 性能 / accepted | 有界 LCS 对大跨度密集修改可因工作预算 TooLarge；保留变化条目及原因。R5 测量后再考虑 Myers/patience/缓存。 |
| R3-L2 | 内容 / accepted | 基线内容因分类/预算未保留时 GetDiff=Unavailable，绝不用当前内容充当旧内容。可调整预算并重启/Reset。 |
| R3-L3 | 文件系统 / accepted | 严格完整启动/Reset 基线；运行时错误保留最后已知状态并警告重试。不是跨文件系统原子快照或对抗恶意并发目录替换的沙箱。 |
| R3-L4 | 资源 / accepted | 三个私有目录；Reset最多两代保留预算加一个Diff当前快照。清单、矩阵、返回副本和kqueue描述符另计；R5未完成长时/P12指标。 |
| R3-L5 | 功能 / deferred | 无Ignore热更新、强杀缓存自动清扫、可靠rename关联或常驻Diff缓存。R4没有义务用UI绕过这些核心策略。 |
| R3-L6 | 产品 / deferred | TUI/键鼠/交互Diff与Reset/Pause/日志/编辑器/GUI尚未开始。实际VS Code GUI及Terminal/iTerm人工验收未宣称通过。 |

本轮已执行测试范围内没有未解决的实现 blocker。Round 主分支集成/独立评审与自动化验收是不同状态，详见下节。

## GitHub delivery / integration

本地实现与验收已完成；R3 提交、分支推送、实际远端 CI、PR 与 checkpoint 的证据将在观察后原文补记。未观察到的运行不提前写 PASS。

## Next developer — R4 / P9–P11

使用 Session.Changes/ChangeState/GetDiff 和语义 Batch，不从 Paths/fsnotify flags 派生状态。Reload 取 View，以 generation/version 拒绝旧消息；Removed 意为不再变化。Diff 按需且可取消，处理 ErrNotChanged/ErrStale、Binary/Unsupported/TooLarge/Unavailable。Reset 只调用 core 原子API。保留 R1–R3 tests、三组 CLI smoke 和 vendor guard。确认 R3 PR/集成状态后再接 TUI；Gate A 未通过，不进入 GUI。
