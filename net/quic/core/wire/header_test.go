// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"testing"
)

func TestPacketNumberEncodingDecoding(t *testing.T) {
	// RFC 9000 Appendix A test cases:
	// Example 1: largestPN = 0xa82f30ea, truncated = 0x9b32, pnLen = 2
	largest := uint64(0xa82f30ea)
	truncated := uint64(0x9b32)
	decoded := DecodePacketNumber(largest, truncated, 2)
	expected := uint64(0xa82f9b32)
	if decoded != expected {
		t.Fatalf("DecodePacketNumber failed: got 0x%x, want 0x%x", decoded, expected)
	}

	// Example 2: largestPN = 0xa82f30ea, truncated = 0xace8fe, pnLen = 3
	truncated = uint64(0xace8fe)
	decoded = DecodePacketNumber(largest, truncated, 3)
	expected = uint64(0xa8ace8fe)
	if decoded != expected {
		t.Fatalf("DecodePacketNumber failed: got 0x%x, want 0x%x", decoded, expected)
	}

	// Boundary round-trip tests for 1, 2, 3, 4 bytes
	testPNs := []uint64{0, 1, 63, 64, 127, 128, 255, 256, 16383, 65535, 65536, 1073741823, 1<<31 - 1}
	for _, pn := range testPNs {
		for _, largestAcked := range []uint64{InvalidPacketNumber, 0, pn / 2, pn} {
			trunc, pnLen := EncodePacketNumber(pn, largestAcked)
			rec := DecodePacketNumber(largestAcked, trunc, pnLen)
			if rec != pn {
				t.Fatalf("PN round trip failed for pn=%d, largestAcked=%d: got %d (len=%d, trunc=%d)", pn, largestAcked, rec, pnLen, trunc)
			}
		}
	}
	// Test large packet numbers where largestAcked is within range
	for _, pn := range []uint64{1 << 32, 1 << 33, MaxPacketNumber} {
		for _, largestAcked := range []uint64{pn - 1000, pn - 1, pn} {
			trunc, pnLen := EncodePacketNumber(pn, largestAcked)
			rec := DecodePacketNumber(largestAcked, trunc, pnLen)
			if rec != pn {
				t.Fatalf("PN round trip failed for large pn=%d, largestAcked=%d: got %d (len=%d, trunc=%d)", pn, largestAcked, rec, pnLen, trunc)
			}
		}
	}
}

func TestHeaderRoundTrip_LongHeaders_V1_and_V2(t *testing.T) {
	versions := []Version{Version1, Version2}
	types := []PacketType{PacketTypeInitial, PacketType0RTT, PacketTypeHandshake}
	dcids := [][]byte{nil, {0x01, 0x02, 0x03, 0x04}, bytes.Repeat([]byte{0xaa}, 20)}
	scids := [][]byte{nil, {0x05, 0x06, 0x07, 0x08}, bytes.Repeat([]byte{0xbb}, 20)}

	for _, v := range versions {
		for _, typ := range types {
			for _, dcid := range dcids {
				for _, scid := range scids {
					for pnLen := 1; pnLen <= 4; pnLen++ {
						orig := Header{
							IsLong:          true,
							Type:            typ,
							Version:         v,
							DestConnID:      dcid,
							SrcConnID:       scid,
							Length:          150,
							PacketNumber:    12345,
							PacketNumberLen: pnLen,
						}
						if typ == PacketTypeInitial {
							orig.Token = []byte("initial-token-test")
						}

						buf, pnOffset, err := AppendHeader(nil, &orig)
						if err != nil {
							t.Fatalf("AppendHeader failed: %v", err)
						}
						if pnOffset <= 0 {
							t.Fatalf("Invalid pnOffset: %d", pnOffset)
						}

						// Append dummy payload to satisfy packet Length field
						buf = append(buf, make([]byte, orig.Length-uint64(pnLen))...)

						// Verify ParseHeaderOffsets
						parsedPNOffset, isLong, parsedType, parsedVersion, parsedDCID, parsedSCID, err := ParseHeaderOffsets(buf, len(dcid))
						if err != nil {
							t.Fatalf("ParseHeaderOffsets failed: %v", err)
						}
						if parsedPNOffset != pnOffset {
							t.Fatalf("pnOffset mismatch: got %d, want %d", parsedPNOffset, pnOffset)
						}
						if !isLong || parsedType != typ || parsedVersion != v {
							t.Fatalf("ParseHeaderOffsets metadata mismatch")
						}
						if !bytes.Equal(parsedDCID, dcid) || !bytes.Equal(parsedSCID, scid) {
							t.Fatalf("ParseHeaderOffsets CID mismatch")
						}

						// Verify ParseHeader
						parsed, err := ParseHeader(buf, len(dcid), 12344)
						if err != nil {
							t.Fatalf("ParseHeader failed: %v", err)
						}

						if parsed.Type != typ || parsed.Version != v {
							t.Fatalf("Header type/version mismatch: got %s %s, want %s %s", parsed.Type, parsed.Version, typ, v)
						}
						if !bytes.Equal(parsed.DestConnID, dcid) || !bytes.Equal(parsed.SrcConnID, scid) {
							t.Fatalf("Header CID mismatch")
						}
						if typ == PacketTypeInitial && !bytes.Equal(parsed.Token, orig.Token) {
							t.Fatalf("Token mismatch: got %s, want %s", parsed.Token, orig.Token)
						}
						if parsed.PacketNumber != orig.PacketNumber || parsed.PacketNumberLen != orig.PacketNumberLen {
							t.Fatalf("PN mismatch: got %d (%d), want %d (%d)", parsed.PacketNumber, parsed.PacketNumberLen, orig.PacketNumber, orig.PacketNumberLen)
						}
					}
				}
			}
		}
	}
}

func TestHeaderRoundTrip_ShortHeader(t *testing.T) {
	dcids := [][]byte{nil, {0x01, 0x02, 0x03, 0x04}, bytes.Repeat([]byte{0xcc}, 20)}
	pns := []uint64{1, 42, 654360564, 1 << 40}

	for _, dcid := range dcids {
		for _, pn := range pns {
			for pnLen := 1; pnLen <= 4; pnLen++ {
				for _, spin := range []bool{false, true} {
					for _, kp := range []bool{false, true} {
						orig := Header{
							IsLong:          false,
							Type:            PacketType1RTT,
							DestConnID:      dcid,
							PacketNumber:    pn,
							PacketNumberLen: pnLen,
							SpinBit:         spin,
							KeyPhase:        kp,
						}

						buf, pnOffset, err := AppendHeader(nil, &orig)
						if err != nil {
							t.Fatalf("AppendHeader failed: %v", err)
						}
						if pnOffset != 1+len(dcid) {
							t.Fatalf("pnOffset mismatch: got %d, want %d", pnOffset, 1+len(dcid))
						}

						parsed, err := ParseHeader(buf, len(dcid), pn-1)
						if err != nil {
							t.Fatalf("ParseHeader failed: %v", err)
						}

						if parsed.IsLong || parsed.Type != PacketType1RTT {
							t.Fatalf("Expected 1-RTT short header")
						}
						if parsed.SpinBit != spin || parsed.KeyPhase != kp {
							t.Fatalf("SpinBit/KeyPhase mismatch")
						}
						if !bytes.Equal(parsed.DestConnID, dcid) {
							t.Fatalf("DCID mismatch")
						}
						if parsed.PacketNumber != pn || parsed.PacketNumberLen != pnLen {
							t.Fatalf("PN mismatch: got %d (%d), want %d (%d)", parsed.PacketNumber, parsed.PacketNumberLen, pn, pnLen)
						}
					}
				}
			}
		}
	}
}

func TestHeaderRoundTrip_VersionNegotiation(t *testing.T) {
	orig := Header{
		IsLong:     true,
		Type:       PacketTypeVersionNegotiation,
		DestConnID: []byte{0x01, 0x02, 0x03},
		SrcConnID:  []byte{0x04, 0x05, 0x06, 0x07},
	}

	buf, _, err := AppendHeader(nil, &orig)
	if err != nil {
		t.Fatalf("AppendHeader failed: %v", err)
	}

	parsed, err := ParseHeader(buf, 0, 0)
	if err != nil {
		t.Fatalf("ParseHeader failed: %v", err)
	}

	if parsed.Type != PacketTypeVersionNegotiation || parsed.Version != VersionNegotiation {
		t.Fatalf("Version negotiation mismatch")
	}
	if !bytes.Equal(parsed.DestConnID, orig.DestConnID) || !bytes.Equal(parsed.SrcConnID, orig.SrcConnID) {
		t.Fatalf("CIDs mismatch")
	}
}

func TestHeaderErrorCases(t *testing.T) {
	// Truncated header
	if _, err := ParseHeader(nil, 0, 0); err != ErrTruncatedPacket {
		t.Fatalf("Expected ErrTruncatedPacket, got %v", err)
	}

	// Long header too short
	if _, err := ParseHeader([]byte{0xc0, 0x00, 0x00}, 0, 0); err != ErrTruncatedHeader {
		t.Fatalf("Expected ErrTruncatedHeader, got %v", err)
	}

	// Fixed bit zero in Long Header
	badFixedBit := []byte{0x80, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00}
	if _, err := ParseHeader(badFixedBit, 0, 0); err != ErrInvalidFixedBit {
		t.Fatalf("Expected ErrInvalidFixedBit, got %v", err)
	}

	// Reserved bits non-zero in Long Header (bits 3-2 non-zero: 0xc4)
	badReserved := []byte{0xc4, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00}
	if _, err := ParseHeader(badReserved, 0, 0); err != ErrInvalidReservedBits {
		t.Fatalf("Expected ErrInvalidReservedBits, got %v", err)
	}

	// Fixed bit zero in Short Header
	badShortFixed := []byte{0x00, 0x00}
	if _, err := ParseHeader(badShortFixed, 0, 0); err != ErrInvalidFixedBit {
		t.Fatalf("Expected ErrInvalidFixedBit, got %v", err)
	}

	// Reserved bits non-zero in Short Header (bits 4-3: 0x58)
	badShortReserved := []byte{0x58, 0x00}
	if _, err := ParseHeader(badShortReserved, 0, 0); err != ErrInvalidReservedBits {
		t.Fatalf("Expected ErrInvalidReservedBits, got %v", err)
	}

	// Connection ID exceeding 20 bytes
	longCID := Header{
		IsLong:          false,
		Type:            PacketType1RTT,
		DestConnID:      make([]byte, 21),
		PacketNumber:    1,
		PacketNumberLen: 1,
	}
	if _, _, err := AppendHeader(nil, &longCID); err != ErrInvalidConnectionIDLen {
		t.Fatalf("Expected ErrInvalidConnectionIDLen, got %v", err)
	}
}

func TestHeaderProtectionMasking(t *testing.T) {
	// Test error cases on Apply / Remove Header Protection
	hp, _ := NewAESHeaderProtector(make([]byte, 16))
	shortBuf := make([]byte, 10)
	if err := ApplyHeaderProtection(shortBuf, 0, 1, hp); err != ErrSampleOutOfBounds {
		t.Fatalf("Expected ErrSampleOutOfBounds, got %v", err)
	}
	if _, err := RemoveHeaderProtection(shortBuf, 0, hp); err != ErrSampleOutOfBounds {
		t.Fatalf("Expected ErrSampleOutOfBounds, got %v", err)
	}

	// Invalid packet number length
	fullBuf := make([]byte, 30)
	if err := ApplyHeaderProtection(fullBuf, 0, 5, hp); err != ErrInvalidPacketNumber {
		t.Fatalf("Expected ErrInvalidPacketNumber, got %v", err)
	}

	// In-place masking and unmasking round trip on arbitrary header bytes
	for pnLen := 1; pnLen <= 4; pnLen++ {
		header := Header{
			IsLong:          true,
			Type:            PacketTypeHandshake,
			Version:         Version1,
			DestConnID:      []byte{0x11, 0x22},
			SrcConnID:       []byte{0x33, 0x44},
			Length:          100,
			PacketNumber:    0x123456,
			PacketNumberLen: pnLen,
		}

		packet, pnOffset, err := AppendHeader(nil, &header)
		if err != nil {
			t.Fatalf("AppendHeader failed: %v", err)
		}
		// Pad with dummy ciphertext to have enough sample space
		packet = append(packet, make([]byte, 64)...)
		copy(packet[pnOffset+4:pnOffset+4+16], []byte("0123456789abcdef"))

		origFirstByte := packet[0]
		origPN := ReadPacketNumber(packet[pnOffset:], pnLen)

		// Apply protection
		if err := ApplyHeaderProtection(packet, pnOffset, pnLen, hp); err != nil {
			t.Fatalf("ApplyHeaderProtection failed: %v", err)
		}

		// First byte or PN should be masked
		if packet[0] == origFirstByte && ReadPacketNumber(packet[pnOffset:], pnLen) == origPN {
			t.Fatalf("Header protection did not alter header or packet number")
		}

		// Remove protection
		unmaskedPNLen, err := RemoveHeaderProtection(packet, pnOffset, hp)
		if err != nil {
			t.Fatalf("RemoveHeaderProtection failed: %v", err)
		}
		if unmaskedPNLen != pnLen {
			t.Fatalf("PN len mismatch: got %d, want %d", unmaskedPNLen, pnLen)
		}
		if packet[0] != origFirstByte {
			t.Fatalf("First byte mismatch after unmasking: got 0x%x, want 0x%x", packet[0], origFirstByte)
		}
		unmaskedPN := ReadPacketNumber(packet[pnOffset:], pnLen)
		if unmaskedPN != origPN {
			t.Fatalf("PN mismatch after unmasking: got 0x%x, want 0x%x", unmaskedPN, origPN)
		}
	}
}

func TestVersionSalt(t *testing.T) {
	v1Salt := InitialSalt(Version1)
	if len(v1Salt) != 20 {
		t.Fatalf("Expected 20-byte salt for v1, got %d", len(v1Salt))
	}
	v2Salt := InitialSalt(Version2)
	if len(v2Salt) != 20 {
		t.Fatalf("Expected 20-byte salt for v2, got %d", len(v2Salt))
	}
	if bytes.Equal(v1Salt, v2Salt) {
		t.Fatalf("v1 and v2 salt must not be equal")
	}
}
