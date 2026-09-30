//go:build with_lwip && cgo

package lwip

/*
#cgo CFLAGS: -I${SRCDIR}/c -I${SRCDIR}/c/arch -I${SRCDIR}/../../../third_party/lwip/src/include -DLWIP_NOASSERT -D_POSIX_C_SOURCE=200809L
#include <stdint.h>
#include "c/lwip_adapter.h"
*/
import "C"
import (
	"net"
	"unsafe"
)

//export goPacketOutput
func goPacketOutput(data *C.uint8_t, length C.uint32_t, ctxID C.uint64_t) {
	engineID := uint64(ctxID)
	e := getEngine(engineID)
	if e == nil || data == nil || length == 0 {
		return
	}
	pkt := C.GoBytes(unsafe.Pointer(data), C.int(length))
	e.onPacketOutput(pkt)
}

//export goTcpAccept
func goTcpAccept(connID C.uint64_t, isIPv6 C.int, srcIP unsafe.Pointer, srcPort C.uint16_t, dstIP unsafe.Pointer, dstPort C.uint16_t, ctxID C.uint64_t) {
	engineID := uint64(ctxID)
	e := getEngine(engineID)
	if e == nil {
		return
	}

	var sIP, dIP net.IP
	if isIPv6 != 0 {
		sIP = net.IP(C.GoBytes(srcIP, 16))
		dIP = net.IP(C.GoBytes(dstIP, 16))
	} else {
		sIP = net.IP(C.GoBytes(srcIP, 4))
		dIP = net.IP(C.GoBytes(dstIP, 4))
	}

	e.onTCPAccept(uint64(connID), sIP, uint16(srcPort), dIP, uint16(dstPort))
}

//export goTcpRecv
func goTcpRecv(connID C.uint64_t, data *C.uint8_t, length C.uint16_t, ctxID C.uint64_t) {
	engineID := uint64(ctxID)
	e := getEngine(engineID)
	if e == nil {
		return
	}

	id := uint64(connID)
	if data == nil || length == 0 {
		e.onTCPRecv(id, nil)
		return
	}

	b := C.GoBytes(unsafe.Pointer(data), C.int(length))
	e.onTCPRecv(id, b)
}

//export goTcpSent
func goTcpSent(connID C.uint64_t, length C.uint16_t, ctxID C.uint64_t) {
	engineID := uint64(ctxID)
	e := getEngine(engineID)
	if e == nil {
		return
	}

	e.onTCPSent(uint64(connID), uint16(length))
}

//export goTcpErr
func goTcpErr(connID C.uint64_t, err C.int, ctxID C.uint64_t) {
	engineID := uint64(ctxID)
	e := getEngine(engineID)
	if e == nil {
		return
	}

	e.onTCPErr(uint64(connID), int(err))
}

//export goUdpRecv
func goUdpRecv(connID C.uint64_t, isIPv6 C.int, srcIP unsafe.Pointer, srcPort C.uint16_t, dstIP unsafe.Pointer, dstPort C.uint16_t, data *C.uint8_t, length C.uint16_t, ctxID C.uint64_t) {
	engineID := uint64(ctxID)
	e := getEngine(engineID)
	if e == nil || data == nil || length == 0 {
		return
	}

	var sIP, dIP net.IP
	if isIPv6 != 0 {
		sIP = net.IP(C.GoBytes(srcIP, 16))
		dIP = net.IP(C.GoBytes(dstIP, 16))
	} else {
		sIP = net.IP(C.GoBytes(srcIP, 4))
		dIP = net.IP(C.GoBytes(dstIP, 4))
	}

	b := C.GoBytes(unsafe.Pointer(data), C.int(length))
	e.onUDPRecv(uint64(connID), isIPv6 != 0, sIP, uint16(srcPort), dIP, uint16(dstPort), b)
}
