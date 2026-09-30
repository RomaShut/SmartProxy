# lwIP TUN backend

First-stage lwIP backend for SmartProxy's TUN path.

- lwIP is the userspace IP/TCP/UDP engine.
- TUN fd ownership remains in Go.
- NO_SYS=1/raw API; all lwIP calls must be serialized by the owner.
- lwIP DNS/netconn/socket APIs are disabled.
- TCP/UDP are exposed through callbacks for the Go net.Conn/net.PacketConn adapter.
- The lwIP source is pinned as a git submodule and is not modified.

This stage is intentionally not selected by the production TUN handler yet. The next stage wires the TCP/UDP adapters into NewConnectionEx/NewPacketConnectionEx and adds Android benchmarks against gVisor.
