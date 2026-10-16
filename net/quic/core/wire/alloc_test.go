// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"testing"
)

func TestAlloc_HeaderAppendAndParse(t *testing.T) {
	buf := make([]byte, 128)
	dcid := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	scid := []byte{9, 10, 11, 12, 13, 14, 15, 16}

	longHdr := Header{
		IsLong:          true,
		Type:            PacketTypeInitial,
		Version:         Version1,
		DestConnID:      dcid,
		SrcConnID:       scid,
		Token:           nil,
		Length:          50,
		PacketNumber:    100,
		PacketNumberLen: 2,
	}

	shortHdr := Header{
		IsLong:          false,
		Type:            PacketType1RTT,
		DestConnID:      dcid,
		PacketNumber:    200,
		PacketNumberLen: 2,
		SpinBit:         true,
		KeyPhase:        false,
	}

	// 1. Long Header Append
	allocs := testing.AllocsPerRun(100, func() {
		b := buf[:0]
		b, _, _ = AppendHeader(b, &longHdr)
		_ = b
	})
	if allocs != 0 {
		t.Fatalf("AppendHeader (Long) allocated: %f allocs/run", allocs)
	}

	// 2. Short Header Append
	allocs = testing.AllocsPerRun(100, func() {
		b := buf[:0]
		b, _, _ = AppendHeader(b, &shortHdr)
		_ = b
	})
	if allocs != 0 {
		t.Fatalf("AppendHeader (Short) allocated: %f allocs/run", allocs)
	}

	// Prepare encoded slices for parsing
	longBuf, _, _ := AppendHeader(buf[:0], &longHdr)
	longPacket := append(longBuf, make([]byte, 48)...) // pad for Length=50 (2 bytes PN + 48 payload)
	shortBuf, _, _ := AppendHeader(buf[:0], &shortHdr)

	// 3. Long Header Parse
	allocs = testing.AllocsPerRun(100, func() {
		h, err := ParseHeader(longPacket, len(dcid), 99)
		if err != nil {
			t.Fatal(err)
		}
		_ = h
	})
	if allocs != 0 {
		t.Fatalf("ParseHeader (Long) allocated: %f allocs/run", allocs)
	}

	// 4. Short Header Parse
	allocs = testing.AllocsPerRun(100, func() {
		h, err := ParseHeader(shortBuf, len(dcid), 199)
		if err != nil {
			t.Fatal(err)
		}
		_ = h
	})
	if allocs != 0 {
		t.Fatalf("ParseHeader (Short) allocated: %f allocs/run", allocs)
	}

	// 5. ParseHeaderOffsets
	allocs = testing.AllocsPerRun(100, func() {
		pnOffset, _, _, _, _, _, err := ParseHeaderOffsets(longPacket, len(dcid))
		if err != nil {
			t.Fatal(err)
		}
		_ = pnOffset
	})
	if allocs != 0 {
		t.Fatalf("ParseHeaderOffsets allocated: %f allocs/run", allocs)
	}
}

func TestAlloc_HeaderProtection(t *testing.T) {
	hpKeyAES := make([]byte, 16)
	hpAES, _ := NewAESHeaderProtector(hpKeyAES)

	hpKeyChaCha := make([]byte, 32)
	hpChaCha, _ := NewChaChaHeaderProtector(hpKeyChaCha)

	sample := make([]byte, 16)
	// 1. AES Mask
	allocs := testing.AllocsPerRun(100, func() {
		m := hpAES.Mask(sample)
		_ = m
	})
	if allocs != 0 {
		t.Fatalf("AESHeaderProtector.Mask allocated: %f allocs/run", allocs)
	}

	// 2. ChaCha20 Mask
	allocs = testing.AllocsPerRun(100, func() {
		m := hpChaCha.Mask(sample)
		_ = m
	})
	if allocs != 0 {
		t.Fatalf("ChaChaHeaderProtector.Mask allocated: %f allocs/run", allocs)
	}

	packet := make([]byte, 64)
	packet[0] = 0xc3 // Long header, 4-byte PN

	// 3. Apply and Remove Header Protection in-place
	allocs = testing.AllocsPerRun(100, func() {
		_ = ApplyHeaderProtection(packet, 10, 4, hpAES)
		_, _ = RemoveHeaderProtection(packet, 10, hpAES)
	})
	if allocs != 0 {
		t.Fatalf("Apply/Remove Header Protection allocated: %f allocs/run", allocs)
	}
}

func TestAlloc_AEADSealAndOpen(t *testing.T) {
	suites := []CipherSuite{TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384, TLS_CHACHA20_POLY1305_SHA256}

	for _, suite := range suites {
		keyLen := 16
		if suite != TLS_AES_128_GCM_SHA256 {
			keyLen = 32
		}
		hpKeyLen := 16
		if suite == TLS_CHACHA20_POLY1305_SHA256 {
			hpKeyLen = 32
		}

		key := make([]byte, keyLen)
		iv := make([]byte, 12)
		hpKey := make([]byte, hpKeyLen)

		protector, err := NewPacketProtector(suite, key, iv, hpKey)
		if err != nil {
			t.Fatalf("NewPacketProtector failed: %v", err)
		}
		opener, err := NewPacketOpener(suite, key, iv, hpKey)
		if err != nil {
			t.Fatalf("NewPacketOpener failed: %v", err)
		}

		dcid := []byte{1, 2, 3, 4}
		hdr := Header{
			IsLong:          false,
			Type:            PacketType1RTT,
			DestConnID:      dcid,
			PacketNumber:    42,
			PacketNumberLen: 2,
		}
		payload := []byte("aead-payload-zero-alloc-test")

		dstBuf := make([]byte, 0, 256)
		decBuf := make([]byte, 0, 256)
		packetBuf := make([]byte, 256)

		// 1. SealPacket
		allocs := testing.AllocsPerRun(100, func() {
			out, err := protector.SealPacket(dstBuf[:0], &hdr, payload)
			if err != nil {
				t.Fatal(err)
			}
			_ = out
		})
		if allocs != 0 {
			t.Fatalf("SealPacket (%s) allocated: %f allocs/run", suite, allocs)
		}

		// Prepare a sealed packet for OpenPacket
		sealed, _ := protector.SealPacket(dstBuf[:0], &hdr, payload)

		// 2. OpenPacket
		allocs = testing.AllocsPerRun(100, func() {
			copy(packetBuf, sealed)
			pkt := packetBuf[:len(sealed)]
			_, dec, err := opener.OpenPacket(decBuf[:0], pkt, len(dcid), 41)
			if err != nil {
				t.Fatal(err)
			}
			_ = dec
		})
		if allocs != 0 {
			t.Fatalf("OpenPacket (%s) allocated: %f allocs/run", suite, allocs)
		}
	}
}

func TestAlloc_FrameCodecs(t *testing.T) {
	buf := make([]byte, 512)
	var f Frame

	// 1. Stream Frame Append & Parse
	stream := StreamFrame{
		StreamID: 4,
		Offset:   100,
		Fin:      true,
		HasLen:   true,
		Data:     []byte("test data zero alloc"),
	}
	allocs := testing.AllocsPerRun(100, func() {
		b := AppendStreamFrame(buf[:0], &stream)
		_, err := ParseNextFrame(b, &f)
		if err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("Stream frame codec allocated: %f allocs/run", allocs)
	}

	// 2. ACK Frame Append & Parse (inline ranges)
	ack := AckFrame{
		LargestAcked:  500,
		AckDelay:      10,
		FirstAckRange: 2,
		HasECN:        true,
		ECN:           ECNCounts{ECT0: 1, ECT1: 2, ECNCE: 3},
	}
	ack.AppendRange(2, 5)
	ack.AppendRange(3, 1)

	allocs = testing.AllocsPerRun(100, func() {
		b := AppendAckFrame(buf[:0], &ack)
		_, err := ParseNextFrame(b, &f)
		if err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("ACK frame codec allocated: %f allocs/run", allocs)
	}

	// 3. CRYPTO Frame Append & Parse
	crypto := CryptoFrame{
		Offset: 0,
		Data:   []byte("crypto handshake bytes"),
	}
	allocs = testing.AllocsPerRun(100, func() {
		b := AppendCryptoFrame(buf[:0], &crypto)
		_, err := ParseNextFrame(b, &f)
		if err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("Crypto frame codec allocated: %f allocs/run", allocs)
	}

	// 4. RESET_STREAM Frame Append & Parse
	reset := ResetStreamFrame{
		StreamID:  8,
		ErrorCode: 0x11,
		FinalSize: 5000,
	}
	allocs = testing.AllocsPerRun(100, func() {
		b := AppendResetStreamFrame(buf[:0], &reset)
		_, err := ParseNextFrame(b, &f)
		if err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("ResetStream frame codec allocated: %f allocs/run", allocs)
	}

	// 5. Multi-frame sequence parse
	multi := make([]byte, 0, 256)
	multi = AppendPing(multi)
	multi = AppendAckFrame(multi, &ack)
	multi = AppendStreamFrame(multi, &stream)
	multi = AppendPadding(multi, 16)

	allocs = testing.AllocsPerRun(100, func() {
		remaining := multi
		for len(remaining) > 0 {
			n, err := ParseNextFrame(remaining, &f)
			if err != nil {
				t.Fatal(err)
			}
			remaining = remaining[n:]
		}
	})
	if allocs != 0 {
		t.Fatalf("Multi-frame sequence parse allocated: %f allocs/run", allocs)
	}
}
