# FolderWatch

本地文件夹变更检查工具，macOS 优先。长期目标是用同一 Go core 驱动 Terminal/TUI 与 GUI，比较当前文件和 session 启动基线，而不是只比较上一次保存。

**当前交付：R2 / P0–P5。** 在 R1 工程、CLI/Config、扫描和 Ignore 基础上，已实现递归 Watcher、路径事件聚合、有界快照、启动基线和原子 Reset。默认命令与 `--scan` 仍扫描一次；**`--watch` 才持续监听并建立 baseline**。现在输出的是待重新解析的路径/重扫请求，尚无 Added/Modified/Deleted 语义列表、文本 diff、TUI 或 GUI。路线见 [Handoff_Rounds.md](Handoff_Rounds.md)，证据见 [R2 acceptance](docs/rounds/R2-acceptance.md)。

本轮代码交付在 `feat/r2-watch-baseline` 分支和 `r2-complete` checkpoint；实现与自动化验收通过，PR 合并尚未完成，`main` 仍是 R1。详细集成状态见 R2 acceptance。

## 构建与使用

需要 Go 1.23+；lint/CLI 冒烟测试另外需要 Python 3。Git 仅供开发和 Ignore 对照测试使用；被扫描的目录不需要是 Git 仓库。

```sh
git clone https://github.com/StevenWinsir/FolderWatch.git --branch feat/r2-watch-baseline
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

## 持续监听与 Baseline（R2）

`--watch --json` 输出逐行 NDJSON，不是单个 JSON 数组。首条 `ready` 表示已注册 Watcher 并完成启动基线；后续 `paths` 包含已聚合路径，`reconcile: true` 表示必须重新核对当前目录状态。事件含单调递增的 `sequence` 和基线 `generation`，不发送文件正文。Ctrl+C 退出码为 130，并关闭 Watcher、timer、goroutine 和私有临时缓存。未带 `--json` 时输出转义后的文本诊断。

默认 debounce 为 150ms，持续写入最长等待 4 倍 debounce 后产生一次待处理请求。新建/移入的目录递归注册；注册窗口内已有文件通过目录重扫请求补齐。队列满或消费者慢时折叠成 root reconciliation，不能把每个原始事件当成必须保留的操作日志。CHMOD-only 带 metadata 提示；是否内容变化由 R3 的 stat/hash resolver 决定。

Go core 入口是 `app.StartSession(ctx, prepared)`；通过 `Events()` 消费请求，`Baseline()` 获取不可变引用副本，`ReadBaseline(ctx,path)` 按需读取保留的 before 内容，`ResetBaseline(ctx)` 原子替换基线，`Close()` 清理。Reset 失败/取消保留旧基线；成功后 generation 增加，旧引用失效，先发布 reset/reconcile 标记，再处理排队事件。普通保存不会推进基线。R2 没有交互式 reset 按键，CLI 是诊断入口；交互属于 R4。

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
| `--watch`、`--watch --json` | 持续监听、基线与聚合请求；JSON 模式为 NDJSON |
| `--ignore <pattern>` | 可重复的 Ignore 规则 |
| `--ignore-file <path>` | 附加规则文件，必须存在；不替代根 `.folderwatchignore` |
| `--respect-gitignore` | 启用根目录及嵌套 `.gitignore`，默认 false |
| `--include-git` | 关闭内建 `.git/` 排除规则，默认 false |
| `--debounce <duration>` | 实际事件聚合窗口，默认 150ms；最大等待为 4 倍 |
| `--max-diff-bytes <size>` | 默认 5MiB；R2 单文件内容保留上限，未来也是 diff 上限，不过滤扫描/哈希 |
| `--max-pending-events <n>` | raw queue / pending map 容量，默认 4096；有效范围 1..1048576 |
| `--max-watch-dirs <n>` | 默认 8192 个目录；超过时明确停止，不伪装覆盖完整 |
| `--max-snapshot-files <n>` | 默认 100000 个快照引用，包括目录、链接及特殊文件元数据 |
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

快照按 32KiB 块读取/计算 SHA-256，检查取消及读取前后 identity/size/mtime/mode；文件变化时有限重试。64KiB 内的小文本可保留在内存，较大文本在私有 0700 临时目录的 0600 文件；二进制/非法 UTF-8、超限或预算不足的内容仅保留元数据与 hash。这里的 UTF-8 检查只决定保留策略，不替代 R3 Classifier。超大文件仍可能产生完整流式哈希 I/O，但不按文件大小分配内存。

Reset 构建新一代成功后一次发布，峰值可能临时持有两代有界内容，加扫描清单、I/O 缓冲及调用方返回副本。扫描清单内存仍随条目数增长；kqueue 可能内部为文件分配描述符，不能把目录上限当成文件描述符总额。Close 清理缓存；进程被强杀后的私有缓存自动清扫尚未实现。持续监控压力/性能预算留到 R5，当前不宣称 P12 指标通过。

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
internal/app/          Prepare 与 Session 生命周期、Reset
scripts/smoke.py       R1 单次扫描回归
scripts/watch_smoke.py R2 持续监听、真实 Vim、Ctrl+C/清理
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

`make lint` 执行 gofmt 检查及 `go vet`。CI 配置 macOS/Linux、Go 1.23/1.26 的 build/test/race/smoke，并交叉编译 macOS/Windows。实际执行结果以 [Actions](https://github.com/StevenWinsir/FolderWatch/actions) 和 R2 验收记录为准，配置存在不等于远端运行已通过。Windows 目前只要求编译，不宣称运行时正式支持。

下一轮从 R3 / P6–P8 开始：复用 Session 的请求、generation 与 Snapshot Ref，实现 Classifier、Diff 和单一 ChangeStore。`Reconcile=true` 必须核对真实磁盘；恢复为基线 hash 后的列表消失由 R3 负责，不能在 UI 特判。贡献规范见 [CONTRIBUTING.md](CONTRIBUTING.md)。直接依赖及许可证记录在 ADR-001；项目尚未指定对外分发许可证，不应据此假定开放源代码许可。
