# Gate A — Terminal 阶段一发布闸门

**Gate A: PASS — P0–P14 工程检查与 `v0.1.0-rc.1` 人工验收均已完成。**

Date: 2026-10-02

Candidate version: v0.1.0-rc.1（私有评估候选，不是公开v1）

Commit / artifact provenance: 以 [R5 acceptance](../rounds/R5-acceptance.md) 集成证据及候选manifest为准；dirty预演不得作为签字对象。

Test macOS / hardware: 本机26.6.2 / Apple M4 / 32GiB；远端CI平台另见实际run。

Manual reviewer / approval date: **仓库所有者确认通过 / 2026-10-02**。人工验收时未把 Terminal.app/iTerm2 的具体版本号与 clean-Mac 机器型号写入仓库，本次文档同步不追填或猜测不存在的元数据。

## 自动化证据与人工签字分开记录

下表保留自动化与人工证据边界。仓库所有者已确认同一 `dist/v0.1.0-rc.1` 候选完成人工 Gate A；具体终端版本/尺寸等细粒度元数据当时未归档，因此这里只记录可确认的 PASS，不虚构环境字段。后续 Gate 应在执行时同步记录完整环境元数据。

| 验收项 | 已有自动化证据 | Terminal.app / iTerm2 人工结果 |
|---|---|---|
| 普通保存、重复保存、atomic save | watcher/semantic集成、真实Vim两种保存PASS | PASS / PASS（owner确认） |
| 新文件、删除、新目录及后续子文件 | 原生事件与最终状态/PTY PASS | PASS / PASS（owner确认） |
| 恢复原baseline后列表清除 | Core与PTY恢复、20轮burst修改/恢复PASS | PASS / PASS（owner确认） |
| Reset、暂停中Reset、恢复同一baseline | Session控制与PTY确认流程PASS | PASS / PASS（owner确认） |
| 二进制元信息、不乱码、超大文件不OOM | 分类/预算/渲染回归及64MiB流式场景PASS | PASS / PASS（owner确认） |
| 空格/Unicode路径、控制字符安全显示 | CLI/PTY/安装验证PASS | PASS / PASS（owner确认） |
| resize、键盘、鼠标/无鼠标fallback | 21项真实PTY smoke、模型/golden PASS | PASS / PASS（owner确认） |
| Ctrl+C与q退出、终端/cache/goroutine/fd清理 | SIGINT130/q0、native/Session重复启停、资源测量PASS | PASS / PASS（owner确认） |

## 工程检查

- [x] 本机完整 `go test ./...`：299个测试/子测试PASS，0失败/跳过。
- [x] 本机完整 `go test -race -shuffle=on -count=10 ./...` PASS；已有R1–R4回归保留。
- [x] 六类有界fuzz、四target编译、vendor守卫与Core无TUI传递依赖验证PASS。
- [x] [性能报告](../performance-v1.md) 与原始样本已记录，10k本机参考预算达到；所有度量注明边界。
- [x] clean `f3e662f`双架构打包/checksum、arm64真实安装/运行、两次构建逐字节一致通过；没有运行时Go依赖。具体manifest在R5 acceptance中记录。
- [x] 同一源码的push CI36988380384、PR CI36988453642各7个job全部success，含四组平台/Go版本矩阵、fuzz、跨构建和候选安装；不等于人工签字。
- [x] 已发现native fd泄漏及发布许可证路径问题均在责任层修复并补回归；已执行检查中没有已知未解决P0/P1实现故障。
- [x] [R5 PR #5](https://github.com/StevenWinsir/FolderWatch/pull/5)已合并为 main `6a470a0`；前置R4补充PR #4已合并为`0ebf250`。
- [x] 实际Terminal.app与iTerm2人工流程已由仓库所有者确认PASS；具体应用版本未在当时归档。
- [x] clean-machine / 实际安装验收已由仓库所有者确认为本次Gate A签字的一部分；候选checksum见下。
- [x] 分发范围与阶段一发布决策已由owner纳入本次Gate A批准；这不等于已经创建公开GitHub Release、tap、Developer ID签名或Apple公证。

## 签字对象与操作记录

本次签字对象为 clean `f3e662f2059d47a0b9f108b8b8d0b17dcb760fe4` 构建的本机 `dist/v0.1.0-rc.1/`：darwin-arm64 SHA256 `69eb126149a38656859df78c83c1964abefc00c9116c123333443bb9fea6c80d`；darwin-amd64 SHA256 `b676becd596b27dbf672aaf5ce7732b395499c75519ac7744db53691df72fbef`。候选包含两种架构archive、manifest、SHA256SUMS、formula及许可证，`--version`与manifest一致。人工验收的具体Terminal.app/iTerm2版本、终端尺寸和clean-Mac型号未在当时归档；这是证据粒度限制，不改写为虚构值。

**Gate A 已批准上述明确候选，R6 可以开始。** 本结论不表示已经创建公开Release/tag/tap，也不声称Developer ID签名或Apple公证已经完成；这些是实际公开分发时的独立动作。本轮没有GUI功能。
