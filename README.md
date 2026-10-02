# FolderWatch

本地文件夹变更检查工具，macOS 优先。长期目标是用同一 Go core 驱动 Terminal/TUI 与 GUI，比较当前文件和 session 启动基线，而不是只比较上一次保存。

**当前交付：R1 / P0–P2。** 已实现工程基础、CLI/Config、路径规范化、递归元数据扫描和 Ignore。当前命令扫描一次后退出，**不会持续监听，不会建立内容 baseline，也没有 diff、TUI 或 GUI**。后续路线见 [Handoff_Rounds.md](Handoff_Rounds.md)，本轮结果见 [R1 acceptance](docs/rounds/R1-acceptance.md)。

## 构建与使用

需要 Go 1.23+；CLI 冒烟测试另外需要 Python 3。Git 仅供开发和 Ignore 对照测试使用；被扫描的目录不需要是 Git 仓库。

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
```

仓库当前为私有，clone 需要访问权限。省略 path 时扫描当前目录；选项可放在 path 前后，`--` 终止选项解析。带空格的路径和 glob 请加引号。R1 默认命令也等价于单次扫描；未来接入 TUI 后仍保留显式 `--scan` 诊断模式。`make run ARGS='--scan --json .'` 可不生成二进制直接运行。

文本输出会转义文件名中的控制字符。JSON 包含 `root`、`entries`、`warnings`；每项有 `path`、`kind`、`size`、`mode`、`mod_time`，不包含文件内容。根目录键为 `.`，其他键为根目录相对、`/` 分隔、保留大小写和 Unicode 的路径。目录、普通文件、symlink、其他特殊文件分别标记为 `directory`、`file`、`symlink`、`other`。遍历顺序确定，遵循 `filepath.WalkDir` 的词法遍历顺序。

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
no_mouse = false
editor = ""
log_file = ""
debug = false
# ignore_file = "rules.ignore"  # 设置后必须存在
```

完整示例见 [testdata/config.example.toml](testdata/config.example.toml)。未知键、类型错误、无效 glob、非法 duration/size 都会报错，包括被后续层覆盖的非法配置。高优先级 `ignore` 数组**替换**低优先级数组；`ignore = []` 可清空。CLI 重复 `--ignore` 构成最高优先级数组。布尔选项支持显式覆盖，例如 `--respect-gitignore=false`。

配置文件内的路径相对于该 TOML 文件；CLI 路径相对于工作目录。支持 `~` / `~/...`，不展开环境变量或 `~otheruser`。自动发现的项目配置和规则文件不跟随 symlink；显式选中的额外 ignore 文件可以是 symlink。配置和规则文件都必须是普通文件，单文件读取上限 1 MiB。

| 选项 | R1 行为 |
|---|---|
| `--scan`、`--json` | 单次元数据扫描；可输出 JSON |
| `--ignore <pattern>` | 可重复的 Ignore 规则 |
| `--ignore-file <path>` | 附加规则文件，必须存在；不替代根 `.folderwatchignore` |
| `--respect-gitignore` | 启用根目录及嵌套 `.gitignore`，默认 false |
| `--include-git` | 关闭内建 `.git/` 排除规则，默认 false |
| `--debounce <duration>` | 校验并保存，默认 150ms；R2 才执行 debounce |
| `--max-diff-bytes <size>` | 校验并保存，默认 5MiB；R3 才限制 diff，不影响扫描集合 |
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

规则按 matcher/session 缓存，包含未找到文件及错误结果。新出现的目录首次处理时加载其规则；修改已缓存的规则文件需要重新启动/新建 Matcher。R2 若增加热更新，必须统一重建 Matcher 并 reconciliation，不能单独实现运行时过滤器。

## 错误、资源与隐私

单个文件消失、子目录权限不足等会产生 warning 并继续处理其他项；无法读取 root 是致命错误。嵌套规则文件不可读或无效时跳过对应子树，而不是忽略错误后扩大扫描范围。带 warning 的清单可能不完整，不能把它作为“所有项均已成功读取”的证明。

R1 不读取普通文件正文，不打开 FIFO/device 内容，也不因扩展名或文件大小过滤文件。5 GiB 稀疏文件在 smoke 中仅做元数据检查。扫描清单占用与条目数成正比，10k 文件性能与持续监听资源治理留到 R5。不会写入扫描目录、执行 editor、创建 log/snapshot，也不联网或上传扫描内容。开发时 Go 下载依赖以及 GitHub 提交不属于应用运行行为。

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
internal/app/          Prepare，组合上述 core
scripts/smoke.py       真实 CLI 的隔离临时目录验收
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

`make lint` 执行 gofmt 检查及 `go vet`。CI 配置 macOS/Linux、Go 1.23/1.26 的 build/test/race/smoke，并交叉编译 macOS/Windows。实际执行结果以 [Actions](https://github.com/StevenWinsir/FolderWatch/actions) 和 R1 验收记录为准，配置存在不等于远端运行已通过。Windows 目前只要求编译，不宣称运行时正式支持。

后续从 `app.Prepare` 返回的 `Config`、`Matcher`、`Inventory` 开始 P3–P5；扫描元数据不是内容 baseline。贡献规范见 [CONTRIBUTING.md](CONTRIBUTING.md)。直接依赖及许可证记录在 ADR-001；项目尚未指定对外分发许可证，不应据此假定开放源代码许可。
