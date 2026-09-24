// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tcp_test

import (
	"encoding/binary"
	"net/netip"
	"testing"

	"github.com/lemon4ksan/mach/proto/packet/tcp"
)

func makeIPv4TCPPacket(flags byte, currentMSS uint16, extraOptions []byte, payloadLen int) []byte {
	// TCP header standard: 20 bytes.
	// Options: if extraOptions is nil, standard MSS option is Kind 2, Len 4, MSS uint16 (4 bytes).
	// Total TCP header: 24 bytes (or 20 + len(extraOptions) padded to 4 bytes).
	optLen := 4
	if extraOptions != nil {
		optLen = len(extraOptions)
	}
	paddedOptLen := (optLen + 3) &^ 3
	tcpHdrLen := 20 + paddedOptLen

	totalLen := 20 + tcpHdrLen + payloadLen
	pkt := make([]byte, totalLen)

	// IPv4 header
	pkt[0] = 0x45 // Version 4, IHL 5
	binary.BigEndian.PutUint16(pkt[2:4], uint16(totalLen))
	pkt[8] = 64
	pkt[9] = 6 // TCP
	copy(pkt[12:16], []byte{192, 168, 1, 10})
	copy(pkt[16:20], []byte{192, 168, 1, 20})

	// TCP header
	tcpHdr := pkt[20:]
	binary.BigEndian.PutUint16(tcpHdr[0:2], 12345) // Src port
	binary.BigEndian.PutUint16(tcpHdr[2:4], 80)    // Dst port
	tcpHdr[12] = byte((tcpHdrLen / 4) << 4)        // Data offset
	tcpHdr[13] = flags

	if extraOptions != nil {
		copy(tcpHdr[20:], extraOptions)
	} else {
		tcpHdr[20] = 2
		tcpHdr[21] = 4
		binary.BigEndian.PutUint16(tcpHdr[22:24], currentMSS)
	}

	for i := 0; i < payloadLen; i++ {
		pkt[20+tcpHdrLen+i] = byte(i + 1)
	}

	return pkt
}

func makeIPv6TCPPacket(flags byte, currentMSS uint16, extraOptions []byte, payloadLen int) []byte {
	optLen := 4
	if extraOptions != nil {
		optLen = len(extraOptions)
	}
	paddedOptLen := (optLen + 3) &^ 3
	tcpHdrLen := 20 + paddedOptLen

	totalLen := 40 + tcpHdrLen + payloadLen
	pkt := make([]byte, totalLen)

	// IPv6 header
	pkt[0] = 0x60
	binary.BigEndian.PutUint16(pkt[4:6], uint16(tcpHdrLen+payloadLen))
	pkt[6] = 6 // TCP Next Header
	pkt[7] = 64
	src := netip.MustParseAddr("2001:db8::1").As16()
	dst := netip.MustParseAddr("2001:db8::2").As16()
	copy(pkt[8:24], src[:])
	copy(pkt[24:40], dst[:])

	// TCP header
	tcpHdr := pkt[40:]
	binary.BigEndian.PutUint16(tcpHdr[0:2], 54321)
	binary.BigEndian.PutUint16(tcpHdr[2:4], 443)
	tcpHdr[12] = byte((tcpHdrLen / 4) << 4)
	tcpHdr[13] = flags

	if extraOptions != nil {
		copy(tcpHdr[20:], extraOptions)
	} else {
		tcpHdr[20] = 2
		tcpHdr[21] = 4
		binary.BigEndian.PutUint16(tcpHdr[22:24], currentMSS)
	}

	for i := 0; i < payloadLen; i++ {
		pkt[40+tcpHdrLen+i] = byte(i + 1)
	}

	return pkt
}

func verifyTCPChecksum(pkt []byte, version byte, ipHdrLen int) bool {
	tcpHdr := pkt[ipHdrLen:]
	tcpLen := len(pkt) - ipHdrLen

	var sum uint32
	if version == 4 {
		sum += uint32(binary.BigEndian.Uint16(pkt[12:14]))
		sum += uint32(binary.BigEndian.Uint16(pkt[14:16]))
		sum += uint32(binary.BigEndian.Uint16(pkt[16:18]))
		sum += uint32(binary.BigEndian.Uint16(pkt[18:20]))
		sum += uint32(6)
		sum += uint32(tcpLen)
	} else {
		for i := 8; i < 40; i += 2 {
			sum += uint32(binary.BigEndian.Uint16(pkt[i : i+2]))
		}
		sum += uint32(tcpLen)
		sum += uint32(6)
	}

	for i := 0; i < tcpLen-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(tcpHdr[i : i+2]))
	}
	if tcpLen%2 == 1 {
		sum += uint32(tcpHdr[tcpLen-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum) == 0
}

func TestClampMSSInPlace_IPv4(t *testing.T) {
	// 1. Standard SYN clamping: MTU 1300 -> Max MSS = 1300 - 40 = 1260
	pkt := makeIPv4TCPPacket(0x02, 1460, nil, 0)
	tcp.ClampMSSInPlace(pkt, 1300)
	clampedMSS := binary.BigEndian.Uint16(pkt[42:44])
	if clampedMSS != 1260 {
		t.Fatalf("Clamped MSS = %d, want 1260", clampedMSS)
	}
	if !verifyTCPChecksum(pkt, 4, 20) {
		t.Fatalf("TCP checksum verification failed after clamping")
	}

	// 2. SYN with MSS already lower: MTU 1500 (Max MSS 1460), current MSS 1260 -> unchanged
	csumBefore := binary.BigEndian.Uint16(pkt[36:38])
	tcp.ClampMSSInPlace(pkt, 1500)
	if mss := binary.BigEndian.Uint16(pkt[42:44]); mss != 1260 {
		t.Fatalf("MSS changed from 1260 to %d", mss)
	}
	csumAfter := binary.BigEndian.Uint16(pkt[36:38])
	if csumBefore != csumAfter {
		t.Fatalf("Checksum should remain untouched when MSS is unchanged")
	}

	// 3. SYN with MSS equal to Max MSS: MTU 1300 (Max MSS 1260), current MSS 1260 -> unchanged
	tcp.ClampMSSInPlace(pkt, 1300)
	if mss := binary.BigEndian.Uint16(pkt[42:44]); mss != 1260 {
		t.Fatalf("MSS changed when equal: got %d, want 1260", mss)
	}

	// 4. SYN/ACK packet (flags = 0x12): should clamp
	pktSynAck := makeIPv4TCPPacket(0x12, 1460, nil, 0)
	tcp.ClampMSSInPlace(pktSynAck, 1300)
	if mss := binary.BigEndian.Uint16(pktSynAck[42:44]); mss != 1260 {
		t.Fatalf("SYN/ACK MSS = %d, want 1260", mss)
	}
	if !verifyTCPChecksum(pktSynAck, 4, 20) {
		t.Fatalf("SYN/ACK checksum invalid")
	}

	// 5. Non-SYN packets: ACK (0x10), FIN (0x01), PSH-ACK (0x18), RST (0x04) -> untouched
	nonSYNFlags := []byte{0x10, 0x01, 0x18, 0x04, 0x00}
	for _, fl := range nonSYNFlags {
		p := makeIPv4TCPPacket(fl, 1460, nil, 0)
		tcp.ClampMSSInPlace(p, 1300)
		if mss := binary.BigEndian.Uint16(p[42:44]); mss != 1460 {
			t.Fatalf("Non-SYN flag 0x%02x: MSS modified to %d", fl, mss)
		}
	}

	// 6. Non-TCP packet: IPv4 protocol 17 (UDP) -> ignored
	pktUDP := makeIPv4TCPPacket(0x02, 1460, nil, 0)
	pktUDP[9] = 17
	tcp.ClampMSSInPlace(pktUDP, 1300)
	if mss := binary.BigEndian.Uint16(pktUDP[42:44]); mss != 1460 {
		t.Fatalf("Non-TCP IPv4 packet MSS modified to %d", mss)
	}

	// 7. IPv4 MTU boundary: MTU <= 40 -> ignored
	pktLowMTU := makeIPv4TCPPacket(0x02, 1460, nil, 0)
	tcp.ClampMSSInPlace(pktLowMTU, 40)
	tcp.ClampMSSInPlace(pktLowMTU, 30)
	tcp.ClampMSSInPlace(pktLowMTU, 0)
	tcp.ClampMSSInPlace(pktLowMTU, -10)
	if mss := binary.BigEndian.Uint16(pktLowMTU[42:44]); mss != 1460 {
		t.Fatalf("MTU <= 40 MSS modified to %d", mss)
	}
}

func TestClampMSSInPlace_IPv6(t *testing.T) {
	// 1. Standard IPv6 SYN clamping: MTU 1400 -> Max MSS = 1400 - 60 = 1340
	pkt6 := makeIPv6TCPPacket(0x02, 1440, nil, 0)
	tcp.ClampMSSInPlace(pkt6, 1400)
	clampedMSS := binary.BigEndian.Uint16(pkt6[62:64])
	if clampedMSS != 1340 {
		t.Fatalf("IPv6 Clamped MSS = %d, want 1340", clampedMSS)
	}
	if !verifyTCPChecksum(pkt6, 6, 40) {
		t.Fatalf("IPv6 TCP checksum invalid after clamping")
	}

	// 2. IPv6 SYN with MSS already lower: MTU 1500 (Max MSS 1440), current MSS 1340 -> unchanged
	csumBefore := binary.BigEndian.Uint16(pkt6[56:58])
	tcp.ClampMSSInPlace(pkt6, 1500)
	if mss := binary.BigEndian.Uint16(pkt6[62:64]); mss != 1340 {
		t.Fatalf("IPv6 MSS modified to %d", mss)
	}
	if csum := binary.BigEndian.Uint16(pkt6[56:58]); csum != csumBefore {
		t.Fatalf("IPv6 checksum should remain untouched")
	}

	// 3. IPv6 SYN/ACK (flags 0x12) -> clamped
	pkt6SynAck := makeIPv6TCPPacket(0x12, 1440, nil, 0)
	tcp.ClampMSSInPlace(pkt6SynAck, 1400)
	if mss := binary.BigEndian.Uint16(pkt6SynAck[62:64]); mss != 1340 {
		t.Fatalf("IPv6 SYN/ACK MSS = %d, want 1340", mss)
	}
	if !verifyTCPChecksum(pkt6SynAck, 6, 40) {
		t.Fatalf("IPv6 SYN/ACK checksum invalid")
	}

	// 4. IPv6 non-TCP packet: Next Header 17 (UDP) -> ignored
	pkt6UDP := makeIPv6TCPPacket(0x02, 1440, nil, 0)
	pkt6UDP[6] = 17
	tcp.ClampMSSInPlace(pkt6UDP, 1400)
	if mss := binary.BigEndian.Uint16(pkt6UDP[62:64]); mss != 1440 {
		t.Fatalf("IPv6 non-TCP MSS modified to %d", mss)
	}

	// 5. IPv6 MTU <= 60 -> ignored
	pkt6LowMTU := makeIPv6TCPPacket(0x02, 1440, nil, 0)
	tcp.ClampMSSInPlace(pkt6LowMTU, 60)
	tcp.ClampMSSInPlace(pkt6LowMTU, 50)
	if mss := binary.BigEndian.Uint16(pkt6LowMTU[62:64]); mss != 1440 {
		t.Fatalf("IPv6 MTU <= 60 MSS modified to %d", mss)
	}

	// 6. IPv6 packet length < 60 -> ignored
	shortIPv6 := make([]byte, 59)
	shortIPv6[0] = 0x60
	shortIPv6[6] = 6
	tcp.ClampMSSInPlace(shortIPv6, 1400) // should return safely without panic
}

func TestClampMSSInPlace_TruncatedAndInvalid(t *testing.T) {
	// packet len < 20
	tcp.ClampMSSInPlace(nil, 1400)
	tcp.ClampMSSInPlace([]byte{0x45}, 1400)
	tcp.ClampMSSInPlace(make([]byte, 19), 1400)

	// Unknown IP version (e.g. 5 or 7)
	unknownVer := make([]byte, 60)
	unknownVer[0] = 0x50
	tcp.ClampMSSInPlace(unknownVer, 1400)

	// IPv4 with ipHdrLen < 20 (e.g. IHL 4 or 0)
	invIHL := make([]byte, 60)
	invIHL[0] = 0x44
	invIHL[9] = 6
	tcp.ClampMSSInPlace(invIHL, 1400)

	invIHL0 := make([]byte, 60)
	invIHL0[0] = 0x40
	invIHL0[9] = 6
	tcp.ClampMSSInPlace(invIHL0, 1400)

	// IPv4 len < ipHdrLen + 20
	shortPkt := make([]byte, 35)
	shortPkt[0] = 0x45 // requires 20 + 20 = 40 bytes minimum
	shortPkt[9] = 6
	tcp.ClampMSSInPlace(shortPkt, 1400)

	// len(tcpHdr) < 20
	hdrLen20 := make([]byte, 39)
	hdrLen20[0] = 0x45
	hdrLen20[9] = 6
	tcp.ClampMSSInPlace(hdrLen20, 1400)

	// tcpDataOffset < 20
	invDataOffset := makeIPv4TCPPacket(0x02, 1460, nil, 0)
	invDataOffset[32] = 0x40 // data offset = 4 (16 bytes < 20)
	tcp.ClampMSSInPlace(invDataOffset, 1300)
	if mss := binary.BigEndian.Uint16(invDataOffset[42:44]); mss != 1460 {
		t.Fatalf("MSS modified on invalid tcpDataOffset < 20: %d", mss)
	}

	// len(tcpHdr) < tcpDataOffset
	invOffsetBig := makeIPv4TCPPacket(0x02, 1460, nil, 0)
	invOffsetBig[32] = 0xf0 // data offset = 15 (60 bytes, but tcpHdr is 24)
	tcp.ClampMSSInPlace(invOffsetBig, 1300)
	if mss := binary.BigEndian.Uint16(invOffsetBig[42:44]); mss != 1460 {
		t.Fatalf("MSS modified on len(tcpHdr) < tcpDataOffset: %d", mss)
	}
}

func TestClampMSSInPlace_OptionsEdgeCases(t *testing.T) {
	// 1. EOL (Kind 0) before MSS option: parsing should stop and NOT clamp MSS
	// Options: [EOL(0), Kind(2), Len(4), MSS(1460)]
	optEOL := []byte{0, 2, 4, 0x05, 0xb4, 0, 0, 0}
	pktEOL := makeIPv4TCPPacket(0x02, 0, optEOL, 0)
	tcp.ClampMSSInPlace(pktEOL, 1300)
	if mss := binary.BigEndian.Uint16(pktEOL[43:45]); mss != 1460 {
		t.Fatalf("MSS clamped after EOL: got %d, want 1460", mss)
	}

	// 2. NOP (Kind 1) preceding MSS option: should parse past NOP and clamp MSS
	// Options: [NOP(1), NOP(1), Kind(2), Len(4), MSS(1460), NOP(1), NOP(1)]
	optNOP := []byte{1, 1, 2, 4, 0x05, 0xb4, 1, 1}
	pktNOP := makeIPv4TCPPacket(0x02, 0, optNOP, 0)
	tcp.ClampMSSInPlace(pktNOP, 1300)
	if mss := binary.BigEndian.Uint16(pktNOP[44:46]); mss != 1260 {
		t.Fatalf("MSS not clamped after NOP: got %d, want 1260", mss)
	}
	if !verifyTCPChecksum(pktNOP, 4, 20) {
		t.Fatalf("Checksum invalid after NOP clamped")
	}

	// 3. Truncated option: optIdx + 1 >= len(options)
	optTrunc := []byte{1, 1, 1, 2} // last byte is Kind 2 without length
	pktTrunc := makeIPv4TCPPacket(0x02, 0, optTrunc, 0)
	tcp.ClampMSSInPlace(pktTrunc, 1300) // no panic, graceful break

	// 4. optLen == 0: must break cleanly without hanging
	optLen0 := []byte{3, 0, 0, 0}
	pktLen0 := makeIPv4TCPPacket(0x02, 0, optLen0, 0)
	tcp.ClampMSSInPlace(pktLen0, 1300)

	// 5. optLen == 1: invalid length (< 2), must break cleanly
	optLen1 := []byte{3, 1, 0, 0}
	pktLen1 := makeIPv4TCPPacket(0x02, 0, optLen1, 0)
	tcp.ClampMSSInPlace(pktLen1, 1300)

	// 6. optLen exceeds buffer: optIdx + optLen > len(options)
	optExceed := []byte{3, 10, 0, 0}
	pktExceed := makeIPv4TCPPacket(0x02, 0, optExceed, 0)
	tcp.ClampMSSInPlace(pktExceed, 1300)

	// 7. MSS option with invalid length: Kind 2, Len 3 or Len 5 -> should be ignored
	optMSSLen3 := []byte{2, 3, 0x05, 0}
	pktMSSLen3 := makeIPv4TCPPacket(0x02, 0, optMSSLen3, 0)
	tcp.ClampMSSInPlace(pktMSSLen3, 1300)

	optMSSLen5 := []byte{2, 5, 0x05, 0xb4, 0, 0, 0, 0}
	pktMSSLen5 := makeIPv4TCPPacket(0x02, 0, optMSSLen5, 0)
	tcp.ClampMSSInPlace(pktMSSLen5, 1300)
	if mss := binary.BigEndian.Uint16(pktMSSLen5[42:44]); mss != 1460 {
		t.Fatalf("MSS with optLen 5 should not be clamped: got %d", mss)
	}

	// 8. Odd-length TCP segment padding: 24 bytes TCP header + 1 byte data = 25 bytes
	pktOdd := makeIPv4TCPPacket(0x02, 1460, nil, 1)
	tcp.ClampMSSInPlace(pktOdd, 1300)
	if mss := binary.BigEndian.Uint16(pktOdd[42:44]); mss != 1260 {
		t.Fatalf("Odd-length TCP segment MSS = %d, want 1260", mss)
	}
	if !verifyTCPChecksum(pktOdd, 4, 20) {
		t.Fatalf("Odd-length TCP segment checksum invalid")
	}

	// Also test odd-length TCP segment for IPv6
	pkt6Odd := makeIPv6TCPPacket(0x02, 1440, nil, 1)
	tcp.ClampMSSInPlace(pkt6Odd, 1400)
	if mss := binary.BigEndian.Uint16(pkt6Odd[62:64]); mss != 1340 {
		t.Fatalf("IPv6 odd-length TCP segment MSS = %d, want 1340", mss)
	}
	if !verifyTCPChecksum(pkt6Odd, 6, 40) {
		t.Fatalf("IPv6 odd-length TCP segment checksum invalid")
	}
}

func BenchmarkClampMSSInPlace_IPv4_Clamped(b *testing.B) {
	pkt := makeIPv4TCPPacket(0x02, 1460, nil, 0)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Reset MSS before each clamp
		binary.BigEndian.PutUint16(pkt[42:44], 1460)
		tcp.ClampMSSInPlace(pkt, 1300)
	}
}

func BenchmarkClampMSSInPlace_IPv4_Unchanged(b *testing.B) {
	pkt := makeIPv4TCPPacket(0x02, 1200, nil, 0)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tcp.ClampMSSInPlace(pkt, 1300)
	}
}

func BenchmarkClampMSSInPlace_IPv6_Clamped(b *testing.B) {
	pkt := makeIPv6TCPPacket(0x02, 1440, nil, 0)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		binary.BigEndian.PutUint16(pkt[62:64], 1440)
		tcp.ClampMSSInPlace(pkt, 1400)
	}
}

func BenchmarkClampMSSInPlace_IPv6_Unchanged(b *testing.B) {
	pkt := makeIPv6TCPPacket(0x02, 1200, nil, 0)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tcp.ClampMSSInPlace(pkt, 1400)
	}
}

func BenchmarkClampMSSInPlace_WithOptions(b *testing.B) {
	// NOP, NOP, Timestamp (kind 8, len 10), NOP, NOP, MSS (kind 2, len 4) -> 20 bytes options
	options := []byte{
		1, 1, // NOP, NOP
		8, 10, 0, 1, 2, 3, 4, 5, 6, 7, // Timestamp
		1, 1, // NOP, NOP
		2, 4, 0x05, 0xb4, // MSS 1460
	}
	pkt := makeIPv4TCPPacket(0x02, 0, options, 0)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		binary.BigEndian.PutUint16(pkt[56:58], 1460)
		tcp.ClampMSSInPlace(pkt, 1300)
	}
}
