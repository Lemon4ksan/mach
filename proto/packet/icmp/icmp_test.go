// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package icmp_test

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"

	"github.com/lemon4ksan/mach/proto/packet"
	"github.com/lemon4ksan/mach/proto/packet/icmp"
)

func TestBuildPacketTooBig4(t *testing.T) {
	// Standard 40-byte IPv4 packet
	ip := make([]byte, 40)
	ip[0] = 0x45 // IPv4, IHL 5 (20 bytes)
	binary.BigEndian.PutUint16(ip[2:4], 40)
	copy(ip[12:16], []byte{192, 168, 1, 10})
	copy(ip[16:20], []byte{192, 168, 1, 20})

	pkt, err := icmp.BuildPacketTooBig4(ip, 1400)
	if err != nil {
		t.Fatalf("BuildPacketTooBig4 failed: %v", err)
	}

	// Verify IPv4 header
	if (pkt[0] >> 4) != 4 {
		t.Fatalf("IPv4 version = %d, want 4", pkt[0]>>4)
	}
	if (pkt[0] & 0x0f) != 5 {
		t.Fatalf("IPv4 IHL = %d, want 5", pkt[0]&0x0f)
	}
	if pkt[8] != 64 {
		t.Fatalf("IPv4 TTL = %d, want 64", pkt[8])
	}
	if pkt[9] != 1 {
		t.Fatalf("IPv4 protocol = %d, want 1 (ICMP)", pkt[9])
	}
	// Check swapped IP addresses
	if !equalBytes(pkt[12:16], []byte{192, 168, 1, 20}) {
		t.Fatalf("IPv4 src IP = %v, want 192.168.1.20", pkt[12:16])
	}
	if !equalBytes(pkt[16:20], []byte{192, 168, 1, 10}) {
		t.Fatalf("IPv4 dst IP = %v, want 192.168.1.10", pkt[16:20])
	}
	// Verify IPv4 checksum
	if !packet.ValidateInternetChecksum(pkt[:20]) {
		t.Fatalf("IPv4 header checksum invalid")
	}

	// Verify ICMPv4 header
	if pkt[20] != icmp.ICMPv4TypeDestinationUnreachable {
		t.Fatalf("ICMP type = %d, want %d", pkt[20], icmp.ICMPv4TypeDestinationUnreachable)
	}
	if pkt[21] != icmp.ICMPv4CodeFragmentationNeeded {
		t.Fatalf("ICMP code = %d, want %d", pkt[21], icmp.ICMPv4CodeFragmentationNeeded)
	}
	if !packet.ValidateInternetChecksum(pkt[20:]) {
		t.Fatalf("ICMPv4 checksum invalid")
	}

	// Next-hop MTU
	mtu := binary.BigEndian.Uint16(pkt[26:28])
	if mtu != 1400 {
		t.Fatalf("nextHopMTU = %d, want 1400", mtu)
	}

	// Original datagram length in 32-bit units: 40 bytes is clamped to min 128 -> 128/4 = 32
	if pkt[25] != 32 {
		t.Fatalf("length byte = %d, want 32", pkt[25])
	}

	// IPv4 packet with options (IHL = 6, 24 bytes header)
	ipWithOptions := make([]byte, 44)
	ipWithOptions[0] = 0x46 // IHL 6 (24 bytes)
	copy(ipWithOptions[12:16], []byte{10, 0, 0, 1})
	copy(ipWithOptions[16:20], []byte{10, 0, 0, 2})
	pktOpt, err := icmp.BuildPacketTooBig4(ipWithOptions, 1400)
	if err != nil {
		t.Fatalf("BuildPacketTooBig4 with options failed: %v", err)
	}
	if !packet.ValidateInternetChecksum(pktOpt[:20]) {
		t.Fatalf("IPv4 header checksum with options invalid")
	}
	if !packet.ValidateInternetChecksum(pktOpt[20:]) {
		t.Fatalf("ICMP checksum with options invalid")
	}

	// MTU boundary tests: 68 succeeds, 67 and 0 fail ErrMTUTooSmall
	if _, err := icmp.BuildPacketTooBig4(ip, icmp.IPv4MinMTU); err != nil {
		t.Fatalf("BuildPacketTooBig4 with MTU 68 failed: %v", err)
	}
	if _, err := icmp.BuildPacketTooBig4(ip, 67); !errors.Is(err, icmp.ErrMTUTooSmall) {
		t.Fatalf("MTU 67 err = %v, want ErrMTUTooSmall", err)
	}
	if _, err := icmp.BuildPacketTooBig4(ip, 0); !errors.Is(err, icmp.ErrMTUTooSmall) {
		t.Fatalf("MTU 0 err = %v, want ErrMTUTooSmall", err)
	}

	// Error paths: truncated packet (< 20 bytes)
	if _, err := icmp.BuildPacketTooBig4([]byte{0x45, 0x00}, 1400); !errors.Is(err, icmp.ErrInvalidIPHeader) {
		t.Fatalf("truncated packet err = %v, want ErrInvalidIPHeader", err)
	}

	// Error paths: non-IPv4 header
	if _, err := icmp.BuildPacketTooBig4(
		[]byte{0x60, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		1400,
	); !errors.Is(
		err,
		icmp.ErrInvalidIPHeader,
	) {
		t.Fatalf("non-IPv4 packet err = %v, want ErrInvalidIPHeader", err)
	}

	// Error paths: invalid IHL (IHL < 5 -> ipHdrLen < 20)
	invalidIHL := make([]byte, 24)
	invalidIHL[0] = 0x44 // IHL 4 -> 16 bytes (< 20)
	if _, err := icmp.BuildPacketTooBig4(invalidIHL, 1400); !errors.Is(err, icmp.ErrInvalidIPHeader) {
		t.Fatalf("invalid IHL 4 err = %v, want ErrInvalidIPHeader", err)
	}
	invalidIHL0 := make([]byte, 24)
	invalidIHL0[0] = 0x40 // IHL 0 -> 0 bytes (< 20)
	if _, err := icmp.BuildPacketTooBig4(invalidIHL0, 1400); !errors.Is(err, icmp.ErrInvalidIPHeader) {
		t.Fatalf("invalid IHL 0 err = %v, want ErrInvalidIPHeader", err)
	}

	// Error paths: packet shorter than ipHdrLen
	shortPkt := make([]byte, 22)
	shortPkt[0] = 0x46 // IHL 6 -> requires 24 bytes, but len is 22
	if _, err := icmp.BuildPacketTooBig4(shortPkt, 1400); !errors.Is(err, icmp.ErrInvalidIPHeader) {
		t.Fatalf("len < ipHdrLen err = %v, want ErrInvalidIPHeader", err)
	}

	// Large packet (> 500 bytes): capped at 500 bytes, paddedLen 500, length byte = 125
	largePkt := make([]byte, 1500)
	largePkt[0] = 0x45
	pktLarge, err := icmp.BuildPacketTooBig4(largePkt, 1400)
	if err != nil {
		t.Fatalf("BuildPacketTooBig4 with large packet failed: %v", err)
	}
	if pktLarge[25] != 125 { // 500 / 4 = 125
		t.Fatalf("large packet length byte = %d, want 125", pktLarge[25])
	}
	// Total length = 20 + 8 + 500 = 528
	if len(pktLarge) != 528 {
		t.Fatalf("large packet totalLen = %d, want 528", len(pktLarge))
	}

	// Non-4-aligned length: 131 bytes -> originalLen = 131, paddedLen = 132, length byte = 33
	nonAligned := make([]byte, 131)
	nonAligned[0] = 0x45
	pktNonAligned, err := icmp.BuildPacketTooBig4(nonAligned, 1400)
	if err != nil {
		t.Fatalf("BuildPacketTooBig4 with 131-byte packet failed: %v", err)
	}
	if pktNonAligned[25] != 33 { // 132 / 4 = 33
		t.Fatalf("131-byte packet length byte = %d, want 33", pktNonAligned[25])
	}
	if len(pktNonAligned) != 20+8+132 {
		t.Fatalf("131-byte packet totalLen = %d, want %d", len(pktNonAligned), 20+8+132)
	}
}

func TestBuildPacketTooBig6(t *testing.T) {
	// Standard 60-byte IPv6 packet
	ip6 := make([]byte, 60)
	ip6[0] = 0x60 // IPv6
	src := netip.MustParseAddr("2001:db8::1")
	dst := netip.MustParseAddr("2001:db8::2")
	srcB := src.As16()
	dstB := dst.As16()
	copy(ip6[8:24], srcB[:])
	copy(ip6[24:40], dstB[:])

	pkt, err := icmp.BuildPacketTooBig6(ip6, 1300)
	if err != nil {
		t.Fatalf("BuildPacketTooBig6 failed: %v", err)
	}

	// IPv6 header: version 6, payload len 8+60 = 68, next header 58, hop limit 64
	if (pkt[0] >> 4) != 6 {
		t.Fatalf("IPv6 version = %d, want 6", pkt[0]>>4)
	}
	payloadLen := binary.BigEndian.Uint16(pkt[4:6])
	if payloadLen != 68 {
		t.Fatalf("IPv6 payloadLen = %d, want 68", payloadLen)
	}
	if pkt[6] != 58 {
		t.Fatalf("IPv6 next header = %d, want 58 (ICMPv6)", pkt[6])
	}
	if pkt[7] != 64 {
		t.Fatalf("IPv6 hop limit = %d, want 64", pkt[7])
	}
	// Check swapped IP addresses
	if !equalBytes(pkt[8:24], dstB[:]) {
		t.Fatalf("IPv6 src IP not swapped properly")
	}
	if !equalBytes(pkt[24:40], srcB[:]) {
		t.Fatalf("IPv6 dst IP not swapped properly")
	}

	// ICMPv6 header starts at offset 40: Type 2, Code 0
	if pkt[40] != icmp.ICMPv6TypePacketTooBig {
		t.Fatalf("ICMPv6 type = %d, want %d", pkt[40], icmp.ICMPv6TypePacketTooBig)
	}
	if pkt[41] != icmp.ICMPv6CodePacketTooBig {
		t.Fatalf("ICMPv6 code = %d, want %d", pkt[41], icmp.ICMPv6CodePacketTooBig)
	}

	// MTU field at offset 44..48
	mtu := binary.BigEndian.Uint32(pkt[44:48])
	if mtu != 1300 {
		t.Fatalf("nextHopMTU = %d, want 1300", mtu)
	}

	// Verify ICMPv6 checksum using ValidateICMPv6Checksum
	srcPkt, _ := netip.AddrFromSlice(pkt[8:24])
	dstPkt, _ := netip.AddrFromSlice(pkt[24:40])
	if !packet.ValidateICMPv6Checksum(srcPkt, dstPkt, pkt[40:]) {
		t.Fatalf("ICMPv6 checksum invalid")
	}

	// MTU boundary tests: 1280 succeeds, 1279 and 0 fail ErrMTUTooSmall
	if _, err := icmp.BuildPacketTooBig6(ip6, icmp.IPv6MinMTU); err != nil {
		t.Fatalf("BuildPacketTooBig6 with MTU 1280 failed: %v", err)
	}
	if _, err := icmp.BuildPacketTooBig6(ip6, 1279); !errors.Is(err, icmp.ErrMTUTooSmall) {
		t.Fatalf("MTU 1279 err = %v, want ErrMTUTooSmall", err)
	}
	if _, err := icmp.BuildPacketTooBig6(ip6, 0); !errors.Is(err, icmp.ErrMTUTooSmall) {
		t.Fatalf("MTU 0 err = %v, want ErrMTUTooSmall", err)
	}

	// Error paths: truncated packet (< 40 bytes)
	if _, err := icmp.BuildPacketTooBig6(make([]byte, 39), 1280); !errors.Is(err, icmp.ErrInvalidIPHeader) {
		t.Fatalf("truncated IPv6 err = %v, want ErrInvalidIPHeader", err)
	}

	// Error paths: non-IPv6 header
	nonIPv6 := make([]byte, 40)
	nonIPv6[0] = 0x45
	if _, err := icmp.BuildPacketTooBig6(nonIPv6, 1280); !errors.Is(err, icmp.ErrInvalidIPHeader) {
		t.Fatalf("non-IPv6 err = %v, want ErrInvalidIPHeader", err)
	}

	// Large packet (> 1200 bytes): capped at 1200 bytes
	largePkt := make([]byte, 1500)
	largePkt[0] = 0x60
	copy(largePkt[8:24], srcB[:])
	copy(largePkt[24:40], dstB[:])
	pktLarge, err := icmp.BuildPacketTooBig6(largePkt, 1280)
	if err != nil {
		t.Fatalf("BuildPacketTooBig6 with large packet failed: %v", err)
	}
	if len(pktLarge) != 40+8+1200 {
		t.Fatalf("large packet totalLen = %d, want %d", len(pktLarge), 40+8+1200)
	}
	if !packet.ValidateICMPv6Checksum(dst, src, pktLarge[40:]) {
		t.Fatalf("large packet ICMPv6 checksum invalid")
	}
}

func BenchmarkBuildPacketTooBig4_Small(b *testing.B) {
	ip := make([]byte, 40)
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], 40)
	copy(ip[12:16], []byte{192, 168, 1, 10})
	copy(ip[16:20], []byte{192, 168, 1, 20})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = icmp.BuildPacketTooBig4(ip, 1400)
	}
}

func BenchmarkBuildPacketTooBig4_Large(b *testing.B) {
	ip := make([]byte, 1500)
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], 1500)
	copy(ip[12:16], []byte{192, 168, 1, 10})
	copy(ip[16:20], []byte{192, 168, 1, 20})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = icmp.BuildPacketTooBig4(ip, 1400)
	}
}

func BenchmarkBuildPacketTooBig6_Small(b *testing.B) {
	ip6 := make([]byte, 60)
	ip6[0] = 0x60
	src := netip.MustParseAddr("2001:db8::1")
	dst := netip.MustParseAddr("2001:db8::2")
	srcB := src.As16()
	dstB := dst.As16()
	copy(ip6[8:24], srcB[:])
	copy(ip6[24:40], dstB[:])

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = icmp.BuildPacketTooBig6(ip6, 1280)
	}
}

func BenchmarkBuildPacketTooBig6_Large(b *testing.B) {
	ip6 := make([]byte, 1500)
	ip6[0] = 0x60
	src := netip.MustParseAddr("2001:db8::1")
	dst := netip.MustParseAddr("2001:db8::2")
	srcB := src.As16()
	dstB := dst.As16()
	copy(ip6[8:24], srcB[:])
	copy(ip6[24:40], dstB[:])

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = icmp.BuildPacketTooBig6(ip6, 1280)
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
