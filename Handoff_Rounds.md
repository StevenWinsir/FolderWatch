# FolderWatch — Handoff_Rounds.md

> 项目代号：**FolderWatch**
> 文档用途：工程实施、多人接力开发、代码评审、测试与发布交接  
> 目标平台：**macOS 优先**，架构保留 Windows / Linux 扩展能力  
> 核心技术栈：**Go + fsnotify + Bubble Tea + Lip Gloss**  
> GUI 技术栈：**Wails + Svelte + TypeScript + Monaco Diff Editor**  
> 开发阶段：**阶段一 Terminal/TUI → 质量闸门 → 阶段二 GUI**
>
> **当前工程状态（2026-10-02）：R8 / P19–P21 的实现与本地自动化验收完成，集成状态为 IN_REVIEW。** 已补齐 GUI Settings、Pause/Resume/Reset/Change folder、外部编辑器/Finder/剪贴板系统集成、sleep/wake 续租与刷新、主题、键盘可访问性、错误态和生命周期清理；原 P0–P18 与 Gate A 仍为既有验收结果。R8 代码尚未合并；本轮 `.app` 是开发构建，不是签名/公证或正式 GUI 发布。
> 本轮完成范围、验收数字、修复与限制见 **第32节**、[`docs/rounds/R8-acceptance.md`](docs/rounds/R8-acceptance.md)、第31节的 R7 记录及 [`docs/gui-ipc-v1.md`](docs/gui-ipc-v1.md)。第25–31节保留 R1–R7 历史证据；Gate A 人工签字仍以 [`docs/gates/Gate-A.md`](docs/gates/Gate-A.md) 的既有候选为准。原生 WKWebView 按钮/菜单人工交互尚未复验：本次机器未授予辅助功能和屏幕录制权限，浏览器真实 IPC 测试不替代该项。PR、远端 CI 与合并只记录实际结果。

---

## 1. 项目目标

FolderWatch 是一个本地文件夹变更监控工具。用户选择或传入一个目录后，程序建立“启动时基线（baseline）”，持续监听该目录及其子目录中的文件变化，并在终端或 GUI 中实时显示变化文件列表；对于文本文件，可展开查看类似 GitHub / `git diff` 的增删改内容；对于二进制或超大文件，至少显示文件发生了变化以及元信息变化。

### 1.1 核心用户故事

1. 用户执行 `folderwatch .` 后，程序递归扫描当前目录并建立 baseline。
2. 编辑 `A.txt`、`B.go`、`C.py` 中任意文件，TUI 在一次合理的 debounce 后显示变化文件。
3. 文件列表至少区分：新增、修改、删除；重命名采用 best-effort 识别，无法可靠识别时允许降级为“删除 + 新增”。
4. 用户选中文件后按 Enter/Space，或使用鼠标点击展开控件，看到行级 diff：新增为绿色、删除为红色、上下文保持普通颜色。
5. 用户可以将当前状态重置为新的 baseline，变化列表随之清空。
6. 程序不要求目录是 Git 仓库。
7. 任意扩展名文件都必须能被“检测到变化”；只有可安全解释为文本的文件才进行文本 diff。
8. 阶段二 GUI 必须复用阶段一的 Go core，不允许重新实现第二套 watcher / snapshot / diff 逻辑。

### 1.2 v1 非目标

以下内容不应阻塞 v1：

- 不实现 Git commit / stage / push / branch 等版本控制功能。
- 不承诺任意二进制格式的语义级 diff，例如图片像素 diff、PDF 页面 diff、Office 文档内容 diff。
- 不做云同步、账户系统、团队协作。
- 不做远程文件系统同步。
- 不默认跟随目录符号链接，避免循环和越界访问。
- 不在阶段一实现 GUI。
- 不在阶段二重写 core。
- 不在 v1 实现编辑器内嵌修改/保存；GUI diff 默认只读。

---

## 2. 两大开发阶段与硬性 Gate

### 阶段一：Terminal/TUI

范围：P0–P14。

交付物：可安装、可运行、可测试的 `folderwatch` CLI/TUI 二进制；支持递归目录监控、变化聚合、baseline、文本 diff、二进制安全处理、TUI 展示、基础配置、日志、自动化测试和 macOS 发布。

### Gate A：阶段一完成闸门

**P14 未签字通过之前，不允许开始 GUI 功能开发。**

Gate A 最低要求：

- `go test ./...` 全绿。
- `go test -race ./...` 全绿。
- 核心事件集成测试稳定通过：创建、编辑、连续保存、atomic save、删除、新建子目录、子目录内文件变化。
- TUI 变化列表和 diff 展示可人工验收。
- 大文件、二进制文件不会导致崩溃或明显卡死。
- 停止监控后无明显 goroutine 泄漏。
- 至少在 macOS Terminal.app 与 iTerm2 中人工测试。
- 已产出一个可发布的命令行二进制。
- Core API 已从 TUI 中解耦。

### 阶段二：GUI

范围：P15–P25。

前置条件：Gate A 通过。

交付物：可安装的 macOS `.app`，使用 Wails + Svelte 调用同一 Go core，实现目录选择、监控状态、变化列表、GitHub 风格 diff、暂停/继续、重置 baseline、设置、打开编辑器/访达等功能。


### 2.1 开发轮次（Round）总览

P0–P25 继续作为能力阶段编号；**Round 才是实际开发、Code Review、QA 与人员交接的最小管理单元**。每个 Round 可以包含多个 PR，但只有完成本轮集成验收并记录结果后，才能标记为 `PASS`。

| Round | 大阶段 | 包含 P | 单元主题 | 核心结果 | 出口 |
|---|---|---:|---|---|---|
| R1 | Terminal | P0–P2 | 工程基础与输入边界 | 工程骨架、CLI/Config、扫描与 Ignore | **PASS（2026-10-01，本地 + 远端 CI）**，可稳定确定应监控集合 |
| R2 | Terminal | P3–P5 | 文件事件与 Baseline 内核 | Watcher、Debounce/Coalesce、Snapshot | **PASS，PR #1 已合并（2026-10-02）** |
| R3 | Terminal | P6–P8 | 内容分析与变更语义 | Classifier、Diff、ChangeStore | **PASS，PR #2 已合并（2026-10-02）**，可获取语义列表与 GetDiff |
| R4 | Terminal | P9–P11 | TUI 产品体验 | 文件列表、Diff Viewer、Session 控制与日志 | **PASS；主PR #3与测试同步补充PR #4均已合并**；历史证据见§28 |
| R5 | Terminal | P12–P14 | 工程硬化与发布 | 性能、测试、CI、Terminal 发布 | **PASS（2026-10-02）；Gate A PASS**，阶段一验收完成，可进入R6；集成状态见§29 |
| R6 | GUI | P15–P16 | GUI 壳层与 IPC 契约 | Wails/Svelte、Core Facade、DTO/Event | **实现/本地自动化 PASS；集成 IN_REVIEW**；真实 IPC、资源/安全回归通过，原生交互与 PR 评审边界见§30 |
| R7 | GUI | P17–P18 | GUI 主流程与 Diff | Folder Picker、变化列表、Diff Viewer | **实现/本地自动化 PASS；集成 IN_REVIEW** |
| R8 | GUI | P19–P21 | GUI 功能补全与系统鲁棒性 | Settings、系统集成、可访问性 | **实现/本地自动化 PASS；集成 IN_REVIEW（2026-10-02）** |
| R9 | GUI | P22–P24 | 测试、打包与发布 | E2E、签名/公证、最终 QA | **Gate B PASS** |
| R10 | v1 收尾 | P25 | 冻结与长期维护交接 | 文档、兼容性矩阵、backlog | v1 可长期维护 |

### 2.2 Round 通用管理规则

1. **上一 Round 的阻塞项关闭后再进入下一轮。** 可接受的遗留问题必须记录在 `Known Issues`。
2. **一个 Round = 一个可验收开发单元。** 可以拆多个 PR，但必须有 Round 级集成验收。
3. **不得用下一轮代码掩盖本轮缺陷。** 例如 R3 的 ChangeStore 问题不能通过 R4 TUI 特判解决。
4. **核心接口优先于 UI。** UI 可以使用 mock 等待依赖，但禁止复制 watcher、baseline、diff、ChangeStore。
5. **每轮建议打 checkpoint tag**：`r1-complete`、`r2-complete`……正式发行仍使用 SemVer。
6. **每轮提交验收记录**：`docs/rounds/RX-acceptance.md`。
7. 后续 Round 发现前轮核心 bug，必须在责任 package 增加 regression test。
8. **R5 Gate A 不通过，不进入 R6。R9 Gate B 不通过，不发布 GUI v1。**
9. Round 内允许并行开发；跨 Round 只能依赖已经验收的接口或 mock。
10. 交接以代码、ADR、测试、Gate/Acceptance 文件为准，不依赖口头信息。

---

### 2.3 R1 — 工程基础与输入边界（P0–P2）

**当前状态：PASS（2026-10-01）**。已完成实现、自评审和自动化集成验收；未将独立人工评审或未来的 Terminal/iTerm 交互验收记为已完成。`--scan` / `--json` 为 R1 可复现入口；R2/P3–P5 已在下节继续交付。

**目标**：固定项目骨架、CLI/Config、路径与 ignore 语义，让后续 watcher 接收稳定输入。

**包含**：
- P0 项目初始化、工程规范、ADR
- P1 CLI 契约与配置模型
- P2 初始目录扫描、路径规范化与 Ignore Engine

**本轮交付**：
- 可编译 Go 工程与基础 CI
- Config schema 与参数校验
- path normalize 规则
- Ignore Engine
- 初始递归扫描
- P0 ADR 与单元测试

**R1 验收**：
- [x] `go build ./...` 成功
- [x] `folderwatch --help` 可用
- [x] 非法 path/duration/size 返回明确错误
- [x] 嵌套目录、空格、Unicode 扫描正确
- [x] `.folderwatchignore` 与 CLI ignore 生效
- [x] symlink 行为符合 ADR-005，覆盖目录环、外部链接和失效链接
- [x] 权限不足目录产生 warning，继续扫描其他项
- [x] initial scan 与同一 runtime ignore matcher 有自动化一致性测试
- [x] `main.go` 仅组装 CLI/context/版本，不包含 watcher/diff 业务

最终本地验证：127 个 Go 测试/子测试通过，`go test -race -count=1 ./...` 通过，真实二进制 CLI smoke 60 项通过、0 跳过，build/lint/dependency verify 通过。完整命令、环境、覆盖率与远端 CI 证据归入 R1 acceptance。

**出口定义**：系统能够可靠回答“给定 root + config，本次 session 应关注哪些文件与目录？”

---

### 2.4 R2 — 文件事件与 Baseline 内核（P3–P5）

**当前状态：PASS（实现及自动化验收通过，PR #1 已合并，2026-10-02）**。R3 期间核对远端：R2 最终 `32529a7` 已于 2026-10-02 06:00:44 UTC 合并为 `1dbdb5b`。R2 自评审/自动化验证不冒充独立人工评审，实际 VS Code GUI 仍未验收。R2 当时未提前开发语义变化；本轮 R3 实现见下节。

**目标**：完成不依赖 UI 的稳定监听核心，解决递归 watcher、编辑器噪音事件和 baseline 生命周期。

**包含**：
- P3 Watcher 抽象与递归文件系统监控
- P4 Event Normalizer、Debounce 与 Coalescing
- P5 Snapshot / Baseline Engine

**本轮交付**：
- `Watcher` interface + fsnotify adapter
- 新建/删除目录的动态 watch 管理
- Event Normalizer
- path 级 debounce/coalescing
- SnapshotStore
- initial baseline / reset baseline
- context cancel / close 清理
- temp-dir 集成测试

**R2 验收**：
- [x] create/write/remove/rename 产生路径 invalidation；迁移/溢出可降级 root reconciliation
- [x] 新建/移入子目录、注册前已有文件与注册后独立写入均有集成测试
- [x] ignored 临时文件替换真实目标的 atomic save 不永久丢失目标
- [x] 同一聚合窗口每 path 一次请求；最终语义状态唯一性由 R3 ChangeStore 接续验收
- [x] 普通保存不推进 baseline，连续编辑/恢复原文的 before 内容/hash 正确
- [x] Reset 构建新 generation 后一次替换；取消/缺失/不可读失败保留旧基线
- [x] stop/reset 并发与多轮生命周期测试、race、缓存清理测试通过
- [x] 实际 Vim backupcopy=yes/no 保存通过；VS Code 类直接写入/替换模型通过（非实际 VS Code GUI 验收）

**出口定义**：不论编辑器如何保存，core 最终都能稳定得到需要重新解析的真实路径，并拥有正确 baseline。

---

### 2.5 R3 — 内容分析与变更语义（P6–P8）

**当前状态：PASS（实现与自动化验收通过，PR #2 已合并，2026-10-02）**。R4 开始时实查：R3 最终 `c0f626b` 已于 2026-10-02 07:08:58 UTC 合为 `1b52fcc23d199f6962206205e585c1d695cc0bae`，合并树与 R3 head 相同。R3 原有 Classifier、Diff、ChangeStore、Session/CLI 集成、248 个测试/子测试、全量 race 10 轮和三组 smoke 证据保留在 R3 acceptance；不据合并推断额外人工 QA，不重写历史标签。

**目标**：把“某 path 有事件”升级成明确、唯一的文件变化语义。

**包含**：
- P6 Text/Binary Classifier 与安全读取
- P7 Diff Engine
- P8 Change Resolver / Change Store

**本轮交付**：
- bounded-read Classifier
- Text/Binary/Unsupported/TooLarge 状态
- UI 无关的 DiffResult
- hunk、line number、Added/Removed/Context
- Change Resolver
- 单一语义 ChangeStore
- rename best-effort/fallback
- ChangeBatch
- golden/state-transition/race tests

**R3 验收**：
- [x] 新文件 = Added（包括任意扩展名）
- [x] 删除 baseline 文件 = Deleted，缺失 before 不用当前内容冒充
- [x] 内容 hash 改变 = Modified；纯 mtime/chmod 不生成内容变化
- [x] 恢复 baseline 后自动从 Changes/CLI 状态消失
- [x] Added→Deleted、Deleted→相同/不同内容重建等迁移正确
- [x] Binary / UTF-16/32 unsupported-text 不进入文本 diff
- [x] 字节/行数/计算量超限返回 TooLarge，有零读取和有界矩阵测试
- [x] 9 个黄金样例覆盖 Unicode、空文件、末尾换行、多 hunk 与增删改
- [x] 每个 path 唯一状态；排序副本、版本批次、慢消费者 Reload 和旧 Diff 失效有回归
- [x] 全量 `go test -race -count=10 ./...` 通过；旧 R1/R2 回归保留

**出口定义**：无 TUI 情况下即可可靠获得 `[]ChangeSummary` 和 `GetDiff(path)`；R4 只能展示结果，不能重新推导文件状态。

---

### 2.6 R4 — Terminal/TUI 产品体验（P9–P11）

**当前状态：IN_REVIEW（实现、本地及远端 CI 自动化验收 PASS，2026-10-02）**。287 个 Go 测试/子测试、全包 race 10 轮、R1–R3 原有 60/17/16 项与 R4 新增 21 项真实二进制 PTY 冒烟均通过；四种 target 交叉编译通过。PTY 不是 Terminal.app/iTerm2 人工签字。远端 PR/CI 实测与已知边界见第 28 节、R4 acceptance。

**目标**：把稳定 core 变成可完整使用的 Terminal 产品。

**包含**：
- P9 TUI 变化列表与状态栏
- P10 TUI Diff Viewer 与交互
- P11 Pause/Resume、Reset、错误处理与日志

**本轮交付**：
- Bubble Tea model
- Lip Gloss styles
- Changed Files list
- unified diff viewer
- 红/绿增删显示，同时保留 `+/-`
- keyboard navigation
- mouse enhancement
- resize/scroll
- Pause/Resume/Reset
- warning/recoverable/fatal 错误体验
- debug logging

**R4 验收**：
- [x] 0/1/100+ changed files 正常显示（model 120 条，PTY 112 条）
- [x] `↑↓`、`j/k`、Enter、Space、`p`、`r`、`?`、`q` 可用
- [x] 鼠标失效时核心功能仍可纯键盘完成；--no-mouse 拒绝残留报告
- [x] 1500 行 diff 可滚动；单请求、取消、epoch/path/generation/version 防旧结果覆盖
- [x] Binary/Unsupported/TooLarge/Unavailable 有友好状态与尺寸
- [x] Pause 保持 watcher、冻结状态；Resume 全 root reconciliation，baseline 不变
- [x] Reset 确认后原子换代/清空；取消或失败保留旧基线；暂停中 Reset 仍暂停
- [x] 单文件 recoverable error 不终止 session；fatal 非 0 退出并恢复终端
- [x] 日志不写 TUI stdout/stderr，不含正文；200 条 ring 与新建 root 外私有文件上限有测试

**出口定义**：测试人员可纯 Terminal 完成“启动→修改→查看列表→看 diff→pause/resume→reset→退出”。

---

### 2.7 R5 — Terminal 工程硬化与 Gate A（P12–P14）

**目标**：将 Terminal 从“能用”提升到“可正式发布，并足以作为 GUI 唯一 core 基础”。

**包含**：
- P12 性能、并发与资源治理
- P13 自动化测试矩阵、CI 与回归集
- P14 阶段一发布、验收与 Gate A

**本轮交付**：
- benchmark/pprof/race 结果
- `docs/performance-v1.md`
- Unit/Integration/Golden/Fuzz/TUI model tests
- CI
- Terminal release artifact
- README / CHANGELOG
- Homebrew 方案
- Gate A 验收记录

**R5 验收**：
- [x] P14 Gate A 全部满足；`.r5-worktree/dist/v0.1.0-rc.1` 已完成人工 Gate A 验收
- [x] 性能数据标明真实硬件与规模；1k/10k、burst、大文件、深目录与原始JSON已记录
- [x] channel/timer/goroutine有界设计及测量/回归通过；fd泄漏已修复，不将短时观察等同小时级保证
- [x] 本轮核心bug在native责任层及Session补regression test，全包race10轮通过
- [x] clean machine / 实际安装验收已纳入 Gate A 人工签字
- [x] Core API不依赖Bubble Tea/Lip Gloss，传递依赖自动检查通过
- [x] Gate A 验收记录已完成，最终状态 PASS

**出口：Gate A**：`PASS` 才进入 R6；`FAIL` 必须回到对应责任 Round 修复并重新执行 Gate A。

---

### 2.8 R6 — GUI 壳层与 IPC 契约（P15–P16）

**当前状态：实现完成、本地自动化 PASS，Round 集成 IN_REVIEW（2026-10-02）。** Wails 生产构建、11 项前端测试、2 条真实 Wails IPC E2E、完整 Terminal 回归已通过；未将未完成的独立评审、原生按钮/菜单人工交互或 Gate B 记为 PASS。实际 PR/CI 见§30.6。

**目标**：建立 GUI 技术底座，并验证 Wails/Svelte 稳定调用同一 Go Core。

**包含**：
- P15 Wails + Svelte 工程壳层
- P16 Core Facade、GUI IPC 与事件协议

**本轮交付**：
- Wails app shell
- Svelte + TypeScript frontend
- Core Facade
- IPC DTO/Event contract
- Start/Stop 基础流程
- root/path 安全校验

**R6 验收**：
- [x] GUI 不包含第二套 watcher/baseline/diff；Facade 调用同一 `internal/app`，Core 依赖边界检查通过
- [x] Start/Stop/Start 可重复；真实 Wails IPC E2E 与 Go 生命周期回归通过，Scanning 可取消
- [x] frontend reload/disconnect 撤销旧 client/session；租约到期取消，12 次 Stop/reload 后 fd 6→6、goroutine 2→2、缓存为空（短时测量，不代表小时级耐久度）
- [x] change event 只广播有界 metadata invalidation；内容哨兵与 32 槽背压测试通过
- [x] diff 按需获取；单请求、15 秒取消上下文、版本围栏与 16 MiB JSON 降级预算
- [x] DTO 与内部 Go struct 解耦；Go/TypeScript 共享 fixture、反射检查与大整数精度测试通过
- [x] path 限制为当前 root 下 canonical key；穿越/绝对路径/祖先 symlink 拒绝，最终 symlink 仅元数据；不宣称抵御恶意并发换目录的原子沙箱

**出口定义**：GUI 成为另一个 UI adapter，而不是另一套业务实现。

---

### 2.9 R7 — GUI 主流程与 Diff 体验（P17–P18）

**目标**：完成 GUI 最核心用户闭环。

**包含**：
- P17 Folder Picker、状态与变化列表
- P18 GUI Diff Viewer

**本轮交付**：
- Folder Picker
- Session status
- Changed Files list/filter
- Monaco Diff Editor
- loading/error/binary/too-large 状态
- stale diff request 防护
- keyboard navigation / empty state

**R7 验收**：
- [x] 无需 Terminal 可选择目录并启动监控
- [x] 文件修改后列表出现
- [x] 文本文件显示正确 diff
- [x] Binary/TooLarge 不进入文本编辑器渲染
- [x] 快速切换多个文件不显示旧 diff
- [x] 文件再次变化后，旧请求不能覆盖新版本

**出口定义**：GUI 完成 `Open Folder → Monitoring → Modify → Changed List → Select → View Diff`。

---

### 2.10 R8 — GUI 功能补全与系统鲁棒性（P19–P21）

**目标**：补齐长期使用所需的设置、系统集成、可访问性和生命周期稳定性。

**包含**：
- P19 GUI Session 控制与 Settings
- P20 外部编辑器与 macOS 系统集成
- P21 GUI 可用性、可访问性与鲁棒性

**本轮交付**：
- Pause/Resume/Reset/Stop/change-folder
- 与 CLI 共用 Config schema
- theme / ignore / debounce / max diff settings
- Open in Editor / Reveal in Finder / Copy Path
- sleep/wake reconciliation
- resize/font scaling/listener cleanup
- 权限/目录移动等错误态

**R8 验收**：
- [ ] GUI/CLI 使用同一默认值与配置模型
- [ ] Reset Baseline 有清晰提示
- [ ] 空格/引号/Unicode path 可安全传递
- [ ] 外部编辑器不使用 `sh -c` 字符串拼接
- [ ] sleep/wake 后状态恢复一致
- [ ] 长时间运行、反复切目录无明显资源泄漏
- [ ] Light/Dark/System 正常
- [ ] 颜色不是唯一状态信息
- [ ] component destroy 时注销 event listener

**出口定义**：GUI 功能层面冻结；R9 不再新增主功能，只处理测试、发布和 release blocker。

---

### 2.11 R9 — GUI 测试、打包与 Gate B（P22–P24）

**目标**：证明 GUI 与 Terminal/Core 行为一致，并产出可安装的 macOS 版本。

**包含**：
- P22 GUI E2E / 集成测试
- P23 macOS 打包、签名、公证与安装体验
- P24 最终 QA、安全与 Gate B

**本轮交付**：
- backend integration tests
- DTO contract tests
- Svelte component/store tests
- E2E 主流程
- `.app` / DMG
- signing / notarization（正式外部分发时）
- checksums / release metadata
- clean Mac install test
- Gate B 验收记录

**R9 验收**：
- [ ] P24 Gate B 全部满足
- [ ] 同一 fixture 下 Terminal 与 GUI ChangeStore 结果一致
- [ ] stale diff race 有回归测试
- [ ] Start/Stop/Start stress test 通过
- [ ] 未配置开发环境的 Mac 可安装运行
- [ ] Gatekeeper 体验验证完成
- [ ] Gate A 回归保持 PASS
- [ ] 无 P0/P1 release blocker

**出口：Gate B**：只有 `PASS` 才可把 GUI 标记为 v1 可发布版本。

---

### 2.12 R10 — v1 冻结与长期维护交接（P25）

**目标**：把项目从“开发完成”转成“任何下一位程序员都能继续维护”。

**包含**：
- P25 v1 收尾、跨平台准备与后续路线

**本轮交付**：
- v1 API/行为冻结说明
- README / FAQ / Troubleshooting
- release notes
- macOS platform-specific code 收口
- Windows/Linux compatibility matrix
- 后续 feature backlog
- 最终 Handoff 更新

**R10 验收**：
- [ ] 新开发者不依赖口头知识即可 build/test/modify/release
- [ ] 支持与不支持的平台边界明确
- [ ] Known Issues 有 owner/优先级/复现步骤
- [ ] ADR、Gate、Round 验收记录与代码状态一致
- [ ] v1 后新需求进入 backlog，不再修改已冻结 Gate 结论

---

### 2.13 每轮统一 Acceptance / Handoff 模板

每轮结束在 `docs/rounds/` 新建文件，例如 `R3-acceptance.md`：

```markdown
# Round R3 Acceptance

Status: PASS / FAIL / IN_REVIEW
Date:
Owner:
Reviewers:
Start commit:
End commit:
Related PRs:

## Scope
- P6
- P7
- P8

## Delivered
- ...

## Automated Tests
- Command:
- Result:

## Manual Tests
- Environment:
- Cases:
- Result:

## Performance / Resource Notes
- ...

## Known Issues
- ID / severity / workaround / owner

## Architecture Decisions
- ADR links

## Deferred Items
- ...

## Handoff Notes
- 下一轮可以依赖的稳定接口：
- 下一轮禁止假设的未完成功能：
- 风险：
```

**只有 `PASS` 才代表该 Round 真正完成。** 代码已合并但验收未结束时，状态应为 `IN_REVIEW` 或 `ACCEPTANCE`，不能写成完成。

---

## 3. 技术决策

### 3.1 语言

**Go** 作为唯一 core/backend 语言。

原因：

- 文件监听与并发事件处理适合 goroutine/channel。
- 单二进制发布简单。
- `fsnotify`、Bubble Tea、Lip Gloss 生态成熟。
- 同一 Go core 可直接复用给 Wails GUI。
- 本项目瓶颈通常是 I/O 与用户事件，不需要为了 CPU 极限性能引入额外复杂度。

### 3.2 核心依赖

依赖版本在 P0 中锁定，禁止在业务代码里直接耦合第三方实现，应通过本项目接口封装。

建议依赖：

- 文件监听：`github.com/fsnotify/fsnotify`
- TUI：`github.com/charmbracelet/bubbletea`
- TUI 样式：`github.com/charmbracelet/lipgloss`
- 文本 diff：选定一个稳定 Go diff 库作为底层，实现必须包在 `DiffEngine` 接口后；可采用 `diffmatchpatch` / `go-difflib` 等成熟方案。
- ignore/glob：优先采用支持 gitignore 语义的成熟库，或在 P2 封装自己的 Matcher 接口。
- 哈希：标准库优先；用于变更判定时可选择快速、稳定的算法，具体算法不是外部协议的一部分。

### 3.3 GUI 技术栈

- Wails：Go backend + 桌面 WebView 壳层。
- Svelte + TypeScript：前端。
- Monaco Diff Editor：v1 GUI 的 diff viewer 首选，功能完整；若包体积成为硬指标，可在后续 ADR 中评估 CodeMirror。

---

## 4. 总体架构

```text
┌──────────────────────────────────────────────┐
│                 UI Adapters                  │
│                                              │
│   Bubble Tea TUI            Wails + Svelte  │
└──────────────┬───────────────────┬───────────┘
               │                   │
               └─────────┬─────────┘
                         │
                 Application Service
                         │
┌────────────────────────▼─────────────────────┐
│                  Go Core                     │
│                                              │
│ Watcher → Normalizer → Debouncer/Coalescer  │
│                      ↓                       │
│               Change Resolver               │
│                      ↓                       │
│ Snapshot/Baseline ←→ Diff Engine             │
│                      ↓                       │
│               Change Store                  │
│                      ↓                       │
│                Event Publisher              │
└──────────────────────────────────────────────┘
                         │
                  Local File System
```

### 4.1 最重要的分层规则

1. `core` 不 import Bubble Tea、Lip Gloss、Wails、Svelte 相关包。
2. TUI 只消费 core 暴露的状态与事件，不直接创建 `fsnotify.Watcher`。
3. GUI 只通过 application service / Wails binding 调用 core。
4. diff 计算不能发生在 UI render 函数中。
5. 文件读取、hash、diff 等可能耗时任务必须能被 context 取消或通过队列控制。
6. 原始 OS 文件事件不得直接暴露给 UI；UI 只消费“语义化 Change”。

---

## 5. 建议仓库结构

```text
folderwatch/
├── cmd/
│   └── folderwatch/
│       └── main.go
├── internal/
│   ├── app/                 # application service / session orchestration
│   ├── watcher/             # fsnotify adapter + recursive directory management
│   ├── eventnorm/           # OS event normalization
│   ├── debounce/            # debounce / coalescing
│   ├── snapshot/            # baseline and content storage
│   ├── diff/                # diff engine and models
│   ├── changes/             # current changed-file state
│   ├── ignore/              # ignore rules
│   ├── filetype/            # binary/text/encoding/size decisions
│   ├── config/              # config loading / validation
│   ├── editor/              # external editor integration
│   ├── logging/
│   └── platform/            # OS-specific helpers
├── tui/
│   ├── model/
│   ├── views/
│   ├── keys/
│   └── styles/
├── gui/                     # P15 之后创建
│   ├── frontend/
│   └── backend/
├── testdata/
├── scripts/
├── docs/
│   ├── adr/
│   └── release/
├── .github/workflows/
├── go.mod
├── go.sum
├── Makefile
├── README.md
├── CHANGELOG.md
└── Handoff.md
```

禁止创建一个包含绝大多数逻辑的巨大 `main.go`。

---

## 6. 核心领域模型

建议从 P0 开始固定领域概念，字段可以迭代，但含义不要漂移。

```go
type ChangeKind string

const (
    ChangeAdded    ChangeKind = "added"
    ChangeModified ChangeKind = "modified"
    ChangeDeleted  ChangeKind = "deleted"
    ChangeRenamed  ChangeKind = "renamed"
)

type FileMeta struct {
    Path       string
    Size       int64
    Mode       fs.FileMode
    ModTime    time.Time
    IsDir      bool
    IsSymlink  bool
    IsBinary   bool
    ContentHash string
}

type FileChange struct {
    Path       string
    OldPath    string // renamed 时使用
    Kind       ChangeKind
    Before     *SnapshotRef
    After      *SnapshotRef
    FirstSeen  time.Time
    LastSeen   time.Time
}

type SnapshotRef struct {
    Meta       FileMeta
    StorageKey string
    HasContent bool
}

type DiffResult struct {
    Path       string
    Kind       ChangeKind
    Binary     bool
    TooLarge   bool
    Hunks      []Hunk
}
```

### 6.1 Change Store 状态规则

- baseline 中存在、当前不存在 → `Deleted`
- baseline 中不存在、当前存在 → `Added`
- baseline 与当前都存在，内容 hash 不同 → `Modified`
- 内容恢复到 baseline → 从 changed list 中移除
- 权限/mtime 变化但内容 hash 一致：默认不显示为内容修改；是否显示 metadata change 留作后续配置
- rename：best-effort；无法确定时拆成 `Deleted + Added`

---

## 7. Baseline / Snapshot 策略

### 7.1 默认语义

一次监控 session 启动时的文件状态为 baseline：

```text
Session start
   ↓
Initial scan
   ↓
Baseline
   ↓
Current state changes repeatedly
   ↓
Always compare Current vs Baseline
```

连续保存 20 次也仍然比较“启动时状态 vs 当前状态”，而不是“上一次保存 vs 当前保存”。

### 7.2 Reset Baseline

用户执行 Reset 后：

1. 暂停对 UI 发布中间状态。
2. 获取当前稳定状态。
3. 将当前状态写成新 baseline。
4. 清空 changed list。
5. 恢复监听并处理 reset 窗口内排队事件。

Reset 必须是 application service 的原子语义操作，UI 不允许自己逐文件清空。

### 7.3 Snapshot 存储

禁止无上限把所有文件完整内容永久保存在内存中。

建议默认策略：

- 小文本文件：内存内容快照。
- 中型文本文件：允许落在 OS 临时目录的 session cache。
- 超过分类/保留上限 `max-snapshot-bytes`（R3 默认8MiB）只保留元数据 + hash；`max-diff-bytes`（默认5MiB）独立控制 Diff，不再与正文保留阈值混用。
- 二进制：默认保存元数据 + hash，不保存内容。
- 临时快照目录必须位于系统临时目录，默认不得修改用户监控目录。
- session 正常退出时清理；异常残留可在下次启动做过期清理。

阈值全部做成配置项；初始建议值必须在 P10 压测后再固定。

---

## 8. 文件事件处理流水线

```text
fsnotify raw event
      ↓
Path normalize
      ↓
Ignore filter
      ↓
Event normalize
      ↓
Debounce / coalesce by path
      ↓
Stable read / stat
      ↓
Hash / classify
      ↓
Compare with baseline
      ↓
Update Change Store
      ↓
Publish semantic batch
      ↓
TUI / GUI refresh
```

### 8.1 为什么必须 debounce/coalesce

编辑器保存一个文件可能触发 WRITE、CHMOD、RENAME、CREATE 等多个事件；部分编辑器采用“写临时文件 → 替换原文件”的 atomic save。如果每个事件都直接计算 diff，会造成重复 I/O、UI 闪烁和错误的短暂状态。

默认 debounce 建议从 100–200ms 区间测试，最终值在 P4 通过真实编辑器测试确定。

### 8.2 递归监听

`fsnotify` 并不意味着“对一个根目录调用一次后自动永久覆盖所有未来子目录”。实现必须：

- 初始扫描时把需要监听的目录加入 watcher。
- 新目录创建后递归注册。
- 目录删除/重命名后注销或清理内部记录。
- 遵守 ignore 与 symlink 策略。

---

# 阶段一：Terminal / TUI

---

## P0 — 项目初始化、工程规范、ADR

**完成状态：已完成（R1，2026-10-01）。** 已初始化 `github.com/StevenWinsir/FolderWatch`（Go 1.23+）、锁定依赖及 go.sum，建立最小分层工程、Make targets、gofmt/go vet、CI、README/CHANGELOG/CONTRIBUTING 与 ADR-001–006。R1 时 fsnotify/Bubble Tea/Lip Gloss/difflib 由 `tools` build-tag 文件锁定；R2 接入 fsnotify，R3 实现独立有界 LCS，R4 正式接入 Bubble Tea/Lip Gloss 及既有 x/term/uniseg，未升级依赖版本。R2 新增 vendor 与可复现 kqueue 补丁，详见 ADR-009。CI 配置覆盖 macOS/Linux 与 Go 1.23/1.26；实际运行状态以 R1 acceptance/Actions 为准，不把配置文件存在当成远端已通过。

### 目标

创建可编译的 Go 工程骨架，锁定技术边界和工程规范。

### 依赖

无。

### 实现任务

- 初始化 Go module。
- 建立本文第 5 节目录结构的最小版本。
- 引入并锁定阶段一必要依赖。
- 添加 `Makefile` 或等价 task runner，至少包含：`build`、`test`、`race`、`lint`、`fmt`、`run`。
- 建立 `docs/adr/`：
  - ADR-001：为什么选择 Go。
  - ADR-002：为什么 core 与 UI 解耦。
  - ADR-003：baseline 语义。
  - ADR-004：文本/二进制与大文件策略。
- 配置 formatter、lint、CI 初版。
- 定义提交约定与分支策略。

### 输出

- 工程可运行，`go build ./...` 成功。
- CI 能执行基础 build/test。

### 验收标准

- 新开发者 clone 后，只看 README + Handoff 即可在本机完成 build。
- `main.go` 不包含业务实现，仅负责组装依赖和启动。

---

## P1 — CLI 契约与配置模型

**完成状态：已完成（R1，2026-10-01）。** `internal/cli` 与 `internal/config` 已实现推荐 flags、严格 TOML、存在性感知的显式覆盖、路径/duration/size 校验和退出码。项目配置为 `<root>/.folderwatch.toml`；用户配置使用 OS user config directory。高优先级 ignore 数组替换低优先级数组，`[]` 清空；布尔值可用 `--flag=false` 覆盖。配置路径相对其文件，CLI 路径相对 cwd。R4 交互终端默认启动 TUI；非终端默认、`--scan` 与 `--json` 单次元数据扫描，新增 `--tui` 明确要求终端。R2 `--watch` 与资源限额保留，R3 已将 classification/snapshot 8MiB 与 diff 5MiB 分离。diff/mouse/log/debug 已有实际行为；editor 仍只存储不执行。配置原则见 ADR-006，R2–R4 行为见 ADR-007–013。

### 目标

先定义用户和 core 的边界，避免后续反复改参数。

### 依赖

P0。

### 初始 CLI

```bash
folderwatch [path]
folderwatch .
folderwatch ~/Projects/demo
```

建议参数：

```text
--debounce <duration>
--ignore <pattern>        # 可重复
--ignore-file <path>
--respect-gitignore       # R1 默认 false
--include-git             # R1：关闭内建 .git/ 排除
--scan                    # R1：显式单次扫描
--json                    # 单次扫描 JSON；--watch 时为 NDJSON
--watch                   # R2：持续监听 + 基线 + 聚合请求
--max-pending-events <n>   # R2：默认 4096
--max-watch-dirs <n>       # R2：默认 8192
--max-snapshot-files <n>   # R2：默认 100000
--snapshot-memory-bytes <size> # R2：默认 32MiB/代
--snapshot-cache-bytes <size>  # R2：默认 256MiB/代
--no-mouse
--max-diff-bytes <size>
--editor <command>
--log-file <path>
--debug
--version
--help
```

### 配置优先级

```text
CLI flags > project/local config > user config > built-in defaults
```

建议 user config：

```text
~/Library/Application Support/FolderWatch/config.toml
```

不得默认在监控目录写状态文件。

### 实现任务

- 定义 `Config` 结构体及校验。
- 所有 duration/size/path 配置必须经过 normalize。
- 为错误参数提供明确错误信息与非 0 exit code。
- `path` 省略时默认当前目录。

### 验收标准

- 单测覆盖配置优先级、非法路径、非法 duration、非法 size。
- `folderwatch --help` 文案完整。

---

## P2 — 初始目录扫描、路径规范化与 Ignore Engine

**完成状态：已完成（R1，2026-10-01）。** `internal/scan` 使用 WalkDir；`pathutil` 固定根相对 `/` 分隔 canonical key，`.` 代表根，保留大小写与 Unicode。明确选择的 root symlink 可解析；后代 symlink 仅记录元数据、不跟随，特殊文件不读取内容。不可读子目录/瞬间消失文件产生 warning，root 失败则退出。

统一 `ignore.Matcher.Match(path, isDir)` 已同时供扫描、Watcher 与 Event Normalizer 调用。规则优先级固定为：内建 `.git/` < 可选根/嵌套 `.gitignore` < 根 `.folderwatchignore` < 显式 ignore 文件 < config/CLI ignore；支持 ancestor-aware 否定。默认不启用 `.gitignore`，可显式 `--include-git`。规则按 Matcher 缓存，修改已加载规则需重启/新 Matcher；嵌套规则错误在进入目录前检查并跳过子树。完整策略与安全边界见 ADR-005。

### 目标

稳定得到“应该被监控的文件/目录集合”。

### 依赖

P1。

### 实现任务

- 使用 `filepath.WalkDir` 递归扫描。
- 所有内部 key 使用规范化 absolute path 或 root-relative canonical path；选择一种并在 ADR 固定。
- UI 显示 root-relative path。
- 默认不跟随目录 symlink。
- 处理权限不足：记录 warning，扫描继续，不可整程序崩溃。
- 支持 `.folderwatchignore`。
- 支持 CLI `--ignore`。
- `--respect-gitignore` 为显式开关；是否默认启用由产品决策固定，不能静默改变。
- `.git/` 建议默认忽略，但必须有明确配置能包含它。
- ignore matcher 必须同时被 initial scan 与 runtime new-path 使用，不能出现“启动时忽略、运行时又被加回”的分叉。

### 测试

- 嵌套目录。
- 通配规则。
- 路径包含空格、Unicode。
- symlink 环。
- 无权限目录。

### 验收标准

同一 ignore 规则对 initial scan 和 runtime create 的结果完全一致。

---

## P3 — Watcher 抽象与递归文件系统监控

**完成状态：R2 已实现并通过本地集成验收。** `internal/watcher` 封装 fsnotify；单 owner 管理递归目录、新建/移入/删除/重命名、identity 检查与 `Reconcile(ctx)`。事件队列有界、errors 分离，溢出转 root invalidation，root 丢失/目录容量耗尽明确停止。目录 symlink 不跟随，包含对 fsnotify kqueue 内部行为的补丁和绕过 wrapper 的原生回归测试。注册发生在新 baseline 扫描之前，减少启动窗口漏事件；不是仅依赖 R1 旧清单。详见 ADR-007、009。

### 目标

将 OS 事件转换为项目内部 raw event stream。

### 依赖

P2。

### 接口建议

```go
type Watcher interface {
    Start(ctx context.Context, root string) (<-chan RawEvent, <-chan error, error)
    Reconcile(ctx context.Context) error // R2 实际增加：校准目录登记
    Close() error
}
```

### 实现任务

- `fsnotify` adapter。
- initial scan 后注册所有非忽略目录。
- 运行时新建目录后注册该目录及其已有子目录。
- 目录删除/重命名时清理 watch bookkeeping。
- watcher 生命周期必须服从 context。
- 输出事件通道需要有明确的容量与 backpressure 策略，禁止无限队列。
- error channel 与 event channel 分离。

### 测试

使用临时目录进行真实文件系统集成测试：

- create file
- write file
- remove file
- rename file
- create subdir
- write nested file
- delete subdir

### 验收标准

在监控启动后新建的子目录中继续新建/修改文件，仍能收到事件。

---

## P4 — Event Normalizer、Debounce 与 Coalescing

**完成状态：R2 已实现并通过本地集成验收。** `internal/eventnorm` 产出 canonical path + metadata-only hint；`internal/debounce` 用单 goroutine、单 ticker、有限 pending map 聚合，默认 150ms，连续繁忙路径最多等 4 倍窗口。输出背压/容量溢出不无限排队而转 root reconciliation。R2 请求不是 Added/Modified/Deleted，也不计算 diff；同 path 在后续独立窗口仍可再次要求解析，R3 必须幂等 resolve。取消和 Reset generation 边界由 Session 统一管理。

### 目标

把“嘈杂的 OS 事件”转换成“稳定的路径变更请求”。

### 依赖

P3。

### 实现任务

- 统一 CREATE/WRITE/REMOVE/RENAME/CHMOD 等事件。
- 以 path 为 key 做 debounce。
- 同一路径窗口内多个事件合并。
- 支持 atomic save：临时文件替换原文件时，最终必须把真实目标文件重新解析为当前状态。
- CHMOD-only 默认不触发内容 diff，除非后续 stat/hash 判断内容确实变化。
- 使用 timer map 时必须防止 timer/goroutine 泄漏。
- stop/reset 时有明确 drain/cancel 行为。

### 重点人工测试编辑器

- macOS TextEdit（若保存格式适用）
- VS Code
- Vim/Neovim
- JetBrains 系列至少一种（若本机可用）
- shell：`echo >> file`、`sed -i`/临时替换方式

### 验收标准

保存一次文件不会在最终 UI 中重复出现多条相同 change；连续快速保存不会造成 UI 闪烁或无限 diff 计算。

---

## P5 — Snapshot / Baseline Engine

**完成状态：R2 已实现并通过本地集成验收。** `internal/snapshot` 提供小文件 memory、中型文本 private temp disk、流式 SHA-256、元数据/hash 降级、不可变 Ref、原子 generation Reset 与清理。`app.Session` 注册 Watcher 后 fresh scan/capture；普通保存不推进 baseline。Reset 失败/取消不发布部分基线，成功先发 reset/reconcile，再处理排队事件。快照/队列限额见 README 与 ADR-008。R2 当时验收 before/hash；R3 已通过 core/CLI 的 Diff、列表恢复消失及 Reset 清空测试；R4 现已通过完整交互 TUI 的本地及远端 CI 自动化验收（见第 28 节），实际 Terminal.app/iTerm2 人工 QA 仍由 R5 负责。

### 目标

建立可比较的启动基线，支持 reset。

### 依赖

P2、P4。

### 接口建议

```go
type SnapshotStore interface {
    Capture(ctx context.Context, path string) (SnapshotRef, error)
    ReadContent(ctx context.Context, ref SnapshotRef) ([]byte, error)
    Delete(ref SnapshotRef) error
    Reset(ctx context.Context, files []string) error
    Close() error
}
```

### 实现任务

- initial baseline capture。
- 小文件 memory snapshot。
- 中型文本可选 temp-disk snapshot。
- hash 计算。
- reset baseline 的原子语义。
- session cleanup。
- baseline 不得因普通文件保存自动推进。

### 验收标准

1. 启动时 A=`hello`。
2. 改为 `hello1`，显示 diff。
3. 再改为 `hello2`，diff 仍然以 `hello` 为 before。
4. 改回 `hello`，A 从 changed list 消失。
5. reset 后 changed list 清空，之后再修改以 reset 时版本为 before。

---

## P6 — Text/Binary Classifier 与安全读取

**完成状态：R3 已实现并通过本地验收。** `internal/filetype` 提供 Classifier 与共享流式 Probe。快照携带确切字节的不可变分类，移除 R2 私有独立探针。已知超限分类零读取；UTF-8/控制字节完整校验，UTF-16/32 BOM 标为 unsupported-text，所有扩展名照常检测。`max_snapshot_bytes` 默认 8MiB，与默认 5MiB `max_diff_bytes` 分离；两者配置上限 64MiB。巨大 regular file 仍单独做有界块哈希，瞬时读取失败保留最后已知状态并重试。详见 ADR-010。

### 目标

保证“任何扩展名都能监控”，同时避免把任意字节当文本渲染。

### 依赖

P5。

### 判定原则

- **不能只靠文件扩展名。**
- 优先检查 size 上限。
- 检查 NUL/控制字节比例与 UTF-8 有效性。
- v1 主要支持 UTF-8 文本；UTF-16 等编码可识别后标记 unsupported-text，不能乱码渲染。
- 二进制也要在 changed list 出现。

### 二进制展示

```text
logo.png
Binary file changed
Size: 126 KB → 141 KB
```

### 实现任务

- `Classifier` 接口。
- bounded read：分类不应为了判断类型把超大文件整个读入内存。
- size limit 与 diff limit 分离。
- 文件在读取过程中消失时按 transient error 处理并允许后续事件修正状态。

### 验收标准

图片、zip、随机二进制不会导致终端乱码、异常内存增长或 panic。

---

## P7 — Diff Engine

**完成状态：R3 已实现并通过本地验收。** `internal/diff.Engine` / BoundedLCS 先检查字节/行数/分类，裁剪公共首尾、行 ID 化、构建有界矩阵；计算中可取消，不产生遗留后台任务。默认每侧 5MiB / 20000 行 / 2000000 矩阵单元 / 3 行上下文；复杂输入可因计算预算降级 TooLarge。Result/Hunk/Line 保留增删上下文、行号、CRLF 与无末尾换行，Unified 为无 ANSI 文本。9 个 golden 与重构 fuzz 验证；无常驻 Diff 缓存。GetDiff 有路径版本/基线代际防旧结果覆盖。详见 ADR-011。

### 目标

提供稳定、UI 无关的文本差异结构。

### 依赖

P5、P6。

### 接口建议

```go
type DiffEngine interface {
    Diff(ctx context.Context, before, after []byte, opts DiffOptions) (DiffResult, error)
}
```

### v1 必须支持

- 行级 Added / Removed / Context。
- hunk 与上下文行数。
- 行号。
- 新文件：全部视为 added。
- 删除文件：全部视为 removed。
- modified：正常 unified-style diff。
- 二进制/too-large：返回结构化状态，而不是 error。

### 可选增强

在行级 diff 稳定后增加“行内 word/character diff”，仅作为 decoration，不改变行级结果。

### 性能要求

- diff 必须可取消。
- 超过阈值直接降级为 `TooLarge`，禁止 OOM。
- UI 请求同一文件 diff 时可以缓存，文件版本/hash 变化后缓存失效。

### 验收标准

黄金文件（golden test）覆盖：新增、删除、单行修改、多 hunk、空文件、无末尾换行、Unicode。

---

## P8 — Change Resolver / Change Store

**完成状态：R3 已实现并通过本地验收。** `internal/changes` 是唯一语义所有者，ResolveBatch 以可取消 token 串行解析、短锁提交，View 返回排序防御副本。相对启动基线判定 Added/Modified/Deleted；恢复/先增后删自动移除。rename 本轮安全降级 Deleted+Added，不做相似内容猜测。不可读子树保留上次状态并 warning；原始队列溢出由 core 重查，消费端溢出由 Batch.Reload/ChangeState 恢复。Session 暴露 Changes/ChangeState/GetDiff/ResetBaseline；CLI 不再输出原始路径来冒充语义变化。详见 ADR-012。

### 目标

把当前文件状态与 baseline 比较，维护唯一的 changed-file 集合。

### 依赖

P4–P7。

### 实现任务

- `Resolve(path)`：stat/classify/hash，与 baseline 比较。
- `ChangeStore` 保证每个 path 最终只对应一个当前语义状态。
- 支持状态迁移：
  - Added → deleted before reset：从列表消失。
  - Modified → restored：从列表消失。
  - Deleted → recreated with same baseline content：从列表消失。
  - Deleted → recreated different：Modified。
- rename best-effort：利用可用文件 identity / event 邻接关系；不可靠时降级。
- 发布 `ChangeBatch`，而不是对 UI 发布每个 raw event。
- 排序默认按 root-relative path；后续允许 UI 按时间/状态排序。

### 并发规则

- ChangeStore 必须有单一所有权 goroutine，或使用明确锁策略。
- 禁止 UI 与 watcher 直接共享可变 map。
- 运行 `go test -race` 必须无 race。

### 验收标准

状态迁移单测覆盖完整，并且快速连续操作后最终状态正确。

---

## P9 — TUI 第一版：变化列表与状态栏

**完成状态：已完成（R4，2026-10-02；本地验收 PASS）**。`internal/tui` 直接展示权威 ChangeState：A/M/D、展开标志、数量/状态、选中路径保持、键鼠、过滤、滚动与 resize。Model 覆盖 0/1/120 条和极小/宽屏尺寸，PTY 覆盖 112 条、纯键盘与实际鼠标报告。GUI 未引入；结构路径采用 `internal/tui` 而非上方建议的顶层 tui 子包。

### 目标

把 core 状态可靠展示在 Terminal。

### 依赖

P8。

### 建议布局

```text
FolderWatch  ~/Projects/demo                 ● Monitoring
3 files changed

[+] M  A.txt
[+] A  src/new.go
[+] D  scripts/old.py

↑/↓ or j/k select  Enter expand  r reset  p pause  ? help  q quit
```

说明：

- `[+]` / `[-]` 是展开控件。
- 状态使用 `M` / `A` / `D` / `R`，避免把“展开 +”与“新增 +”混淆。
- 可以在主题中把 Added/Modified/Deleted 使用不同前景色，但颜色不能是唯一信息渠道。

### 键盘

最低要求：

```text
↑ / ↓       移动选择
j / k       Vim 风格移动
Enter       展开/收起
Space       展开/收起
p           pause/resume
r           reset baseline
/           filter/search（若 P9 来不及可放 P10）
?           help
q / Ctrl+C  quit
```

### 鼠标

- 默认可启用鼠标，也提供 `--no-mouse`。
- 点击文件行或 `[+]` 展开。
- 滚轮滚动。
- 鼠标只是增强，所有关键功能必须可纯键盘完成。

### 验收标准

终端 resize、列表滚动、0/1/100+ change 时不 panic，状态栏正确。

---

## P10 — TUI Diff Viewer 与交互完善

**完成状态：已完成（R4，2026-10-02；本地验收 PASS）**。采用“列表 + 单一选中文件 unified Diff”布局控制成本，最多一个可取消请求和一份预览；epoch/path/generation/version 严格防旧结果覆盖，stale 最多重试三次。行号/hunk/红绿与 +/-、末尾换行、友好降级、纵横滚动、控制字符转义与 Unicode 格裁切均覆盖；1500 行实际 PTY 滚动通过。预览 16MiB/50000 行/单逻辑行约16KiB 有明确截断提示；不是 P12 进程内存验收或多文件全文常驻缓存。

### 目标

实现接近 GitHub 的终端 diff 阅读体验。

### 依赖

P7、P9。

### 展示规则

```diff
@@ -10,4 +10,5 @@
  unchanged line
- old text
+ new text
+ another line
  unchanged line
```

- 删除：红色。
- 新增：绿色。
- hunk header：强调样式。
- context：普通样式。
- 终端不支持颜色或用户关闭颜色时仍需通过 `+` / `-` 语义可读。

### 实现任务

- viewport / scrolling。
- 展开多个文件时控制渲染成本；建议只对选中/展开文件请求 diff。
- diff loading 状态。
- too-large / binary friendly message。
- 可选 side-by-side 模式留到后续，v1 默认 unified diff。
- 搜索/filter 可在此阶段补齐。

### 验收标准

1000+ 行 diff 可滚动，UI 不冻结；切换选中文件不会产生明显累积延迟。

---

## P11 — Pause/Resume、Reset、错误处理与日志

**完成状态：已完成（R4，2026-10-02；本地验收 PASS）**。Session 统一串行 Pause/Resume/Reset；暂停保留 watcher 并冻结最后已知状态，恢复全 root 校准且不推进基线。确认式 Reset 失败保持旧代，暂停中 Reset 保持暂停；并发 Close 的底层关闭错误在 app 层统一。单文件 warning 可查看详情并继续；q/Ctrl+C/fatal 分别 0/130/1，终端与缓存清理有真实二进制测试。日志为 200 条 ring，可显式新建 root 外0600文件、最多4MiB，拒绝已有文件/链接/目录身份别名；不记录正文，详见 ADR-013。

### 目标

完善 session 生命周期和可诊断性。

### 依赖

P8–P10。

### Pause 语义

必须在 ADR 中选定并实现一致语义。推荐：

- Pause：暂停处理/发布变化，但保持 watcher 存活。
- Resume：对根目录做轻量 reconciliation，避免 pause 期间丢事件导致状态错误。

### 错误分类

- Fatal：root 不存在、无法建立核心 watcher、内部不可恢复错误。
- Recoverable：单文件权限不足、文件瞬间消失、临时读取失败。
- Warning：unsupported encoding、too-large、部分目录不可读。

### 日志

- TUI 正常运行时禁止把 debug log 直接写 stdout/stderr 破坏界面。
- debug 日志写文件或内部 ring buffer。
- 日志不得默认记录完整文件内容。
- 错误 UI 只显示必要信息，并提供查看详情方式。

### 验收标准

单文件读取失败不会终止 session；fatal error 能正确退出并带有非 0 code。

---

## P12 — 性能、并发与资源治理

### 目标

证明程序在真实目录中稳定，不靠“机器够快”掩盖设计问题。

### 依赖

P0–P11。

### 必须建立的基准场景

- 1k 文件。
- 10k 文件。
- 100 个文件短时间 burst 修改。
- 单个 5–10MB 文本文件（阈值按实际配置）。
- 大型二进制文件。
- 深层目录。

### 目标指标

以下为首轮工程预算，必须记录测试硬件；若未达到，应给出测量结果和原因，不允许伪造通过：

- 空闲监控 CPU 近似 0，目标通常 < 1%。
- debounce 后至变化列表出现：P95 目标 < 250ms（小文件、本地 SSD）。
- 10k 小文件 baseline：目标 < 3s（Apple Silicon + SSD 参考环境）。
- 10k 个约 1KB 小文本 baseline 内存：目标 < 150MB。
- 无无限增长 channel、timer、goroutine。

### 工具

- Go benchmark。
- `pprof`。
- `go test -race`。
- goroutine profile。

### 验收标准

输出 `docs/performance-v1.md`，记录硬件、数据规模、结果、瓶颈与未解决项。

---

## P13 — 自动化测试矩阵、CI 与回归集

### 目标

把“能跑”升级为“敢改”。

### 依赖

P0–P12。

### 测试层级

1. Unit tests
   - config
   - ignore
   - path normalize
   - classifier
   - diff
   - state transitions
   - debounce/coalesce

2. Integration tests
   - 真实 temp dir + watcher
   - create/write/delete/rename
   - nested dir
   - atomic save 模拟
   - reset baseline

3. TUI model tests
   - Bubble Tea update/model，不依赖真实交互终端
   - keyboard commands
   - resize
   - list selection

4. Golden tests
   - diff rendering
   - binary/too-large message

5. Fuzz tests
   - classifier
   - diff adapter 输入边界
   - path/event normalize

### CI 最低任务

- `go fmt` 检查
- lint
- unit/integration tests
- race（可独立 job）
- build macOS/Linux；Windows 可先作为 non-blocking，P25 再决定是否正式支持

### 验收标准

修复任何核心 bug 时必须先或同步补 regression test。

---

## P14 — 阶段一发布、验收与 Gate A

### 目标

得到正式可交付的 Terminal v1，并决定是否进入 GUI。

### 依赖

P0–P13 全部完成。

### 发布任务

- 版本号：SemVer，例如 `v0.1.0` / `v1.0.0`，由团队确定产品成熟度。
- 构建 macOS arm64 / amd64（如仍需支持 Intel）。
- 生成校验和。
- README：安装、使用、快捷键、ignore、已知限制。
- CHANGELOG。
- Homebrew 安装方式：优先提供可维护 formula/tap。
- `folderwatch --version` 输出版本/commit/build date（build date 可选）。

### Gate A 人工验收清单

- [ ] 普通保存能检测。
- [ ] atomic save 编辑器能检测。
- [ ] 新文件能检测。
- [ ] 删除能检测。
- [ ] 新目录及其后续文件能检测。
- [ ] 文件恢复到 baseline 后从列表消失。
- [ ] reset baseline 正确。
- [ ] 二进制只显示元信息，不乱码。
- [ ] 超大文件不 OOM。
- [ ] 目录包含空格/Unicode 正常。
- [ ] Terminal resize 正常。
- [ ] 键盘完整可用。
- [ ] 鼠标失败时不影响核心使用。
- [ ] Ctrl+C 正常清理退出。
- [ ] `go test ./...` 全绿。
- [ ] `go test -race ./...` 全绿。
- [ ] 性能报告已记录。
- [ ] 无 P0/P1 级已知 bug。

### Gate A 结论模板

```text
Gate A: PASS / FAIL
Version:
Commit:
Test macOS version:
Hardware:
Known issues:
Approved by:
Date:
```

只有 `PASS` 才能启动 P15。

---

# 阶段二：GUI

---

## P15 — Wails + Svelte 工程壳层

### 目标

创建 GUI 壳，但不复制 core 逻辑。

### 依赖

P14 Gate A = PASS。

### 实现任务

- 创建 Wails app。
- 创建 Svelte + TypeScript frontend。
- 配置开发/生产构建。
- GUI backend 直接依赖 `internal/app` 或抽出的公共 application package。
- 添加最小窗口、菜单、版本信息。
- 验证前后端 IPC。

### 禁止事项

- 不在 TypeScript 中实现文件 watcher。
- 不在 TypeScript 中维护第二份 baseline。
- 不通过轮询文件系统取代 Go watcher。

### 验收标准

GUI 可以启动/停止一个空 session，并收到 Go backend 的状态事件。

---

## P16 — Core Facade、GUI IPC 与事件协议

### 目标

定义稳定、低耦合的 GUI API。

### 依赖

P15。

### 建议后端 API

```text
StartSession(options) -> SessionInfo
StopSession()
PauseSession()
ResumeSession()
ResetBaseline()
GetChanges() -> ChangeSummary[]
GetDiff(path, options) -> DiffResultDTO
GetSessionStatus() -> SessionStatus
SaveSettings(settings)
LoadSettings() -> Settings
OpenInEditor(path, line?)
RevealInFinder(path)
```

### 事件

```text
session.status
changes.updated
session.warning
session.error
```

### 规则

- 事件只推 `ChangeSummary` 或版本号，不在每个事件中发送大段文件内容。
- diff 按需 `GetDiff`。
- DTO 与内部 Go struct 解耦，避免前端被内部字段变化绑死。
- path 输入必须验证属于当前 root，防止 UI API 被滥用读取任意路径。

### 验收标准

前端断开/重载不会导致 watcher 泄漏；重复 Start/Stop 可稳定执行。

---

## P17 — GUI 主界面：Folder Picker、状态与变化列表

### 目标

实现用户第一次打开就能理解的主流程。

### 依赖

P16。

### 页面布局

```text
┌──────────────────────────────────────────────────────┐
│ FolderWatch  /Users/.../demo   ● Monitoring          │
│ [Open Folder] [Pause] [Reset Baseline] [Settings]   │
├──────────────────────┬───────────────────────────────┤
│ Changed Files        │ Diff                          │
│                      │                               │
│ M README.md          │ Select a file...              │
│ A src/new.go         │                               │
│ D old.py             │                               │
└──────────────────────┴───────────────────────────────┘
```

### 实现任务

- native folder picker。
- 最近目录（只保存路径，不保存内容）。
- 状态：Idle / Scanning / Monitoring / Paused / Error。
- changed files list。
- filter：All / Modified / Added / Deleted。
- 文件名过长处理、tooltip。
- keyboard navigation。
- empty state。

### 验收标准

用户无需 terminal 即可完成“选目录 → 监控 → 找到变化文件”。

---

## P18 — GUI Diff Viewer

### 目标

实现接近 GitHub 的可视化 diff。

### 依赖

P16、P17。

### 推荐

Monaco Diff Editor，默认只读。

### 必须支持

- unified 或 split view 至少一种；若 Monaco 直接提供 side-by-side，可作为默认桌面体验。
- 红色删除 / 绿色新增。
- 行号。
- 大文件/二进制状态。
- loading / cancelled / error。
- 当前文件再次变化时，旧 diff 请求结果不能覆盖新版本；使用 hash/version token 防止 stale response。

### 数据策略

- 前端不常驻所有文件全文。
- 用户选择文件时请求 diff。
- backend 对同一 before/after hash 可缓存结果。
- change 版本更新时前端失效缓存。

### 验收标准

快速切换 20 个文件不会出现“显示了上一个文件 diff”的竞态。

---

## P19 — GUI Session 控制与 Settings

### 目标

把 Terminal 中成熟的行为完整映射到 GUI。

### 依赖

P17–P18。

### 设置项

- debounce。
- ignore patterns。
- respect `.gitignore`。
- max diff bytes。
- theme：System / Light / Dark。
- external editor。
- mouse 相关设置只属于 TUI，不出现在 GUI。

### Session 控制

- Pause / Resume。
- Reset Baseline：必须有清晰提示“当前状态将成为新基线，现有变化列表清空”。
- Stop / change folder。

### 验收标准

修改设置后行为与 CLI 同一配置模型一致；不能有“GUI 专属另一套默认值”。

---

## P20 — 外部编辑器与 macOS 系统集成

### 目标

让 FolderWatch 成为开发工作流中的入口，而不是只读孤岛。

### 依赖

P17。

### 功能

- Open in default editor。
- 支持用户配置命令，例如 VS Code / Cursor / Zed 等。
- Reveal in Finder。
- Copy relative path / absolute path。
- 可选：定位到 diff 行，只有对应编辑器命令稳定时启用。

### 安全要求

禁止通过字符串拼接直接传给 shell。

错误方式：

```go
exec.Command("sh", "-c", editor+" "+path)
```

推荐：解析为可执行文件 + 参数数组，并严格处理 path。

### 验收标准

含空格、引号、Unicode 的路径能安全打开；不能产生 shell injection。

---

## P21 — GUI 可用性、可访问性与鲁棒性

### 目标

从“能显示”升级为“可长期使用”。

### 依赖

P17–P20。

### 实现任务

- Light/Dark/System theme。
- 键盘可访问文件列表与主要按钮。
- 颜色不是唯一状态信息。
- Window resize。
- 空态、错误态、权限态。
- 处理目录被删除/移动。
- 睡眠/唤醒后的 reconciliation。
- 高 DPI 与字体缩放。
- 前端 event listener 在 component destroy 时注销。

### 验收标准

运行数小时、反复切换目录、sleep/wake 后无明显资源泄漏或状态漂移。

---

## P22 — GUI E2E / 集成测试

### 目标

确保 GUI 只是 core 的正确表现层，而不是产生新的行为分叉。

### 依赖

P16–P21。

### 测试

- Backend service integration tests。
- DTO contract tests。
- Svelte component tests。
- 前端 state store tests。
- 关键 E2E：
  1. 选择目录。
  2. 修改文件。
  3. 列表出现。
  4. 点击文件。
  5. diff 显示。
  6. reset。
  7. 列表清空。
- stale diff race test。
- Start/Stop/Start stress test。

### 验收标准

Terminal 与 GUI 对同一 test fixture 的 ChangeStore 结果一致。

---

## P23 — macOS 打包、签名、公证与安装体验

### 目标

产出用户能安装和启动的 `.app`。

### 依赖

P22。

### 实现任务

- Wails production build。
- app icon / bundle metadata。
- 版本号注入。
- Apple Developer ID 签名（正式外部分发时）。
- notarization（正式外部分发时）。
- 验证 Gatekeeper 启动体验。
- 如提供 DMG，验证拖拽安装。
- CLI 与 GUI 是否同包分发，在 ADR 中决定。

### 发布资产

建议：

```text
FolderWatch-vX.Y.Z-macos-arm64.dmg
FolderWatch-vX.Y.Z-macos-amd64.dmg  # 若支持 Intel
folderwatch-vX.Y.Z-darwin-arm64.tar.gz
checksums.txt
```

### 验收标准

在一台未配置开发环境的测试 Mac 上可安装、启动、选择目录并监控。

---

## P24 — 最终 QA、安全与发布 Gate B

### 目标

决定 GUI v1 是否可正式发布。

### 依赖

P15–P23。

### Gate B 清单

- [ ] Gate A 仍保持通过。
- [ ] GUI 不包含第二套 watcher/baseline/diff 实现。
- [ ] 所有核心状态与 CLI 一致。
- [ ] 目录选择、启动、暂停、恢复、重置、停止正常。
- [ ] 文本 diff 正常。
- [ ] 二进制/超大文件正常。
- [ ] 外部编辑器打开安全。
- [ ] sleep/wake 测试完成。
- [ ] 窗口 resize / dark mode 正常。
- [ ] E2E 回归通过。
- [ ] macOS 安装包验证完成。
- [ ] 隐私说明明确：默认纯本地运行，不上传文件内容。
- [ ] 无 P0/P1 级已知 bug。

### Gate B 结论

```text
Gate B: PASS / FAIL
Version:
Commit:
macOS versions tested:
Hardware tested:
Known issues:
Approved by:
Date:
```

---

## P25 — v1 收尾、跨平台准备与后续路线

### 目标

整理 v1，确保后续开发不破坏基础架构。

### 依赖

P24 PASS。

### 工作项

- 冻结 v1 API/行为文档。
- 补齐 README、FAQ、Troubleshooting。
- 整理 release notes。
- 将 macOS 特殊逻辑集中在 `internal/platform/darwin` 或 build tags 中。
- 建立 Windows/Linux compatibility matrix，但没有完成验证前不得宣称正式支持。
- 收集后续候选功能：
  - side-by-side / unified 切换。
  - word-level diff 增强。
  - history / session snapshots。
  - 图片预览或图片 diff。
  - regex filter。
  - 多 root 监控。
  - system tray。
  - 自动更新。
  - plugin architecture。

### 验收标准

任何后续开发者能从 Handoff、ADR、测试和 issue backlog 判断当前行为，不需要通过口头知识才能继续开发。

---

## 9. Application Service 建议接口

为了让 TUI 与 GUI 复用，建议尽早形成统一 facade：

```go
type Service interface {
    Start(ctx context.Context, opts StartOptions) error
    Stop(ctx context.Context) error
    Pause(ctx context.Context) error
    Resume(ctx context.Context) error
    ResetBaseline(ctx context.Context) error

    Status() SessionStatus
    Changes() []ChangeSummary
    Diff(ctx context.Context, path string, opts DiffOptions) (DiffResult, error)

    Events() <-chan AppEvent
}
```

UI 的职责只有：

- 发 command。
- 订阅 event。
- 展示 view state。

UI 不应该知道 fsnotify event flag，不应该直接打开 snapshot 文件，也不应该自己 hash 文件。

---

## 10. 并发模型建议

推荐使用“有限数量 worker + 明确 owner”的模型，而不是“每个事件开一个 goroutine”。

```text
Watcher goroutine
      ↓
bounded raw-event channel
      ↓
Normalizer/Coalescer owner goroutine
      ↓
bounded resolve queue
      ↓
N resolver workers
      ↓
ChangeStore owner goroutine
      ↓
App event channel
```

### 规则

- 所有 channel 必须定义容量和满载策略。
- 不能静默永久丢事件；队列过载时触发 reconciliation 或合并为“root dirty”。
- 所有 goroutine 必须能通过 context/Close 收敛退出。
- diff 可采用独立 worker/cached on-demand，不应阻塞 watcher pipeline。
- UI 慢不能反向卡死 watcher；AppEvent 可以批量/coalesce。

---

## 11. Reconciliation 机制

文件事件系统不是数据库事务日志。为了提高鲁棒性，需要一个“重新确认真实磁盘状态”的能力。

至少在以下情况触发：

- resume。
- event queue overflow。
- watcher 报告可能丢事件。
- sleep/wake。
- reset baseline 完成后必要的稳定检查。

Reconciliation 不等于每 100ms 全盘轮询。它是异常/边界条件下的恢复机制。

---

## 12. Rename 的现实处理

跨编辑器、跨文件系统、跨 OS 的 rename 语义不总是可靠。

v1 策略：

1. 如果底层事件和文件 identity 足以关联旧路径/新路径，则产生 `Renamed`。
2. 不能可靠关联时，输出 `Deleted(old) + Added(new)`。
3. **禁止为了“看起来像 rename”根据相似内容做昂贵且不确定的全目录猜测。**

这种降级是可接受行为，README 中应说明。

---

## 13. 大文件与资源限制

必须在代码中存在硬边界。

建议配置概念：

```text
classifierProbeBytes
maxInMemorySnapshotBytes
maxSnapshotBytes
maxDiffBytes
maxDiffLines
maxPendingEvents
maxConcurrentReads
maxConcurrentDiffs
```

具体默认值在 P12 测试后确定。

超过限制时：

- changed list 仍显示文件。
- 展开显示 “File changed; diff skipped because file exceeds configured limit.”
- 不崩溃，不无限读取。

---

## 14. 安全与隐私要求

FolderWatch 会读取用户选择目录的内容，因此必须采用最小权限原则。

- 默认纯本地运行。
- 默认不联网、不上传文件内容。
- 日志不得写入完整文件内容。
- snapshot temp cache 只对当前用户可访问；关闭时清理。
- path 必须限制在 session root 范围内，GUI 请求不能越界读取 `../../`。
- 不跟随 symlink 目录默认可降低 root 越界风险。
- 外部编辑器启动禁止 shell 注入。
- 配置文件中不得保存不必要的文件内容。

---

## 15. UI 状态与文案规范

统一状态名，避免 TUI/GUI 各自发明：

```text
Idle
Scanning
Monitoring
Paused
Stopping
Error
```

变化状态：

```text
A  Added
M  Modified
D  Deleted
R  Renamed
```

用户可见错误要说明动作，例如：

```text
Cannot read "secret.txt": permission denied.
Monitoring continues for other files.
```

而不是只显示：

```text
EPERM
```

---

## 16. Definition of Done（所有 P 通用）

一个 P 只有同时满足以下条件才算完成：

- 代码已合并并通过 review。
- 新功能有自动化测试。
- 关键错误路径有测试。
- `go test ./...` 通过。
- 涉及并发时 `go test -race ./...` 通过。
- 文档/ADR 与实际实现一致。
- 没有未记录的 TODO 作为隐藏依赖。
- 若改动用户行为，README / CHANGELOG 已更新。
- 若引入新依赖，记录理由和 license 风险。
- 下一阶段开发者不需要询问上一位开发者才能知道如何继续。

---

## 17. Bug 优先级

### P0 — Blocker

- 数据破坏。
- 任意路径安全漏洞。
- 正常使用频繁 crash。
- 无限内存/CPU 增长。
- Reset 导致 baseline 错乱并无法恢复。

### P1 — Critical

- 常见编辑器保存检测不到。
- changed list 明显错误且不会自动恢复。
- diff 显示错误文件。
- watcher 长时间运行后失效。

### P2 — Major

- 特定边界路径失败。
- rename 降级不理想。
- 视觉/交互问题但核心功能可用。

### P3 — Minor

- 文案、样式、小体验问题。

Gate A / B 不允许存在未接受的 P0/P1。

---

## 18. 开发者交接模板

每位开发者结束一个 P 或交棒时，必须在 PR/issue 中填写：

```markdown
## Handoff

Phase/P:
Branch/PR:
Commit:

### Completed
- 

### Behavior decisions
- 

### Tests added
- 

### Commands verified
- go test ./...
- go test -race ./...

### Known issues
- 

### Deferred intentionally
- 

### Files/modules touched
- 

### Next developer should start with
1. 
2. 
3. 

### Risk notes
- 
```

如果行为与本 Handoff 不一致，必须同步更新 ADR/Handoff；禁止只在聊天或口头交接。

---

## 19. Code Review Checklist

Reviewer 至少检查：

- core 是否错误依赖 UI 包。
- 是否存在 UI render 中读文件/算 diff。
- 是否存在不受控 goroutine。
- channel 是否可能永久阻塞。
- path 是否 normalize 并限制在 root。
- 是否把二进制读成字符串。
- 是否有无上限读文件。
- reset/pause/stop 是否存在竞态。
- 新目录是否正确加入 watch。
- ignore 规则是否 initial/runtime 一致。
- 错误是否被吞掉。
- 新行为是否有测试。

---

## 20. Round 与项目管理映射

详细 Round 定义见 **2.1–2.13**。统一使用：

```text
R1   Foundation & Input       P0–P2
R2   Watch & Baseline         P3–P5
R3   Semantic Change          P6–P8
R4   Terminal Product         P9–P11
R5   Terminal Release         P12–P14  → Gate A

R6   GUI Shell & IPC          P15–P16
R7   GUI Core Experience      P17–P18
R8   GUI Completion           P19–P21
R9   GUI Release              P22–P24  → Gate B
R10  v1 Handoff               P25
```

### 20.1 Round 状态

统一状态：

```text
NOT_STARTED
IN_PROGRESS
IN_REVIEW
ACCEPTANCE
PASS
FAIL
BLOCKED
```

只有 `PASS` 能作为下一轮的可靠依赖。

### 20.2 多人并行原则

R3 内 Classifier、Diff Engine 可由不同开发者并行，但最终必须经同一 Change Resolver/Store 验收。R4 可以提前用 interface mock 搭建 TUI，但禁止自行维护另一套 change state。

GUI 同理：R6 可并行开发前端壳和 Go facade，R7 只能依赖 R6 已验收的 DTO/Event contract。若协议需要破坏性调整，应回到 R6 更新 contract 与验收记录，而不是在前端增加隐式兼容逻辑。

### 20.3 建议分支/PR 粒度

- 一个 Round 可拆多个 feature PR。
- 每个 PR 解决一个清晰能力或测试面。
- Round 结束用 integration/release PR 汇总。
- Gate A / Gate B 对应独立验收 PR 或 release-candidate commit。
- 禁止把 R6–R8 GUI 代码混入 Gate A 修复分支。

---

## 21. 最终产品验收场景

### 场景 A：普通文本修改

1. 目录有 `A.txt`。
2. 启动 FolderWatch。
3. 修改一行。
4. 列表显示 `M A.txt`。
5. 展开显示删除红色、新增绿色。
6. 改回原文。
7. `A.txt` 自动从列表消失。

### 场景 B：多类型文件

目录有：

```text
A.txt
B.go
C.py
logo.png
archive.zip
```

修改任意文件都必须出现在 list；文本显示 diff，二进制显示 metadata change。

### 场景 C：Atomic Save

用 VS Code/Vim 等保存，尽管底层发生 rename/replace，最终只显示用户关心的目标文件状态。

### 场景 D：新增后删除

baseline 不存在 `new.txt` → 新建后显示 Added → 在 reset 前删除 → change 自动消失。

### 场景 E：删除后恢复

baseline 有 `old.txt` → 删除显示 Deleted → 恢复与 baseline 相同内容 → change 消失。

### 场景 F：Reset

有 5 个 change → Reset Baseline → 列表清空 → 再改其中一个 → diff before 是 reset 后内容。

### 场景 G：大文件

超过 diff 限制 → 文件变化仍出现 → 不全文 diff → UI 无卡死/OOM。

### 场景 H：GUI

Open Folder → Monitoring → 编辑文件 → 左侧列表出现 → 点击文件 → 右侧 Monaco diff → Reset → 清空。

---

## 22. 未来修改本规划的规则

本文件不是不可变的，但任何影响以下内容的修改必须写 ADR：

- baseline 语义。
- watcher 策略。
- ignore 默认行为。
- snapshot 存储策略。
- diff 引擎/算法。
- TUI/GUI 与 core 的分层。
- GUI IPC contract。
- path/symlink 安全策略。
- 大文件限制。

ADR 必须写：Context、Decision、Alternatives、Consequences、Migration。

---

## 23. 下一位开发者从哪里开始（R6 集成后）

P0–P14 与 Gate A 已验收通过；R6/P15–P16 已实现并通过本地自动化，**下一能力轮次是 R7 / P17–P18（Folder Picker、变化列表与 Monaco）**。先完成 R6 PR 评审/合并及明确列出的原生交互验证，再从集成后的 main 开始 R7；不要把 IN_REVIEW 写成已合并 PASS。

1. 阅读第30节、R6 acceptance、`gui/README.md`、`docs/gui-ipc-v1.md`、ADR-015，并保留第29节/ADR-009、014 的 Terminal 与 vendor 约束。R6 基线为 PR #6 合并 `88b3541`。
2. 运行 `make lint scripts-test test race smoke`、六类 fuzz、`make gui-setup gui-check gui-build` 与真实 `gui-e2e`；核对最终 head 的实际 CI，Windows 仍只声明 headless compile。
3. R7 必须使用生成的绑定：每次异步请求携带 clientId/sessionId，列表续页携带 generation/global version，Diff 使用选中路径的 version。事件是 invalidation，不是完整列表；旧 client/sequence/selection 回复必须丢弃。
4. 保持唯一 ChangeStore、Classifier、Snapshot、Diff；Removed 是“不再变化”而不是 Deleted。不要把后台全量 Diff 或 JS 文件扫描放进 render、store 或 heartbeat。变更摘要不含正文。
5. 保留 native Close fence/reader join/fd 清理和根目录 no-follow 策略；依赖刷新不得抹去 fsnotify 补丁。单 root、单 core session，启动/停止/重载均需等待资源回收。
6. 2 秒心跳/15 秒租约在休眠或 WebView 长时间停顿后可能主动停止监控；R8 负责后台/睡眠 UX，不得悄悄自动恢复。小时级 soak、旧版 macOS、网络盘等仍需另行测量。
7. Gate A 签字仍对应原 Terminal 候选，不以 GUI 二进制替代。R7 已实现原生 picker/list/只读 Monaco；R8 承接 settings/editor/reveal；R9 才完成签名、公证、安装与 Gate B。当前并非正式 GUI v1。

项目的核心价值不是“终端上有颜色”，而是：**文件事件再混乱，最终仍然能稳定、正确、可恢复地告诉用户“相对于 baseline，到底哪些文件变了，以及变了什么”。**

---

## 24. 交付结论

完成 P0–P14 后，应得到一个可以独立发布的 Terminal/TUI 产品；完成 Gate A 后，P15–P24 将相同的 Go core 包装为 Wails + Svelte GUI；P25 负责正式冻结 v1 行为、文档与后续路线。

任何程序员接手时，优先相信：

1. 自动化测试；
2. ADR；
3. 本 Handoff；
4. 当前代码行为；

若四者冲突，必须在继续开发前通过 issue/PR 统一事实，并更新文档，避免“口头正确、代码另一套”的长期漂移。

---

## 25. 历史交付记录 — R1 / P0–P2（2026-10-01）

本节保留R1时点的交付和未完成功能；当前完成范围以第30节R6为准。

### 25.1 实际完成范围

本轮从仅有交接文档的目录开始。原始文档完整保存在首个提交 `9fca76f3d72ef02fb656c70a4bddc7c2ff076186`。初始化 `main` 分支，工程目标仓库为私有 `https://github.com/StevenWinsir/FolderWatch`；本轮 checkpoint 使用 `r1-complete`，精确最终提交可通过该 tag 查询。

| 阶段 | 本轮交付 | 核心文件 |
|---|---|---|
| P0 | Go module/依赖锁定，薄 main、Makefile、CI、提交约定、README/CHANGELOG、六份 ADR | go.mod/go.sum、cmd/folderwatch、tools/deps.go、.github/workflows/ci.yml、docs/adr |
| P1 | CLI、严格 TOML、四层优先级、显式 false/空数组、duration/size/path 校验、help/version、错误码、转义输出 | internal/cli、internal/config、testdata/config.example.toml |
| P2 | 元数据递归扫描、canonical key、symlink/特殊文件策略、共享 Ignore Matcher、嵌套规则、权限/消失 warning | internal/pathutil、fileutil、ignore、model、scan、app |
| 测试与交接 | 单元/集成、Git 对照、race/fuzz、真实 CLI smoke、验收报告、原 Handoff 就地更新 | 各包 *_test.go、scripts/smoke.py、docs/rounds/R1-acceptance.md、本文件 |

### 25.2 本轮可运行行为

```sh
make build
./bin/folderwatch --help
./bin/folderwatch --scan --json "/path/to/项目 with spaces"
./bin/folderwatch . --ignore '*.tmp' --ignore 'node_modules/'
./bin/folderwatch --scan --respect-gitignore .
```

清单包含根 `.`、目录、普通文件、symlink 与特殊文件元数据，不读取正文，不创建状态/log/snapshot。JSON 返回 `root/entries/warnings`。任何扩展名和大小的普通文件均可进入清单，`max_diff_bytes` 不是扫描过滤器。输出中的路径控制字符会转义。错误码为 0 成功（可有 warning）、2 参数/配置、1 runtime/I/O、130 取消。

### 25.3 已确认的实现约定

默认 `.git/` 排除、`.gitignore` 默认关闭、规则来源顺序与父目录否定限制已固定；Git 全局 excludes、`.git/info/exclude` 和 tracked 状态不参与匹配。`.folderwatchignore` 不会被额外 ignore 文件取代。自动规则文件不跟随 symlink，配置/规则读取有 1 MiB 上限。

目录自己的 `.gitignore` 在进入目录前预检，仅影响其后代，不影响该目录自身。Ignore 缓存包括 missing/error；新目录首次出现时读取规则，但已缓存规则修改不热更新。R2 如新增热更新，必须统一重建 Matcher 并 reconciliation。

### 25.4 本轮验证与修复

环境：macOS 26.6.2、darwin/arm64、Go 1.26.6、Git 2.54.0。本地 `go build ./...`、gofmt/go vet、`go mod verify`、完整 Go tests、race 及 CLI smoke 均通过；127 个测试/子测试（38 个顶层测试及 fuzz 目标）无失败，60 项 smoke 无跳过。整体语句覆盖率 88.4%，扫描 96.1%，Ignore 94.0%。macOS arm64/amd64、Linux amd64、Windows amd64 交叉编译成功；不据此宣称 Windows 运行支持。Fuzz/远端 CI 的确切结果与运行链接记录在 R1 acceptance 的交付补记。

远端验证已实际通过：主实现提交 `8b59f7c92af293ec13fe1421983ff618abb7ec78` 的 [GitHub Actions 36956805061](https://github.com/StevenWinsir/FolderWatch/actions/runs/36956805061) 为 `success`，5 个 job 全部成功。覆盖 macOS/Linux × Go 1.23/1.26 的 lint/build/test/race/CLI smoke，以及 macOS/Windows 交叉编译。之后的收尾提交只补充验收文档，不更改实现；完整交付由 `r1-complete` 标记。

真实测试覆盖 Unicode/空格路径、根与后代 symlink、符号链接环、权限不足、文件消失、FIFO、5 GiB 稀疏大文件、配置不落盘、输出错误与取消。通过 16 组规则 × 22 条路径的本机 `git check-ignore` 对照修复 trailing `/**` 对父目录的错误匹配；同步修复嵌套规则预检、错误目录类型提示下的 symlink 越界、空 CLI root、低优先级非法 glob 被覆盖，以及最后一次扫描回调中的取消丢失。首轮失败保留为开发事实，最终通过结果有复现命令。

### 25.5 明确未完成与下一轮责任

**尚未实现：P3 watcher、P4 debounce/coalesce、P5 snapshot/baseline/reset，以及 P6–P25。** 没有持续监控、内容分类/diff、ChangeStore、TUI、GUI、外部编辑器执行或文件日志。保留参数仅定义配置接口；baseline/大文件的 ADR 部分是后续设计，不是假实现。Gate A/B 未通过。

本轮未发现测试范围内未解决的阻塞 bug。接受的边界是：Ignore 修改需重启；warning 清单可能不完整；内存随条目数增长；路径检查不是对抗并发恶意目录替换的原子沙箱。P12 性能预算、持续监控压力测试、独立人工评审、Terminal/iTerm 交互验收与签名分发尚未执行，不记为本轮已完成。

下一轮直接复用 `Prepared.Config`、`Prepared.Matcher`、`Prepared.Inventory`，实施 P3–P5 并保持 core/UI 分离。详细下一步与风险见 R1 acceptance 和第 23 节。

---

## 26. 历史交付记录 — R2 / P3–P5（2026-10-01 至 2026-10-02）

本节保留 R2 当时功能与验收，以下“未实现 R3”是历史时点。R3 期间已核实 PR #1 合并及未创建修订标签的事实，并原文修正；当前功能看第 28 节。

### 26.1 完成范围与入口

从 R1 `720ecdb06044145a0f9f8a15b0220a0b4be45113` 开始，开发分支 `feat/r2-watch-baseline`，目标仍为私有 `StevenWinsir/FolderWatch`。实现提交 `9d7b675419ab0f79ea46c43ace8b7594bb57b6db` 已推送，GitHub CI 5 个 job 全部成功。R2 当时预告的 `r2-complete.1` 实际未创建/推送；最终修复以 `32529a7dad7014c5cb54f2aff8791a8969e66bc7` 为准，现已合入 main 的 `1dbdb5b`。首次 `r2-complete` 是修复前历史，不能作为最终 R2 起点，不改写该旧标签。

R2 当轮 PR 创建曾受阻；后续仓库所有者流程已完成 [PR #1](https://github.com/StevenWinsir/FolderWatch/pull/1) 合并（2026-10-02 06:00:44 UTC）。R3 实际核对 main 的 merge parents 和源码树，与最终 R2 修复相同；不再沿用“main 仍为 R1”的过期状态，也不据合并推断额外人工验收。

| 阶段 | 本轮实现 | 主要位置 |
|---|---|---|
| P3 | Watcher interface、递归 fsnotify、动态目录、identity、忽略、背压/容量错误与关闭 | internal/watcher |
| P4 | canonical path invalidation、CHMOD hint、单 ticker 有界 debounce/coalesce、root reconciliation | internal/eventnorm、internal/debounce |
| P5 | SHA-256、有界 memory/disk、内容资格探针、Ref 所有权、原子 baseline generation/reset/cleanup | internal/snapshot |
| 应用/CLI | Session 启停、排队 Reset、代际事件、--watch 文本/NDJSON、共享资源配置 | internal/app、internal/cli、internal/config |
| 加固/交接 | 目录 identity Ignore 缓存退役、fsnotify kqueue 补丁、vendor guard、测试、ADR-007–009 | internal/ignore、vendor、patches、scripts、docs |

```sh
make build
./bin/folderwatch --scan --json .
./bin/folderwatch --watch --json --respect-gitignore "/path/to/项目 with spaces"
# Ctrl+C 关闭监听并清理；ResetBaseline(ctx) 是 Go API，交互按键留给 R4。
```

### 26.2 固定的行为与资源边界

事件是“需要重新解析”的请求，不是 R3 语义 changed list。新建/移入目录窗口里的文件、原始队列满、慢消费者都会转成明确的 root reconciliation。默认 raw/pending 上限 4096，coalescer 输出 1、应用输出 32、warning 通道 16；不使用 per-path goroutine/timer。目录上限 8192，耗尽或 root 丢失则 fatal。

基线默认最多 100000 个引用，32MiB memory + 256MiB disk/代；64KiB 以下小文本优先内存，单文件保留上限复用 5MiB max-diff-bytes。二进制/非法 UTF-8、超限或预算不足只保留 metadata/hash。regular file 的 hash 采用有界块读取、取消及稳定性检查；symlink 只保留链接本身/目标字符串 hash，FIFO/device 不读内容。cache 位于 root 外 OS temp，目录 0700、文件 0600。

Reset 先构建下一代，成功后一次发布；失败或取消保留旧代。普通保存不推进基线。成功后旧 Ref 失效，排队事件按新 generation 重新解析。Reset 峰值可持有两代有界内容；不是跨整个文件系统的瞬时事务，capture 窗口内变动需排队事件/reconciliation 接续核对。扫描清单和调用方读取副本不属于 retained-content budget，内存仍与清单规模相关。

`--scan` 保留 R1 warning 后继续的行为；watch 启动/Reset 要求完整可读基线，出现扫描 warning/内容读取失败明确报错，不假装成功或抹掉旧路径。已有目录的 Ignore 文件仍不热更新；目录 identity 删除/替换会退役旧 scope，防止无限缓存和同名新目录沿用旧规则。

### 26.3 本轮测试与修复

本地 macOS 26.6.2 / Go 1.26.6 / darwin-arm64：完整 Go tests、全量 `-race -count=10`、R1 CLI 60 项与 R2 watch 17 项冒烟均通过；最终语句覆盖率 84.3%。实际 Vim backupcopy=yes/no 无界面保存通过，直接写入/atomic replacement 模型通过；本机无 VS Code，未宣称实际 VS Code GUI 验收。覆盖率、fuzz、跨编译与远端 CI 证据详见 R2 acceptance。

发现并修复：kqueue 退役描述符空路径、内部 symlink 跟随、skipped-link seen 缓存未退役、目录通知回退缺失、目录迁移/重建登记、Ignore 生命周期、等待快照所有权时不可取消、等待基线发布锁期间取消仍可能提交等边界。fsnotify 保持 v1.8.0，只有 kqueue 后端的受审查 vendor 补丁；附 125 行 unified patch、上游/补丁 SHA-256 及可逆/构建源路径校验。所有测试在修改后复跑，不将初期失败伪装为一次全过。

远端证据：[GitHub Actions 36968383149](https://github.com/StevenWinsir/FolderWatch/actions/runs/36968383149)，head=`9d7b675419ab0f79ea46c43ace8b7594bb57b6db`，结论 success。macOS/Linux × Go 1.23/1.26 的四个 test job 和 cross-build 全部成功。后续交付补记只更新文档；最终分支/tag 的检查以 Actions 和验收报告为准。

补充回归：首次标签 [CI 36968813218](https://github.com/StevenWinsir/FolderWatch/actions/runs/36968813218) 的 macOS/Go 1.23 重复目录迁移测试遇到 `fsnotify.dirChange: no such file or directory`。修复将 native 子项消失归类为 root reconciliation（不能静默丢弃），其他错误仍发 warning；新增确定性测试，修复后 Watcher `-race -count=20` 与完整 build/test/race/lint/smoke 再次通过。修订标签 `r2-complete.1` 当时只预告、未实际创建；R2 最终修复提交为 `32529a7`，其 [CI 36969190085](https://github.com/StevenWinsir/FolderWatch/actions/runs/36969190085) 通过，旧标签没有重写。

### 26.4 明确未完成与下一轮责任

P6–P8 Classifier/Diff/ChangeStore 尚未实现；没有 Changed List、diff、重命名语义判定或“恢复后列表消失”的 UI。P9 以后 TUI、Pause/Resume、交互 Reset、日志、编辑器集成、GUI 和发布均未开始。Gate A / B 仍未通过。

接受的已知边界：严格完整基线；无 Ignore 热更新；异常强杀可能残留私有缓存且未自动清扫；Unix 路径防护不是对抗并发恶意替换的原子沙箱；kqueue 描述符随文件数增加；没有完成 P12 性能预算、小时级监控、实际 VS Code GUI 或独立人工评审。下一轮复用稳定 core，所有错误/重扫/generation 语义必须保留，禁止用 UI 绕过。

---

## 27. 历史交付记录 — R3 / P6–P8（2026-10-02）

本节保留 R3 交付时的功能范围、验证和后续项；“未实现 TUI”是历史时点，当前范围看第 28 节。R4 已实查 PR #2 于 2026-10-02 07:08:58 UTC 合并为 `1b52fcc`，源码树与 R3 最终 `c0f626b` 相同；旧的待合并状态已原文更新。

### 27.1 实际完成范围

本轮从 R2 最终 `32529a7` 开始，在 `feat/r3-classify-diff-changes` 开发。核对到 R2 已合并后，将本分支 fast-forward 到源码树完全相同的 `origin/main` 合并提交 `1dbdb5bfdb99f1f1e2f04a6f366e3e78fc640627`，保留全部本轮修改，没有改写共享历史。最终 R3 提交、标签与远端 CI/PR 以 R3 acceptance 的交付补记为准。

| 阶段 | 本轮已实现 | 主要位置 |
|---|---|---|
| P6 | 扩展名无关有界分类、编码状态、共享全流 Probe、快照不可变分类、独立分类/保留与 Diff 上限 | internal/filetype、snapshot、config |
| P7 | 可取消有界 LCS、公共首尾裁剪/行 ID、增删上下文/hunk/行号/末尾换行、大小/工作降级、按需版本化 Diff | internal/diff、changes/diff.go |
| P8 | 唯一状态表、批量 Resolve/重查、恢复消失与完整迁移、rename fallback、错误保护、代际/版本批次 | internal/changes、app/semantic.go |
| 集成 | Session Changes/ChangeState/GetDiff、原子 Reset 清表、重试退避、CLI A/M/D 与语义 NDJSON、Reload 水位保护 | internal/app、internal/cli |
| 验证/交接 | 单元/真实FS/并发/黄金/模糊/CLI、新旧回归、ADR-010–012、原 Handoff 就地更新、R3 acceptance | 各测试包、scripts、docs |

### 27.2 运行方式与稳定接口

```sh
make build
./bin/folderwatch --scan --json .
./bin/folderwatch --watch --json --respect-gitignore "/path/to/项目 with spaces"
```

默认/scan 行为未改，watch 才持续监听。文本模式显示 A/M/D、路径及分类；NDJSON 首条 ready/state，之后 changes.batch 的 upserts/removed，必要时附 state 重新加载。事件不含正文、Diff 全文或缓存路径。Removed 表示恢复基线/先增后删后“不再变化”；真正删除是 Kind=deleted。

Go API：`Session.Changes()` 返回排序副本；`ChangeState()` 返回列表、generation/version；`GetDiff(ctx,path)` 按需返回结构化差异；`ResetBaseline(ctx)` 原子换代并清空语义状态。GetDiff 用旧基线和当前 hash 校验，不拿最新内容伪造 before；文件/基线版本变化返回 ErrStale。未变化返回 ErrNotChanged，未保留内容返回 Unavailable。

### 27.3 行为和资源决策

分类/保留默认 8MiB（max_snapshot_bytes），Diff 每侧默认 5MiB / 20000 行，独立配置。Diff 默认 2000000 个 uint32 LCS 单元，密集大跨度变化可能触发计算预算 TooLarge；并非任何小于5MiB的文件都保证计算完整差异。二进制/未支持编码/特殊文件不文本渲染。CRLF、Unicode 与无末尾换行字节保留。

Current Resolver 不常驻全文：一个 metadata-only scratch store；按需 Diff 有独立单引用 scratch/token，不阻塞解析所有权。正常三个私有目录，Reset 峰值两代有界基线加一个有界 Diff 快照；扫描/矩阵/调用方副本另计。没有无限事件/goroutine/定时器，失败重查仅一个100ms–2s退避计时器，成功后停止。快照、Matcher、Config 不得跨 root 混用。

读取失败保留最后已知状态并 warning，不把不可读子树误删；下一次成功重查纠正。排除但仍存在的文件不误报 Deleted。rename 本轮一律可靠降级删除+新增，不声称已实现 Renamed 识别。慢消费者 Reload 后按版本丢弃旧批次；P9 UI 只能展示，不能重算状态。

### 27.4 测试结果

本机 macOS 26.6.2 / darwin-arm64 / Go 1.26.6。`make build test race lint smoke` 通过；248 个 Go 测试/子测试（101 个顶层测试/模糊目标），无失败和测试级跳过；全量 race 连续10轮通过。修订测试后再次全量10轮的整体语句覆盖率86.1%，Classifier93.5%、Diff94.2%、ChangeStore84.4%。9个黄金文件、R1扫描60项、R2监听17项、R3语义16项冒烟通过；实际Vim两种保存继续通过。

5秒分类 fuzz 执行1,061,660次、Diff重构 fuzz执行911,178次，均PASS；这是有界找错实验，不是形式证明或性能指标。macOS arm64/amd64、Linux amd64、Windows amd64交叉编译和go mod verify通过。R3未新增外部依赖，R2 vendor补丁保持原样并通过来源/校验守卫。

本轮复核修复/防护包括：共享分类消除双探针、巨大文件避免无谓文本探测、混用root拒绝、瞬时消失/不可读保护、恢复/Reset代际清理、旧Diff失效、消费者Reload后旧批次回退防护。测试与源码已实际复跑，不将尚未观察的远端CI或人工QA写为通过。

首次推送 CI 的 Ubuntu/Go1.23 揭示实时 Diff 集成测试错误假定“kind=Modified 即版本固定”。生产 GetDiff 合法返回 ErrStale；测试改为限时且仅重试 ErrStale，保留所有最终断言及确定性 stale 拒绝测试，未移除生产版本保护。修订后针对用例50轮race、全量10轮race和完整构建/三组smoke再次通过。首次失败链接及修订远端结论见R3 acceptance。

远端已验证：修订提交 `15e9c4dbf11b3bd4076e3ff342b3f1b78c8ccfa3` 的 [分支 CI 36975768688](https://github.com/StevenWinsir/FolderWatch/actions/runs/36975768688) 与 [PR CI 36975771977](https://github.com/StevenWinsir/FolderWatch/actions/runs/36975771977) 均 success，各5个job全绿。覆盖macOS/Linux×Go1.23/1.26的build/test/race/核心重复/三组smoke及cross-build。R3 交付时 [PR #2](https://github.com/StevenWinsir/FolderWatch/pull/2) 未直接写入 main；后续已由仓库流程于 2026-10-02 07:08:58 UTC 合并为 `1b52fcc`。最后证据补记只改文档，R3 checkpoint 由 `r3-complete` 标识。精确SHA/后续运行以远端引用及R3 acceptance为准。

### 27.5 明确未完成与后续

P9–P11 TUI、交互Diff/Reset、Pause/Resume、日志/编辑器均未实现；GUI与发布Gate尚未开始。GetDiff和Reset是Go API，不是已有鼠标/键盘UI。R5性能预算、长期监控、真实VS Code GUI和Terminal/iTerm人工验收未执行。强杀残留缓存自动清扫、Ignore热更新、更高效大跨度Diff/缓存、可靠rename关联仍为后续候选，详见ADR与R3 acceptance。R3的实现/自动化验收与PR主分支集成状态分开记录。

---

## 28. 历史交付记录 — R4 / P9–P11（2026-10-02）

### 28.1 起点、范围与集成状态

本轮完整阅读交接后，先核实本地无用户未提交改动及 R3 PR #2 已合并。R3 head=`c0f626b63735fba6ae43feaeb0c860089ffad0ea`，main merge=`1b52fcc23d199f6962206205e585c1d695cc0bae`（2026-10-02 07:08:58 UTC），源码树一致。从该 main 建立 `feat/r4-terminal-tui`，没有重写旧标签、直接提交 main 或提前开发 GUI。

**当前范围：P0–P11 已实现；R4 实现、本地及远端 CI 自动化验收 PASS，Round 集成状态 IN_REVIEW。** 实现提交 `305b097005df6635606c68eaa5ebe671e2be7e4d` 已推送，并创建 [PR #3](https://github.com/StevenWinsir/FolderWatch/pull/3)，目标 main。实查 PR #3 已于 2026-10-02 08:23:07 UTC 合并为 `bb3a75eba5d36d18c461de3fcb2e97d61d95844a`，合入 head=`86b2191`；本轮没有调用自动合并。随后从已合并 main 建立 `fix/r4-native-test-ordering`，单独提交原生测试同步/交接补充，不向已合并分支追加提交冒充集成。详见本节及 [R4 acceptance](docs/rounds/R4-acceptance.md)；实际终端人工 QA 与 R5 Gate A 仍未通过。

| P / 能力 | 本轮完成内容 | 主要位置 |
|---|---|---|
| P9 文件列表 | A/M/D/R与独立展开标识、数量/状态、保持选择、0/1/120条模型与112条PTY、过滤/帮助、极小到宽屏resize | internal/tui/model.go、view.go |
| P9 交互 | ↑↓/j/k、Enter/Space、p/r/?/q、过滤、分页/横向、鼠标click/wheel；--no-mouse主动禁用并忽略报告 | internal/tui、internal/cli |
| P10 Diff | 单一选中unified预览、可取消单请求、epoch/path/generation/version防旧结果、有限stale重试、hunk/双侧行号/末尾换行/红绿与+/- | internal/tui/model.go、diff.go |
| P10 安全降级 | Binary/Unsupported/TooLarge/Unavailable、尺寸/原因、NO_COLOR、控制字符转义、Unicode格裁切、16MiB/50000行/单行约16KiB预览上限 | internal/tui/view.go、diff.go |
| P11 控制 | core串行Pause/Resume/Reset、暂停保留watcher/冻结列表、恢复全root校准/同baseline、确认式Reset、暂停中Reset/失败保持旧代 | internal/app/control.go、session.go、semantic.go |
| P11 错误/日志 | warning详情/继续、fatal非0退出、q=0/Ctrl+C=130及终端/cache恢复；200条ring、显式root外新建0600/4MiB JSONL、不含正文、不串入TUI/NDJSON | internal/logging、internal/tui、internal/cli |
| 回归/交接 | model/golden/control/logging/CLI/PTY测试，make smoke与CI接入，ADR-013、README/CHANGELOG/配置例、R3合并事实原文更新 | 各测试、scripts、docs、本文件 |

### 28.2 运行与交互

```sh
make build
./bin/folderwatch "/path/to/项目 with spaces"
./bin/folderwatch --tui --no-mouse .
NO_COLOR=1 ./bin/folderwatch --tui .
./bin/folderwatch --scan --json .
./bin/folderwatch --watch --json .
# 必须是 root 外尚不存在的新文件；父目录需已存在。
./bin/folderwatch --debug --log-file /tmp/folderwatch-new-session.jsonl .
```

stdin/stdout均为TTY时默认TUI；任一重定向则默认仍扫描一次。--tui无终端或与scan/watch/json混用返回输入错误；--scan/--json明确保留诊断入口。第一次metadata/config准备后，baseline异步建立，Scanning可取消。

列表用上下/jk选择，Enter/Space展开收起；PgUp/PgDn或Ctrl+u/d翻Diff页，Home/End/g/G跳首尾，左右/hl横向滚动。`/`先进入过滤再输入，Enter应用，Esc撤回输入/清除过滤。?帮助、e详情可滚动；p暂停/恢复，r后y/Enter确认Reset，其余键取消；q或Ctrl+C退出。单选Diff布局不常驻多个全文。R标记虽可展示，core仍采用Deleted+Added的rename降级。

### 28.3 关键语义、隐私和资源

所有文件状态仍由唯一 ChangeStore 决定；UI每次通知取权威View并防版本回退，View/Update不读文件或计算Diff。只允许一个Diff命令在途，切换/更新/Reset会取消并递增epoch，旧命令回收后才请求最新选择；三次stale重试有明确上限，预览截断会告知。core分类8MiB与Diff5MiB/20000行/200万工作单元继续独立。

Pause冻结最后已知状态而不是销毁watcher，队列有界排空，不积累全量事件历史；Resume先全root校准，对比同一baseline。Reset使用原子API，失败不清表；暂停中Reset仍暂停。root丢失在暂停时仍fatal。关闭竞态的backend错误在app门面统一，不把已成功提交Reset改报失败。

debug仅含元数据。默认日志在内存200条ring，显式日志必须root外NEW文件，O_EXCL/0600不覆盖已有数据，拒绝symlink/大小写身份别名；单条约2KiB，文件4MiB或I/O错误后停止写文件、保留ring并提示。路径/正文控制符转义防终端注入。安全边界不是对抗恶意并发目录替换的原子沙箱；显式慢日志磁盘也不是硬实时保证。fsnotify vendor补丁原样保留，依赖版本未升级。

### 28.4 实际验证与修复

本机macOS26.6.2 / Apple M4 / darwin-arm64 / Go1.26.6：`make build test race lint smoke`、`go mod tidy -diff`、`go mod verify`通过；**287个测试/子测试、129个顶层测试/模糊目标，失败0/测试级跳过0；最终源码全包race连续10轮PASS**。语句覆盖率85.7%，TUI84.2%、app83.0%、watcher81.7%、logging86.4%。父子测试计数并非独立场景数，真实PTY进程不计入进程内coverage。

R1扫描60项、R2监听17项、R3语义16项、R4真实二进制PTY21项全部通过，实际headless Vim两种保存保留。PTY走通0/1/112文件、1500行滚动、过滤、鼠标/无鼠标、无色/红绿、权限warning/继续、pause/resume/reset、baseline before变更、启动中退出、q/Ctrl+C/fatal与缓存/终端恢复。只掩蔽macOS内核维护的PENDIN位，其余终端flags/控制字符/速率、光标与alt screen严格检查。**这不是实际Terminal.app/iTerm2人工验收。**

macOS arm64/amd64、Linux amd64、Windows amd64交叉编译通过；Windows只声明compile。1500行、120×40视口孤立View微基准为55,925ns/op、5,162B/op、136allocs/op，不是P12目录级/端到端性能Gate。详细命令与结果见R4 acceptance。

开发过程发现并修复：控制/Close并发泄漏watcher closed；Diff错误预览阻止真正重试；no-mouse残留报告；日志root大小写别名。旧scan-only帮助断言按明确TTY/管道分流契约更新，实际扫描断言全保留。PTY过滤输入改为先进入过滤再提交粘贴查询，并正确区分内核PENDIN位；未删除生产版本保护或正确性测试来掩盖问题。所有最终源码/测试/脚本均已复跑。

首次 [分支 CI 36980505335](https://github.com/StevenWinsir/FolderWatch/actions/runs/36980505335) 暴露 PTY 测试继承 `CI=true` 导致 termenv 按设计关闭自动颜色，而断言期望红绿输出。本机先带CI变量重现，再仅隔离PTY子进程的CI/颜色偏好环境；保留全部颜色/无色/内容/恢复断言，未改生产行为或依赖。带CI及禁用颜色变量的外部环境复测21项通过；详细修订验证见R4 acceptance。

随后 `e3b22c0` 的 [PR CI 36981497710](https://github.com/StevenWinsir/FolderWatch/actions/runs/36981497710) 中，macOS两个Go版本通过，Linux在暂停时删除作为cwd的root后没有退出。Linux持有目录引用时可以完全没有root原生事件，不能只检查Chmod。修复在watcher责任层：每个既有事件循环增加一个1秒ticker，只做root的Lstat/目录身份比较；不扫描子树、不读正文、不推进暂停语义状态，退出停止ticker。新增无原生订阅时健康检查不发语义事件/根消失仍fatal、保留open-directory引用、root通知身份回归，保留原PTY断言。1秒是调度间隔，不是慢文件系统上的硬实时承诺。测试fixture使用实际注册的w.root规范路径，不忽略macOS /var→/private/var差异导致的Remove错误。

同时补齐状态消息乱序回归：旧event/control的Status快照不再覆盖更新后的Pause/Resume/Error；UI处理消息时读取core短锁Status，列表仍有generation/version水位保护。最终源码重新执行`CI=true make build test race lint smoke`、全包10轮race/coverage、JSON计数及四target构建，均通过；上方计数已原文更新。修订源码提交 **`9592658bd9a60961c06fc42d00a21e03d5d69cc0`** 已推送：**[PR CI 36982861222](https://github.com/StevenWinsir/FolderWatch/actions/runs/36982861222)** 与 **[push CI 36982857314](https://github.com/StevenWinsir/FolderWatch/actions/runs/36982857314)** 均实测 completed/success；macOS/Linux × Go1.23/1.26四组测试及cross-build全通过，原Linux暂停root删除/终端恢复PTY断言也通过。`a58b9ca`/`86b2191`的证据收尾仅更新Markdown；PR #3随后已合并。后续原生测试同步修订见下文，生产Go源码、脚本、依赖仍与`9592658`一致。未创建完成标签。

**合并后的测试同步补充。** `a58b9ca` 的 PR CI通过，但 [push CI 36983209590](https://github.com/StevenWinsir/FolderWatch/actions/runs/36983209590) 在macOS/Go1.23的旧R2原生测试丢失精确Write断言：kqueue先发送Create，再注册子文件；保留的父目录Write才是注册完成后的事件。测试在创建和重命名后增加父目录Write屏障，未删原文件Write/Rename/Remove断言。100轮压力测试又暴露一次Reconcile后立刻查询WatchList早于原生清理完成；改为有3秒期限的事件收敛后仍严格检查无残留watch。仅对主动删除的`moved`子树（精确路径或分隔符限定后代）的PathError/ENOENT允许继续校准；权限、其他路径、root丢失等仍使测试失败，不广泛吞warning。

最终该场景在`GOMAXPROCS=1`下**race连续100轮PASS**；补充版本重新执行`CI=true make build test race lint smoke`、全包race10轮、JSON计数全部通过，仍为**287/129、失败0/跳过0、85.7%覆盖率**。补充仅改两个测试文件及交接Markdown，生产代码/vendor不变。主PR合并与补充评审分开记录，不将早期偶发失败抹掉，也不将旧CI冒充补充head的CI；补充PR中的检查为其远端验收依据。

### 28.5 R4 时点的未完成与下一轮（历史保留）

**以下为R4交付时点的历史边界，不代表R5当前状态；R5已完成部分见第29节。** 当时P12–P14尚未实施、Gate A未通过，实际Terminal.app/iTerm2/VS Code GUI人工QA、10k/小时级监控/CPU/内存/P95、clean-Mac、打包/签名/分发许可留待后续。没有GUI，editor仍只存储；Ignore热更新、强杀缓存清扫、可靠rename关联、side-by-side/多Diff常驻不是R4能力。

下一轮先关闭`fix/r4-native-test-ordering`补充PR的评审与合并项，再依第23节执行R5；不得用GUI绕过Gate A。尚未创建r4-complete标签，不改写已有checkpoint。当前已执行测试范围内没有未解决实现blocker，集成/人工验收状态单独记录。

---

## 29. 已验收交付记录 — R5 / P12–P14（2026-10-02）

### 29.1 当前结论与集成范围

**P12性能/资源治理、P13测试/CI、P14 Terminal 候选发布与人工验收均已完成；R5=PASS，Gate A=PASS。** `.r5-worktree/dist/v0.1.0-rc.1` 为本次人工 Gate A 验收对象；阶段一 P0–P14 完成。GitHub PR #4 已合并为 `0ebf250`，PR #5 已合并为 main `6a470a0`，因此可从最新 main 进入 R6。完整工程证据见`docs/rounds/R5-acceptance.md`、`docs/performance-v1.md`、`docs/gates/Gate-A.md`及ADR-014。

本轮完整阅读原交接2537行并核实R4主PR #3已合并main `bb3a75e`。原checkout有另一窗口的R4原生测试修复，因此在`/Users/stevenlee/Desktop/diffList/.r5-worktree`创建隔离分支`feat/r5-terminal-release`，未覆盖用户改动。随后快进纳入`a775148decf0655575c0dc2933d2aaddd9d8865f`；该补充随后通过[PR #4](https://github.com/StevenWinsir/FolderWatch/pull/4)合并为`0ebf250`，R5 [PR #5](https://github.com/StevenWinsir/FolderWatch/pull/5)随后合并为main `6a470a0`。

### 29.2 本轮实际完成

| 阶段 | 新增/修订交付 | 验证 |
|---|---|---|
| P12 | `cmd/fwbench`生成隔离1k/10k、100-file burst、5/10MiB文本、64MiB binary、depth32、50次启停；CPU/heap/goroutine profiles与fd/缓存计数；真实binary PTY计时 | 七组精确hash/资源检查PASS；原始JSON、失败样本与profile top纳入`docs/benchmarks/r5` |
| P12 内核修复 | kqueue Close完整owned-fd回收、reader join、并发Close/注册屏障、close-on-exec；小文本capture独占buffer转交，少一次正文复制 | native/app/ownership责任层regression及全包race10轮，vendor patch/hash可逆校验 |
| P13 | 保留全部旧unit/integration/golden/model/真实CLI/PTY；新增overflow/资源/ownership/eventnorm fuzz；六目标fuzz日志；Core传递依赖检查 | 299/137计数、0失败/跳过；fuzz/四smoke全绿 |
| P13 CI | macOS/Linux×Go1.23/1.26、随机race、coverage/log artifacts、fuzz、四target构建、native候选安装/资源smoke | 本机等价命令通过；远端最终head需按实际run记录 |
| P14 工程 | darwin arm64/amd64 SemVer候选、version/full commit/build-date、CGO0/vendor、确定性tar/gzip、manifest/SHA256SUMS/依赖与Go许可证、Homebrew真实checksum formula、私有候选workflow | 双架构校验、arm64隔离HOME/PATH安装/watch/hash/Ctrl+C/cache PASS；未声称干净Mac或公开tap |
| 交接 | README/CHANGELOG原文更新、ADR-014、性能/验收/Gate文档、Handoff当前状态及§29 | Gate A 人工验收已完成并转为 PASS；历史§25–28保留 |

### 29.3 实际发现的资源缺陷与修复

初版Darwin `/dev/fd` ReadDir不可用，fd=-1未被当作无泄漏。改为lsof只数当前进程数值fd后，1000文件场景从**6→1010 active→1007 after Close**，虽然缓存已清空仍真实泄漏。根因为fsnotify kqueue先关闭done，再Remove时closed guard直接返回，所有文件watch fd残留。

修复在原有vendor责任层：单fd注册/移除受doneMu保护，递归在锁外；Close建立关闭屏障、关闭wake pipe、等待reader退出、回收全部owned fd并清空map，其他Close等待同一closeDone。新注册不能越过关闭屏障；kqueue/file fd加close-on-exec；重注册失败不误关旧owned fd。保留原有nofollow/未知fd overflow/目录校准/seen退休补丁。同步更新可复现patch、SHA manifest与ADR，不升级依赖、不在TUI隐藏错误。

新增128文件20轮native启停、并发Close/注册、30轮Session Pause/Reset/Close、100文件overflow/no-consumer精确hash、snapshot防御副本回归。修复后七组测试fd全部回到6、goroutine回到1、cache0。小文本buffer保留仍限额，ReadContent仍防御拷贝；10k微基准累计分配约53.4→43.2MB（少约19%），耗时相近，不宣称无依据的加速。

### 29.4 实测结果

本机macOS26.6.2、Apple M4/10 logical CPU、32GiB、Go1.26.6、系统APFS/SSD。10k×1KiB ready约455ms、owner屏障后settled837ms、GC heap27.29MB、进程峰值RSS72.73MB、10秒idle CPU0.517%。20轮100文件burst：Core权威状态P95=170.31ms；真实120×36 PTY新列表输出P95=196.57ms，均包含150ms debounce；减去名义debounce只是估计，不是内部埋点。达到本机P12参考预算，不能推广至任何硬件/冷缓存/小时级运行。

`go test -count=1 ./...`及JSON/coverage重跑：**299个测试/子测试、137个顶层测试/模糊目标，失败0、测试级跳过0**；完整`go test -race -shuffle=on -count=10 ./...` PASS。`make lint`/build、最终8个Python发布工具回归、R1 60/R2 17/R3 16/R4 21项smoke全部PASS，保留实际headless Vim两种保存和严格终端恢复。六fuzz每目标10秒/2workers全绿，四target darwin arm64/amd64、linux amd64、windows amd64编译通过，`go mod verify`/`go mod tidy -diff`通过；Windows仅compile。

发布预演显式`--allow-dirty`，版本commit带dirty；双架构checksum/Mach-O/安全归档/许可证与native arm64安装通过，PATH只有`/usr/bin:/bin`、HOME/TMP隔离；版本、Unicode扫描、真实修改hash、SIGINT130和空缓存均检查。Homebrew Go许可证位置差异先失败后在release.py补两种布局及regression，缺许可证仍拒绝打包。正式clean候选commit及PR/远端CI在下方实查后记录，不拿dirty预演或旧R4 CI冒充正式交付。

### 29.5 Gate A 结论、剩余边界与下一步

`dist/v0.1.0-rc.1` 已完成人工 Gate A 验收，P14 与阶段一出口均为 PASS。人工验收不改变性能证据边界：基础性能CPU为每组10秒/50次启停，另对clean提交补120秒idle/100轮burst/100次启停（见下）；未执行小时/天级soak，不得伪造时长。未知硬件/网络盘、强杀缓存清扫、Ignore热更新、可靠rename关联、editor执行仍是已注明边界。公开 Release/tag/tap 与 GitHub PR 合并属于后续集成/分发动作，不反向改变已完成的 Gate A 结论。

目前实际执行范围无已知未解决P0/P1实现故障。R4 补充 PR #4 与 R5 PR #5 均已合并；可从最新 main 开始 R6。Gate A 无需重复执行，除非候选或阶段一核心代码发生会使原签字失效的实质变更。

### 29.6 最终源码、候选与远端证据

实现提交 **`f3e662f2059d47a0b9f108b8b8d0b17dcb760fe4`** 及收尾文档提交 `9c39637` 已推送，并通过 **[PR #5](https://github.com/StevenWinsir/FolderWatch/pull/5)** 合并为 main `6a470a0`（2026-10-02 12:38:01 UTC）。源码提交的 **[push CI 36988380384](https://github.com/StevenWinsir/FolderWatch/actions/runs/36988380384)** 与 **[PR CI 36988453642](https://github.com/StevenWinsir/FolderWatch/actions/runs/36988453642)** 均completed/success，各7个job全绿；最终 head `9c39637` 的 push/PR CI `36989468534` / `36989473906` 也已记录为全绿。精确job/artifact标识在`docs/benchmarks/r5/ci-source.json`。

clean `f3e662f`以Go1.26.6构建的本机候选在本worktree `dist/v0.1.0-rc.1/`：dirty=false、完整commit/build-date、两架构checksum/安全归档与native arm64安装检查PASS；Ruby formula语法PASS；同机两次构建的两archive、manifest、formula、SHA256SUMS **五个文件逐字节相同**。本机arm64 SHA=`69eb126149a38656859df78c83c1964abefc00c9116c123333443bb9fea6c80d`；amd64 SHA=`b676becd596b27dbf672aaf5ce7732b395499c75519ac7744db53691df72fbef`。完整元数据保留在`docs/release/r5-candidate-*`与`r5-reproducibility.json`。

远端已上传[私有候选artifact 11218432417](https://github.com/StevenWinsir/FolderWatch/actions/runs/36988380384/artifacts/11218432417)，runner为Go1.26.8/darwin-arm64，native安装PASS。工具链与本机不同，必须使用该artifact自身manifest/SHA256SUMS，不能套用上述本机SHA；private Actions候选不等于公开Release或永久下载链接。

clean源码额外资源实验于UTC09:13:10.924784–09:15:35.936662实际运行145.01秒：1k×1KiB、120.001秒idle、100文件burst×100轮、100次完整启停。CPU0.3929%、idle语义事件0、Core P95 166.34ms、峰值RSS20.69MB；fd6→1010→6、goroutine1→5→1、cache0，全断言PASS。命令、时间与100样本保留在`soak-clean.json`/`soak-environment.json`；仍不宣称小时级耐久度，人工 Gate A 则已另行完成并签字 PASS。

---

## 30. 本轮交付记录 — R6 / P15–P16（2026-10-02）

### 30.1 状态与实际完成范围

**P15–P16 已实现并完成本地自动化验收；Round 集成状态 IN_REVIEW。** 基线为 PR #6 合并 `88b3541bf4d4b9bc237202276db63613a9ca2aac`，分支 `feat/r6-gui-shell-ipc`。本轮没有扩展到 P17–P25，也没有撤销或重复签署既有 Gate A。独立代码评审、合并及原生 WKWebView 人工操作不冒充已完成。

P15：新增 `gui/main.go` Wails 入口、原生菜单/版本信息、1040×720 默认/680×480 最小窗口、Svelte/TypeScript 壳层、根目录输入、Start/Stop、Scanning/Monitoring/Paused/Stopping/Error 与重连状态。开发服务仅 loopback；生产模式嵌入静态资源。`gui/host` 统一应用上下文、事件泵与并发关闭，应用 context 取消也回收 Facade。

P16：`gui/backend.API` 是唯一 JS binding，调用已有 `internal/app`；支持 Start/Stop/Pause/Resume/Reset、状态、分页 changes 与按需 Diff。随机 client/session capability 隔离新旧页面；2 秒 heartbeat、15 秒 lease、卸载 detach、启动取消和 owner join 保证单会话。事件仅 metadata，32 槽合并背压；generation/version/sequence/size 用十进制字符串避免 JS 精度丢失。列表最多 500 项，Diff 单请求、15 秒上下文及 16 MiB JSON 预算；root-relative path/no-follow 与 Diff 返回前版本复核均有回归。详见 IPC v1 与 ADR-015。

### 30.2 工程、兼容性与本轮修复

保留单 Go module 和 fsnotify v1.8.0 原补丁字节，新增锁定 Wails v2.10.1 与前端 lockfile。依赖升级所带来的 Terminal 传递依赖变化已执行全量回归；`make lint` 校验 patch SHA/可逆性/实际编译路径与 Core 无 UI 依赖。默认 Go 构建不引入 WebView；生成的 JS/TS/runtime 入库，node_modules/dist/.app 不入库。

修复 Wails 生成绑定时切换 build tag 的兼容问题（`desktop || bindings`）；补空 checkout 的 dist bootstrap；禁用 Wails 自动 tidy/sync 以保护 vendor。修复 Svelte 测试误选 server entry、E2E Node 类型/语法以及缺 favicon 引发的 HTTP 404；补应用 context 取消和并发 native lifecycle 回归。初次失败与后续通过均记录于 R6 acceptance，不把中断前旧日志当成最终版本验收。

### 30.3 最终本地测试证据

2026-10-02 UTC **17:03:11–17:05:26**，macOS 26.6.2 arm64 / Go 1.26.6 / Node 24.19.0：`make lint`、8 项 Python 发布脚本测试、`go build ./...`、`go test -count=1 -json -coverprofile=... ./...`、`go test -race -shuffle=on -count=10 ./...`、GUI backend/host verbose race、四套 smoke、六类 fuzz、`go mod verify`、`go mod tidy -diff`、四平台 headless cross-build 全部通过。Go 为 **158 个顶层测试 / 320 个测试与子测试，0 fail/skip，总语句覆盖率 80.4%**（不包含 native WebView/UI 自动化覆盖率）。CLI/watch/semantic/TUI PTY 为 **60/17/16/21** 项。

GUI backend 18 项、host 3 项；12 次 Stop/reload 资源测试 **fd 6→6、goroutine 2→2，临时快照目录为空**；另外覆盖 30 次启停、50 组 host 并发启动/关闭、启动扫描取消、租约到期、旧 client/session、Reset/stale Diff、路径穿越/symlink、binary/unsupported/too-large、分页和大整数/共享 fixture。机器可读记录见 `docs/rounds/R6-validation.json`，原始日志在 ignored `artifacts/r6-final/`。

### 30.4 GUI 验证与未验证边界

`make gui-check`：Svelte/TS **0 errors / 0 warnings**、Vitest **3 文件 11 测试 PASS**、Vite 生产构建 PASS。`make gui-build VERSION=0.2.0-dev` 实际生成 `gui/build/bin/FolderWatch.app`；生产进程启动/存活检查通过，观测时无 TCP 监听，随后退出码 0。该构建来自当时未提交源码，嵌入的 base commit 不等于最终源码身份，不作为发布候选证据。

`FW_GUI_START_SERVER=1 FW_BROWSER_CHANNEL=chrome make gui-e2e`：**2/2 PASS（10.2s）**，真实 Wails dev WebSocket → Go bindings → core，并非 mock。验证 Start/Stop/Start、Unicode/空格/引号路径、真实变化/结构化 Diff、Pause/Resume/Reset、重载撤销旧能力、错误恢复、正文不广播、越界拒绝；1040×720、680×480 及 380×800 CSS 压力检查无横向溢出，title/非空页面/无 Vite overlay/console/pageerror/HTTP error 均通过。浏览器截图保留在 `/tmp/folderwatch-r6-final-gui-qa/` 和 ignored `artifacts/r6-final/browser/`。

本次无 Browser plugin，使用现有 Playwright + 本机 Chrome。**辅助功能 trusted=false、Screen Recording permission denied**，因此原生 WKWebView 截图、按钮/菜单/键盘实际交互尚未验证；浏览器截图不作为原生截图。无 Developer ID 签名/公证、clean-Mac 安装、旧 macOS/Intel GUI、小时级 GUI soak 或 Gate B 结论。原生 smoke 待有权限的评审者补充；不得据此把整轮标记为已合并 PASS。

### 30.5 下一轮与已知限制

R7 已在§31完成 native Folder Picker、变化列表/过滤及只读 Monaco，复用 IPC v1 并保留 selection/client/session/generation/path-version 过期防护。R8 处理 settings、editor/reveal、后台/睡眠、长时间 UX；R9 承接签名、公证、安装及 Gate B。租约在休眠/严重 WebView 停顿后可能结束监控，不自动重启。路径检查不是抵御恶意并发更换祖先目录的原子沙箱；原有 Ignore 热更新、强杀缓存清扫和可靠 rename 关联边界保持。

### 30.6 GitHub 集成证据

实现提交 **`265e19891cfb119e09f02121910be991030eba48`** 已推送至 `feat/r6-gui-shell-ipc`，并创建 **[PR #7](https://github.com/StevenWinsir/FolderWatch/pull/7)**（base=`main`，OPEN，未自动合并）。该源码的 **[push CI 37039054920](https://github.com/StevenWinsir/FolderWatch/actions/runs/37039054920)** 与 **[PR CI 37039099850](https://github.com/StevenWinsir/FolderWatch/actions/runs/37039099850)** 均 completed/success，**各10个job、共20/20通过**：macOS/Linux × Go1.23/1.26、Node22/24 frontend、macOS Wails build/生成绑定无漂移、cross-build、fuzz、Terminal candidate。逐job ID/时间/链接保留在 `docs/rounds/R6-ci-source.json`。本段冻结的是实现提交的源码 CI；后续纯文档提交的最新 head/check 以 PR 实际状态为准，不能混同。

提交后从干净 `265e198` 再次执行 `make gui-build VERSION=0.2.0-dev`，frontend 11测试与 native build 通过，`git diff --exit-code` 确认生成绑定与 module/vendor 无漂移。Go build info 实际包含 commit=`265e198`、version=`0.2.0-dev`、buildDate=`2026-10-02T12:11:33-05:00`；本机 arm64 可执行文件 SHA-256 为 `d1cdb6983f32ecb1bda2d82f6bf05fbf6ebb416a40b4c682b658ed446cb1b3a7`，仅适用于该本机构建，不套用其他工具链的 CI 产物。UTC17:16:05 对此干净构建复测 native 启动/存活、无 TCP listener 观测、已确认 graceful Quit/exit 0，详见 `docs/rounds/R6-build.json`。只有 linker ad-hoc 签名，没有 Developer ID、密封 bundle 或公证；不等于 GUI 发布候选。

最终状态仍为 **实现/自动化 PASS，集成 IN_REVIEW**：独立评审、原生 WKWebView 人工交互和合并尚未完成。R7 从评审集成后的基线进入，不把剩余原生检查或 Gate B 省略。

---

## 31. 本轮交付记录 — R7 / P17–P18（2026-10-02）

### 31.1 状态与实现范围

**P17–P18 已实现并完成本地自动化验收；Round 集成状态 IN_REVIEW。** 本轮从 R6/P15–P16 的 Facade 与 IPC v1 继续，未新建第二套 watcher、baseline 或 diff。原生 macOS folder picker 已通过 Wails runtime 接入；变化列表和 Diff Viewer 使用既有 `GetChanges` / `GetDiff` DTO。

P17 完成：原生 `SelectFolder`、目录输入回退、Start/Stop、Scanning/Monitoring/Paused 状态、变化计数与 generation/version、Added/Modified/Deleted 列表、路径过滤、空态/加载态/错误态、键盘方向键/Home/End 导航。

P18 完成：Monaco 只读 side-by-side Diff Editor；文本按 Go hunks/line kinds 显示，Binary、Unsupported Text、Too Large、Unavailable 使用状态说明；refresh epoch、diff epoch、path/generation/version 复核阻止 stale response 覆盖当前选择；Monaco model key 只在实际版本变化时替换，窗口停止后保留容器避免异步 view layer 访问已移除节点。

### 31.2 工程改动

- `gui/backend` 新增 `FolderReply` 与 `SelectFolder`，Picker callback 由 `gui/main.go` 注入，backend 不引入 Wails runtime。
- `gui/frontend/src/workspace.ts` 新增列表/选择/diff 的 view adapter；`ipc.ts`/`bridge.ts` 扩展分页摘要、Diff DTO 和 folder picker binding。
- `App.svelte` 与 `style.css` 完成 FolderWatch 主工作区、筛选列表、状态条、空态和窄窗口响应式布局。
- `DiffEditor.svelte` 使用锁定版本 `monaco-editor@0.52.2`，重新生成 Wails `API`/`models` bindings；`gui/README.md` 与本节同步。

### 31.3 自动化验收

本机 macOS 26.6.2 arm64、Go 1.26.6、Node 26.10.0、Chrome、Wails 2.10.1：

- `make gui-check`：Svelte/TypeScript 0 errors/0 warnings、Vitest 3 files/11 tests、Vite production build PASS。
- `make lint`、`go test -count=1 ./...`、`go test -race ./gui/backend ./gui/host` PASS；Core boundary、vendor guard PASS。
- `make gui-build VERSION=0.2.0-dev` 成功产出 `gui/build/bin/FolderWatch.app`。
- `FW_GUI_START_SERVER=1 FW_BROWSER_CHANNEL=chrome make gui-e2e`：2/2 PASS，13.1s；真实 Wails dev WebSocket → Go binding → core，验证 Start/Stop/Start、Unicode/空格/引号路径、真实变更、结构化文本 diff、Pause/Resume/Reset、reload revoke、非法路径、responsive 680×480/380×800、无横向溢出、无 pageerror/HTTP error。
- 首次 e2e 暴露 Monaco stop 时异步 DOM 访问错误；保留 Diff Editor 容器并延迟 Monaco 创建、增加稳定模型 key 后修复，最终 e2e 2/2 通过。

机器可读与完整执行记录见 `docs/rounds/R7-acceptance.md`；Playwright 临时截图保留在系统临时目录，未写入仓库。`gui/build/bin/FolderWatch.app` 为 unsigned development build，不代表签名、公证或 Gate B 发布。

### 31.4 未完成边界与下一轮

辅助功能/屏幕录制权限未授予，因此原生 WKWebView 的人工按钮、菜单、folder picker 点击验收仍待有权限的评审者补充；browser e2e 不替代该项。Monaco 初始 bundle 约 2.36 MB（gzip 约 616 KB），后续可在 R8/R9 代码分割。Settings、外部编辑器/Finder、sleep/wake、长时间 GUI soak、签名/公证和 Gate B 仍留给 R8/R9。

### 31.5 GitHub integration

R7 实现与生成绑定收尾提交 **`911c52f`** 已推送至分支 `codex/r7-gui-main-diff`，GitHub **[PR #8](https://github.com/StevenWinsir/FolderWatch/pull/8)**（base=`main`，OPEN）。首轮远端 GUI build 检出生成 bindings 的文件模式/位置漂移，已按生成器实际输出修正并推送 `911c52f`；随后 Actions run `37087365946`（push）与 `37087371245`（PR）均已完成，GUI build、前端矩阵、Go 测试矩阵、fuzz、cross-build 和 terminal-candidate 全部 PASS。原生 WKWebView / picker 人工验收与 PR 合并仍保留 IN_REVIEW，不将 CI 全绿误写为 Gate B 完成。

---

## 32. 本轮交付记录 — R8 / P19–P21（2026-10-02）

### 32.1 状态与实现范围

**P19–P21 已实现并完成本地自动化验收；Round 实现/自动化状态 PASS，集成状态 IN_REVIEW。** 本轮延续 R7 的唯一 Core Facade 与 IPC v1，没有在 GUI 重写 watcher、baseline、ChangeStore 或 diff。完整交付和命令结果见 [`docs/rounds/R8-acceptance.md`](docs/rounds/R8-acceptance.md)。

P19 完成：Settings 面板读取同一份 user/project TOML 配置，支持 debounce、ignore patterns、respect `.gitignore`、max diff bytes 和 external editor；Start 把设置映射为现有 `config.Overlay`。新增 Pause、Resume、Reset Baseline、Stop、Change folder；Reset 在调用前显示“当前状态将成为新基线，现有变化列表清空”的确认提示。

P20 完成：选中文件可 Open in Editor、Reveal in Finder、Copy relative path。路径先按活动 session 的 canonical root-relative key 校验，再转换为绝对路径；编辑器命令用带引号解析器生成可执行文件 + 参数数组并调用 `exec.CommandContext`，拒绝 NUL、换行、shell operator、未闭合引号/转义，不经过 `sh -c`。默认 macOS editor 使用 `open`，Finder 使用 `open -R`，剪贴板使用 `pbcopy`；其他平台返回可见错误。

P21 完成：System/Light/Dark 主题、持久化主题选择、键盘列表/按钮和文本状态、空态/加载态/错误态、目录消失/移动错误反馈、窗口 resize、visibility wake 的 heartbeat/reconnect + authoritative list refresh、pagehide/destroy listener/timer/subscription/Monaco model 清理。空 Settings ignore 列表始终编码为空数组，避免恢复时把 `null` 当成可迭代列表。

### 32.2 代码与接口变化

| 位置 | 变化 |
|---|---|
| `gui/backend/dto.go` | `GUISettings`、`SettingsRequest/Reply`、`PathRequest`、`EditorRequest`、`CopyPathRequest`；`StartOptions.Editor` |
| `gui/backend/api.go` | `GetSettings`、`OpenInEditor`、`RevealInFinder`、`CopyPath`，统一 session/root 校验 |
| `gui/backend/system.go` | 无 shell 的命令解析与 macOS system actions；`system_test.go` 覆盖注入动作、Unicode/空格/引号和 traversal 拒绝 |
| `gui/frontend/src/controller.ts` | settings 加载、会话控制、系统操作、wake/reconnect、可取消/清理生命周期 |
| `gui/frontend/src/App.svelte` / `style.css` | Settings UI、Reset confirmation、session action bar、editor/Finder/copy buttons、三种主题、responsive/accessibility 状态 |
| `gui/frontend/wailsjs/` | 重新生成 API/models bindings |

### 32.3 自动化验收

本机 macOS arm64、Go 1.26.6、Node 26.10.0、Wails 2.10.1、Chrome：

- `go test ./...`：PASS。
- `go test -race ./...`：PASS；另行 `go test -race ./gui/backend ./gui/host` PASS。
- `make lint`：vendor guard、Core boundary、`go vet` PASS。
- `make gui-check`：Svelte/TypeScript 0 errors/0 warnings、Vitest 3 files/11 tests PASS、Vite production build PASS。
- `make gui-build VERSION=0.2.0-dev`：产出 `gui/build/bin/FolderWatch.app`，unsigned development build。
- `FW_GUI_START_SERVER=1 FW_BROWSER_CHANNEL=chrome make gui-e2e`：真实 Wails IPC **2/2 PASS（13.2s）**。

首次真实 E2E 暴露 GetSettings 空 ignore 序列化为 `null`，造成页面恢复时 `join` 错误；修复 `guiSettings` 保证空数组后完整套件再次 2/2 PASS。该问题、修复和边界已记录在 R8 acceptance，不把首次失败隐藏为最终结果。

### 32.4 验收对应关系与边界

- [x] GUI/CLI 共用默认值、配置加载和 Overlay schema；GUI 不创建第二套 watcher/config。
- [x] Reset Baseline 有清晰确认提示并清空权威变化列表。
- [x] 空格、引号、Unicode path 经 root-relative canonical key 校验后安全传递。
- [x] 外部编辑器不使用 `sh -c` 字符串拼接；shell operators/未闭合 quote 拒绝。
- [x] visibility wake 后续租/重连并刷新权威状态；lease 已过期时不假装自动恢复旧 session。
- [x] 反复切目录、reload、pagehide 和 component destroy 有 listener/timer/subscription/model 清理；已有 core descriptor/goroutine regression 继续通过。
- [x] System/Light/Dark 可切换且选择持久化；字母/文本状态不只依赖颜色。
- [ ] 原生 WKWebView 人工按钮、菜单、folder picker、VoiceOver/辅助功能仍需拥有权限的独立评审；真实 Playwright IPC 不替代该项。
- [ ] 签名、公证、clean-machine 安装与 Gate B 由 R9/P23–P24 负责。

### 32.5 R9 交接

R9 只继续 E2E 覆盖、发布打包、签名/公证和 Gate B，不应把编辑器命令执行移回前端或复制 core 语义。当前开发构建不代表正式 GUI 发布候选；PR、远端 CI、人工原生 QA 和合并状态必须分别记录实际结果。
