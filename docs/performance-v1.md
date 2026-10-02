# FolderWatch Terminal v1 — R5 性能与资源报告

测量日期：2026-10-02。结论：**本机记录场景达到 P12 首轮工程预算；这不是跨硬件、小时级稳定性或 Gate A 人工签字。** 原始数据与 profiles 摘要见 [benchmarks/r5](benchmarks/r5/)，验收见 [R5 acceptance](rounds/R5-acceptance.md)。

## 环境与测量口径

Apple M4，10 logical CPU，32GiB RAM，macOS 26.6.2，darwin/arm64，Go1.26.6。系统卷查询为 APFS、SSD。数据来自本机临时目录的合成文件，刚生成后读取，未清除文件系统缓存；不是网络盘或真实大型工程的普遍结论。执行环境 JSON 保留测量时 HEAD=`a775148`、`dirty=true`：测量针对本 PR 的 R5 工作树，非未经修改的 R4 提交。

`cmd/fwbench` 是开发探针，不打入发布的 folderwatch；fixture、私有缓存、日志和 profiles 均为生成数据。每个场景独立进程，20 次 burst，默认 debounce=150ms。启动计时包括 Prepare 的配置/扫描、watch 注册与完整基线；另报告 Pause/Resume owner 屏障后的 settled 时间，避免把启动 reconciliation 算成空闲。每次 burst 结束于最后一次写入返回，终点为 Core 权威变化集合的精确数量和 SHA-256 全部吻合。没有仅凭事件数或固定 sleep 宣告成功。

CPU 使用进程 user+system 时间除以实际观察时长，100%=占满一个核；每场景空闲观察约10秒，确认没有语义事件。Heap 是 GC 后 Go HeapAlloc；累计分配、HeapSys、逻辑 retained content 和进程峰值 RSS 分别报告，不混为同一指标。RSS 包括探针、fixture 写入缓冲与 profile 开销；尤其64MiB fixture 自身占内存，不能归因为应用保留了64MiB二进制正文。P95 使用20个样本的 nearest-rank 第19个，有采样粗糙度；没有统计置信区间。

## 目录规模、burst 与资源结果

以下 MB 为十进制；文件尺寸中的 MiB 为二进制单位。每组结束都检查原始缓存目录为空，进程 goroutine 回到1，观察到的 fd 回到6。

| 场景 | Prepare→ready ms | settled ms | GC 后 heap MB | 峰值 RSS MB | idle CPU % | Core P95 ms | Diff 结果 |
|---|---:|---:|---:|---:|---:|---:|---|
| 1k × 1KiB，100文件 burst | 39.59 | 106.57 | 3.11 | 19.25 | 0.417 | 166.34 | text |
| 10k × 1KiB，100文件 burst | 454.87 | 836.90 | 27.29 | 72.73 | 0.517 | 170.31 | text |
| 单个5MiB文本 | 9.39 | 25.43 | 5.70 | 47.79 | 0.411 | 194.26 | text，单次 Diff 53.27ms |
| 单个10MiB文本 | 4.84 | 12.68 | 10.94 | 28.93 | 0.429 | 168.25 | too-large |
| 单个64MiB、含NUL二进制 | 28.91 | 53.64 | 67.57 | 142.59 | 0.406 | 218.23 | too-large |
| 32层目录，100 × 1KiB | 25.91 | 48.56 | 0.86 | 16.61 | 0.434 | 196.32 | text |
| 10文件，另加50次完整启停 | 1.29 | 1.82 | 0.48 | 13.71 | 0.430 | 174.23 | text |

64MiB二进制先命中8MiB分类/快照阈值，报告 too-large 而不是伪装完成全文二进制分析。5MiB文本是单长行、非复杂多行最坏 Diff；复杂度预算由已有LCS/行数边界与 fuzz 回归验证。以上大文件均被检测到，未出现 OOM 或文本乱码。

10k 行场景保留正文恰为10,240,000 bytes，ready455ms和settled837ms均低于3秒预算；进程高水位72.73MB低于150MB参考预算。10k active goroutine=6（包含CPU profiler），关闭后=1；active fd 观测约10k，关闭后由6回到6。**活动fd仍随文件数量增长**，不是每目录一个fd或无限容量承诺；OS fd 配额不足必须可见失败。

## 真正的 TUI 输出延迟

`scripts/tui_performance.py` 启动真实 folderwatch 二进制，PTY视口120×36，10k×1KiB文件，`--no-mouse`，显式150ms debounce；20轮交替修改100文件/全部恢复baseline。每轮仅接受新的变化计数输出（100/0交替），最后 q=0 并核验终端flags、光标、alt-screen与空缓存。该测量不是 fake model 或直接调用 View。

**从最后写入返回到新 TUI 输出的 P95=196.57ms，包含debounce。** 减去名义150ms得到46.57ms仅是便于比较的估计，不是内部debounce结束时刻的精确埋点。含debounce的总路径已低于250ms参考预算；PTY字节输出不能证明 Terminal.app/iTerm2 compositor像素延迟或代替人工视觉验收。轮询粒度约2ms，原始20个样本全部保留。

## 清洁提交追加的持续资源检查

生产源码提交`f3e662f2059d47a0b9f108b8b8d0b17dcb760fe4`重新编译探针后，另外执行1k×1KiB、100文件burst×100轮、120秒空闲以及100次完整启停。UTC 09:13:10.924784开始、09:15:35.936662结束，实际整体约145.01秒。空闲120.001秒CPU=0.3929%，语义事件0；burst Core P95=166.34ms；峰值RSS20.69MB；fd 6→1010→6、goroutine1→5→1、缓存0，全部正确性和清理断言PASS。原始100样本、命令与时间在`soak-clean.json` / `soak-environment.json`，与前面的短场景分开记录。这扩展了观察窗口，仍不是小时或天级soak。

## 测量发现并修复的实际 fd 泄漏

最初 macOS `/dev/fd` 的 Go ReadDir 不可用，旧探针输出fd=-1；未把它视为无泄漏证据。改用 `/usr/sbin/lsof -p <self> -F f`，只统计数值fd，排除cwd/text映射，观察管道在前后采样中一致；Linux使用 `/proc/self/fd`。不支持测量时基准失败，不默认为0。

修复前1000文件样本：**fd 6 → 1010 active → 1007 after Close**，缓存却已清空。根因为 fsnotify v1.8.0 的 kqueue Close 先关闭 done，再调用 Remove，而 Remove 的 closed guard 直接返回；所有 watch fd 没有回收。证据保存在 `pre-fix-descriptor-leak.json`，明确是失败样本。

修复扩展原有可复现 vendor patch：串行化单fd注册/移除与关闭屏障，目录递归在锁外；Close唤醒并等待reader，再关闭剩余owned fd、清空映射，所有并发Close等待同一完成信号。新fd不能穿过关闭屏障；native open和kqueue设置close-on-exec。已有O_NOFOLLOW、目录失效校准、未知fd overflow和seen退休逻辑保留。详见 [ADR-014](adr/014-terminal-resource-hardening-and-release.md)。

新增原生128文件×20启停、4并发Close、Add/Close竞态与30轮Session Pause/Reset/Close回归；专项race10轮与全包race随机顺序10轮均通过。修复后七组场景的fd全部回到起始6、goroutine回到1、缓存0。这证明这些有界观察内无线性残留，不是小时/天级耐久度的替代。

## 分配优化与 profile

快照32KiB读取缓冲在最初微基准中没有表现为每文件32KiB堆分配，因此未凭猜测引入全局pool。实际优化为小文本 bytes.Buffer 在既有保留预算内预分配，并将单次capture独占的buffer转交snapshot，去掉结束时的第二次正文复制。ReadContent仍返回防御副本；新增不可变性/独立删除回归。

同机三次 `-benchtime=1x` 的10k×1KiB Reset微基准：修改前约53.41MB/op、319,979–320,007 allocations；修改后约42.69–43.18MB/op、309,971–309,999 allocations。减少约10MB和每文件一次分配，约19%；中位耗时199.02ms→197.69ms，仅视为相近，不宣称显著速度提升。该 B/op 是累计分配，不是RSS。

TUI模型接收并渲染10k变化列表的单次微基准为1.39–1.79ms、约6.21MB/op；100条为0.14–0.45ms。1500行Diff的可见窗口渲染约0.116–0.135ms、5,264B/op。列表整体副本仍与条目数线性增长；没有为了低分配绕过权威状态/版本保护。

10k CPU profile包含启动、10秒空闲、bursts与关闭：14.57秒墙钟，1.64秒采样，约73.8% flat落在syscall，约6.1% runtime.kevent；不是单独 idle CPU profile。heap采样中保留正文buffer、path字符串、snapshot与watch索引为主要项；profile有采样误差，不能将采样估计加总当成精确HeapAlloc。goroutine profile与CPU/heap二进制保留在本地 `artifacts/performance-r5-fixed/`，可重跑生成；Git保留原始指标JSON与可读top摘要，不提交大体积二进制profile。

## 边界与复跑

raw/pending上限默认4096，Session事件输出32、聚合输出1；有界溢出以reconciliation恢复精确最终状态，新增不消费Events的100文件burst回归。每watcher一个1秒root身份ticker，空闲不扫描子树；retry有界退避，不构成永久全盘轮询。基线32MiB内存/256MiB磁盘为每代逻辑预算；Reset可能短暂两代，按需Diff另有5MiB/20k行/200万工作单元限制。不存在新常驻current正文或多Diff缓存。

```sh
make lint test race smoke scripts-test
go test ./internal/snapshot ./internal/tui -run='^$' -bench=. -benchtime=1x -count=3 -benchmem
python3 scripts/performance.py --output artifacts/performance-new --idle-seconds 10 --samples 20
python3 scripts/tui_performance.py bin/folderwatch --files 10000 --samples 20 --output artifacts/tui-new.json
go tool pprof -top bin/fwbench artifacts/performance-new/10k.cpu.pprof
go tool pprof -top bin/fwbench artifacts/performance-new/10k.heap.pprof
```

目录必须尚不存在，工具拒绝覆盖证据。未知硬件、冷缓存、机械/网络盘、长时间压力、真正干净Mac与实际终端人工验收仍需另测。强杀后的缓存自动清扫、恶意并发目录替换的原子沙箱、可靠rename关联、Ignore热更新不在本轮实现。Gate A最终状态见独立文件，不能由性能报告自动签字。
