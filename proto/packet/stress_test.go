// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packet_test

import (
	"encoding/binary"
	"math/rand/v2"
	"net/netip"
	"testing"

	"github.com/lemon4ksan/mach/proto/packet"
)

func TestStressInternetChecksum_OddLengthEquivalence(t *testing.T) {
	// RFC 1071 §4.1: An odd-length buffer must produce the exact same checksum
	// as an even-length buffer formed by appending a zero octet.
	for length := 1; length <= 255; length += 2 {
		buf := make([]byte, length)
		for i := range buf {
			buf[i] = byte((i*37 + 13) & 0xff)
		}

		csumOdd := packet.CalculateInternetChecksum(buf)

		bufEven := append([]byte(nil), buf...)
		bufEven = append(bufEven, 0)
		csumEven := packet.CalculateInternetChecksum(bufEven)

		if csumOdd != csumEven {
			t.Fatalf("length %d: csumOdd=0x%04x != csumEven=0x%04x", length, csumOdd, csumEven)
		}
	}
}

func TestStressInternetChecksum_RFC1071ByteSwapping(t *testing.T) {
	// RFC 1071 §4.1: If bytes in each 16-bit word are swapped, the checksum is swapped.
	r := rand.New(rand.NewPCG(1234, 5678)) //nolint:gosec
	for trial := 0; trial < 100; trial++ {
		length := (r.IntN(50) + 1) * 2 // even length
		orig := make([]byte, length)
		swapped := make([]byte, length)
		for i := range orig {
			orig[i] = byte(r.Uint32())
		}

		for i := 0; i < length; i += 2 {
			swapped[i] = orig[i+1]
			swapped[i+1] = orig[i]
		}

		csumOrig := packet.CalculateInternetChecksum(orig)
		csumSwapped := packet.CalculateInternetChecksum(swapped)

		expectedSwapped := (csumOrig >> 8) | ((csumOrig & 0xff) << 8)
		if csumSwapped != expectedSwapped {
			t.Fatalf("trial %d len %d: swapped csum 0x%04x != expected 0x%04x (orig 0x%04x)",
				trial, length, csumSwapped, expectedSwapped, csumOrig)
		}
	}
}

func TestStressInternetChecksum_AllOnesLarge(t *testing.T) {
	// 65534 bytes of 0xff (even length) -> 32767 words of 0xffff
	// 1's complement sum of any number of 0xffff words (-0) is 0xffff (-0).
	// Complement of 0xffff is 0x0000.
	data := make([]byte, 65534)
	for i := range data {
		data[i] = 0xff
	}

	csum := packet.CalculateInternetChecksum(data)
	if csum != 0x0000 {
		t.Fatalf("checksum of 65534 0xff bytes = 0x%04x, want 0x0000", csum)
	}

	// 65535 bytes of 0xff (odd length) -> last byte is 0xff padded to 0xff00.
	// Sum is 32767 * 0xffff + 0xff00 -> folded to 0xff00.
	// Complement of 0xff00 is 0x00ff.
	dataOdd := make([]byte, 65535)
	for i := range dataOdd {
		dataOdd[i] = 0xff
	}
	csumOdd := packet.CalculateInternetChecksum(dataOdd)
	if csumOdd != 0x00ff {
		t.Fatalf("checksum of 65535 0xff bytes = 0x%04x, want 0x00ff", csumOdd)
	}
}

func TestStressInternetChecksum_SingleBitCorruption(t *testing.T) {
	r := rand.New(rand.NewPCG(9999, 8888)) //nolint:gosec

	for _, length := range []int{20, 40, 64, 128, 512, 1400} {
		pkt := make([]byte, length)
		for i := range pkt {
			pkt[i] = byte(r.Uint32())
		}

		// Set checksum field at bytes 10:12 to 0
		pkt[10] = 0
		pkt[11] = 0
		csum := packet.CalculateInternetChecksum(pkt)
		binary.BigEndian.PutUint16(pkt[10:12], csum)

		if !packet.ValidateInternetChecksum(pkt) {
			t.Fatalf("length %d: valid packet failed ValidateInternetChecksum", length)
		}

		// Test every single bit flipped in checksum field
		for bit := 0; bit < 16; bit++ {
			corruptPkt := append([]byte(nil), pkt...)
			corruptPkt[10+(bit/8)] ^= (1 << (bit % 8))
			if packet.ValidateInternetChecksum(corruptPkt) {
				t.Fatalf("length %d bit %d: flipped bit was not detected!", length, bit)
			}
		}

		// Test 20 random byte corruptions
		for i := 0; i < 20; i++ {
			corruptPkt := append([]byte(nil), pkt...)
			corruptIdx := r.IntN(length)
			corruptPkt[corruptIdx] ^= byte(r.IntN(255) + 1)
			// Recalculating checksum over corrupted packet should NOT be zero
			if packet.CalculateInternetChecksum(corruptPkt) == 0 {
				t.Fatalf("length %d: corruption at index %d produced zero checksum!", length, corruptIdx)
			}
		}
	}
}

func TestStressICMPv6Checksum_OddAndEvenLengths(t *testing.T) {
	src := netip.MustParseAddr("fe80::1")
	dst := netip.MustParseAddr("ff02::1")

	for length := 4; length <= 128; length++ {
		msg := make([]byte, length)
		for i := range msg {
			msg[i] = byte(i + 1)
		}
		// Zero out checksum field (bytes 2:4)
		msg[2] = 0
		msg[3] = 0

		csum := packet.CalculateICMPv6Checksum(src, dst, msg)
		binary.BigEndian.PutUint16(msg[2:4], csum)

		if !packet.ValidateICMPv6Checksum(src, dst, msg) {
			t.Fatalf("ICMPv6 length %d failed validation with computed checksum 0x%04x", length, csum)
		}

		// Bit corruption
		msg[2] ^= 0x01
		if packet.ValidateICMPv6Checksum(src, dst, msg) {
			t.Fatalf("ICMPv6 length %d succeeded validation after corruption!", length)
		}
	}
}
