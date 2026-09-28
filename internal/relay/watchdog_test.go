package relay

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestWatchdog_Stall_TriggersRSTAndBlacklist(t *testing.T) {
	clientR, clientW := net.Pipe()
	remoteR, remoteW := net.Pipe()
	defer clientR.Close()
	defer remoteW.Close()

	var stalled atomic.Bool
	var stallReason string
	var stallMu sync.Mutex

	cfg := WatchdogConfig{
		Timeout: 50 * time.Millisecond,
		Host:    "140.82.116.4",
		Port:    443,
		Domain:  "github.com",
		OnStall: func(h string, p int, d, reason string) {
			stallMu.Lock()
			stalled.Store(true)
			stallReason = reason
			stallMu.Unlock()
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run relay in background
	relayDone := make(chan struct{})
	go func() {
		defer close(relayDone)
		TCPRelay(ctx, clientR, remoteR, false, nil, WithWatchdog(cfg))
	}()

	// Simulate client writing HTTP request
	go func() {
		clientW.Write([]byte("GET / HTTP/2\r\n\r\n"))
	}()

	// Remote does NOT reply (simulating GFW blackhole silent drop)
	// We read client's request on remoteW to simulate request leaving client
	buf := make([]byte, 1024)
	n, err := remoteW.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("failed to read client request on remote end: %v", err)
	}

	// Wait for watchdog to trigger (timeout is 50ms). Generous select: under
	// -race the post-timer teardown scheduling can take hundreds of ms.
	select {
	case <-relayDone:
	case <-time.After(10 * time.Second):
		t.Fatal("TCPRelay did not terminate after watchdog timeout")
	}

	if !stalled.Load() {
		t.Fatal("expected OnStall callback to be invoked, but was not")
	}

	stallMu.Lock()
	defer stallMu.Unlock()
	if stallReason != "gfw_silent_drop_watchdog" {
		t.Fatalf("expected reason 'gfw_silent_drop_watchdog', got '%s'", stallReason)
	}

	// Verify clientW got closed/reset
	_, writeErr := clientW.Write([]byte("test"))
	if writeErr == nil {
		t.Error("expected client connection to be closed by watchdog, but write succeeded")
	}
}

func TestWatchdog_NormalResponse_Disarms(t *testing.T) {
	clientR, clientW := net.Pipe()
	remoteR, remoteW := net.Pipe()
	defer clientR.Close()
	defer clientW.Close()
	defer remoteR.Close()
	defer remoteW.Close()

	var stalled atomic.Bool
	// Timeout is effectively a "never" bound here: the test synchronizes the
	// response delivery by hand, so the timer must only be unable to fire from
	// scheduling delay (notably under -race).
	cfg := WatchdogConfig{
		Timeout: 10 * time.Second,
		Host:    "1.1.1.1",
		Port:    443,
		Domain:  "cloudflare.com",
		OnStall: func(h string, p int, d, reason string) {
			stalled.Store(true)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		TCPRelay(ctx, clientR, remoteR, false, nil, WithWatchdog(cfg))
	}()

	// 1. Client writes request
	go func() {
		clientW.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
	}()

	// 2. Remote reads request
	buf := make([]byte, 1024)
	n, err := remoteW.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("remote read failed: %v", err)
	}

	// 3. Remote immediately replies with response
	_, err = remoteW.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"))
	if err != nil {
		t.Fatalf("remote write failed: %v", err)
	}

	// 4. Client reads response
	respBuf := make([]byte, 1024)
	rn, err := clientW.Read(respBuf)
	if err != nil || rn == 0 {
		t.Fatalf("client read response failed: %v", err)
	}

	// Wait beyond the watchdog timeout (500ms)
	time.Sleep(600 * time.Millisecond)

	if stalled.Load() {
		t.Fatal("watchdog should have been disarmed by normal response, but OnStall was called!")
	}
}

func TestWatchdog_PreConnectIdle_DoesNotTrigger(t *testing.T) {
	clientR, clientW := net.Pipe()
	remoteR, remoteW := net.Pipe()
	defer clientR.Close()
	defer clientW.Close()
	defer remoteR.Close()
	defer remoteW.Close()

	var stalled atomic.Bool
	cfg := WatchdogConfig{
		Timeout: 100 * time.Millisecond,
		Host:    "1.1.1.1",
		Port:    443,
		Domain:  "beacon-api.aliyuncs.com",
		OnStall: func(h string, p int, d, reason string) {
			stalled.Store(true)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go func() {
		TCPRelay(ctx, clientR, remoteR, false, nil, WithWatchdog(cfg))
	}()

	// Client opens connection (speculative connection pool pre-connect) but does not write any request yet.
	// Wait well beyond the 100ms watchdog timeout:
	time.Sleep(250 * time.Millisecond)

	if stalled.Load() {
		t.Fatal("watchdog should not trigger on idle pre-connected connections where no request is in flight!")
	}
}

func TestWatchdog_KeepAliveIdle_DoesNotTrigger(t *testing.T) {
	clientR, clientW := net.Pipe()
	remoteR, remoteW := net.Pipe()
	defer clientR.Close()
	defer clientW.Close()
	defer remoteR.Close()
	defer remoteW.Close()

	var stalled atomic.Bool
	cfg := WatchdogConfig{
		Timeout: 10 * time.Second,
		Host:    "1.1.1.1",
		Port:    443,
		Domain:  "api-normal-m.amemv.com",
		OnStall: func(h string, p int, d, reason string) {
			t.Logf("OnStall called: reason=%s", reason)
			stalled.Store(true)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		TCPRelay(ctx, clientR, remoteR, false, nil, WithWatchdog(cfg))
	}()

	// 1. Client writes small request
	go func() {
		clientW.Write([]byte("GET /ping HTTP/1.1\r\n\r\n"))
	}()

	// 2. Remote reads request
	buf := make([]byte, 1024)
	n, err := remoteW.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("remote read failed: %v", err)
	}

	// 3. Remote writes small response (<1KB, less than 16KB threshold)
	_, err = remoteW.Write([]byte("HTTP/1.1 200 OK\r\n\r\npong"))
	if err != nil {
		t.Fatalf("remote write failed: %v", err)
	}

	// 4. Client reads response
	respBuf := make([]byte, 1024)
	rn, err := clientW.Read(respBuf)
	if err != nil || rn == 0 {
		t.Fatalf("client read response failed: %v", err)
	}

	// 5. Connection enters Keep-Alive idle state! Client sits idle for 600ms (watchdog timeout is 500ms)
	time.Sleep(600 * time.Millisecond)

	if stalled.Load() {
		t.Fatal("watchdog falsely triggered on idle Keep-Alive connection after response was received!")
	}
}

func TestWatchdog_EarlyResetAfterRequest_Triggers(t *testing.T) {
	clientR, clientW := net.Pipe()
	defer clientR.Close()
	defer clientW.Close()

	var stalled atomic.Bool
	var stallReason string
	var stallMu sync.Mutex

	cfg := WatchdogConfig{
		Timeout: 10 * time.Second,
		Host:    "140.82.116.4",
		Port:    443,
		Domain:  "github.com",
		OnStall: func(h string, p int, d, reason string) {
			stallMu.Lock()
			stalled.Store(true)
			stallReason = reason
			stallMu.Unlock()
		},
	}

	// Mock remote: accept the ClientHello, then answer every Read with ECONNRESET
	// (GFW injects the reset once it parses the plaintext SNI).
	mockRemote := &scriptedResetConn{err: syscall.ECONNRESET}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	relayDone := make(chan struct{})
	go func() {
		defer close(relayDone)
		TCPRelay(ctx, clientR, mockRemote, false, nil, WithWatchdog(cfg))
	}()

	// Client sends ClientHello after relay starts.
	if _, err := clientW.Write([]byte("GARBAGE_TLS_CLIENT_HELLO")); err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	select {
	case <-relayDone:
	case <-time.After(10 * time.Second):
		t.Fatal("TCPRelay did not terminate after early reset")
	}

	if !stalled.Load() {
		t.Fatal("expected reset after a sent request to trigger OnStall, but it did not")
	}

	stallMu.Lock()
	defer stallMu.Unlock()
	if stallReason != "gfw_rst_injected" {
		t.Fatalf("expected reason 'gfw_rst_injected', got '%s'", stallReason)
	}
}

func TestWatchdog_ResetWithNoRequest_DoesNotTrigger(t *testing.T) {
	clientR, clientW := net.Pipe()
	defer clientR.Close()
	defer clientW.Close()

	var stalled atomic.Bool
	cfg := WatchdogConfig{
		Timeout: 100 * time.Millisecond,
		Host:    "1.1.1.1",
		Port:    443,
		Domain:  "speculative-preconnect.example.com",
		OnStall: func(h string, p int, d, reason string) {
			stalled.Store(true)
		},
	}

	// Remote immediately resets even though the client never sent a request.
	mockRemote := &errorConn{err: syscall.ECONNRESET}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go TCPRelay(ctx, clientR, mockRemote, false, nil, WithWatchdog(cfg))

	time.Sleep(200 * time.Millisecond)
	if stalled.Load() {
		t.Fatal("RST on a connection that never sent a request must not trigger the watchdog")
	}
}

func TestWatchdog_IdleKeepAliveReset_DoesNotTrigger(t *testing.T) {
	clientR, clientW := net.Pipe()
	defer clientR.Close()
	defer clientW.Close()

	var stalled atomic.Bool
	cfg := WatchdogConfig{
		// Timeout is effectively a "never" bound: the response delivery is
		// synchronized by hand, so only RSTGraceWindow bounds the test.
		Timeout:        10 * time.Second,
		RSTGraceWindow: 300 * time.Millisecond,
		Host:           "1.1.1.1",
		Port:           443,
		Domain:         "api-normal.example.com",
		OnStall: func(h string, p int, d, reason string) {
			t.Logf("unexpected OnStall: reason=%s", reason)
			stalled.Store(true)
		},
	}

	// Remote serves one small response, then RSTs only when the test asks for it.
	mockRemote := &scriptedResetConn{
		resp:  []byte("HTTP/1.1 200 OK\r\n\r\npong"),
		reset: make(chan struct{}),
		err:   syscall.ECONNRESET,
	}
	defer mockRemote.Abort()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	relayDone := make(chan struct{})
	go func() {
		defer close(relayDone)
		TCPRelay(ctx, clientR, mockRemote, false, nil, WithWatchdog(cfg))
	}()

	if _, err := clientW.Write([]byte("GET /ping HTTP/1.1\r\n\r\n")); err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	respBuf := make([]byte, 1024)
	if _, err := clientW.Read(respBuf); err != nil {
		t.Fatalf("client read response failed: %v", err)
	}

	// Connection idles beyond the RST grace window, then the server/load-balancer
	// closes the keep-alive socket with a reset.
	time.Sleep(700 * time.Millisecond)
	close(mockRemote.reset)

	// The ignored RST still ends io.Copy, so the relay terminates — but no stall
	// callback may have fired.
	select {
	case <-relayDone:
	case <-time.After(10 * time.Second):
		t.Fatal("TCPRelay did not terminate after idle reset")
	}

	if stalled.Load() {
		t.Fatal("idle keep-alive RST (no request recently in flight) must not trigger the watchdog")
	}
}

func TestWatchdog_LateResetWithinGrace_Triggers(t *testing.T) {
	clientR, clientW := net.Pipe()
	defer clientR.Close()
	defer clientW.Close()

	var stalled atomic.Bool
	var stallReason string
	var stallMu sync.Mutex

	cfg := WatchdogConfig{
		Timeout:        10 * time.Second,
		RSTGraceWindow: 5 * time.Second,
		Host:           "140.82.116.4",
		Port:           443,
		Domain:         "github.com",
		OnStall: func(h string, p int, d, reason string) {
			stallMu.Lock()
			stalled.Store(true)
			stallReason = reason
			stallMu.Unlock()
		},
	}

	// Remote starts a partial response, then injects a reset shortly after.
	mockRemote := &scriptedResetConn{
		resp:  []byte("HTTP/1.1 200 OK\r\n"),
		reset: make(chan struct{}),
		err:   syscall.ECONNRESET,
	}
	defer mockRemote.Abort()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	relayDone := make(chan struct{})
	go func() {
		defer close(relayDone)
		TCPRelay(ctx, clientR, mockRemote, false, nil, WithWatchdog(cfg))
	}()

	if _, err := clientW.Write([]byte("GET / HTTP/1.1\r\n\r\n")); err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	respBuf := make([]byte, 1024)
	if _, err := clientW.Read(respBuf); err != nil {
		t.Fatalf("client read partial response failed: %v", err)
	}

	// GFW reset lands after the partial response but inside the grace window.
	time.Sleep(300 * time.Millisecond)
	close(mockRemote.reset)

	select {
	case <-relayDone:
	case <-time.After(10 * time.Second):
		t.Fatal("TCPRelay did not terminate after late reset")
	}

	if !stalled.Load() {
		t.Fatal("reset within the grace window after a partial response must trigger the watchdog")
	}
	stallMu.Lock()
	defer stallMu.Unlock()
	if stallReason != "gfw_rst_injected" {
		t.Fatalf("expected reason 'gfw_rst_injected', got '%s'", stallReason)
	}
}

func TestWatchdog_ProxyBypassed(t *testing.T) {
	clientR, clientW := net.Pipe()
	remoteR, remoteW := net.Pipe()
	defer clientR.Close()
	defer clientW.Close()
	defer remoteR.Close()
	defer remoteW.Close()

	var stalled atomic.Bool
	cfg := WatchdogConfig{
		Timeout: 30 * time.Millisecond,
		Host:    "1.2.3.4",
		Port:    443,
		OnStall: func(h string, p int, d, reason string) {
			stalled.Store(true)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// proxy = true: watchdog should NOT attach!
	go TCPRelay(ctx, clientR, remoteR, true, nil, WithWatchdog(cfg))

	// Client writes, remote is silent
	go func() { clientW.Write([]byte("data")) }()

	time.Sleep(60 * time.Millisecond)

	if stalled.Load() {
		t.Fatal("watchdog should not trigger for proxy connections")
	}
}

// scriptedResetConn models a server: it waits for the client's first Write,
// optionally delivers one response, and afterwards either returns err on the
// next Read (reset == nil) or blocks until reset is closed and then returns err.
type scriptedResetConn struct {
	resp  []byte
	reset chan struct{}
	err   error

	requested chan struct{}

	respMu  sync.Mutex
	respOff int

	// initOnce creates the requested channel (whichever of Read/Write runs
	// first); signalOnce closes it (only Write signals). Keeping them separate
	// matters: a shared Once would let Read's initialization swallow Write's
	// signal and block Read forever.
	initOnce   sync.Once
	signalOnce sync.Once

	abortOnce sync.Once
	aborted   chan struct{}
}

func (c *scriptedResetConn) Write(b []byte) (int, error) {
	c.initOnce.Do(func() { c.requested = make(chan struct{}) })
	c.signalOnce.Do(func() { close(c.requested) })
	return len(b), nil
}

func (c *scriptedResetConn) Read(b []byte) (int, error) {
	// Block until the client has actually sent a request, mirroring a server
	// that never RSTs an idle pre-connect on its own.
	c.initOnce.Do(func() { c.requested = make(chan struct{}) })
	<-c.requested

	c.respMu.Lock()
	if c.resp != nil && c.respOff < len(c.resp) {
		n := copy(b, c.resp[c.respOff:])
		c.respOff += n
		c.respMu.Unlock()
		return n, nil
	}
	c.respMu.Unlock()

	if c.reset == nil {
		return 0, c.err
	}
	select {
	case <-c.reset:
		return 0, c.err
	case <-c.aborted:
		return 0, net.ErrClosed
	}
}

// Abort unblocks a pending Read so the conn never leaks a test goroutine.
func (c *scriptedResetConn) Abort() {
	c.abortOnce.Do(func() {
		c.aborted = make(chan struct{})
		close(c.aborted)
	})
}

func (c *scriptedResetConn) Close() error                       { c.Abort(); return nil }
func (c *scriptedResetConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (c *scriptedResetConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (c *scriptedResetConn) SetDeadline(t time.Time) error      { return nil }
func (c *scriptedResetConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *scriptedResetConn) SetWriteDeadline(t time.Time) error { return nil }

type errorConn struct {
	err error
}

func (e *errorConn) Read(b []byte) (n int, err error)   { return 0, e.err }
func (e *errorConn) Write(b []byte) (n int, err error)  { return 0, e.err }
func (e *errorConn) Close() error                       { return nil }
func (e *errorConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (e *errorConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (e *errorConn) SetDeadline(t time.Time) error      { return nil }
func (e *errorConn) SetReadDeadline(t time.Time) error  { return nil }
func (e *errorConn) SetWriteDeadline(t time.Time) error { return nil }
