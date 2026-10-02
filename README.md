# FolderWatch

本地文件夹变更检查工具，macOS 优先。长期目标是用同一 Go core 驱动 Terminal/TUI 与 GUI，比较当前文件和 session 启动基线，而不是只比较上一次保存。

**当前交付：R3 / P0–P8。** 已实现文件分类、UI 无关行级 Diff、唯一 ChangeStore，并复用 R2 的递归监听与启动/Reset 基线。`--watch` 输出 Added/Modified/Deleted 语义变化；恢复基线内容后自动消失。默认命令与 `--scan` 仍只扫描一次。Diff 通过 Go API 按需获取，**尚无 TUI、交互 Diff Viewer 或 GUI**。路线见 [Handoff_Rounds.md](Handoff_Rounds.md)，本轮证据见 [R3 acceptance](docs/rounds/R3-acceptance.md)。

本轮分支为 `feat/r3-classify-diff-changes`；交付 checkpoint 使用 `r3-complete`（以验收记录中的实际推送为准）。R2 已通过 [PR #1](https://github.com/StevenWinsir/FolderWatch/pull/1) 合入 main，R3 的提交/CI/PR 状态单独记录。R2 文档曾预告的 `r2-complete.1` 实际未创建，R2 最终代码以 `32529a7` / 合并提交 `1dbdb5b` 为准。

## 构建与使用

需要 Go 1.23+；lint/CLI 冒烟测试另外需要 Python 3。Git 仅供开发和 Ignore 对照测试使用；被扫描的目录不需要是 Git 仓库。

```sh
git clone https://github.com/StevenWinsir/FolderWatch.git --branch feat/r3-classify-diff-changes
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

仓库当前为私有，clone 需要访问权限。省略 path 时扫描当前目录；选项可放在 path 前后，`--` 终止选项解析。带空格的路径和 glob 请加引号。默认命令仍等价于单次扫描，`--scan` 与 `--watch` 不能同时使用；未来接入 TUI 后仍保留显式 `--scan` 诊断模式。`make run ARGS='--scan --json .'` 可不生成二进制直接运行。

文本输出会转义文件名中的控制字符。JSON 包含 `root`、`entries`、`warnings`；每项有 `path`、`kind`、`size`、`mode`、`mod_time`，不包含文件内容。根目录键为 `.`，其他键为根目录相对、`/` 分隔、保留大小写和 Unicode 的路径。目录、普通文件、symlink、其他特殊文件分别标记为 `directory`、`file`、`symlink`、`other`。遍历顺序确定，遵循 `filepath.WalkDir` 的词法遍历顺序。

## 持续监听、语义变化与 Diff（R3）

`--watch --json` 输出逐行 NDJSON。首条 `ready` 表示已注册 Watcher、完成启动基线，并附 `state`。后续 `changes.batch` 含 `generation/version/upserts/removed`，不发送文件正文。`upserts` 的 kind=deleted 才表示文件删除；`removed` 表示该路径已不再变化（如恢复原内容、先新增后删除）。`ready/reset/reload` 带权威 `state`；批次以 generation/version 排序，CLI 会丢弃已被较新 state 覆盖的排队旧批次。`sequence` 是输出事件序号，可能有跳号。Ctrl+C 退出码 130，清理监听、计时器和缓存。文本模式显示 A/M/D、转义路径及分类。

默认 debounce 为 150ms，持续写入最长等待 4 倍窗口。新建/移入目录递归注册，注册窗口、原始事件溢出由 core reconciliation 补齐。只改 mtime/chmod、内容 hash 不变不会新增内容修改。单次保存的重复通知不增加重复条目；重命名安全降级为 Deleted(old)+Added(new)，不基于相似内容猜测。运行时不可读/瞬间消失会保留最后已知状态并发 warning，使用一个 100ms–2s 退避计时器补查，成功后停止重试，不是空闲全盘轮询。

Go core 入口仍是 `app.Prepare` → `app.StartSession`。`Session.Changes()` 返回排序后的变化副本，`ChangeState()` 同时返回 generation/version，`GetDiff(ctx,path)` 返回带 hunk、行号、Added/Removed/Context 与无末尾换行标记的结构化 Diff。`Events()` 只需消费语义 Batch；`Batch.Reload` 时重新取 ChangeState，UI 不得自行 stat/hash 推导变化。`Baseline/ReadBaseline/ResetBaseline/Close` 保留。Reset 失败/取消保留旧基线及变化表，成功后一次清空并换代；正在计算的旧 Diff 返回 ErrStale，未变化路径返回 ErrNotChanged。普通保存不推进基线。交互式 Reset 与 Diff Viewer 留给 R4。

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
max_snapshot_files = 100000
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
| `--max-watch-dirs <n>` | 默认 8192 个目录；超过时明确停止，不伪装覆盖完整 |
| `--max-snapshot-files <n>` | 默认 100000 个快照/清单条目，亦限制 ChangeStore 大小；超出明确报错 |
| `--snapshot-memory-bytes <size>` | 默认每一代保留内存内容总额 32MiB |
| `--snapshot-cache-bytes <size>` | 默认每一代保留磁盘内容总额 256MiB |
| `--no-mouse` | 保存配置；R4 才有 TUI 鼠标 |
| `--editor <command>` | 保存文本，不执行命令 |
| `--log-file <path>` | 校验路径，不创建日志文件 |
| `--debug` | 保存配置；R4 才提供调试日志行为 |
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

默认/`--scan` 不读取普通文件正文，也不打开 FIFO/device 内容或因扩展名/大小过滤文件；原 5 GiB 稀疏文件 smoke 仍只查元数据。`--watch` 启动及 Reset 则要求完整可读的基线：扫描 warning 或快照读取失败会明确报错，Reset 不会因此抹掉旧路径。

快照按 32KiB 块读取/计算 SHA-256，检查取消和 identity/size/mtime/mode；分类结果与这些确切字节一起保存在 Ref 中，R2 私有探针已替换为共享 Classifier。64KiB 内的小文本优先内存，较大文本保存在 root 外 0700 私有目录的 0600 文件。二进制/未支持编码、超限或预算不足仅保留 metadata/hash。已知超限的 Classifier 不读内容，snapshot 为哈希仍会流式读取但跳过文本探测。Current resolver 只保留元数据，不积累全文。

正常 session 有基线、metadata-only resolver、按需 Diff 三个私有缓存目录。Reset 峰值可持有两代有界基线及一个有界 Diff 当前快照，另有扫描清单、矩阵、I/O 和调用方副本；32MiB/256MiB 是每代保留预算，不是整个进程 RAM 上限。扫描清单分配仍随条目数增长；kqueue 描述符也随文件数增加。Close 清理全部缓存；强杀后残留自动清扫未实现。持续压力/性能预算留到 R5，当前不宣称 P12 指标通过。

不会在监控目录写状态/快照，不执行 editor，不创建用户日志，不联网或上传文件内容。开发时依赖下载与 GitHub 提交不属于应用运行行为。

## 工程结构与验证

```text
cmd/folderwatch/       进程入口、版本、取消信号
internal/cli/          参数与终端/JSON 输出
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
internal/app/          Prepare 与 Session 生命周期、Reset
scripts/smoke.py       R1 单次扫描回归
scripts/watch_smoke.py R2 持续监听、真实 Vim、Ctrl+C/清理
scripts/semantic_smoke.py R3 真实 CLI 状态迁移与分类
scripts/vendor_guard.py 依赖补丁校验
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
```

`make lint` 执行 gofmt 检查及 `go vet`。CI 配置 macOS/Linux、Go 1.23/1.26 的 build/test/race/smoke，并交叉编译 macOS/Windows。实际执行结果以 [Actions](https://github.com/StevenWinsir/FolderWatch/actions) 和 R3 验收记录为准，配置存在不等于远端运行已通过。Windows 目前只要求编译，不宣称运行时正式支持。

下一轮从 R4 / P9–P11 开始，直接消费 Changes/ChangeState、GetDiff、语义 Batch 和 Reset API，不在 UI 复制分类/哈希/变化推导。先核对 R3 的 PR/集成状态，保留全部 R1–R3 回归和 vendor patch。架构及边界见 ADR-010–012；贡献规范见 [CONTRIBUTING.md](CONTRIBUTING.md)。本轮没有新增外部依赖，项目尚未指定对外分发许可证。
