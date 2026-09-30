//go:build with_lwip

package lwip

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func checksum(b []byte) uint16 {
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

func buildIPv4TCP(srcIP, dstIP net.IP, srcPort, dstPort uint16, seq, ack uint32, flags uint8, payload []byte) []byte {
	totalLen := 20 + 20 + len(payload)
	pkt := make([]byte, totalLen)

	// IPv4 Header
	pkt[0] = 0x45
	pkt[1] = 0x00
	binary.BigEndian.PutUint16(pkt[2:4], uint16(totalLen))
	binary.BigEndian.PutUint16(pkt[4:6], 0x1234)
	pkt[6] = 0x40 // Don't fragment
	pkt[7] = 0x00
	pkt[8] = 64 // TTL
	pkt[9] = 6  // Protocol TCP
	copy(pkt[12:16], srcIP.To4())
	copy(pkt[16:20], dstIP.To4())
	binary.BigEndian.PutUint16(pkt[10:12], checksum(pkt[0:20]))

	// TCP Header
	binary.BigEndian.PutUint16(pkt[20:22], srcPort)
	binary.BigEndian.PutUint16(pkt[22:24], dstPort)
	binary.BigEndian.PutUint32(pkt[24:28], seq)
	binary.BigEndian.PutUint32(pkt[28:32], ack)
	pkt[32] = 0x50 // Data offset 5 (20 bytes)
	pkt[33] = flags
	binary.BigEndian.PutUint16(pkt[34:36], 65535)

	if len(payload) > 0 {
		copy(pkt[40:], payload)
	}

	pseudo := make([]byte, 12+20+len(payload))
	copy(pseudo[0:4], srcIP.To4())
	copy(pseudo[4:8], dstIP.To4())
	pseudo[8] = 0
	pseudo[9] = 6
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(20+len(payload)))
	copy(pseudo[12:], pkt[20:])
	binary.BigEndian.PutUint16(pkt[36:38], checksum(pseudo))

	return pkt
}

func TestEngine_TCP_Handshake_Data_Close(t *testing.T) {
	outPkts := make(chan []byte, 32)
	connChan := make(chan net.Conn, 1)

	cfg := Config{
		IPv4:    net.IPv4(10, 0, 0, 2),
		Mask:    net.IPv4(255, 255, 255, 0),
		Gateway: net.IPv4(10, 0, 0, 1),
		OutputFn: func(packet []byte) {
			p := make([]byte, len(packet))
			copy(p, packet)
			outPkts <- p
		},
		TCPHandler: func(conn net.Conn) {
			connChan <- conn
		},
	}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()

	clientIP := net.IPv4(10, 0, 0, 2)
	targetIP := net.IPv4(1, 1, 1, 1)
	clientPort := uint16(45678)
	targetPort := uint16(80)

	// Step 1: Send SYN
	synPkt := buildIPv4TCP(clientIP, targetIP, clientPort, targetPort, 1000, 0, 0x02, nil)
	if err := engine.Input(synPkt); err != nil {
		t.Fatalf("engine.Input SYN failed: %v", err)
	}

	// Step 2: Receive SYN/ACK
	var synAck []byte
	select {
	case synAck = <-outPkts:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SYN/ACK")
	}

	if len(synAck) < 40 {
		t.Fatalf("packet too short: %d", len(synAck))
	}
	flags := synAck[33]
	if (flags & 0x12) != 0x12 {
		t.Fatalf("expected SYN|ACK flags (0x12), got 0x%02x", flags)
	}
	synAckSeq := binary.BigEndian.Uint32(synAck[24:28])

	// Step 3: Send client ACK to finish 3-way handshake
	ackPkt := buildIPv4TCP(clientIP, targetIP, clientPort, targetPort, 1001, synAckSeq+1, 0x10, nil)
	if err := engine.Input(ackPkt); err != nil {
		t.Fatalf("engine.Input ACK failed: %v", err)
	}

	// Step 4: Verify TCPHandler is invoked
	var conn net.Conn
	select {
	case conn = <-connChan:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for TCPHandler")
	}
	defer conn.Close()

	if conn.RemoteAddr().String() != "10.0.0.2:45678" {
		t.Errorf("expected RemoteAddr 10.0.0.2:45678, got %s", conn.RemoteAddr().String())
	}
	if conn.LocalAddr().String() != "1.1.1.1:80" {
		t.Errorf("expected LocalAddr 1.1.1.1:80, got %s", conn.LocalAddr().String())
	}

	// Step 5: Send client DATA -> Read from conn
	clientMsg := []byte("hello from client")
	dataPkt := buildIPv4TCP(clientIP, targetIP, clientPort, targetPort, 1001, synAckSeq+1, 0x18, clientMsg)
	if err := engine.Input(dataPkt); err != nil {
		t.Fatalf("engine.Input data failed: %v", err)
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("conn.Read failed: %v", err)
	}
	if !bytes.Equal(buf[:n], clientMsg) {
		t.Fatalf("expected %q, got %q", string(clientMsg), string(buf[:n]))
	}

	// Step 6: Write from conn -> Receive DATA packet from output
	proxyMsg := []byte("hello from proxy")
	wn, err := conn.Write(proxyMsg)
	if err != nil {
		t.Fatalf("conn.Write failed: %v", err)
	}
	if wn != len(proxyMsg) {
		t.Fatalf("conn.Write short write: %d vs %d", wn, len(proxyMsg))
	}

	// Look for outgoing data packet
	var proxyDataPkt []byte
	deadline := time.After(2 * time.Second)
	for {
		select {
		case pkt := <-outPkts:
			if len(pkt) >= 40 && bytes.Contains(pkt, proxyMsg) {
				proxyDataPkt = pkt
				goto dataVerified
			}
		case <-deadline:
			t.Fatal("timeout waiting for proxy DATA packet")
		}
	}
dataVerified:
	if proxyDataPkt == nil {
		t.Fatal("proxy data packet not received")
	}

	// Step 7: Close conn -> Receive FIN packet
	if err := conn.Close(); err != nil {
		t.Fatalf("conn.Close failed: %v", err)
	}

	finFound := false
	finDeadline := time.After(2 * time.Second)
	for !finFound {
		select {
		case pkt := <-outPkts:
			if len(pkt) >= 40 && (pkt[33]&0x01) != 0 {
				finFound = true
			}
		case <-finDeadline:
			t.Fatal("timeout waiting for FIN packet from proxy")
		}
	}

	// Read after close should return EOF or ErrClosed
	_, rerr := conn.Read(buf)
	if rerr != io.EOF && rerr != net.ErrClosed {
		t.Logf("Read after close returned: %v (expected EOF or ErrClosed)", rerr)
	}
}

func TestEngine_TCP_Deadlines(t *testing.T) {
	outPkts := make(chan []byte, 32)
	connChan := make(chan net.Conn, 1)

	cfg := Config{
		OutputFn: func(packet []byte) {
			p := make([]byte, len(packet))
			copy(p, packet)
			outPkts <- p
		},
		TCPHandler: func(conn net.Conn) {
			connChan <- conn
		},
	}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()

	clientIP := net.IPv4(10, 0, 0, 2)
	targetIP := net.IPv4(2, 2, 2, 2)
	clientPort := uint16(51234)
	targetPort := uint16(8080)

	// Handshake
	synPkt := buildIPv4TCP(clientIP, targetIP, clientPort, targetPort, 2000, 0, 0x02, nil)
	_ = engine.Input(synPkt)
	synAck := <-outPkts
	synAckSeq := binary.BigEndian.Uint32(synAck[24:28])

	ackPkt := buildIPv4TCP(clientIP, targetIP, clientPort, targetPort, 2001, synAckSeq+1, 0x10, nil)
	_ = engine.Input(ackPkt)

	conn := <-connChan
	defer conn.Close()

	// Test read deadline timeout
	err = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}

	buf := make([]byte, 128)
	_, rerr := conn.Read(buf)
	if rerr == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// Clear read deadline, then send data
	_ = conn.SetReadDeadline(time.Time{})
	clientMsg := []byte("resume after deadline")
	dataPkt := buildIPv4TCP(clientIP, targetIP, clientPort, targetPort, 2001, synAckSeq+1, 0x18, clientMsg)
	_ = engine.Input(dataPkt)

	n, rerr := conn.Read(buf)
	if rerr != nil {
		t.Fatalf("Read after clearing deadline failed: %v", rerr)
	}
	if !bytes.Equal(buf[:n], clientMsg) {
		t.Fatalf("expected %q, got %q", string(clientMsg), string(buf[:n]))
	}
}

func TestEngine_TCP_MultipleConns(t *testing.T) {
	outPkts := make(chan []byte, 64)
	connChan := make(chan net.Conn, 10)

	cfg := Config{
		OutputFn: func(packet []byte) {
			p := make([]byte, len(packet))
			copy(p, packet)
			outPkts <- p
		},
		TCPHandler: func(conn net.Conn) {
			connChan <- conn
		},
	}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()

	clientIP := net.IPv4(10, 0, 0, 2)

	// Conn 1: to 1.1.1.1:80
	syn1 := buildIPv4TCP(clientIP, net.IPv4(1, 1, 1, 1), 60001, 80, 3000, 0, 0x02, nil)
	_ = engine.Input(syn1)
	synAck1 := <-outPkts
	seq1 := binary.BigEndian.Uint32(synAck1[24:28])
	ack1 := buildIPv4TCP(clientIP, net.IPv4(1, 1, 1, 1), 60001, 80, 3001, seq1+1, 0x10, nil)
	_ = engine.Input(ack1)

	// Conn 2: to 8.8.8.8:443
	syn2 := buildIPv4TCP(clientIP, net.IPv4(8, 8, 8, 8), 60002, 443, 4000, 0, 0x02, nil)
	_ = engine.Input(syn2)
	synAck2 := <-outPkts
	seq2 := binary.BigEndian.Uint32(synAck2[24:28])
	ack2 := buildIPv4TCP(clientIP, net.IPv4(8, 8, 8, 8), 60002, 443, 4001, seq2+1, 0x10, nil)
	_ = engine.Input(ack2)

	c1 := <-connChan
	c2 := <-connChan
	defer c1.Close()
	defer c2.Close()

	// Verify both connections were routed correctly
	addrs := map[string]bool{
		c1.LocalAddr().String(): true,
		c2.LocalAddr().String(): true,
	}
	if !addrs["1.1.1.1:80"] || !addrs["8.8.8.8:443"] {
		t.Fatalf("expected conns to 1.1.1.1:80 and 8.8.8.8:443, got %s and %s", c1.LocalAddr(), c2.LocalAddr())
	}
}

func TestEngine_TCP_Backpressure(t *testing.T) {
	outPkts := make(chan []byte, 128)
	connChan := make(chan net.Conn, 1)

	cfg := Config{
		OutputFn: func(packet []byte) {
			p := make([]byte, len(packet))
			copy(p, packet)
			outPkts <- p
		},
		TCPHandler: func(conn net.Conn) {
			connChan <- conn
		},
	}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()

	clientIP := net.IPv4(10, 0, 0, 2)
	targetIP := net.IPv4(3, 3, 3, 3)
	clientPort := uint16(55555)
	targetPort := uint16(9000)

	// Step 1: Handshake
	syn := buildIPv4TCP(clientIP, targetIP, clientPort, targetPort, 10000, 0, 0x02, nil)
	_ = engine.Input(syn)
	synAck := <-outPkts
	synAckSeq := binary.BigEndian.Uint32(synAck[24:28])

	ack := buildIPv4TCP(clientIP, targetIP, clientPort, targetPort, 10001, synAckSeq+1, 0x10, nil)
	_ = engine.Input(ack)

	conn := <-connChan
	defer conn.Close()

	// Step 2: Prepare a 50KB payload (exceeds default TCP_SND_BUF ~23KB)
	largePayload := make([]byte, 50000)
	for i := range largePayload {
		largePayload[i] = byte(i % 251)
	}

	writeDone := make(chan error, 1)
	go func() {
		n, werr := conn.Write(largePayload)
		if werr != nil {
			writeDone <- werr
			return
		}
		if n != len(largePayload) {
			writeDone <- io.ErrShortWrite
			return
		}
		writeDone <- nil
	}()

	// Step 3: Receive segments and acknowledge them
	receivedBytes := 0
	clientSeq := uint32(10001)
	timeout := time.After(5 * time.Second)

	for receivedBytes < len(largePayload) {
		select {
		case pkt := <-outPkts:
			if len(pkt) < 40 {
				continue
			}
			dataOffset := int((pkt[32] >> 4) * 4)
			payloadLen := len(pkt) - 20 - dataOffset
			if payloadLen > 0 {
				receivedBytes += payloadLen
				seq := binary.BigEndian.Uint32(pkt[24:28])
				ackNum := seq + uint32(payloadLen)

				// Send client ACK to acknowledge received data and reopen sndbuf
				clientAck := buildIPv4TCP(clientIP, targetIP, clientPort, targetPort, clientSeq, ackNum, 0x10, nil)
				_ = engine.Input(clientAck)
			}
		case <-timeout:
			t.Fatalf("timeout during backpressure test: received %d of %d bytes", receivedBytes, len(largePayload))
		}
	}

	select {
	case werr := <-writeDone:
		if werr != nil {
			t.Fatalf("conn.Write error: %v", werr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for conn.Write to complete")
	}

	if receivedBytes != len(largePayload) {
		t.Fatalf("received bytes mismatch: %d vs %d", receivedBytes, len(largePayload))
	}
}


