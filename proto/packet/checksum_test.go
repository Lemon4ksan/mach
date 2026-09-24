// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packet_test

import (
	"encoding/binary"
	"net/netip"
	"testing"

	"github.com/lemon4ksan/mach/proto/packet"
)

func TestCalculateInternetChecksum_RFC1071(t *testing.T) {
	// RFC 1071 §3 numerical vector: 00 01 f2 03 f4 f5 f6 f7 -> 0x220d
	data := []byte{0x00, 0x01, 0xf2, 0x03, 0xf4, 0xf5, 0xf6, 0xf7}
	csum := packet.CalculateInternetChecksum(data)
	if csum != 0x220d {
		t.Fatalf("RFC 1071 checksum = 0x%04x, want 0x220d", csum)
	}

	// Verify appending checksum in big-endian order yields 0 or 0xffff
	withCsum := append([]byte(nil), data...)
	withCsum = append(withCsum, byte(csum>>8), byte(csum&0xff))
	verify := packet.CalculateInternetChecksum(withCsum)
	if verify != 0 && verify != 0xffff {
		t.Fatalf("Checksum verification failed: got 0x%04x", verify)
	}
	if !packet.ValidateInternetChecksum(withCsum) {
		t.Fatalf("ValidateInternetChecksum returned false for valid packet")
	}
}

func TestCalculateInternetChecksum_OddLength(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint16
	}{
		{
			name:     "1_byte",
			data:     []byte{0x45},
			expected: 0xbaff, // ^(0x4500) = 0xbaff
		},
		{
			name:     "3_bytes",
			data:     []byte{0x45, 0x00, 0x01},
			expected: 0xb9ff, // ^(0x4500 + 0x0100) = ^(0x4600) = 0xb9ff
		},
		{
			name:     "5_bytes",
			data:     []byte{0x45, 0x00, 0x00, 0x3c, 0x1c},
			expected: 0x9ec3, // ^(0x4500 + 0x003c + 0x1c00) = ^(0x613c) = 0x9ec3
		},
		{
			name:     "7_bytes",
			data:     []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
			expected: 0xeff3, // ^(0x0102 + 0x0304 + 0x0506 + 0x0700) = ^(0x100c) = 0xeff3
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			csum := packet.CalculateInternetChecksum(tc.data)
			if csum != tc.expected {
				t.Fatalf("got 0x%04x, want 0x%04x", csum, tc.expected)
			}
		})
	}

	// 1501 bytes odd-length buffer
	data1501 := make([]byte, 1501)
	for i := range data1501 {
		data1501[i] = byte(i*7 + 3)
	}
	csum1501 := packet.CalculateInternetChecksum(data1501)
	if csum1501 == 0 {
		t.Fatalf("checksum for 1501 bytes should not be zero")
	}
	// Round trip with even padding
	data1502 := append([]byte(nil), data1501...)
	data1502 = append(data1502, 0) // zero pad to even length
	csumEven := packet.CalculateInternetChecksum(data1502)
	if csumEven != csum1501 {
		t.Fatalf("checksum mismatch between odd buffer and zero-padded buffer: 0x%04x vs 0x%04x", csum1501, csumEven)
	}
}

func TestCalculateInternetChecksum_AllZero(t *testing.T) {
	lengths := []int{0, 2, 20, 1500}
	for _, l := range lengths {
		data := make([]byte, l)
		csum := packet.CalculateInternetChecksum(data)
		if csum != 0xffff {
			t.Fatalf("len %d all-zero checksum = 0x%04x, want 0xffff", l, csum)
		}
	}
}

func TestCalculateInternetChecksum_AllOnes(t *testing.T) {
	// 2 bytes 0xffff -> sum 0xffff -> ^0xffff = 0x0000
	csum2 := packet.CalculateInternetChecksum([]byte{0xff, 0xff})
	if csum2 != 0x0000 {
		t.Fatalf("2 bytes all ones = 0x%04x, want 0x0000", csum2)
	}

	// 4 bytes 0xffff, 0xffff -> sum 0x1fffe -> 0xffff -> ^0xffff = 0x0000
	csum4 := packet.CalculateInternetChecksum([]byte{0xff, 0xff, 0xff, 0xff})
	if csum4 != 0x0000 {
		t.Fatalf("4 bytes all ones = 0x%04x, want 0x0000", csum4)
	}

	// 3 bytes 0xffff, 0xff -> sum 0xffff + 0xff00 = 0x1feff -> 0xff00 -> ^0xff00 = 0x00ff
	csum3 := packet.CalculateInternetChecksum([]byte{0xff, 0xff, 0xff})
	if csum3 != 0x00ff {
		t.Fatalf("3 bytes all ones = 0x%04x, want 0x00ff", csum3)
	}
}

func TestCalculateInternetChecksum_MultipleFolds(t *testing.T) {
	// sum = 0xffff + 0xffff + 0xffff + 0x0001 = 0x2fffe
	// fold 1: 0xfffe + 2 = 0x10000 (overflows 16 bits again!)
	// fold 2: 0x0000 + 1 = 0x0001
	// result: ^0x0001 = 0xfffe
	data := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x01}
	csum := packet.CalculateInternetChecksum(data)
	if csum != 0xfffe {
		t.Fatalf("multiple fold checksum = 0x%04x, want 0xfffe", csum)
	}

	// Multiple fold in ICMPv6 as well
	src := netip.MustParseAddr("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")
	dst := netip.MustParseAddr("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")
	msg := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x01}
	csumV6 := packet.CalculateICMPv6Checksum(src, dst, msg)
	if csumV6 == 0 {
		t.Fatalf("ICMPv6 multiple fold checksum returned 0")
	}
}

func TestRFC791IPv4HeaderVerification(t *testing.T) {
	// Standard IPv4 20-byte header (RFC 791)
	hdr := []byte{
		0x45, 0x00, 0x00, 0x3c, // Version 4, IHL 5, TOS 0, TotalLen 60
		0x1c, 0x46, 0x40, 0x00, // ID 0x1c46, Flags DF, FragOffset 0
		0x40, 0x06, 0x00, 0x00, // TTL 64, Proto TCP(6), Checksum 0x0000
		0xc0, 0xa8, 0x01, 0x01, // Src 192.168.1.1
		0xc0, 0xa8, 0x01, 0x02, // Dst 192.168.1.2
	}

	csum := packet.CalculateInternetChecksum(hdr)
	binary.BigEndian.PutUint16(hdr[10:12], csum)

	if !packet.ValidateInternetChecksum(hdr) {
		t.Fatalf("ValidateInternetChecksum failed for valid IPv4 header")
	}

	// Verify that recalculation over the header with embedded checksum yields 0
	if c := packet.CalculateInternetChecksum(hdr); c != 0 {
		t.Fatalf("Recalculate checksum = 0x%04x, want 0", c)
	}

	// Corrupt a byte and verify validation failure
	hdr[10] ^= 0xff
	if packet.ValidateInternetChecksum(hdr) {
		t.Fatalf("ValidateInternetChecksum succeeded for corrupted IPv4 header")
	}
}

func TestCalculateICMPv6Checksum_RFC4443(t *testing.T) {
	src := netip.MustParseAddr("2001:db8::1")
	dst := netip.MustParseAddr("2001:db8::2")

	// RFC 4443 Echo Request packet: Type 128, Code 0, Checksum 0, ID 0x122c, Seq 1, Data "PING" -> Checksum 0x7386
	msg := []byte{128, 0, 0, 0, 0x12, 0x2c, 0x00, 0x01, 'P', 'I', 'N', 'G'}
	csum := packet.CalculateICMPv6Checksum(src, dst, msg)
	if csum != 0x7386 {
		t.Fatalf("CalculateICMPv6Checksum = 0x%04x, want 0x7386", csum)
	}

	// Round-trip validation: embed checksum into message
	binary.BigEndian.PutUint16(msg[2:4], csum)
	if !packet.ValidateICMPv6Checksum(src, dst, msg) {
		t.Fatalf("ValidateICMPv6Checksum failed for valid ICMPv6 message")
	}
	if verify := packet.CalculateICMPv6Checksum(src, dst, msg); verify != 0 {
		t.Fatalf("CalculateICMPv6Checksum with embedded checksum = 0x%04x, want 0", verify)
	}

	// Corrupt checksum and ensure validation fails
	msg[2] ^= 0x55
	if packet.ValidateICMPv6Checksum(src, dst, msg) {
		t.Fatalf("ValidateICMPv6Checksum succeeded for corrupted ICMPv6 message")
	}
}

func TestValidationHelpers_EdgeCases(t *testing.T) {
	// ValidateInternetChecksum on empty slice
	if packet.ValidateInternetChecksum(nil) {
		t.Fatalf("ValidateInternetChecksum(nil) = true, want false")
	}
	if packet.ValidateInternetChecksum([]byte{}) {
		t.Fatalf("ValidateInternetChecksum([]byte{}) = true, want false")
	}

	// ValidateICMPv6Checksum on short message (< 4 bytes)
	src := netip.MustParseAddr("2001:db8::1")
	dst := netip.MustParseAddr("2001:db8::2")
	if packet.ValidateICMPv6Checksum(src, dst, nil) {
		t.Fatalf("ValidateICMPv6Checksum(nil) = true, want false")
	}
	if packet.ValidateICMPv6Checksum(src, dst, []byte{1, 2, 3}) {
		t.Fatalf("ValidateICMPv6Checksum(3 bytes) = true, want false")
	}

	// Round trip validation on arbitrary length buffers
	lengths := []int{2, 4, 10, 25, 100, 1500}
	for _, l := range lengths {
		buf := make([]byte, l)
		for i := range buf {
			buf[i] = byte(i*13 + 5)
		}
		// Internet checksum operates on 16-bit words; odd buffers must be padded before appending checksum
		padded := append([]byte(nil), buf...)
		if len(padded)%2 == 1 {
			padded = append(padded, 0)
		}
		c := packet.CalculateInternetChecksum(padded)
		withC := append(padded, byte(c>>8), byte(c&0xff))
		if !packet.ValidateInternetChecksum(withC) {
			t.Fatalf("ValidateInternetChecksum failed for length %d round trip", l)
		}
	}
}

func BenchmarkCalculateInternetChecksum_20B(b *testing.B) {
	data := make([]byte, 20)
	for i := range data {
		data[i] = byte(i)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = packet.CalculateInternetChecksum(data)
	}
}

func BenchmarkCalculateInternetChecksum_64B(b *testing.B) {
	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = packet.CalculateInternetChecksum(data)
	}
}

func BenchmarkCalculateInternetChecksum_1500B(b *testing.B) {
	data := make([]byte, 1500)
	for i := range data {
		data[i] = byte(i)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = packet.CalculateInternetChecksum(data)
	}
}

func BenchmarkCalculateInternetChecksum_9000B(b *testing.B) {
	data := make([]byte, 9000)
	for i := range data {
		data[i] = byte(i)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = packet.CalculateInternetChecksum(data)
	}
}

func BenchmarkCalculateICMPv6Checksum_48B(b *testing.B) {
	src := netip.MustParseAddr("2001:db8::1")
	dst := netip.MustParseAddr("2001:db8::2")
	msg := make([]byte, 48)
	for i := range msg {
		msg[i] = byte(i)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = packet.CalculateICMPv6Checksum(src, dst, msg)
	}
}

func BenchmarkCalculateICMPv6Checksum_1280B(b *testing.B) {
	src := netip.MustParseAddr("2001:db8::1")
	dst := netip.MustParseAddr("2001:db8::2")
	msg := make([]byte, 1280)
	for i := range msg {
		msg[i] = byte(i)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = packet.CalculateICMPv6Checksum(src, dst, msg)
	}
}
