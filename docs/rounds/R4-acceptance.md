# R4 Acceptance — Terminal/TUI 产品体验（P9–P11）

Status: **实现与本地自动化验收 PASS；Round 集成状态 IN_REVIEW**。独立人工评审、PR 合并与 Gate A 不由本地测试代替。GitHub 的实际提交/PR/CI 结果在末节补记，不预告成功。

Date: 2026-10-02

Owner / Review: 编码助手按仓库所有者授权实施；完成源码/边界自评审、模型/黄金/真实文件系统/CLI/PTY/race 回归。没有声称实际 Terminal.app、iTerm2、VS Code GUI 人工验收或独立人工 reviewer 已签字。

Repository: https://github.com/StevenWinsir/FolderWatch

Branch: `feat/r4-terminal-tui` → `main`

Integration base: `1b52fcc23d199f6962206205e585c1d695cc0bae`。R4 开始先完整阅读 Handoff、核对工作区干净，实查 [R3 PR #2](https://github.com/StevenWinsir/FolderWatch/pull/2) 已于 **2026-10-02 07:08:58 UTC** 合并；merge 树与 R3 head `c0f626b63735fba6ae43feaeb0c860089ffad0ea` 相同。从已合并 main 建立新分支，不夹带用户改动、不强推共享历史。旧“R3 待合并”在 Handoff/README/R3 acceptance 原文修正。

## 本轮实际交付

| 阶段 | 实现 | 主要位置 |
|---|---|---|
| P9 | 变化列表、A/M/D/R 与独立展开标识、数量/状态、键盘/鼠标、过滤、滚动、resize、帮助 | internal/tui/model.go、view.go、run.go |
| P10 | 按需单请求可取消 unified Diff；旧请求隔离、有限 stale 重试；hunk/双侧行号/末尾换行、红绿与 +/-、友好状态、安全 Unicode 裁切、有界预览 | internal/tui/model.go、diff.go、view.go |
| P11 | Session Pause/Resume/Status、暂停冻结且 watcher 存活、恢复全 root 校准、确认式原子 Reset、warning/fatal、清理与退出码 | internal/app/control.go、session.go、semantic.go；internal/tui |
| P11 日志 | 200 条元数据 ring、显式新建 root 外0600 JSONL、4MiB文件上限、无正文/无TUI串流污染、路径/大小写身份别名防护 | internal/logging；internal/cli/watch.go、tui.go |
| CLI/交接 | TTY 默认 TUI、管道默认单次扫描、--tui 明确要求终端；完整四组 smoke/CI 接续；原 Handoff 就地更新和新增第28节、README、配置例、CHANGELOG、ADR-013 | internal/cli、scripts、Makefile、.github/workflows、docs |

UI 没有第二套 watcher/snapshot/classifier/diff/ChangeStore，也没有从 OS flags 推导变化。Reset 不由 UI 自行清表，完成后取新权威 View。R 标记可展示，但 core 的 rename 仍是可靠 Deleted+Added fallback，不宣称新增 rename 关联。

## 验收映射

| 要求 | 具体证据 | 结果 |
|---|---|---|
| 0/1/100+ list、不越界 | model 0/1/120；0/极小/正常/宽屏尺寸矩阵；PTY 112 文件 | PASS |
| 纯键盘全部主流程 | ↑↓/j/k、Enter/Space、p/r/?/q、过滤取消/应用、分页/横向与帮助 | PASS |
| 鼠标增强/可禁用 | model click/wheel；PTY SGR 实际鼠标包；--no-mouse 关闭报告且忽略残留输入 | PASS |
| 1500 行 Diff 可滚动 | 模型与真实二进制 PTY 页滚动/跳尾/resize | PASS |
| 旧文件/版本/代际结果不能覆盖 | single-flight、取消 epoch、path/gen/version mismatch、旧 View 水位、Reset 清理、最多3次重试 | PASS |
| 文本渲染和降级 | unified golden、双侧行号/无末尾换行、Binary/Unsupported/TooLarge/Unavailable、empty text | PASS |
| 终端注入和宽字符 | 控制/ANSI/bidi 转义、grapheme cell clipping、NO_COLOR、预览行数/字节上限 | PASS |
| Pause 与同基线 Resume | 暂停期间100次写入、恢复/删除/新嵌套目录及其后新增文件；暂停状态精确不变 | PASS |
| Reset 原子语义 | 确认/取消、失败保留、暂停中Reset、新before读取、并发控制/Close | PASS |
| 可恢复与致命错误 | mode-000 文件真实 warning/继续；暂停时 root 丢失仍 fatal；startup cancel | PASS |
| 退出/清理 | q=0、Ctrl+C=130、root fatal=1；私有cache删除、终端模式/光标/alt screen恢复 | PASS |
| 安全日志 | ring边界/并发/防御副本、0600/O_EXCL、root/symlink/大小写别名、4MiB停止、JSONL不含正文 | PASS |
| CLI兼容/边界 | 非TTY默认scan、--tui无TTY报错2、组合冲突、--watch NDJSON日志不混流，原扫描/监听/语义回归保留 | PASS |
| Core分层与依赖 | 原AST边界回归、无新版本依赖、vendor SHA/可逆性/构建源guard | PASS |

## 最终本地测试

Environment: **macOS 26.6.2 / darwin-arm64 / Apple M4 / Go 1.26.6**。

| 命令/验证 | 实测结果 |
|---|---|
| `make fmt`、`go mod tidy -diff`、`go mod verify` | PASS；all modules verified |
| `make build test race lint smoke` | PASS；包含 gofmt/go vet/vendor guard 与全部四组 smoke |
| `go test -json -count=1 ./...` | **280 个测试/子测试 PASS**，125 个顶层测试/模糊目标；失败0、测试级跳过0 |
| `go test -race -count=10 -coverprofile=coverage.out ./...` | **全包连续10轮 PASS** |
| R1 `scripts/smoke.py` | **60 checks，0 skipped**；包含更新后的TTY/管道帮助契约 |
| R2 `scripts/watch_smoke.py` | **17 checks**；实际 headless Vim backupcopy=yes/no 保存继续 PASS |
| R3 `scripts/semantic_smoke.py` | **16 checks**；创建/恢复/删除/rename降级/分类/清理 |
| R4 `scripts/tui_smoke.py` | **21 checks**；真实二进制 PTY，非 Terminal/iTerm2 人工 QA |
| darwin/arm64、darwin/amd64、linux/amd64、windows/amd64 `go build ./...` | 全部 PASS；跨编译不是 Windows runtime 支持 |

整体语句覆盖率 **85.6%**，app **83.3%**、TUI **84.2%**、logging **86.4%**、CLI **82.9%**。真实二进制 PTY/CLI 进程不计入 Go 进程内覆盖率；上述280包括父测试与子测试，不是280个互不重叠场景。JSON 证据在本机被忽略的 `artifacts/validation/r4-tests.jsonl`，coverage.out 同为验证产物，不作为业务文件提交。

孤立渲染微基准 `go test -run='^$' -bench='^BenchmarkVisibleDiffRendering$' -benchtime=1s -benchmem ./internal/tui`：1500行预览、120×40视口，**55,925 ns/op、5,162 B/op、136 allocs/op**（20,485次）。只测选定可见帧 View，不包括 FS、Diff、终端输出延迟或复杂目录；**不是 P12/P95/内存/长期稳定性 Gate**。

## 本轮验证中发现与修正

1. 全量回归暴露 Close 与控制命令并发时，native watcher 可能早于 AfterFunc 取消而关闭，向调用方泄漏 `watcher closed`。责任修复在 app：错误统一为 ErrSessionClosed，关闭过程不误报 fatal，已提交 Reset 的成功不变为失败；保留并发控制/停止回归，并经全包10轮race复跑。
2. Diff重试展示错误后若不清空预览哨兵，会阻止真正的新请求。已修复 retry handler，确定性测试验证总计4次尝试上限和实际重发；没有去掉版本校验。
3. `--no-mouse` 不应仅“不启用报告”；现在主动禁用并在模型层拒绝残留报告，键盘仍可用。日志 root fence 同时检查目录身份，防止 macOS大小写别名导致反馈。
4. 旧 R1 smoke 把“任何默认都单次扫描”的帮助文字当不变契约。R4 的批准入口变为 TTY默认TUI/管道默认scan：更新该检查，同时保留实际非TTY扫描、显式scan、所有原边界断言和新PTY默认启动测试，不删除回归以掩盖行为。
5. PTY驱动原先把 `/query` 一次发送成粘贴文本，未先进入过滤模式；改为先发送 `/`，再发送查询，符合实际键盘/粘贴消息边界，不让任意粘贴字符串被逐字符执行为q/r等命令。macOS恢复canonical mode后内核设置PENDIN，测试仅掩蔽这个内核位，其余flags、控制字符、速率、光标和alt screen严格检查。

初期失败如实保留为开发事实；最终实现、测试、依赖及脚本均修改后实际重跑。没有修改 fsnotify vendor 补丁或升级依赖来规避问题。

## 行为边界 / R5责任

| ID | 范围 | 已知边界与下一轮责任 |
|---|---|---|
| R4-L1 | 发布/人工QA | 未做实际 Terminal.app/iTerm2 人工视觉验收、clean-machine安装、打包/签名/分发；R5 P14 / Gate A负责。 |
| R4-L2 | 性能 | 未完成10k文件/小时级监控/系统CPU与峰值内存/P95指标；有界队列/单Diff/预览上限不等于进程总资源上限，R5 P12负责。 |
| R4-L3 | UI | 当前只保留选中文件的一份unified预览，不做多文件全文常驻/side-by-side；阈值内也可能受core工作预算降级。长行/预览截断都有提示。 |
| R4-L4 | 文件系统 | 继承严格完整基线、运行时last-known保护、无Ignore热更新/强杀缓存自动清扫/可靠rename关联；不声称原子对抗恶意并发目录替换。 |
| R4-L5 | 日志 | 显式日志必须是root外新文件；4MiB后停止写文件而非自动轮转。目录/日志I/O不是硬实时保证。默认仅ring，无正文。 |
| R4-L6 | 后续功能 | editor只存储不执行；GUI、Wails、Svelte、签名发布和Gate B未开始。Gate A未通过不得开发GUI。 |

本轮自动化范围内没有未解决的实现 blocker；PR review/merge和R5出口仍需分别完成。路线不借本轮自动化代签。

## GitHub 交付 / 集成

此节在实际推送、创建 PR 并查询 Actions 后补充精确 SHA、URL、结果。R4 不直接改写 main，不强推，不自动合并；尚未创建 `r4-complete`，集成前不把标签名称当已交付事实。
