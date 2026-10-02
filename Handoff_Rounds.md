# FolderWatch — Handoff.md

> 项目代号：**FolderWatch**
> 文档用途：工程实施、多人接力开发、代码评审、测试与发布交接  
> 目标平台：**macOS 优先**，架构保留 Windows / Linux 扩展能力  
> 核心技术栈：**Go + fsnotify + Bubble Tea + Lip Gloss**  
> GUI 技术栈：**Wails + Svelte + TypeScript + Monaco Diff Editor**  
> 开发阶段：**阶段一 Terminal/TUI → 质量闸门 → 阶段二 GUI**

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
| R1 | Terminal | P0–P2 | 工程基础与输入边界 | 工程骨架、CLI/Config、扫描与 Ignore | 可稳定确定应监控集合 |
| R2 | Terminal | P3–P5 | 文件事件与 Baseline 内核 | Watcher、Debounce/Coalesce、Snapshot | 可稳定监听并建立/重置基线 |
| R3 | Terminal | P6–P8 | 内容分析与变更语义 | Classifier、Diff、ChangeStore | 可输出可靠 changed-file 语义 |
| R4 | Terminal | P9–P11 | TUI 产品体验 | 文件列表、Diff Viewer、Session 控制 | Terminal 主流程完整可用 |
| R5 | Terminal | P12–P14 | 工程硬化与发布 | 性能、测试、CI、Terminal 发布 | **Gate A PASS** |
| R6 | GUI | P15–P16 | GUI 壳层与 IPC 契约 | Wails/Svelte、Core Facade、DTO/Event | GUI 稳定调用同一 Go Core |
| R7 | GUI | P17–P18 | GUI 主流程与 Diff | Folder Picker、变化列表、Diff Viewer | GUI 核心用户路径闭环 |
| R8 | GUI | P19–P21 | GUI 功能补全与系统鲁棒性 | Settings、系统集成、可访问性 | GUI 功能完整可长期运行 |
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
- [ ] `go build ./...` 成功
- [ ] `folderwatch --help` 可用
- [ ] 非法 path/duration/size 返回明确错误
- [ ] 嵌套目录、空格、Unicode 扫描正确
- [ ] `.folderwatchignore` 与 CLI ignore 生效
- [ ] symlink 行为符合 ADR
- [ ] 权限不足目录不导致整个程序崩溃
- [ ] initial scan 与 ignore matcher 有自动化测试
- [ ] `main.go` 不包含 watcher/diff 业务实现

**出口定义**：系统能够可靠回答“给定 root + config，本次 session 应关注哪些文件与目录？”

---

### 2.4 R2 — 文件事件与 Baseline 内核（P3–P5）

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
- [ ] create/write/remove/rename 产生稳定内部事件
- [ ] 新建子目录后的内部文件继续被监听
- [ ] atomic save 不永久丢失真实目标文件
- [ ] 单次保存不会留下重复语义 change request
- [ ] baseline 不因普通保存自动推进
- [ ] reset 后当前状态成为新 baseline
- [ ] stop/reset 无明显 timer/goroutine 泄漏
- [ ] 至少验证 VS Code 与 Vim/Neovim 两类保存模式

**出口定义**：不论编辑器如何保存，core 最终都能稳定得到需要重新解析的真实路径，并拥有正确 baseline。

---

### 2.5 R3 — 内容分析与变更语义（P6–P8）

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
- [ ] 新文件 = Added
- [ ] 删除 baseline 文件 = Deleted
- [ ] 内容改变 = Modified
- [ ] 恢复 baseline 后自动从列表消失
- [ ] Added→Deleted、Deleted→Recreated 等迁移正确
- [ ] 二进制不会进入文本 diff
- [ ] TooLarge 不 OOM
- [ ] Unicode、空文件、无末尾换行有 golden tests
- [ ] 每个 path 只保留一个最终语义状态
- [ ] 核心 store/resolve 路径通过 `go test -race`

**出口定义**：无 TUI 情况下即可可靠获得 `[]ChangeSummary` 和 `GetDiff(path)`；R4 只能展示结果，不能重新推导文件状态。

---

### 2.6 R4 — Terminal/TUI 产品体验（P9–P11）

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
- [ ] 0/1/100+ changed files 正常显示
- [ ] `↑↓`、`j/k`、Enter、Space、`p`、`r`、`?`、`q` 可用
- [ ] 鼠标失效时核心功能仍可纯键盘完成
- [ ] 1000+ 行 diff 可滚动且 UI 不冻结
- [ ] Binary/TooLarge 有友好状态
- [ ] Pause/Resume 后 reconciliation 正确
- [ ] Reset 清空当前变化并使用新 baseline
- [ ] 单文件 recoverable error 不终止 session
- [ ] 日志不会破坏 TUI

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
- [ ] P14 Gate A 全部满足
- [ ] 性能数据标明真实硬件与数据规模
- [ ] 无无限增长 channel/timer/goroutine
- [ ] 核心 bug 均补 regression test
- [ ] clean machine 可安装并运行 Terminal 版本
- [ ] Core API 不依赖 Bubble Tea/Lip Gloss
- [ ] Gate A 文件已提交仓库

**出口：Gate A**：`PASS` 才进入 R6；`FAIL` 必须回到对应责任 Round 修复并重新执行 Gate A。

---

### 2.8 R6 — GUI 壳层与 IPC 契约（P15–P16）

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
- [ ] GUI 不包含第二套 watcher/baseline/diff
- [ ] Start/Stop/Start 可重复
- [ ] frontend reload/disconnect 不泄漏 watcher/session
- [ ] change event 不广播大段文件全文
- [ ] diff 按需获取
- [ ] DTO 与内部 Go struct 解耦
- [ ] path 访问被限制在当前 root

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
- [ ] 无需 Terminal 可选择目录并启动监控
- [ ] 文件修改后列表出现
- [ ] 文本文件显示正确 diff
- [ ] Binary/TooLarge 不进入文本编辑器渲染
- [ ] 快速切换多个文件不显示旧 diff
- [ ] 文件再次变化后，旧请求不能覆盖新版本

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
- 超过 `max-diff-bytes`：保存元数据 + hash，不保存完整内容，不做全文 diff。
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
--respect-gitignore
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

### 目标

将 OS 事件转换为项目内部 raw event stream。

### 依赖

P2。

### 接口建议

```go
type Watcher interface {
    Start(ctx context.Context, root string) (<-chan RawEvent, <-chan error, error)
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

## 23. 第一位开发者从哪里开始

第一位接手者严格按以下顺序：

1. 完成 P0：工程骨架、依赖、CI、ADR。
2. 完成 P1：CLI/config contract。
3. 完成 P2：scan + ignore + path policy。
4. 不要提前做漂亮 TUI。
5. 优先把 P3–P8 的 core 做稳，并用测试证明状态正确。
6. 到 P9 才正式接 Bubble Tea。
7. P14 Gate A 不通过，不创建 GUI 业务分支。

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
