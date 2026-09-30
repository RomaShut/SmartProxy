//go:build with_lwip && with_gvisor && cgo

package tun

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/gvisor/pkg/buffer"
	"github.com/sagernet/gvisor/pkg/tcpip/header"
	"github.com/sagernet/gvisor/pkg/tcpip/link/channel"
	"github.com/sagernet/gvisor/pkg/tcpip/stack"
	singtun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/stretchr/testify/require"

	"smartproxy/internal/config"
)

type benchGVisorTun struct {
	*pipeTun
	ep *channel.Endpoint
}

func newBenchGVisorTun(name string, mtu uint32) *benchGVisorTun {
	bt := &benchGVisorTun{
		pipeTun: newPipeTun(name),
		ep:      channel.New(4096, mtu, ""),
	}
	go func() {
		for {
			pkt := bt.ep.ReadContext(context.Background())
			if pkt == nil || bt.closed.Load() {
				return
			}
			data := pkt.ToView().AsSlice()
			bt.pipeTun.Write(data)
			pkt.DecRef()
		}
	}()
	return bt
}

func (t *benchGVisorTun) WritePacket(pkt *stack.PacketBuffer) (int, error) {
	if t.closed.Load() {
		return 0, net.ErrClosed
	}
	data := pkt.ToView().AsSlice()
	return t.pipeTun.Write(data)
}

func (t *benchGVisorTun) NewEndpoint() (stack.LinkEndpoint, stack.NICOptions, error) {
	return t.ep, stack.NICOptions{}, nil
}

type benchHandler struct {
	onTCP func(conn net.Conn)
	onUDP func(conn N.PacketConn)
}

func (h *benchHandler) JudgeFlow(network uint8, source, destination netip.AddrPort, firstPacket []byte) singtun.FlowVerdict {
	return singtun.FlowVerdict{Action: singtun.ActionAccept}
}

func (h *benchHandler) NewDNSPacket(payload []byte, source, destination M.Socksaddr, writer N.PacketWriter) {
}

func (h *benchHandler) NewConnectionEx(ctx context.Context, conn net.Conn, source, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	if h.onTCP != nil {
		h.onTCP(conn)
	} else {
		conn.Close()
	}
}

func (h *benchHandler) NewPacketConnectionEx(ctx context.Context, conn N.PacketConn, source, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	if h.onUDP != nil {
		h.onUDP(conn)
	} else {
		conn.Close()
	}
}

func TestGVisorStack_Verification(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TUN.Enabled = true
	cfg.TUN.Stack = "gvisor"
	cfg.TUN.Name = "pipe_tun_gvisor"
	cfg.TUN.FileDescriptor = 100

	handler := NewHandler(cfg, nil, nil, nil, nil)
	defer handler.Close()

	tun := newBenchGVisorTun("pipe_tun_gvisor", 1500)
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

	syn := testBuildIPv4TCP(clientIP, serverIP, clientPort, serverPort, 1000, 0, 0x02, nil)
	pb := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buffer.MakeWithData(syn),
	})
	tun.ep.InjectInbound(header.IPv4ProtocolNumber, pb)
	pb.DecRef()

	select {
	case synAck := <-tun.writeCh:
		require.GreaterOrEqual(t, len(synAck), 40)
		t.Logf("gVisor SYN/ACK received successfully: flags=0x%02x len=%d", synAck[33], len(synAck))
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SYN/ACK from gVisor")
	}
}

func BenchmarkStack_UDP_Throughput_gVisor(b *testing.B) {
	tun := newBenchGVisorTun("bench_udp_gvisor", 1500)
	defer tun.Close()

	var received atomic.Int64
	bh := &benchHandler{
		onUDP: func(conn N.PacketConn) {
			defer conn.Close()
			b := buf.NewPacket()
			defer b.Release()
			for {
				b.Reset()
				_, err := conn.ReadPacket(b)
				if err != nil {
					return
				}
				received.Add(int64(b.Len()))
			}
		},
	}

	gStack, err := singtun.NewStack("gvisor", singtun.StackOptions{
		Context: context.Background(),
		Tun:     tun,
		TunOptions: singtun.Options{
			MTU: 1500,
			Inet4Address: []netip.Prefix{
				netip.MustParsePrefix("10.0.0.2/24"),
			},
		},
		Handler:     bh,
		Logger:      logger.NOP(),
		UDPTimeout:  time.Minute,
		ICMPTimeout: time.Second,
	})
	require.NoError(b, err)
	require.NoError(b, gStack.Start())
	defer gStack.Close()

	clientIP := net.IPv4(10, 0, 0, 2)
	serverIP := net.IPv4(8, 8, 8, 8)
	payload := make([]byte, 1400)
	for i := range payload {
		payload[i] = byte(i)
	}
	pkt := testBuildIPv4UDP(clientIP, serverIP, 45678, 53, payload)

	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pb := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(pkt),
		})
		tun.ep.InjectInbound(header.IPv4ProtocolNumber, pb)
		pb.DecRef()
	}
}

func BenchmarkStack_UDP_Throughput_lwIP(b *testing.B) {
	tun := newPipeTun("bench_udp_lwip")
	defer tun.Close()

	var received atomic.Int64
	bh := &benchHandler{
		onUDP: func(conn N.PacketConn) {
			defer conn.Close()
			b := buf.NewPacket()
			defer b.Release()
			for {
				b.Reset()
				_, err := conn.ReadPacket(b)
				if err != nil {
					return
				}
				received.Add(int64(b.Len()))
			}
		},
	}

	lStack, err := NewLWIPStack(singtun.StackOptions{
		Context: context.Background(),
		Tun:     tun,
		TunOptions: singtun.Options{
			MTU: 1500,
			Inet4Address: []netip.Prefix{
				netip.MustParsePrefix("10.0.0.2/24"),
			},
		},
		Handler:     bh,
		Logger:      logger.NOP(),
		UDPTimeout:  time.Minute,
		ICMPTimeout: time.Second,
	})
	require.NoError(b, err)
	require.NoError(b, lStack.Start())
	defer lStack.Close()

	clientIP := net.IPv4(10, 0, 0, 2)
	serverIP := net.IPv4(8, 8, 8, 8)
	payload := make([]byte, 1400)
	for i := range payload {
		payload[i] = byte(i)
	}
	pkt := testBuildIPv4UDP(clientIP, serverIP, 45678, 53, payload)

	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tun.readCh <- pkt
	}
}

func BenchmarkStack_TCP_Handshake_gVisor(b *testing.B) {
	tun := newBenchGVisorTun("bench_tcp_gvisor", 1500)
	defer tun.Close()

	bh := &benchHandler{
		onTCP: func(conn net.Conn) {
			conn.Close()
		},
	}

	gStack, err := singtun.NewStack("gvisor", singtun.StackOptions{
		Context: context.Background(),
		Tun:     tun,
		TunOptions: singtun.Options{
			MTU: 1500,
			Inet4Address: []netip.Prefix{
				netip.MustParsePrefix("10.0.0.2/24"),
			},
		},
		Handler:     bh,
		Logger:      logger.NOP(),
		UDPTimeout:  time.Minute,
		ICMPTimeout: time.Second,
	})
	require.NoError(b, err)
	require.NoError(b, gStack.Start())
	defer gStack.Close()

	clientIP := net.IPv4(10, 0, 0, 2)
	serverIP := net.IPv4(1, 2, 3, 4)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		clientPort := uint16(10000 + (i % 50000))
		syn := testBuildIPv4TCP(clientIP, serverIP, clientPort, 80, 1000, 0, 0x02, nil)
		pb := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(syn),
		})
		tun.ep.InjectInbound(header.IPv4ProtocolNumber, pb)
		pb.DecRef()

		// Drain the SYN/ACK response
		select {
		case <-tun.writeCh:
		case <-time.After(500 * time.Millisecond):
			b.Fatal("timeout waiting for SYN/ACK")
		}
	}
}

func BenchmarkStack_TCP_Handshake_lwIP(b *testing.B) {
	tun := newPipeTun("bench_tcp_lwip")
	defer tun.Close()

	bh := &benchHandler{
		onTCP: func(conn net.Conn) {
			conn.Close()
		},
	}

	lStack, err := NewLWIPStack(singtun.StackOptions{
		Context: context.Background(),
		Tun:     tun,
		TunOptions: singtun.Options{
			MTU: 1500,
			Inet4Address: []netip.Prefix{
				netip.MustParsePrefix("10.0.0.2/24"),
			},
		},
		Handler:     bh,
		Logger:      logger.NOP(),
		UDPTimeout:  time.Minute,
		ICMPTimeout: time.Second,
	})
	require.NoError(b, err)
	require.NoError(b, lStack.Start())
	defer lStack.Close()

	clientIP := net.IPv4(10, 0, 0, 2)
	serverIP := net.IPv4(1, 2, 3, 4)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		clientPort := uint16(10000 + (i % 50000))
		syn := testBuildIPv4TCP(clientIP, serverIP, clientPort, 80, 1000, 0, 0x02, nil)
		tun.readCh <- syn

		// Wait for SYN/ACK from lwIP
		var synAck []byte
		select {
		case synAck = <-tun.writeCh:
		case <-time.After(500 * time.Millisecond):
			b.Fatal("timeout waiting for SYN/ACK")
		}

		serverSeq := binary.BigEndian.Uint32(synAck[24:28])
		// Send RST to clean up TCP state
		rst := testBuildIPv4TCP(clientIP, serverIP, clientPort, 80, 1001, serverSeq+1, 0x04, nil)
		tun.readCh <- rst
	}
}
