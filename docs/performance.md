# 性能优化实践

本文档面向的目标场景：**万级连接/秒**下做到无锁竞争、低 GC 压力、低延迟毛刺。所有结论可对照对应源码复核。

## §1 无锁读：Copy-on-Write 快照

**问题**：`rules.Engine` 早期用单一 `sync.RWMutex` 保护全部 ACL 字段。热重载或健康检查触发**写锁**时，所有新建连接被阻塞 → CPU 抖动 + 延迟毛刺。

**方案**：`ruleSet` 不可变快照 + `atomic.Pointer` 原子交换（`internal/rules/engine.go`）。

```go
type ruleSet struct { /* 全部 ACL 数据：maps / chnroute.Trie / suffixTrie ... */ }
type Engine struct {
	rules atomic.Pointer[ruleSet]
}
```

- 读者只需一次 `e.rules.Load()` 拿到当前快照指针，之后**无锁**读取；快照只读、绝不修改。
- 写者（`Load` / `Reload`）构建**全新** `ruleSet`，全部解析成功后 `e.rules.Store(rs)` 一次性原子换入；解析失败则旧快照继续生效，只报错。
- 热路径方法 `IsPortBlocked` / `IsIPBlocked` / `IsDomainBlocked` / `MatchProxyRule` / `ProxyRules` 全部遵循"Load 后无锁取值"。

**类似模式**（同一套思想，不同载体）：

- `chnroute.Trie.root` 为 `atomic.Pointer[trieData]`，热重载走 `Pull` 原子换根（`internal/chnroute/trie.go`）。
- `config/dns/router/tun` 的配置快照均为 `atomic.Pointer`：`dns.Handler.cfg`（`dnsConfig`）、`route.Router.cfg`（`routerConfig`）、`tun.TUNHandler.config`、`engine.Config`，热重载时 `Store` 换新。

## §2 缓冲池

| 池 | 位置 | 大小 | 用途 |
| --- | --- | --- | --- |
| `bufferPool` | `internal/relay/tcp.go` | 32 KiB | TCP relay 的 `io.CopyBuffer` 缓冲 |
| `UDPBufPool` / `udpBufPool` | `relay` / `udp` | 65535 字节 | UDP 数据报缓冲 |
| `PacketPool` | `internal/relay/tcp.go` | 4096 字节 | DNS 查询短包缓冲 |
| `clientHelloBufPool` | `internal/tun/handler.go` | 4096 字节 | `ReadClientHello` 预读首包 |

- **TCP relay**：32 KiB 池化缓冲 + `tcpSplice` 内核零拷贝（两端都是 `*net.TCPConn` 时 `dst.ReadFrom(src)` 走 `splice(2)`，避免用户态拷贝，低 CPU）。
- **TUN ReadClientHello**：从 `clientHelloBufPool` 取池化缓冲预读，但返回给调用方的是**精确尺寸独立拷贝**（`out := make([]byte, exact)`），池复用不污染调用方、也不与调用方生命周期耦合。
- **UDP**：`udpBufPool` 65535 满尺寸；`buf.NewPacket` 分配 packet buffer、`Release` 归还；非托管缓冲 `buf.As` 的 `Release` 是 no-op。给 TUN 发已有数据必须用 `buf.As` 而非 `buf.With`（`With` 不设置 end 导致 `Bytes()` 返回空切片，会把 UDP/DNS 回包写成空数据报）。

## §3 并发去重

- **DNS singleflight**（`internal/dns/handler.go`）：`Handler.group singleflight.Group`，key 为 `qname + "|" + qtype`，同一域名同类型并发查询只发一次上游；共享结果由各 caller 修正自己的 DNS transaction ID。
- **UDP 会话创建**（`internal/udp/handler.go`）：`createGroup.Do(key, createUDPSession)` 串行化同一目标的会话建立，避免**并发首包重复拨号**与连接泄漏；TUN 侧 `getOrCreateRemote` 还用"锁外拨号 + 二次检查"避免重复建连。

## §4 减少每包 / 每连接开销

- **UDP 每包免解析 IP**：`udp.Handler` 构造时把 `clientIP` 用 `net.ParseIP` 解析一次存入 `clientIPParsed`，`HandlePacket` 每包直接复用，避免每包分配（`internal/udp/handler.go`）。
- **热路径日志降级**：高频日志从 `slog.Info` 改为 `slog.Debug`，在高查询/连接率下是纯开销：
  - DNS `"handling DNS query"`（`internal/dns/handler.go`）；
  - TUN `"new connection"`、`"extracted domain"`（`internal/tun/handler.go`）。
  - 规则 `"rules loaded"` 等在加载时打，不在热路径。

## §5 DNS 单次解析

`isDNSCleanAndPrefer`（`internal/dns/handler.go`）把**污染检查 + IP 优选**合并为**一次 `Unpack`**（原来 `Unpack` 两次）：

```go
func (h *Handler) isDNSCleanAndPrefer(ctx, wire, qname) (out []byte, preferCached, clean bool)
```

- 单次 `msg.Unpack(wire)` 后先遍历 Answer 做 chnroute 污染检查；
- 未污染且启用 IP 优选时才进入 `filterIPPreference` 过滤最快 IP 并重新 `Pack`；
- 返回 `(输出 wire, 是否命中优选缓存, 是否干净)`，调用方据此决定缓存/回退国外 DNS。

## §6 协议栈基准测试与对比（Benchmark）

SmartProxy 支持 5 种 TUN 协议栈实现（`gvisor`、`lwip`、`system`、`mixed`、`go`）。为了解各协议栈在真实场景下的开销特征与性能边界，在相同基准测试套件（Ubuntu 24.04 x86_64 4-Core Runner）下执行了基准评测：

```bash
go test -tags "with_gvisor,with_lwip" -bench="BenchmarkStack_" -benchmem -benchtime=500x -run=^$ ./internal/tun/
```

### 1. UDP 吞吐量对比 (`BenchmarkStack_UDP_Throughput`)

> **测试场景**：单连接连续发送 1400 字节 UDP 报文，测定协议栈在内存中的分发吞吐、延迟与 Go 运行时堆分配开销。

| 协议栈 | 架构分类 | 权限要求 | 单包耗时 (ns/op) | 内存处理吞吐 | 堆内存分配 (B/op) | 分配次数 (allocs/op) |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **`mixed`** | 混合栈 (Kernel TCP + gVisor UDP) | 需 Root | **77.4 ns** | **18,091.6 MB/s (18.1 GB/s)** | **0 B** | **0 allocs** |
| **`system`** | 主机原生内核协议栈 | 需 Root | 118.0 ns | 11,863.6 MB/s (11.9 GB/s) | 15 B | 0 allocs |
| **`lwip`** | 用户态 C 语言栈 (Lightweight IP) | **非 Root 兼容** | **137.8 ns** | **10,156.8 MB/s (10.2 GB/s)** | **135 B** | **0 allocs** |
| **`gvisor`** | 用户态 Go 语言栈 (Google gVisor) | **非 Root 兼容** | 3,415.0 ns | 410.0 MB/s | 551 B | 3 allocs |
| **`go`** | 纯 Go 原生简易协议栈 | 需 Root | 11,586.0 ns | 120.8 MB/s | 4,948 B | 0 allocs |

> **数据分析与客观说明**：
> - **吞吐率定义**：表中吞吐率为协议栈内存层面的封包/解包基准上限，不代表实际广域网物理传输速度，实际下载/上传速率受网卡吞吐、上游带宽与网络 RTT 约束。
> - **lwIP 的优势**：lwIP 展现出优异的 UDP 吞吐处理能力（~10.2 GB/s），且全程保持 **0 次 Go 堆内存分配（0 allocs/op）**，在高频 UDP 传输（如 DNS 密集查询、QUIC/HTTP3 流媒体、实时通信）中不会对 Go 垃圾回收器（GC）造成抖动压力。
> - **gVisor 的特征**：gVisor 为全 Go 语言实现，包含较完整的网络子系统抽象层与对象分配，因此在纯 UDP 单流吞吐上开销较高，但其生态兼容度好，无需 CGO 编译器。

### 2. TCP 握手开销对比 (`BenchmarkStack_TCP_Handshake`)

> **测试场景**：客户端向 TUN 注入 SYN 并完成 TCP 三次握手建立连接。

| 协议栈 | 架构分类 | 权限要求 | 握手耗时 (ns/op) | 堆内存消耗 (B/op) | Go 堆分配 (allocs/op) |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **`system`** | 主机内核原生协议栈 | 需 Root | **2,050 ns (~2.1 µs)** | **375 B** | **5 allocs** |
| **`mixed`** | 混合栈 (Kernel TCP + gVisor UDP) | 需 Root | 3,204 ns (~3.2 µs) | 375 B | 5 allocs |
| **`gvisor`** | 用户态 Go 语言栈 (Google gVisor) | **非 Root 兼容** | **39,659 ns (~39.7 µs)** | 2,043 B | 27 allocs |
| **`go`** | 纯 Go 原生简易协议栈 | 需 Root | 402,877 ns (~403 µs) | 722 B | 5 allocs |
| **`lwip`** | 用户态 C 语言栈 (Lightweight IP) | **非 Root 兼容** | ~1,004,152 ns (~1.0 ms) | **574 B** | **7 allocs** |

> **数据分析与客观说明**：
> - **gVisor 适合高频短连接**：gVisor 采用并发协程驱动网络事件，单次 TCP 握手耗时约 40 µs，在网页浏览等伴随大量首屏并发短连接场景下响应迅捷。
> - **lwIP 握手耗时成因与内存优势**：lwIP 源码面向单线程事件循环设计，Go CGO 适配层使用互斥锁配合定时器轮询步进（timer tick）推进状态机，因而握手阶段需经过数轮 tick 推进，单次握手耗时约为 1ms。但在连接建立之后，其单连接内存占用极小（仅 574 B，仅为 gVisor 的 ~28%），GC 压力大幅降低（7 allocs vs 27 allocs）。
> - **协议栈选型建议**：
>   - **默认推荐 `gvisor`**：全 Go 实现，零 CGO 编译依赖，跨平台成熟稳定，短连接响应快，适合全平台通用默认。
>   - **移动端推荐 `lwip`**：极低内存 Footprint、极低 GC 压力、超高 UDP 吞吐，非常契合 Android 客户端长期后台驻留，有效避免系统低内存清理（LMK 杀进程）。
>   - **特权环境可选 `system` / `mixed`**：在 Linux 服务器等具备 root / `CAP_NET_ADMIN` 权限且需要极致吞吐的环境下可选用。

## §7 检查清单（优化后验证）

| 检查 | 命令 |
| --- | --- |
| 编译通过 | `go build ./...` |
| 竞态检测 | `go test -race ./...` 全绿、无数据竞争 |
| 格式规范 | `gofmt -l .` 无输出 |

改动热路径（ACL 查询、relay、UDP/DNS 转发）后应回归以上三项；新增共享状态时优先考虑"不可变快照 + atomic 换新"，避免引入锁竞争。
