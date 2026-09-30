package tun

import (
	"fmt"
	"log/slog"

	"github.com/sagernet/sing/common/logger"
)

// singSlogLogger 把 sing 系(sing / sing-tun / sing-box)的 logger.Logger 接口转发到项目
// 全局 slog —— 也就是 logbuf 环形缓冲(App「Go 日志」tab 与面板 /logs)加 stdout(logcat GoLog)。
//
// # 为什么是必需品而不是可选优化
//
// sing-tun 的 go 栈(*Go / *goEngine,third_party/sing-tun/stack_go*.go)在二十多个错误
// 分支上直接调 stack.logger.*,全部没有 nil 判断;而 StackOptions.Logger 在 sing-tun 内部
// 没有任何默认值兜底——stack.go 的 NewStack 把 options 原样透传给 NewGo,NewGo 再原样赋给
// Go.logger(stack_go.go:52),栈上没有第二道防线。
//
// 所以传 nil 的后果不是"少几条日志":第一条错误日志就是 nil interface 方法调用 → 引擎
// goroutine 未捕获 panic。该 goroutine 由 (*Go).Start 创建(stack_go.go:138),不在
// safego.Go 的 recover 覆盖内,panic 直接 abort 整个进程——tombstone 只有 libgojni.so
// 一帧(SIGABRT),Java 层看不到任何异常。
//
// 触发点是确定的:停止 VPN 时 Engine.Stop() 刻意先关 tun fd 再关栈(见 engine.go:900 注释,
// 为了状态栏图标立刻消失),于是栈 shutdown 里 closeAllFlows 给存量连接发 RST 必然写失败
// (stack_go_io_linux.go 的 writeFrame 返回 E.Cause(errno,"go: write tun"),不会被
// goIgnoreDropped 吞掉),必然走到 stack_go_tcp_input.go:884 的 logger.Trace → 必然闪退。
//
// 修法就是在这里给 StackOptions 一个非 nil 的 Logger,而不是去 sing-tun 里逐个补 nil 判断:
// 那些调用点全部合法,缺的只是依赖注入。
type singSlogLogger struct{}

// 编译期断言:sing 的 Logger 接口新增方法时必须在这里补齐实现,而不是留一个
// 部分实现导致运行时才炸。
var _ logger.Logger = singSlogLogger{}

// singLogger 是喂给 singtun.StackOptions.Logger 的包级单例(无状态,可并发调用)。
var singLogger logger.Logger = singSlogLogger{}

// singLogMessage 按 sing 的惯例把所有参数拼成一行。sing 的日志实现普遍用
// fmt.Sprint(args...)(参数里常见 E.Cause 包装过的 error),这里保持一致,避免同一批
// 底层日志在两条输出路径上格式不同。
func singLogMessage(args []any) string {
	return fmt.Sprint(args...)
}

// Trace 映射到 slog.Debug:本项目日志级别只有 DEBUG/INFO/WARN/ERROR(config.log_level),
// 没有 TRACE,Debug 是语义上最近的一档。默认 INFO 级别下这些高频 trace 会被 logbuf 过滤,
// 需要时把 log_level 调成 DEBUG 即可看到——与调用方"排障时才打开"的用意一致。
func (singSlogLogger) Trace(args ...any) { slog.Debug(singLogMessage(args)) }
func (singSlogLogger) Debug(args ...any) { slog.Debug(singLogMessage(args)) }
func (singSlogLogger) Info(args ...any)  { slog.Info(singLogMessage(args)) }
func (singSlogLogger) Warn(args ...any)  { slog.Warn(singLogMessage(args)) }
func (singSlogLogger) Error(args ...any) { slog.Error(singLogMessage(args)) }

// Fatal / Panic 只落日志、不终止进程。sing 的默认实现分别会 os.Exit(1) 和 panic,
// 但这些调用点(Fatal/Panic 在 sing-tun 内主要出现在内部一致性断言上)若真按原语义执行,
// 等于把刚修掉的"底层一个 io 错误打死整个 App"从 logger 层面重新引入——闪退正是本次要
// 修的问题。降到 Error 让它在日志里可见,由上层决定怎么处理。
func (singSlogLogger) Fatal(args ...any) { slog.Error(singLogMessage(args)) }
func (singSlogLogger) Panic(args ...any) { slog.Error(singLogMessage(args)) }
