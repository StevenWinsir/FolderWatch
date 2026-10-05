# FolderWatch

本地文件夹变更检查工具，macOS 优先。长期目标是用同一 Go core 驱动 Terminal/TUI 与 GUI，比较当前文件和 session 启动基线，而不是只比较上一次保存。

**当前范围：P0–P21 的 Terminal/Core 与 GUI 功能，加上大目录可靠性修复。** macOS 原生构建使用 FSEvents；原生注册资源不足时可见地降级为轮询，而不是因 8192 个目录退出。基线/变更元数据使用私有磁盘索引，GUI 支持完整分页和跨页搜索。实现、自动化与集成状态分别见 [大目录验收](docs/rounds/large-folder-acceptance.md)、[ADR-016](docs/adr/016-large-folder-coverage-and-disk-indexes.md) 和 [Handoff §33](Handoff_Rounds.md)。

P0–P14 的历史 Gate A、各轮验收证据保持原样；本次核心变更有独立回归，不借用旧签字。GUI 仍不是 Developer ID 签名/公证的正式发行版，Gate B 与原生 WKWebView 人工 QA 不由浏览器 IPC 测试代替。

R1–R5 的历史源码、性能与候选证据保留在各轮 acceptance、[性能报告](docs/performance-v1.md)和 Gate A 记录中；不将旧 CI 或旧候选签字套用到新的 GUI 二进制。

## 构建与使用

需要 Go 1.23+；lint/CLI 冒烟测试另外需要 Python 3。Git 仅供开发和 Ignore 对照测试使用；被扫描的目录不需要是 Git 仓库。

```sh
git clone https://github.com/StevenWinsir/FolderWatch.git
cd FolderWatch
go mod download
make build

./bin/folderwatch --help
./bin/folderwatch --version
./bin/folderwatch --scan .
./bin/folderwatch --scan --json "/path/to/项目 with spaces"
./bin/folderwatch . --ignore 'node_modules/' --ignore '*.tmp'
./bin/folderwatch --scan --respect-gitignore .
./bin/folderwatch --watch --json --respect-gitignore "/path/to/项目 with spaces"
```

仓库当前为私有，clone 需要访问权限。省略 path 时扫描当前目录；选项可放在 path 前后，`--` 终止选项解析。带空格的路径和 glob 请加引号。stdin 与 stdout 均为终端时，默认进入 TUI；任一被重定向时，默认保持单次扫描。`--tui` 强制交互模式，无终端时明确报错，且不能与 `--scan`、`--watch`、`--json` 合用；`--scan` 与 `--watch` 也不能同时使用。`make run ARGS='--scan --json .'` 可不生成二进制直接运行。

文本输出会转义文件名中的控制字符。JSON 包含 `root`、`entries`、`warnings`；每项有 `path`、`kind`、`size`、`mode`、`mod_time`，不包含文件内容。根目录键为 `.`，其他键为根目录相对、`/` 分隔、保留大小写和 Unicode 的路径。目录、普通文件、symlink、其他特殊文件分别标记为 `directory`、`file`、`symlink`、`other`。遍历顺序确定，遵循 `filepath.WalkDir` 的词法遍历顺序。

## GUI

本机目标为 macOS；还需要 Node 22 或 24、npm、Xcode Command Line Tools。Wails CLI 固定 v2.10.1；应用继续使用根 Go module 和已审查 vendor，不要另建 GUI module 或绕过补丁。

```sh
make gui-setup
make gui-build VERSION=0.2.0-dev
open gui/build/bin/FolderWatch.app
# 仅 loopback 的开发服务
make gui-dev
# 真实 Go binding/core 的浏览器测试，自动启停自有 dev server
FW_GUI_START_SERVER=1 FW_BROWSER_CHANNEL=chrome make gui-e2e
```

输入绝对目录或 `~/path` 后 Start monitoring，Stop 可取消扫描、恢复或重置。界面提供 Folder Picker、Monaco Diff、Settings、Pause/Resume/Reset、编辑器/Finder/复制路径。每页最多 500 条变更，Previous/Next 可访问全部结果，搜索覆盖整个索引，不限于当前页。页面重载会结束旧会话；2 秒 heartbeat / 15 秒租约回收断开的页面，不会悄悄恢复旧监控。

`make gui-check` 执行 Svelte/TypeScript、Vitest 与 Vite 构建；`make gui-bindings` 重生成已入库绑定。真实 Wails IPC E2E 包含 9000 目录和 1001 条变更分页/搜索/Diff，不是 mock，也不代替生产 WKWebView 人工 QA。构建产物在 `gui/build/bin/FolderWatch.app`。

## Terminal 使用

```sh
./bin/folderwatch "/path/to/项目 with spaces"
./bin/folderwatch --tui --no-mouse .
NO_COLOR=1 ./bin/folderwatch --tui .
# 日志必须是 root 外尚不存在的新文件；不自动创建父目录。
./bin/folderwatch --tui --debug --log-file /tmp/folderwatch-session-new.jsonl .
```

列表显示变化数、A/M/D 与 `[+]`/`[-]`，下方显示选中文件的 unified Diff。`↑↓` / `j/k` 选文件，Enter / Space 展开收起；PgUp/PgDn 或 Ctrl+u/d 翻页，Home/End（或 g/G）到开头/结尾，`←→` / `h/l` 横向滚动。`/` 进入路径过滤，输入后 Enter 应用，Esc 取消编辑或清除已应用过滤。`?` 打开帮助，`e` 查看可滚动的诊断记录。点击文件行展开，列表和 Diff 分区各自响应滚轮；`--no-mouse` 下全部主流程可纯键盘完成。小于 40 列 × 12 行时显示 resize 提示，仍能退出。

`p` 暂停/恢复。暂停保留 watcher、冻结最后已知变化；恢复先对整个 root 校准，仍比较同一个 baseline。`r` 发起 Reset，`y` / Enter 确认，其他键取消；成功后变化清零，再次修改比较新基线。暂停状态下也可 Reset，并保持暂停。失败不会丢弃旧基线。`q` 正常退出为 0，Ctrl+C 为 130，清理缓存、恢复终端。扫描中可取消；运行时单文件不可读显示 warning 并继续，根目录丢失等 fatal 错误在恢复终端后打印并返回 1。

Diff 仅按需获取并且可取消，快速切换/再次保存/Reset 不会让旧结果覆盖新文件。最多保留一份选中 Diff；core 限制之外，终端预览最多 16MiB / 50,000 行，每逻辑行约 16KiB，达到上限明确提示截断。文本显示 hunk、双侧行号、红绿与 `+/-`、无末尾换行标记；Binary/Unsupported/TooLarge/Unavailable 显示尺寸与原因，不虚构旧内容。NO_COLOR 或 TERM=dumb 关闭样式后仍可读。文件名和内容的控制/双向格式字符会转义，Unicode 按终端字符格裁切。

## 持续监听、语义变化与 Core Diff

`--watch --json` 输出逐行 NDJSON。首条 `ready` 表示已注册 Watcher、完成启动基线，并附 `state`。后续 `changes.batch` 含 `generation/version/upserts/removed`，不发送文件正文。`upserts` 的 kind=deleted 才表示文件删除；`removed` 表示该路径已不再变化（如恢复原内容、先新增后删除）。`ready/reset/reload` 带权威 `state`；批次以 generation/version 排序，CLI 会丢弃已被较新 state 覆盖的排队旧批次。`sequence` 是输出事件序号，可能有跳号。Ctrl+C 退出码 130，清理监听、计时器和缓存。文本模式显示 A/M/D、转义路径及分类。

默认 debounce 为 150ms，持续写入最长等待 4 倍窗口。新建/移入目录递归注册，注册窗口、原始事件溢出由 core reconciliation 补齐。只改 mtime/chmod、内容 hash 不变不会新增内容修改。单次保存的重复通知不增加重复条目；重命名安全降级为 Deleted(old)+Added(new)，不基于相似内容猜测。运行时不可读/瞬间消失会保留最后已知状态并发 warning，使用一个 100ms–2s 退避计时器补查，成功后停止重试，不是空闲全盘轮询。另有每watcher一个1秒root身份健康检查，仅Lstat根目录并比较身份，不扫描子树或读取正文；用于Linux持有cwd/目录fd时，根被删除却没有原生通知的情况。暂停时也检查，退出停止计时器。调度间隔不是慢文件系统上的硬实时保证。

Go core 入口仍是 `app.Prepare` → `app.StartSession`。`Session.Changes()` 返回排序后的变化副本，`ChangeState()` 同时返回 generation/version，`GetDiff(ctx,path)` 返回带 hunk、行号、Added/Removed/Context 与无末尾换行标记的结构化 Diff。`Events()` 只需消费语义 Batch；`Batch.Reload` 时重新取 ChangeState，UI 不得自行 stat/hash 推导变化。`Baseline/ReadBaseline/ResetBaseline/Close` 保留。Reset 失败/取消保留旧基线及变化表，成功后一次清空并换代；正在计算的旧 Diff 返回 ErrStale，未变化路径返回 ErrNotChanged。普通保存不推进基线。R4 的交互式 Reset、Diff Viewer 直接复用这些 API；新增 `Pause(ctx)`、`Resume(ctx)`、`Status()` 供其他 adapter 同样调用。

分类基于有界完整字节流，不看扩展名：UTF-8/空文件为 text，二进制/无效 UTF-8/危险控制字节为 binary，UTF-16/32 BOM 为 unsupported-text，超过分类上限为 too-large，链接/特殊文件为 unsupported。GetDiff 对 binary/unsupported-text/too-large 不返回文本 hunks；未保留旧内容时为 unavailable。不得拿当前内容替代缺失的 before。

Diff 默认限制每侧 5MiB、20000 行、2000000 个 LCS 矩阵单元；复杂输入即使未超过字节数也可能返回 too-large（计算预算），并保留变化条目。默认上下文 3 行，独立核心选项可调整。相同首尾裁剪、行 ID 化和循环取消检查避免无界计算；不使用超时后遗留的后台 goroutine。不常驻所有 current 内容或 Diff 缓存；同一时刻仅一个按需 Diff。

构建必须使用仓库中已审查的 vendor 依赖补丁，不能通过 `-mod=mod` 绕过。不要把 `go install ...@version` 当成等价交付方式。依赖来源、可复现补丁和再生成步骤见 [ADR-009](docs/adr/009-vendored-fsnotify-kqueue.md)。

## 配置

优先级固定为：**显式 CLI flags > 根目录 `.folderwatch.toml` > 用户配置 > 内建默认值**。

用户配置位置为 Go `os.UserConfigDir()/FolderWatch/config.toml`：macOS 通常是 `~/Library/Application Support/FolderWatch/config.toml`，Linux 通常是 `$XDG_CONFIG_HOME/FolderWatch/config.toml` 或 `~/.config/FolderWatch/config.toml`。不会自动创建配置或状态文件。

```toml
# <root>/.folderwatch.toml
ignore = ["node_modules/", "*.tmp"]
respect_gitignore = false
include_git = false
debounce = "150ms"
max_diff_bytes = "5MiB"
max_snapshot_bytes = "8MiB"
max_diff_lines = 20000
max_pending_events = 4096
max_watch_dirs = 8192
max_snapshot_files = 0
snapshot_memory_bytes = "32MiB"
snapshot_cache_bytes = "256MiB"
no_mouse = false
editor = ""
log_file = ""
debug = false
# ignore_file = "rules.ignore"  # 设置后必须存在
```

完整示例见 [testdata/config.example.toml](testdata/config.example.toml)。未知键、类型错误、无效 glob、非法 duration/size 都会报错，包括被后续层覆盖的非法配置。高优先级 `ignore` 数组**替换**低优先级数组；`ignore = []` 可清空。CLI 重复 `--ignore` 构成最高优先级数组。布尔选项支持显式覆盖，例如 `--respect-gitignore=false`。

配置文件内的路径相对于该 TOML 文件；CLI 路径相对于工作目录。支持 `~` / `~/...`，不展开环境变量或 `~otheruser`。自动发现的项目配置和规则文件不跟随 symlink；显式选中的额外 ignore 文件可以是 symlink。配置和规则文件都必须是普通文件，单文件读取上限 1 MiB。

| 选项 | 当前行为 |
|---|---|
| `--scan`、`--json` | 单次元数据扫描；可输出 JSON |
| `--watch`、`--watch --json` | 持续监听、基线与语义变化批次；JSON 模式为 NDJSON |
| `--ignore <pattern>` | 可重复的 Ignore 规则 |
| `--ignore-file <path>` | 附加规则文件，必须存在；不替代根 `.folderwatchignore` |
| `--respect-gitignore` | 启用根目录及嵌套 `.gitignore`，默认 false |
| `--include-git` | 关闭内建 `.git/` 排除规则，默认 false |
| `--debounce <duration>` | 实际事件聚合窗口，默认 150ms；最大等待为 4 倍 |
| `--max-diff-bytes <size>` | Diff 每侧字节上限，默认 5MiB；有效范围 1B..64MiB |
| `--max-snapshot-bytes <size>` | 独立分类/保留上限，默认 8MiB；有效范围 1B..64MiB，不过滤扫描/哈希 |
| `--max-diff-lines <n>` | Diff 每侧行数，默认 20000；有效范围 1..200000 |
| `--max-pending-events <n>` | raw queue / pending map 容量，默认 4096；有效范围 1..1048576 |
| `--max-watch-dirs <n>` | 默认 8192，非递归原生后端的注册预算；耗尽后降级轮询，不限制监控树总目录数 |
| `--max-snapshot-files <n>` | 默认 0，不设条目数量上限；显式正数仍作为可选限制，负数非法 |
| `--snapshot-memory-bytes <size>` | 默认每一代保留内存内容总额 32MiB |
| `--snapshot-cache-bytes <size>` | 默认每一代保留磁盘内容总额 256MiB |
| `--tui`、`--no-mouse` | 强制终端交互；禁用鼠标报告及鼠标操作 |
| `--editor <command>` | 保存文本，不执行命令 |
| `--log-file <path>` | 仅 live 模式创建 root 外新的 0600 JSONL，最多 4MiB，不覆盖现有文件 |
| `--debug` | 向 200 条诊断 ring 加入事件类型/版本/数量等元数据，TUI 按 e 查看 |
| `--help`、`--version` | 无需加载配置或扫描目录即可退出 |

Duration 使用正的 Go duration，如 `75ms`、`1s`。Size 支持正整数 bytes，或 B/KB/MB/GB/TB、KiB/MiB/GiB/TiB（大小写不敏感）。小数只有在换算后为精确整数字节时才接受，如 `1.5MiB`；0、负数、非整数字节及 int64 溢出会被拒绝。

退出码：0 成功（可伴随 warning），2 参数/配置错误，1 运行或输出 I/O 错误，130 取消。

## Ignore 与 symlink 规则

规则从低到高依次为：内建 `.git/` → 已启用的根/嵌套 `.gitignore` → 根 `.folderwatchignore` → 显式 ignore 文件 → config/CLI ignore 数组。同一路径最后匹配的规则生效；嵌套 `.gitignore` 覆盖上级 Git 规则，FolderWatch 自定义规则优先于所有 Git 规则。

支持 `*`、`?`、`[]`、`**`、根锚点 `/`、目录后缀 `/`、注释 `#`、否定 `!`、转义的首字符 `#`/`!`、转义尾空格、CRLF。无斜杠模式在其作用域任意深度匹配 basename；带斜杠的模式相对于规则文件目录。`{a,b}` 是字面文本，不是 shell 展开。`a/**` 忽略 `a` 的后代但不忽略目录 `a` 本身。

**子项否定规则不能穿过被忽略的父目录。** 例如 `cache/` 加 `!cache/keep.txt` 仍然忽略 `keep.txt`；必须先取消对父目录的忽略。此行为有与 `git check-ignore` 的自动对照测试。

`.git/` 默认在任意层级忽略；`--include-git` 只关闭这条内建规则，其他自定义规则仍然有效。启用 `.gitignore` 不读取 Git 全局 excludes 或 `.git/info/exclude`，也不考虑文件是否已被 Git 跟踪，因此不是完整的 Git 仓库状态模拟。

明确选择的 root symlink 会解析到真实根目录；根目录内的 symlink 只记录链接自身元数据，不跟随文件或目录链接，避免环和越界。runtime Matcher 也拒绝经过 symlink 祖先的路径。此检查不承诺抵御恶意进程同时替换目录的原子文件系统沙箱。

规则按目录 identity / matcher / session 缓存，包含该目录中未找到规则文件及错误结果。新目录首次加载规则；删除/替换目录会退役旧作用域缓存，避免目录反复创建删除时无限增长。已有目录中的规则文件修改仍需重启/新 Matcher；这不是热更新。所有入口复用同一 Matcher。

## 错误、资源与隐私

单个文件消失、子目录权限不足等会产生 warning 并继续处理其他项；无法读取 root 是致命错误。嵌套规则文件不可读或无效时跳过对应子树，而不是忽略错误后扩大扫描范围。带 warning 的清单可能不完整，不能把它作为“所有项均已成功读取”的证明。

单次扫描（`--scan` / `--json` / 非终端默认）只读元数据，保留确定性清单。持续监控启动可保留部分基线：不可读/不稳定路径明确显示 `unknown` / Baseline unknown，并给出覆盖告警；其他路径继续监控。恢复权限不会把当前内容伪造成启动时内容。**Reset 仍要求完整成功**，失败保留旧基线和变化列表；完整 Reset 才消除未知基线。

基线按 32KiB 块计算 SHA-256，保留原有分类/内容/diff 字节预算。64KiB 内文本优先内存，但最多保留 4096 个非空内存内容对象；其余按磁盘内容预算落盘或只保留 metadata/hash。元数据索引、扫描队列和轮询清单使用 root 外 0700 私有目录中的 0600 文件，读写分页有界。目录事件优先核对具体子树；可靠本地文件系统恢复扫描使用 identity/mtime/ctime 等完整签名避免无变化正文反复读取，原生文件事件仍强制验证内容。

32MiB/256MiB 是每代保留的**正文**预算，不是总进程 RSS 或全部磁盘占用。索引、mmap/OS 页缓存、文件系统块开销、可选 ignore 缓存和调用方副本另计。显式 `Scan` / `Baseline` / `ChangeState` 及旧 Terminal/NDJSON 全清单消费者仍可能按结果规模分配；GUI 运行热路径使用分页/Head/Lookup。大树扫描时间、磁盘占用和轮询延迟仍随规模增长，不承诺固定资源处理无限文件。根目录、权限、文件系统路径长度、可用磁盘和缓存必须位于 root 外等约束仍有效。强杀后的缓存自动清扫、网络盘/远端写入和小时级原生 GUI 认证不由本地压力测试推断。

应用默认纯本地，不上传文件内容，也不再加载外部字体。GUI 只有用户主动操作才调用无 shell 拼接的编辑器/Finder/剪贴板集成；CLI editor 配置不自动执行。日志仍为有界 metadata-only ring / root 外新建私有文件。开发依赖下载、GitHub 推送和 CI 不属于应用运行行为。

## 工程结构与验证

```text
cmd/folderwatch/       进程入口、版本、取消信号
cmd/fwbench/           仅开发使用的合成性能/资源探针，不随应用分发
gui/main.go           Wails native 入口，desktop || bindings build tags
gui/host/             应用 context、事件泵、并发 shutdown
gui/backend/          唯一 GUI Core Facade、IPC v1 DTO 与生命周期/安全测试
gui/frontend/         Svelte/TypeScript、生成绑定、Vitest/真实 IPC E2E
internal/cli/          参数与终端/JSON 输出
internal/tui/          Bubble Tea model、键鼠、可取消 Diff、虚拟视口与安全渲染
internal/logging/      有界诊断 ring、显式 root 外私有文件
internal/config/       共享配置、优先级、校验
internal/pathutil/     根目录与 canonical key 策略
internal/fileutil/     配置/规则的有界读取
internal/ignore/       initial/runtime 共用 Matcher
internal/model/        UI 无关元数据
internal/scan/         递归扫描与 warning
internal/watcher/      递归 fsnotify adapter 与目录登记
internal/eventnorm/    OS 无关路径 invalidation
internal/debounce/     单 owner / 单 ticker / 有界聚合
internal/snapshot/     有界快照、不可变引用与原子基线
internal/filetype/     有界分类与共享 UTF-8/编码探针
internal/diff/         有界可取消行级 Diff、hunk 和黄金样例
internal/changes/      唯一语义 ChangeStore、重查、版本与按需 Diff
internal/app/          Prepare 与 Session 生命周期、Pause/Resume、Reset
scripts/smoke.py       R1 单次扫描回归
scripts/watch_smoke.py R2 持续监听、真实 Vim、Ctrl+C/清理
scripts/semantic_smoke.py R3 真实 CLI 状态迁移与分类
scripts/tui_smoke.py   R4 真实二进制 PTY 键鼠、滚动、恢复与错误/清理
scripts/vendor_guard.py 依赖补丁校验
scripts/performance.py / tui_performance.py 目录资源与真实 PTY 延迟
scripts/release.py / release_smoke.py 可复现双架构候选包与安装验证
scripts/fuzz.py       六类有界 fuzz 与日志
patches/ + vendor/    可复现依赖源码与本地 kqueue 补丁
docs/adr/             决策与边界
docs/rounds/          每轮验收记录
tools/deps.go         仅锁定后续 adapter 依赖的 build-tag 包
```

```sh
make build
make test
make race
make lint
make smoke
make scripts-test
make fuzz
make bench
# 输出目录必须尚不存在；真实数据和 profiles 留在 artifacts/。
make performance
python3 scripts/tui_performance.py bin/folderwatch
# 清洁 checkout 才能构建正式候选；不会创建标签或上传 Release。
make release VERSION=v0.1.0-rc.1
make release-smoke VERSION=v0.1.0-rc.1
```

`make lint` 执行 gofmt、vendor hash/补丁可逆性、Core 传递依赖边界检查与 go vet。CI 保留 macOS/Linux × Go1.23/1.26、六类 fuzz、发布回归及四平台 headless cross-build，并增加 Node22/24 前端检查、macOS Wails 构建及生成绑定无漂移检查。实际执行状态以 [Actions](https://github.com/StevenWinsir/FolderWatch/actions) 和 [R6 acceptance](docs/rounds/R6-acceptance.md) 为准；Windows 仍只声明编译支持。

下一步先完成 R6 PR 评审/集成及原生 WKWebView 交互检查，再进入 R7。Gate A 已签字，保留其原候选对象，不以新二进制替代；Gate B、GUI 正式分发、签名/公证尚未进行。R6 新增 Wails 所需锁定依赖并保留 fsnotify 补丁；依赖/构建边界见 ADR-009、014、015。Terminal 候选和 Homebrew 方案见 [发布说明](docs/release/terminal-candidate.md)，贡献规范见 [CONTRIBUTING.md](CONTRIBUTING.md)。
