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

func TestStressBuildPacketTooBig4_MalformedHeaders(t *testing.T) {
	// 1. Packets with lengths from 0 to 19 bytes
	for l := 0; l < 20; l++ {
		b := make([]byte, l)
		if l > 0 {
			b[0] = 0x45
		}
		_, err := icmp.BuildPacketTooBig4(b, 1400)
		if !errors.Is(err, icmp.ErrInvalidIPHeader) {
			t.Fatalf("length %d: got err %v, want ErrInvalidIPHeader", l, err)
		}
	}

	// 2. Packets with invalid IP versions (version != 4)
	for ver := 0; ver <= 15; ver++ {
		if ver == 4 {
			continue
		}
		b := make([]byte, 40)
		b[0] = byte(ver<<4) | 0x05
		_, err := icmp.BuildPacketTooBig4(b, 1400)
		if !errors.Is(err, icmp.ErrInvalidIPHeader) {
			t.Fatalf("version %d: got err %v, want ErrInvalidIPHeader", ver, err)
		}
	}

	// 3. Packets with IHL < 5 (header length < 20)
	for ihl := 0; ihl < 5; ihl++ {
		b := make([]byte, 40)
		b[0] = 0x40 | byte(ihl)
		_, err := icmp.BuildPacketTooBig4(b, 1400)
		if !errors.Is(err, icmp.ErrInvalidIPHeader) {
			t.Fatalf("IHL %d: got err %v, want ErrInvalidIPHeader", ihl, err)
		}
	}

	// 4. Packets with IHL > 5 where len(ipPacket) < ipHdrLen
	for ihl := 6; ihl <= 15; ihl++ {
		requiredLen := ihl * 4
		for l := 20; l < requiredLen; l++ {
			b := make([]byte, l)
			b[0] = 0x40 | byte(ihl)
			_, err := icmp.BuildPacketTooBig4(b, 1400)
			if !errors.Is(err, icmp.ErrInvalidIPHeader) {
				t.Fatalf("IHL %d (needs %d) len %d: got err %v, want ErrInvalidIPHeader", ihl, requiredLen, l, err)
			}
		}
	}
}

func TestStressBuildPacketTooBig4_MTUBoundaries(t *testing.T) {
	pkt := make([]byte, 40)
	pkt[0] = 0x45

	// Reject MTU 0 to 67
	for mtu := uint16(0); mtu < 68; mtu++ {
		_, err := icmp.BuildPacketTooBig4(pkt, mtu)
		if !errors.Is(err, icmp.ErrMTUTooSmall) {
			t.Fatalf("MTU %d: got err %v, want ErrMTUTooSmall", mtu, err)
		}
	}

	// Accept MTU 68..65535
	validMTUs := []uint16{68, 69, 576, 1280, 1400, 1500, 9000, 65535}
	for _, mtu := range validMTUs {
		out, err := icmp.BuildPacketTooBig4(pkt, mtu)
		if err != nil {
			t.Fatalf("valid MTU %d failed: %v", mtu, err)
		}
		if gotMTU := binary.BigEndian.Uint16(out[26:28]); gotMTU != mtu {
			t.Fatalf("MTU %d in packet = %d", mtu, gotMTU)
		}
	}
}

func TestStressBuildPacketTooBig4_PayloadLengthsAndChecksums(t *testing.T) {
	// Challenge across various packet lengths from 20 up to 600
	for l := 20; l <= 600; l += 7 {
		pkt := make([]byte, l)
		pkt[0] = 0x45
		binary.BigEndian.PutUint16(pkt[2:4], uint16(l))
		copy(pkt[12:16], []byte{10, 1, 2, 3})
		copy(pkt[16:20], []byte{172, 16, 0, 1})
		for i := 20; i < l; i++ {
			pkt[i] = byte(i)
		}

		out, err := icmp.BuildPacketTooBig4(pkt, 1400)
		if err != nil {
			t.Fatalf("length %d failed: %v", l, err)
		}

		// Verify IP header checksum
		if !packet.ValidateInternetChecksum(out[:20]) {
			t.Fatalf("length %d: IPv4 header checksum invalid", l)
		}

		// Verify ICMP message checksum
		if !packet.ValidateInternetChecksum(out[20:]) {
			t.Fatalf("length %d: ICMPv4 checksum invalid", l)
		}

		// Check RFC 4884 length field
		originalLen := min(max(l, 128), 500)
		paddedLen := (originalLen + 3) &^ 3
		expectedLengthByte := byte(paddedLen / 4)
		if out[25] != expectedLengthByte {
			t.Fatalf("length %d: length byte = %d, want %d", l, out[25], expectedLengthByte)
		}

		// Check total length
		if len(out) != 20+8+paddedLen {
			t.Fatalf("length %d: totalLen = %d, want %d", l, len(out), 20+8+paddedLen)
		}
	}
}

func TestStressBuildPacketTooBig6_MalformedHeadersAndMTU(t *testing.T) {
	// Length < 40
	for l := 0; l < 40; l++ {
		b := make([]byte, l)
		if l > 0 {
			b[0] = 0x60
		}
		_, err := icmp.BuildPacketTooBig6(b, 1300)
		if !errors.Is(err, icmp.ErrInvalidIPHeader) {
			t.Fatalf("IPv6 length %d: got %v, want ErrInvalidIPHeader", l, err)
		}
	}

	// Version != 6
	for ver := 0; ver <= 15; ver++ {
		if ver == 6 {
			continue
		}
		b := make([]byte, 40)
		b[0] = byte(ver << 4)
		_, err := icmp.BuildPacketTooBig6(b, 1300)
		if !errors.Is(err, icmp.ErrInvalidIPHeader) {
			t.Fatalf("IPv6 version %d: got %v, want ErrInvalidIPHeader", ver, err)
		}
	}

	pkt := make([]byte, 40)
	pkt[0] = 0x60
	src := netip.MustParseAddr("2001:db8::10")
	dst := netip.MustParseAddr("2001:db8::20")
	srcB := src.As16()
	dstB := dst.As16()
	copy(pkt[8:24], srcB[:])
	copy(pkt[24:40], dstB[:])

	// MTU < 1280
	for _, mtu := range []uint32{0, 1, 100, 1279} {
		_, err := icmp.BuildPacketTooBig6(pkt, mtu)
		if !errors.Is(err, icmp.ErrMTUTooSmall) {
			t.Fatalf("IPv6 MTU %d: got %v, want ErrMTUTooSmall", mtu, err)
		}
	}

	// Various payload lengths and verify ICMPv6 checksum
	for l := 40; l <= 1400; l += 53 {
		p := make([]byte, l)
		p[0] = 0x60
		copy(p[8:24], srcB[:])
		copy(p[24:40], dstB[:])
		for i := 40; i < l; i++ {
			p[i] = byte(i * 3)
		}

		out, err := icmp.BuildPacketTooBig6(p, 1280)
		if err != nil {
			t.Fatalf("IPv6 len %d failed: %v", l, err)
		}

		srcAddr, _ := netip.AddrFromSlice(out[8:24])
		dstAddr, _ := netip.AddrFromSlice(out[24:40])
		if !packet.ValidateICMPv6Checksum(srcAddr, dstAddr, out[40:]) {
			t.Fatalf("IPv6 len %d: ICMPv6 checksum verification failed", l)
		}
	}
}
