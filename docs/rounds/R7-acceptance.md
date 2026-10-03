# FolderWatch R7 Acceptance — P17–P18

**日期：2026-10-02**
**范围：Folder Picker、session status、changed files list/filter、Monaco Diff Editor**
**结论：实现与本地自动化 PASS；Round 集成 IN_REVIEW。**

## 交付

- 在现有 Wails/Svelte 壳层上加入原生目录选择：Go API 暴露 `SelectFolder` DTO，Wails 入口注入 `OpenDirectoryDialog`，取消选择返回空路径，不会启动会话。
- 主界面加入 FolderWatch 的监控工作区：目录输入、Choose folder、Start watching、Stop、Scanning/Monitoring/Paused 状态、generation/version/change count 和错误/警告状态。
- 新增 `WorkspaceController` 作为前端 view adapter。它只持有列表/选择/加载状态，使用既有 `GetChanges`/`GetDiff` IPC；不会复制 watcher、baseline、classifier 或 diff 逻辑。
- 变化列表支持 Added/Modified/Deleted 标识、文件大小、路径过滤、空列表、无匹配结果、加载态和键盘方向键/Home/End 导航。
- 新增 Monaco read-only side-by-side Diff Editor。文本 diff 使用 Go 返回的 hunks/line kinds 建模；Binary、Unsupported Text、Too Large、Unavailable 只显示说明，不进入编辑器渲染。
- 通过 refresh epoch、diff request epoch、path/generation/version 校验防止快速切换和 reset/change 后旧响应覆盖当前选择。Monaco 模型只在 path/generation/version 真正改变时替换，避免异步渲染在窗口切换时访问已移除 DOM。
- 前端新增 `monaco-editor@0.52.2` 精确依赖，重新生成 Wails API/models bindings；GUI 继续复用同一 Core Facade 和 IPC v1。

## 验收证据

本机环境：macOS 26.6.2 arm64、Go 1.26.6、Node 26.10.0（项目声明支持 Node 22/24）、Chrome、Wails v2.10.1。浏览器插件不可用，因此按既有流程使用 Playwright + 本机 Chrome；原生 WKWebView 的辅助功能/屏幕录制权限仍未授予。

| 检查 | 结果 |
|---|---|
| `make gui-check`（Svelte check、Vitest 3 files/11 tests、Vite build） | PASS |
| `make lint`、`go test -count=1 ./...`、GUI backend/host race | PASS |
| `make gui-build VERSION=0.2.0-dev`，产出 `gui/build/bin/FolderWatch.app` | PASS |
| `FW_GUI_START_SERVER=1 FW_BROWSER_CHANNEL=chrome make gui-e2e` | PASS，2/2，13.1s |
| 真实 Wails dev WebSocket → Go binding → core | PASS；不是 browser mock |
| Start/Stop/Start、Unicode/空格/引号路径、真实变化、结构化文本 diff | PASS |
| Binary/TooLarge/Unsupported 状态和旧请求围栏 | PASS（backend 与前端状态路径） |
| Pause/Resume/Reset、reload revoke、非法路径拒绝 | PASS |
| 680×480 和 380×800 CSS 压力、无横向溢出、无 Vite overlay/page error/HTTP error | PASS |

Playwright e2e 首次发现 Monaco 在 session stop 时异步 view layer 仍访问已移除节点；保留 Diff Editor 容器并延迟到首次文本 diff 才创建编辑器，同时用稳定模型 key 围栏更新，修复后 2/2 通过。前端 jsdom 测试对缺少 `matchMedia` 的环境保留说明态，不伪造 Monaco runtime。

## 已知边界

- 本轮没有把原生 macOS folder picker 的人工点击、按钮/菜单辅助功能签字冒充已完成；browser e2e 验证的是 Wails dev transport 和 Go binding。需要在拥有辅助功能/屏幕录制权限的机器补充 native smoke。
- `gui/build/bin/FolderWatch.app` 是 unsigned development build，不是 Developer ID 签名、公证或 Gate B 发布候选。
- Monaco 初始 bundle 约 2.36 MB（gzip 约 616 KB），后续可在 R8/R9 做代码分割；不影响当前功能验收。
- 旧的 lease、sleep/wake、settings、external editor/Finder、long-running GUI soak 仍归 R8/R9。
