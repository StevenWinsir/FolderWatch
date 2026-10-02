# Gate A — Terminal 阶段一发布闸门

**Gate A: FAIL — 自动化工程检查通过，但人工/分发验收尚未签字。**

Date: 2026-10-02

Candidate version: v0.1.0-rc.1（私有评估候选，不是公开v1）

Commit / artifact provenance: 以 [R5 acceptance](../rounds/R5-acceptance.md) 集成证据及候选manifest为准；dirty预演不得作为签字对象。

Test macOS / hardware: 本机26.6.2 / Apple M4 / 32GiB；远端CI平台另见实际run。

Manual reviewer / approval date: **未填写；未验收**。

## 自动化证据与人工签字分开记录

下表“自动化PASS”只证明已有测试路径，不自动勾选右侧人工验收。所有最终人工步骤应在同一个已记录commit/checksum的候选上，分别记录Terminal.app与iTerm2版本、终端尺寸、机器架构、测试人和结果；失败回到责任package修复。

| 验收项 | 已有自动化证据 | Terminal.app / iTerm2 人工结果 |
|---|---|---|
| 普通保存、重复保存、atomic save | watcher/semantic集成、真实Vim两种保存PASS | 未执行 / 未执行 |
| 新文件、删除、新目录及后续子文件 | 原生事件与最终状态/PTY PASS | 未执行 / 未执行 |
| 恢复原baseline后列表清除 | Core与PTY恢复、20轮burst修改/恢复PASS | 未执行 / 未执行 |
| Reset、暂停中Reset、恢复同一baseline | Session控制与PTY确认流程PASS | 未执行 / 未执行 |
| 二进制元信息、不乱码、超大文件不OOM | 分类/预算/渲染回归及64MiB流式场景PASS | 未执行 / 未执行 |
| 空格/Unicode路径、控制字符安全显示 | CLI/PTY/安装验证PASS | 未执行 / 未执行 |
| resize、键盘、鼠标/无鼠标fallback | 21项真实PTY smoke、模型/golden PASS | 未执行 / 未执行 |
| Ctrl+C与q退出、终端/cache/goroutine/fd清理 | SIGINT130/q0、native/Session重复启停、资源测量PASS | 未执行 / 未执行 |

## 工程检查

- [x] 本机完整 `go test ./...`：299个测试/子测试PASS，0失败/跳过。
- [x] 本机完整 `go test -race -shuffle=on -count=10 ./...` PASS；已有R1–R4回归保留。
- [x] 六类有界fuzz、四target编译、vendor守卫与Core无TUI传递依赖验证PASS。
- [x] [性能报告](../performance-v1.md) 与原始样本已记录，10k本机参考预算达到；所有度量注明边界。
- [x] clean `f3e662f`双架构打包/checksum、arm64真实安装/运行、两次构建逐字节一致通过；没有运行时Go依赖。具体manifest在R5 acceptance中记录。
- [x] 同一源码的push CI36988380384、PR CI36988453642各7个job全部success，含四组平台/Go版本矩阵、fuzz、跨构建和候选安装；不等于人工签字。
- [x] 已发现native fd泄漏及发布许可证路径问题均在责任层修复并补回归；已执行检查中没有已知未解决P0/P1实现故障。
- [ ] [R5 PR #5](https://github.com/StevenWinsir/FolderWatch/pull/5)完成人工评审/合并；最后文档head检查按PR Checks观察，不凭workflow配置推断。
- [ ] 在实际Terminal.app与iTerm2完成并签署上述所有人工流程。
- [ ] 在真正干净Mac上安装已批准候选，确认架构、版本/checksum、终端交互及Gatekeeper行为；当前隔离HOME/PATH不等同此项。
- [ ] 确认Intel分发范围并在相应真实环境验收；amd64交叉编译/校验不等同原生运行。
- [ ] Owner记录项目公开分发许可、Homebrew分发/认证策略及签名/公证决策；私有候选不擅自授予许可证。

## 签字对象与操作记录

候选必须包含两种架构archive、manifest、SHA256SUMS、formula及许可证，`--version`与manifest完全一致。签字前记录：candidate commit、每个archive SHA256、macOS版本/架构、Terminal.app与iTerm2版本、安装来源/工具、测试人、日期、逐项结果和Known Issues。生成默认GitHub Release URL不意味着资产已上传；不得用未发布URL或dirty版本假装可安装的正式发布。

任何未完成项都不得把本文件改为PASS。完成真实签字与项目许可决策后，按[发布说明](../release/terminal-candidate.md)批准明确的候选，再决定是否放行R6。**本轮没有GUI功能、没有自动合并，也没有公开Release/tag/tap发布。**
