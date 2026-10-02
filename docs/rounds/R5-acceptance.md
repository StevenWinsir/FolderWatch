# R5 acceptance — P12–P14 Terminal 工程硬化与候选发布

日期：2026-10-02。**工程自动化验收通过；Round 状态 IN_REVIEW；Gate A=FAIL（待人工/分发签字），不可进入R6。** 具体提交/PR与远端CI在下方“集成证据”记录，未观察的检查不得写为PASS。

## 起点与工作区保护

已完整阅读Handoff。R4主PR#3于2026-10-02 08:23:07 UTC合并为main `bb3a75e`。发现原目录有另一窗口处理R4原生测试同步，因此本轮在 `/Users/stevenlee/Desktop/diffList/.r5-worktree` 的 `feat/r5-terminal-release` 隔离worktree工作，没有覆盖原checkout。随后快进纳入 `a775148decf0655575c0dc2933d2aaddd9d8865f`，即[PR #4](https://github.com/StevenWinsir/FolderWatch/pull/4)的R4补充；实查该PR仍OPEN，本轮PR包含这些前置提交，不冒充已合并。

## 本轮交付

| 阶段 | 实际完成 | 证据/边界 |
|---|---|---|
| P12 | 1k/10k、100文件burst、5/10MiB文本、64MiB二进制、32层目录、50次启停；CPU/heap/goroutine profile；真实二进制PTY延迟；快照少一次正文复制 | [性能报告](../performance-v1.md)、原始JSON；硬件/观察时长明确 |
| P12 核心修复 | 真实测量发现kqueue Close泄漏全部watch fd；原生owner关闭屏障、reader join、fd回收、并发Close与CLOEXEC；可复现vendor patch/manifest | [ADR-014](../adr/014-terminal-resource-hardening-and-release.md)、native/app回归；不在UI规避 |
| P13 | 保留所有R1–R4 unit/integration/golden/model/CLI/PTY；新资源/overflow/ownership回归；event normalize fuzz；六目标fuzz脚本；Core传递依赖边界 | 全包测试/10轮race、六fuzz、四smoke |
| P13 CI | macOS/Linux×Go1.23/1.26、随机race、coverage/log artifacts、fuzz、四target编译、macOS原生候选安装与资源smoke | workflows已实现，实际远端结果单独记录 |
| P14 发布工程 | SemVer、版本/full commit/build-date、vendor/CGO0双macOS架构、确定性tar/gzip、manifest/SHA256SUMS/许可证、checksum专属Homebrew formula、私有候选workflow | 安装步骤与人工边界见[发布说明](../release/terminal-candidate.md) |
| P14 Gate | Gate A独立文件、README/CHANGELOG、原文Handoff状态与新增§29 | 人工Terminal.app/iTerm2、clean-Mac、许可证未获签字；未发布公开v1/GUI |

## 本机实际验证

macOS26.6.2 / Apple M4 / 32GiB / Go1.26.6 / darwin-arm64。日志位于本worktree `artifacts/validation-r5/`；命令退出码与计数取自执行结果，不根据源码中的测试函数数推算。

| 验证 | 实际结果 |
|---|---|
| `go test -count=1 ./...` 与JSON/coverage重跑 | PASS；299个测试及子测试、137个顶层测试/模糊目标；失败0、测试级跳过0 |
| `go test -race -shuffle=on -count=10 ./...` | PASS，全包10轮；专项native Close/Session生命周期/ownership/descriptor测量另有10轮race PASS |
| `make lint` / `go build ./...` | PASS；gofmt、go vet、vendor SHA/补丁可逆性/实际依赖路径与Core/UI传递依赖检查 |
| `make scripts-test` | 最终8个发布工具回归PASS，包括Homebrew Go许可证布局补充 |
| R1/R2/R3/R4 `make smoke` | 60 / 17 / 16 / 21 项PASS；真实headless Vim backupcopy=yes/no；真实PTY含终端恢复/缓存清理；外部CI=true环境 |
| 六目标fuzz，每目标10秒、2workers | PASS：config 895,563；path 1,205,940；eventnorm 1,158,498；classifier 1,098,659；snapshot probe 1,867,222；diff 1,751,964次执行；没有崩溃样本 |
| darwin/arm64、darwin/amd64、linux/amd64、windows/amd64 `go build ./...` | 四target PASS；Windows只声明编译，不声明正式运行支持 |
| `go mod verify` / `go mod tidy -diff` | PASS；依赖版本/graph没有改变，vendor补丁变化有独立hash与ADR |
| 七组性能/资源、10k真实TUI 20轮 | PASS；10k ready455ms、settled837ms、RSS72.73MB、idle0.517%、Core P95 170.31ms、TUI P95 196.57ms；每组goroutine1、fd6、缓存0收尾 |
| dirty候选预演：双架构校验/native arm64安装 | PASS；版本精确、Unicode scan、真实watch/hash、SIGINT130、空缓存、隔离HOME/PATH；该预演显式dirty，不冒充正式候选 |

父子测试计数不是299个独立业务场景；真实子进程PTY不计入Go进程内coverage。Go测试范围包括开发探针包，不能把全仓库coverage和业务core覆盖率混称。CI随机fuzz次数会不同，本表仅为这次本机观察。

## 失败如何处理

1. 初版Darwin fd枚举不可用，改为可校验lsof测量，未把-1当成无泄漏；继而发现6→1010→1007的真实native fd泄漏，修复责任vendor层并同步新增回归。失败JSON原样保留。
2. vendored模块声明Go1.17语言；初版使用clear导致编译失败，改为兼容的typed map重建，未升级依赖掩盖问题；重跑native专项与全包10轮race通过。
3. Homebrew将Go LICENSE放在libexec上一级，发布预演先明确失败；新增两种实际布局的查找/内容校验与回归，缺许可证仍拒绝打包。随后双架构打包、native安装通过。

没有删除hash、版本防回退、原生事件、终端恢复或缓存清理断言，也没有把未知路径/权限错误全部吞掉。R4两个测试同步补充来自前置PR #4，生产语义不是通过改测试放宽。

## 集成证据

开发分支：`feat/r5-terminal-release`；基线：`a775148`（R4补充PR #4，含已合并PR #3）。本机工程检查已通过；提交、clean候选与R5 PR/CI的最终标识在完成实际操作后追加到本节，不使用旧R4 CI冒充R5检查。

## 剩余验收与已知限制

人工Terminal.app、iTerm2、真正clean-Mac安装未执行，未填写不存在的测试人/签字；隔离HOME/PATH不等于另一台Mac。Homebrew formula已生成实际checksum，公开tap/私有认证下载和brew安装验收未完成；默认Release URL只有发布对应资产后才可用。没有项目公开分发许可证决策、Developer ID签名或Apple公证，也未创建release/tag或自动合并PR。CPU观察每组10秒、额外50次启停，不宣称小时/天级耐久度。

当前已执行范围内无已知未解决P0/P1级实现故障；这不替代人工排查或安全审计。强杀缓存自动清扫、Ignore热更新、可靠rename关联、editor执行、GUI均未新增。保持唯一Go Core，Gate A真正签字后才进入R6。
