//go:build with_lwip && cgo

package tun

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	singtun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"smartproxy/internal/config"
)

type pipeTun struct {
	readCh  chan []byte
	writeCh chan []byte
	closed  atomic.Bool
	name    string
}

func newPipeTun(name string) *pipeTun {
	return &pipeTun{
		readCh:  make(chan []byte, 128),
		writeCh: make(chan []byte, 128),
		name:    name,
	}
}

func (t *pipeTun) Read(p []byte) (int, error) {
	if t.closed.Load() {
		return 0, net.ErrClosed
	}
	pkt, ok := <-t.readCh
	if !ok {
		return 0, io.EOF
	}
	n := copy(p, pkt)
	return n, nil
}

func (t *pipeTun) Write(p []byte) (int, error) {
	if t.closed.Load() {
		return 0, net.ErrClosed
	}
	data := make([]byte, len(p))
	copy(data, p)
	select {
	case t.writeCh <- data:
		return len(p), nil
	default:
		return len(p), nil
	}
}

func (t *pipeTun) Name() (string, error) {
	return t.name, nil
}

func (t *pipeTun) Start() error {
	return nil
}

func (t *pipeTun) Close() error {
	if t.closed.Swap(true) {
		return nil
	}
	close(t.readCh)
	return nil
}

func (t *pipeTun) UpdateRouteOptions(tunOptions singtun.Options) error {
	return nil
}

func testChecksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i < len(b)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(b[i:]))
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum > 0xffff {
		sum = (sum >> 16) + (sum & 0xffff)
	}
	return ^uint16(sum)
}

func testBuildIPv4TCP(srcIP, dstIP net.IP, srcPort, dstPort uint16, seq, ack uint32, flags uint8, payload []byte) []byte {
	totalLen := 20 + 20 + len(payload)
	pkt := make([]byte, totalLen)

	// IPv4 Header
	pkt[0] = 0x45
	pkt[1] = 0x00
	binary.BigEndian.PutUint16(pkt[2:4], uint16(totalLen))
	binary.BigEndian.PutUint16(pkt[4:6], 0x1234)
	pkt[6] = 0x40
	pkt[7] = 0x00
	pkt[8] = 64
	pkt[9] = 6 // TCP
	copy(pkt[12:16], srcIP.To4())
	copy(pkt[16:20], dstIP.To4())
	binary.BigEndian.PutUint16(pkt[10:12], testChecksum(pkt[0:20]))

	// TCP Header
	binary.BigEndian.PutUint16(pkt[20:22], srcPort)
	binary.BigEndian.PutUint16(pkt[22:24], dstPort)
	binary.BigEndian.PutUint32(pkt[24:28], seq)
	binary.BigEndian.PutUint32(pkt[28:32], ack)
	pkt[32] = 0x50
	pkt[33] = flags
	binary.BigEndian.PutUint16(pkt[34:36], 65535)

	if len(payload) > 0 {
		copy(pkt[40:], payload)
	}

	pseudo := make([]byte, 12+20+len(payload))
	copy(pseudo[0:4], srcIP.To4())
	copy(pseudo[4:8], dstIP.To4())
	pseudo[9] = 6
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(20+len(payload)))
	copy(pseudo[12:], pkt[20:])

	binary.BigEndian.PutUint16(pkt[36:38], testChecksum(pseudo))
	return pkt
}

func testBuildIPv4UDP(srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte) []byte {
	totalLen := 20 + 8 + len(payload)
	pkt := make([]byte, totalLen)

	pkt[0] = 0x45
	pkt[1] = 0x00
	binary.BigEndian.PutUint16(pkt[2:4], uint16(totalLen))
	binary.BigEndian.PutUint16(pkt[4:6], 0x5678)
	pkt[6] = 0x40
	pkt[7] = 0x00
	pkt[8] = 64
	pkt[9] = 17 // UDP
	copy(pkt[12:16], srcIP.To4())
	copy(pkt[16:20], dstIP.To4())
	binary.BigEndian.PutUint16(pkt[10:12], testChecksum(pkt[0:20]))

	binary.BigEndian.PutUint16(pkt[20:22], srcPort)
	binary.BigEndian.PutUint16(pkt[22:24], dstPort)
	binary.BigEndian.PutUint16(pkt[24:26], uint16(8+len(payload)))

	if len(payload) > 0 {
		copy(pkt[28:], payload)
	}
	return pkt
}

func testBuildIPv4ICMP(srcIP, dstIP net.IP, icmpType, icmpCode uint8, id, seq uint16, payload []byte) []byte {
	totalLen := 20 + 8 + len(payload)
	pkt := make([]byte, totalLen)

	pkt[0] = 0x45
	pkt[1] = 0x00
	binary.BigEndian.PutUint16(pkt[2:4], uint16(totalLen))
	binary.BigEndian.PutUint16(pkt[4:6], 0x1122)
	pkt[6] = 0x40
	pkt[7] = 0x00
	pkt[8] = 64
	pkt[9] = 1 // ICMP
	copy(pkt[12:16], srcIP.To4())
	copy(pkt[16:20], dstIP.To4())
	binary.BigEndian.PutUint16(pkt[10:12], testChecksum(pkt[0:20]))

	pkt[20] = icmpType
	pkt[21] = icmpCode
	binary.BigEndian.PutUint16(pkt[24:26], id)
	binary.BigEndian.PutUint16(pkt[26:28], seq)
	if len(payload) > 0 {
		copy(pkt[28:], payload)
	}
	binary.BigEndian.PutUint16(pkt[22:24], testChecksum(pkt[20:]))
	return pkt
}

func TestLWIPStack_Lifecycle(t *testing.T) {
	_, err := NewLWIPStack(singtun.StackOptions{})
	require.Error(t, err)

	tun := newPipeTun("test0")
	defer tun.Close()

	stack, err := NewLWIPStack(singtun.StackOptions{
		Tun: tun,
		TunOptions: singtun.Options{
			MTU: 1500,
			Inet4Address: []netip.Prefix{
				netip.MustParsePrefix("10.0.0.2/24"),
			},
		},
		Logger: logger.NOP(),
	})
	require.NoError(t, err)

	require.NoError(t, stack.Start())
	stack.ResetNetwork()
	require.NoError(t, stack.Close())
}

func TestLWIPStack_CreateTUNStack(t *testing.T) {
	tun := newPipeTun("test_create")
	defer tun.Close()

	s, err := createTUNStack("lwip", singtun.StackOptions{
		Tun: tun,
		TunOptions: singtun.Options{
			MTU: 1500,
		},
		Logger: logger.NOP(),
	})
	require.NoError(t, err)
	require.IsType(t, &LWIPStack{}, s)
	require.NoError(t, s.Close())
}

func TestLWIPStack_TUNHandler_TCP(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TUN.Enabled = true
	cfg.TUN.Stack = "lwip"
	cfg.TUN.Name = "pipe_tun_tcp"
	cfg.TUN.FileDescriptor = 100

	handler := NewHandler(cfg, nil, nil, nil, nil)
	defer handler.Close()

	tun := newPipeTun("pipe_tun_tcp")
	defer tun.Close()

	oldNewTUN := NewTUN
	oldNewTUNStack := NewTUNStack
	NewTUN = func(opts singtun.Options) (singtun.Tun, error) {
		return tun, nil
	}
	NewTUNStack = createTUNStack
	defer func() {
		NewTUN = oldNewTUN
		NewTUNStack = oldNewTUNStack
	}()

	tunDev, tunStack, err := handler.Start(context.Background(), cfg.TUN)
	require.NoError(t, err)
	defer func() {
		if tunDev != nil {
			tunDev.Close()
		}
		if tunStack != nil {
			tunStack.Close()
		}
	}()

	clientIP := net.IPv4(10, 0, 0, 2)
	serverIP := net.IPv4(1, 2, 3, 4)
	clientPort := uint16(54321)
	serverPort := uint16(80)

	// Send TCP SYN
	syn := testBuildIPv4TCP(clientIP, serverIP, clientPort, serverPort, 1000, 0, 0x02, nil)
	tun.readCh <- syn

	// Read SYN/ACK from tun.writeCh
	var synAck []byte
	select {
	case synAck = <-tun.writeCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SYN/ACK from lwIP")
	}

	require.GreaterOrEqual(t, len(synAck), 40)
	assert.Equal(t, uint8(0x12), synAck[33], "expected SYN|ACK flags (0x12)")

	serverSeq := binary.BigEndian.Uint32(synAck[24:28])

	// Send ACK to complete 3-way handshake
	ack := testBuildIPv4TCP(clientIP, serverIP, clientPort, serverPort, 1001, serverSeq+1, 0x10, nil)
	tun.readCh <- ack

	// Send FIN to close
	fin := testBuildIPv4TCP(clientIP, serverIP, clientPort, serverPort, 1001, serverSeq+1, 0x11, nil)
	tun.readCh <- fin

	// Verify reply packet received without panics
	select {
	case <-tun.writeCh:
	case <-time.After(1 * time.Second):
	}
}

func TestLWIPStack_TUNHandler_ICMP(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TUN.Enabled = true
	cfg.TUN.Stack = "lwip"
	cfg.TUN.Name = "pipe_tun_icmp"
	cfg.TUN.FileDescriptor = 100

	handler := NewHandler(cfg, nil, nil, nil, nil)
	defer handler.Close()

	tun := newPipeTun("pipe_tun_icmp")
	defer tun.Close()

	oldNewTUN := NewTUN
	oldNewTUNStack := NewTUNStack
	NewTUN = func(opts singtun.Options) (singtun.Tun, error) {
		return tun, nil
	}
	NewTUNStack = createTUNStack
	defer func() {
		NewTUN = oldNewTUN
		NewTUNStack = oldNewTUNStack
	}()

	tunDev, tunStack, err := handler.Start(context.Background(), cfg.TUN)
	require.NoError(t, err)
	defer func() {
		if tunDev != nil {
			tunDev.Close()
		}
		if tunStack != nil {
			tunStack.Close()
		}
	}()

	clientIP := net.IPv4(10, 0, 0, 2)
	destIP := net.IPv4(1, 1, 1, 1)

	// Send ICMP Echo Request
	echoReq := testBuildIPv4ICMP(clientIP, destIP, 8, 0, 0x1234, 1, []byte("smartproxy-ping"))
	tun.readCh <- echoReq

	// Read ICMP Echo Reply
	var reply []byte
	select {
	case reply = <-tun.writeCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ICMP echo reply from lwIP stack")
	}

	require.GreaterOrEqual(t, len(reply), 28)
	assert.Equal(t, uint8(0), reply[20], "ICMP type should be 0 (Echo Reply)")
	assert.Equal(t, uint8(0), reply[21], "ICMP code should be 0")
	assert.Equal(t, uint16(0x1234), binary.BigEndian.Uint16(reply[24:26]), "ICMP ID must match")
	assert.Equal(t, uint16(1), binary.BigEndian.Uint16(reply[26:28]), "ICMP seq must match")
	assert.Equal(t, "smartproxy-ping", string(reply[28:]), "ICMP payload must match")
}

func TestLWIPStack_TUNHandler_UDP(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TUN.Enabled = true
	cfg.TUN.Stack = "lwip"
	cfg.TUN.Name = "pipe_tun_udp"
	cfg.TUN.FileDescriptor = 100

	handler := NewHandler(cfg, nil, nil, nil, nil)
	defer handler.Close()

	tun := newPipeTun("pipe_tun_udp")
	defer tun.Close()

	oldNewTUN := NewTUN
	oldNewTUNStack := NewTUNStack
	NewTUN = func(opts singtun.Options) (singtun.Tun, error) {
		return tun, nil
	}
	NewTUNStack = createTUNStack
	defer func() {
		NewTUN = oldNewTUN
		NewTUNStack = oldNewTUNStack
	}()

	tunDev, tunStack, err := handler.Start(context.Background(), cfg.TUN)
	require.NoError(t, err)
	defer func() {
		if tunDev != nil {
			tunDev.Close()
		}
		if tunStack != nil {
			tunStack.Close()
		}
	}()

	clientIP := net.IPv4(10, 0, 0, 2)
	destIP := net.IPv4(1, 2, 3, 4)

	// Send UDP packet
	udpPkt := testBuildIPv4UDP(clientIP, destIP, 12345, 8080, []byte("ping-udp"))
	tun.readCh <- udpPkt

	// Allow UDP packet to be processed by lwip stack and passed to TUNHandler
	time.Sleep(50 * time.Millisecond)
}
