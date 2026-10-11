// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func hexDecode(t *testing.T, s string) []byte {
	t.Helper()
	// Strip whitespace and newlines
	var clean []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			clean = append(clean, c)
		}
	}
	b, err := hex.DecodeString(string(clean))
	if err != nil {
		t.Fatalf("failed to decode hex %q: %v", s, err)
	}
	return b
}

// TestRFC9001AppendixA2_ClientInitial tests the Client Initial packet vector from RFC 9001 §A.2.
func TestRFC9001AppendixA2_ClientInitial(t *testing.T) {
	key := hexDecode(t, "1f369613dd76d5467730efcbe3b1a22d")
	iv := hexDecode(t, "fa044b2f42a3fd3b46fb255c")
	hpKey := hexDecode(t, "9f50449e04a0e810283a1e9933adedd2")

	// Unprotected CRYPTO frame payload from A.2
	cryptoPayload := hexDecode(t, `
		060040f1010000ed0303ebf8fa56f129 39b9584a3896472ec40bb863cfd3e868
		04fe3a47f06a2b69484c000004130113 02010000c000000010000e00000b6578
		616d706c652e636f6dff01000100000a 00080006001d00170018001000070005
		04616c706e0005000501000000000033 00260024001d00209370b2c9caa47fba
		baf4559fedba753de171fa71f50f1ce1 5d43e994ec74d748002b000302030400
		0d0010000e0403050306030203080408 050806002d00020101001c0002400100
		3900320408ffffffffffffffff050480 00ffff07048000ffff08011001048000
		75300901100f088394c8f03e51570806 048000ffff
	`)

	// Pad with PADDING to 1162 bytes
	payload := make([]byte, 1162)
	copy(payload, cryptoPayload)

	expectedUnprotectedHeader := hexDecode(t, "c300000001088394c8f03e5157080000449e00000002")
	expectedProtectedHeader := hexDecode(t, "c000000001088394c8f03e5157080000449e7b9aec34")
	expectedSample := hexDecode(t, "d1b1c98dd7689fb8ec11d242b123dc9b")
	expectedMask := hexDecode(t, "437b9aec36")

	// Step 1: Verify header protection mask computation
	hp, err := NewAESHeaderProtector(hpKey)
	if err != nil {
		t.Fatalf("NewAESHeaderProtector failed: %v", err)
	}
	mask := hp.Mask(expectedSample)
	if !bytes.Equal(mask[:], expectedMask) {
		t.Fatalf("AES mask mismatch: got %x, want %x", mask[:], expectedMask)
	}

	// Step 2: Build Header struct
	destCID := hexDecode(t, "8394c8f03e515708")
	h := Header{
		IsLong:          true,
		Type:            PacketTypeInitial,
		Version:         Version1,
		DestConnID:      destCID,
		SrcConnID:       nil,
		Token:           nil,
		Length:          1182,
		PacketNumber:    2,
		PacketNumberLen: 4,
	}

	// Step 3: Serialize unprotected header
	unprotectedHdr, pnOffset, err := AppendHeader(nil, &h)
	if err != nil {
		t.Fatalf("AppendHeader failed: %v", err)
	}
	if !bytes.Equal(unprotectedHdr, expectedUnprotectedHeader) {
		t.Fatalf("Unprotected header mismatch:\ngot  %x\nwant %x", unprotectedHdr, expectedUnprotectedHeader)
	}
	if pnOffset != 18 {
		t.Fatalf("pnOffset mismatch: got %d, want 18", pnOffset)
	}

	// Step 4: Protect packet with PacketProtector
	protector, err := NewPacketProtector(TLS_AES_128_GCM_SHA256, key, iv, hpKey)
	if err != nil {
		t.Fatalf("NewPacketProtector failed: %v", err)
	}

	buf := make([]byte, 0, 1200)
	protectedPacket, err := protector.SealPacket(buf, &h, payload)
	if err != nil {
		t.Fatalf("SealPacket failed: %v", err)
	}

	if len(protectedPacket) != 1200 {
		t.Fatalf("Protected packet len mismatch: got %d, want 1200", len(protectedPacket))
	}

	// Verify protected header prefix matches
	if !bytes.Equal(protectedPacket[:len(expectedProtectedHeader)], expectedProtectedHeader) {
		t.Fatalf("Protected header mismatch:\ngot  %x\nwant %x", protectedPacket[:len(expectedProtectedHeader)], expectedProtectedHeader)
	}

	// Verify sample matches
	sample := protectedPacket[pnOffset+4 : pnOffset+4+16]
	if !bytes.Equal(sample, expectedSample) {
		t.Fatalf("Sample mismatch:\ngot  %x\nwant %x", sample, expectedSample)
	}

	// Step 5: Decrypt packet using PacketOpener
	opener, err := NewPacketOpener(TLS_AES_128_GCM_SHA256, key, iv, hpKey)
	if err != nil {
		t.Fatalf("NewPacketOpener failed: %v", err)
	}

	packetCopy := append([]byte(nil), protectedPacket...)
	decryptedHdr, decryptedPayload, err := opener.OpenPacket(nil, packetCopy, len(destCID), 0)
	if err != nil {
		t.Fatalf("OpenPacket failed: %v", err)
	}

	if decryptedHdr.PacketNumber != 2 {
		t.Fatalf("Decrypted packet number mismatch: got %d, want 2", decryptedHdr.PacketNumber)
	}
	if !bytes.Equal(decryptedPayload, payload) {
		t.Fatalf("Decrypted payload mismatch")
	}
}

// TestRFC9001AppendixA3_ServerInitial tests the Server Initial packet vector from RFC 9001 §A.3.
func TestRFC9001AppendixA3_ServerInitial(t *testing.T) {
	key := hexDecode(t, "cf3a5331653c364c88f0f379b6067e37")
	iv := hexDecode(t, "0ac1493ca1905853b0bba03e")
	hpKey := hexDecode(t, "c206b8d9b9f0f37644430b490eeaa314")

	payload := hexDecode(t, `
		02000000000600405a020000560303ee fce7f7b37ba1d1632e96677825ddf739
		88cfc79825df566dc5430b9a045a1200 130100002e00330024001d00209d3c94
		0d89690b84d08a60993c144eca684d10 81287c834d5311bcf32bb9da1a002b00
		020304
	`)

	expectedUnprotectedHeader := hexDecode(t, "c1000000010008f067a5502a4262b50040750001")
	expectedProtectedHeader := hexDecode(t, "cf000000010008f067a5502a4262b5004075c0d9")
	expectedSample := hexDecode(t, "2cd0991cd25b0aac406a5816b6394100")
	expectedMask := hexDecode(t, "2ec0d8356a")

	// Verify HP mask
	hp, err := NewAESHeaderProtector(hpKey)
	if err != nil {
		t.Fatalf("NewAESHeaderProtector failed: %v", err)
	}
	mask := hp.Mask(expectedSample)
	if !bytes.Equal(mask[:], expectedMask) {
		t.Fatalf("AES mask mismatch: got %x, want %x", mask[:], expectedMask)
	}

	scid := hexDecode(t, "f067a5502a4262b5")
	h := Header{
		IsLong:          true,
		Type:            PacketTypeInitial,
		Version:         Version1,
		DestConnID:      nil,
		SrcConnID:       scid,
		Length:          uint64(2 + len(payload) + 16), // 2 bytes PN + payload + 16 tag = 117 (0x75)
		PacketNumber:    1,
		PacketNumberLen: 2,
	}

	unprotectedHdr, pnOffset, err := AppendHeader(nil, &h)
	if err != nil {
		t.Fatalf("AppendHeader failed: %v", err)
	}
	if !bytes.Equal(unprotectedHdr, expectedUnprotectedHeader) {
		t.Fatalf("Unprotected header mismatch:\ngot  %x\nwant %x", unprotectedHdr, expectedUnprotectedHeader)
	}

	protector, err := NewPacketProtector(TLS_AES_128_GCM_SHA256, key, iv, hpKey)
	if err != nil {
		t.Fatalf("NewPacketProtector failed: %v", err)
	}

	protectedPacket, err := protector.SealPacket(nil, &h, payload)
	if err != nil {
		t.Fatalf("SealPacket failed: %v", err)
	}

	if !bytes.Equal(protectedPacket[:len(expectedProtectedHeader)], expectedProtectedHeader) {
		t.Fatalf("Protected header mismatch:\ngot  %x\nwant %x", protectedPacket[:len(expectedProtectedHeader)], expectedProtectedHeader)
	}

	sample := protectedPacket[pnOffset+4 : pnOffset+4+16]
	if !bytes.Equal(sample, expectedSample) {
		t.Fatalf("Sample mismatch:\ngot  %x\nwant %x", sample, expectedSample)
	}

	// Round-trip decrypt
	opener, err := NewPacketOpener(TLS_AES_128_GCM_SHA256, key, iv, hpKey)
	if err != nil {
		t.Fatalf("NewPacketOpener failed: %v", err)
	}

	packetCopy := append([]byte(nil), protectedPacket...)
	decryptedHdr, decryptedPayload, err := opener.OpenPacket(nil, packetCopy, 0, 0)
	if err != nil {
		t.Fatalf("OpenPacket failed: %v", err)
	}

	if decryptedHdr.PacketNumber != 1 {
		t.Fatalf("Decrypted packet number mismatch: got %d, want 1", decryptedHdr.PacketNumber)
	}
	if !bytes.Equal(decryptedPayload, payload) {
		t.Fatalf("Decrypted payload mismatch")
	}
}

// TestRFC9001AppendixA4_Retry tests the Retry packet vector from RFC 9001 §A.4.
func TestRFC9001AppendixA4_Retry(t *testing.T) {
	retryPacketHex := "ff000000010008f067a5502a4262b5746f6b656e04a265ba2eff4d829058fb3f0f2496ba"
	raw := hexDecode(t, retryPacketHex)

	h, err := ParseHeader(raw, 0, 0)
	if err != nil {
		t.Fatalf("ParseHeader failed: %v", err)
	}

	if !h.IsLong {
		t.Fatalf("Expected long header")
	}
	if h.Type != PacketTypeRetry {
		t.Fatalf("Expected Retry packet type, got %s", h.Type)
	}
	if h.Version != Version1 {
		t.Fatalf("Expected Version 1, got %s", h.Version)
	}
	if len(h.DestConnID) != 0 {
		t.Fatalf("Expected empty DCID, got %x", h.DestConnID)
	}
	expectedSCID := hexDecode(t, "f067a5502a4262b5")
	if !bytes.Equal(h.SrcConnID, expectedSCID) {
		t.Fatalf("SCID mismatch: got %x, want %x", h.SrcConnID, expectedSCID)
	}
	if string(h.Token) != "token" {
		t.Fatalf("Retry token mismatch: got %q, want 'token'", string(h.Token))
	}
}

// TestRFC9001AppendixA5_ChaCha20Poly1305 tests the ChaCha20-Poly1305 short header vector from RFC 9001 §A.5.
func TestRFC9001AppendixA5_ChaCha20Poly1305(t *testing.T) {
	key := hexDecode(t, "c6d98ff3441c3fe1b2182094f69caa2ed4b716b65488960a7a984979fb23e1c8")
	iv := hexDecode(t, "e0459b3474bdd0e44a41c144")
	hpKey := hexDecode(t, "25a282b9e82f06f21f488917a4fc8f1b73573685608597d0efcb076b0ab7a7a4")

	pn := uint64(654360564)
	payload := []byte{0x01} // single PING frame

	expectedNonce := hexDecode(t, "e0459b3474bdd0e46d417eb0")
	expectedUnprotectedHdr := hexDecode(t, "4200bff4")
	expectedCiphertext := hexDecode(t, "655e5cd55c41f69080575d7999c25a5bfb")
	expectedSample := hexDecode(t, "5e5cd55c41f69080575d7999c25a5bfb")
	expectedMask := hexDecode(t, "aefefe7d03")
	expectedProtectedHdr := hexDecode(t, "4cfe4189")

	// Step 1: Verify nonce calculation
	var nonce [12]byte
	ComputeNonce(iv, pn, &nonce)
	if !bytes.Equal(nonce[:], expectedNonce) {
		t.Fatalf("Nonce mismatch: got %x, want %x", nonce[:], expectedNonce)
	}

	// Step 2: Verify ChaCha20 HP mask
	hp, err := NewChaChaHeaderProtector(hpKey)
	if err != nil {
		t.Fatalf("NewChaChaHeaderProtector failed: %v", err)
	}
	mask := hp.Mask(expectedSample)
	if !bytes.Equal(mask[:], expectedMask) {
		t.Fatalf("ChaCha mask mismatch: got %x, want %x", mask[:], expectedMask)
	}

	// Step 3: Serialize short header
	h := Header{
		IsLong:          false,
		Type:            PacketType1RTT,
		DestConnID:      nil, // empty DCID
		PacketNumber:    pn,
		PacketNumberLen: 3,
	}

	unprotectedHdr, pnOffset, err := AppendHeader(nil, &h)
	if err != nil {
		t.Fatalf("AppendHeader failed: %v", err)
	}
	if !bytes.Equal(unprotectedHdr, expectedUnprotectedHdr) {
		t.Fatalf("Unprotected header mismatch:\ngot  %x\nwant %x", unprotectedHdr, expectedUnprotectedHdr)
	}
	if pnOffset != 1 {
		t.Fatalf("pnOffset mismatch: got %d, want 1", pnOffset)
	}

	// Step 4: Protect packet with PacketProtector
	protector, err := NewPacketProtector(TLS_CHACHA20_POLY1305_SHA256, key, iv, hpKey)
	if err != nil {
		t.Fatalf("NewPacketProtector failed: %v", err)
	}

	protectedPacket, err := protector.SealPacket(nil, &h, payload)
	if err != nil {
		t.Fatalf("SealPacket failed: %v", err)
	}

	if !bytes.Equal(protectedPacket[:len(expectedProtectedHdr)], expectedProtectedHdr) {
		t.Fatalf("Protected header mismatch:\ngot  %x\nwant %x", protectedPacket[:len(expectedProtectedHdr)], expectedProtectedHdr)
	}

	// Verify sample matches
	sample := protectedPacket[pnOffset+4 : pnOffset+4+16]
	if !bytes.Equal(sample, expectedSample) {
		t.Fatalf("Sample mismatch:\ngot  %x\nwant %x", sample, expectedSample)
	}

	// Verify entire protected packet
	expectedFullPacket := append(append([]byte(nil), expectedProtectedHdr...), expectedCiphertext...)
	if !bytes.Equal(protectedPacket, expectedFullPacket) {
		t.Fatalf("Full protected packet mismatch:\ngot  %x\nwant %x", protectedPacket, expectedFullPacket)
	}

	// Step 5: Round-trip decrypt
	opener, err := NewPacketOpener(TLS_CHACHA20_POLY1305_SHA256, key, iv, hpKey)
	if err != nil {
		t.Fatalf("NewPacketOpener failed: %v", err)
	}

	packetCopy := append([]byte(nil), protectedPacket...)
	decryptedHdr, decryptedPayload, err := opener.OpenPacket(nil, packetCopy, 0, pn-1)
	if err != nil {
		t.Fatalf("OpenPacket failed: %v", err)
	}

	if decryptedHdr.PacketNumber != pn {
		t.Fatalf("Decrypted packet number mismatch: got %d, want %d", decryptedHdr.PacketNumber, pn)
	}
	if !bytes.Equal(decryptedPayload, payload) {
		t.Fatalf("Decrypted payload mismatch: got %x, want %x", decryptedPayload, payload)
	}
}
