// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func fromHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("failed to decode hex %q: %v", s, err)
	}
	return b
}

func TestRFC9001AppendixAGoldenVectors(t *testing.T) {
	// RFC 9001 Appendix A.1
	destCID := fromHex(t, "8394c8f03e515708")

	keys := NewInitialKeysWithSalt(destCID, SaltV1, Version1)

	expectedClientSecret := fromHex(t, "c00cf151ca5be075ed0ebfb5c80323c42d6b7db67881289af4008f1f6c357aea")
	expectedClientKey := fromHex(t, "1f369613dd76d5467730efcbe3b1a22d")
	expectedClientIV := fromHex(t, "fa044b2f42a3fd3b46fb255c")
	expectedClientHP := fromHex(t, "9f50449e04a0e810283a1e9933adedd2")

	if !bytes.Equal(keys.ClientSecret, expectedClientSecret) {
		t.Errorf("ClientSecret mismatch:\ngot:  %x\nwant: %x", keys.ClientSecret, expectedClientSecret)
	}
	if !bytes.Equal(keys.ClientKey, expectedClientKey) {
		t.Errorf("ClientKey mismatch:\ngot:  %x\nwant: %x", keys.ClientKey, expectedClientKey)
	}
	if !bytes.Equal(keys.ClientIV, expectedClientIV) {
		t.Errorf("ClientIV mismatch:\ngot:  %x\nwant: %x", keys.ClientIV, expectedClientIV)
	}
	if !bytes.Equal(keys.ClientHPKey, expectedClientHP) {
		t.Errorf("ClientHPKey mismatch:\ngot:  %x\nwant: %x", keys.ClientHPKey, expectedClientHP)
	}

	expectedServerSecret := fromHex(t, "3c199828fd139efd216c155ad844cc81fb82fa8d7446fa7d78be803acdda951b")
	expectedServerKey := fromHex(t, "cf3a5331653c364c88f0f379b6067e37")
	expectedServerIV := fromHex(t, "0ac1493ca1905853b0bba03e")
	expectedServerHP := fromHex(t, "c206b8d9b9f0f37644430b490eeaa314")

	if !bytes.Equal(keys.ServerSecret, expectedServerSecret) {
		t.Errorf("ServerSecret mismatch:\ngot:  %x\nwant: %x", keys.ServerSecret, expectedServerSecret)
	}
	if !bytes.Equal(keys.ServerKey, expectedServerKey) {
		t.Errorf("ServerKey mismatch:\ngot:  %x\nwant: %x", keys.ServerKey, expectedServerKey)
	}
	if !bytes.Equal(keys.ServerIV, expectedServerIV) {
		t.Errorf("ServerIV mismatch:\ngot:  %x\nwant: %x", keys.ServerIV, expectedServerIV)
	}
	if !bytes.Equal(keys.ServerHPKey, expectedServerHP) {
		t.Errorf("ServerHPKey mismatch:\ngot:  %x\nwant: %x", keys.ServerHPKey, expectedServerHP)
	}
}

func TestRFC9369V2GoldenVectors(t *testing.T) {
	// RFC 9369 QUIC v2 test vectors
	destCID := fromHex(t, "8394c8f03e515708")

	keys := NewInitialKeysWithSalt(destCID, SaltV2RFC9369, Version2)

	expectedClientSecret := fromHex(t, "14ec9d6eb9fd7af83bf5a668bc17a7e283766aade7ecd0891f70f9ff7f4bf47b")
	expectedClientKey := fromHex(t, "8b1a0bc121284290a29e0971b5cd045d")
	expectedClientIV := fromHex(t, "91f73e2351d8fa91660e909f")
	expectedClientHP := fromHex(t, "45b95e15235d6f45a6b19cbcb0294ba9")

	if !bytes.Equal(keys.ClientSecret, expectedClientSecret) {
		t.Errorf("ClientSecret mismatch (v2):\ngot:  %x\nwant: %x", keys.ClientSecret, expectedClientSecret)
	}
	if !bytes.Equal(keys.ClientKey, expectedClientKey) {
		t.Errorf("ClientKey mismatch (v2):\ngot:  %x\nwant: %x", keys.ClientKey, expectedClientKey)
	}
	if !bytes.Equal(keys.ClientIV, expectedClientIV) {
		t.Errorf("ClientIV mismatch (v2):\ngot:  %x\nwant: %x", keys.ClientIV, expectedClientIV)
	}
	if !bytes.Equal(keys.ClientHPKey, expectedClientHP) {
		t.Errorf("ClientHPKey mismatch (v2):\ngot:  %x\nwant: %x", keys.ClientHPKey, expectedClientHP)
	}

	expectedServerSecret := fromHex(t, "0263db1782731bf4588e7e4d93b7463907cb8cd8200b5da55a8bd488eafc37c1")
	expectedServerKey := fromHex(t, "82db637861d55e1d011f19ea71d5d2a7")
	expectedServerIV := fromHex(t, "dd13c276499c0249d3310652")
	expectedServerHP := fromHex(t, "edf6d05c83121201b436e16877593c3a")

	if !bytes.Equal(keys.ServerSecret, expectedServerSecret) {
		t.Errorf("ServerSecret mismatch (v2):\ngot:  %x\nwant: %x", keys.ServerSecret, expectedServerSecret)
	}
	if !bytes.Equal(keys.ServerKey, expectedServerKey) {
		t.Errorf("ServerKey mismatch (v2):\ngot:  %x\nwant: %x", keys.ServerKey, expectedServerKey)
	}
	if !bytes.Equal(keys.ServerIV, expectedServerIV) {
		t.Errorf("ServerIV mismatch (v2):\ngot:  %x\nwant: %x", keys.ServerIV, expectedServerIV)
	}
	if !bytes.Equal(keys.ServerHPKey, expectedServerHP) {
		t.Errorf("ServerHPKey mismatch (v2):\ngot:  %x\nwant: %x", keys.ServerHPKey, expectedServerHP)
	}
}

func TestVersion2PromptSaltDerivation(t *testing.T) {
	// Salt requested: 0x0d3e709f25fa60728d85121cd6010f74f34f6a0d
	destCID := fromHex(t, "8394c8f03e515708")

	keys := NewInitialKeysWithSalt(destCID, SaltV2, Version2)
	if len(keys.ClientSecret) != 32 || len(keys.ServerSecret) != 32 {
		t.Fatalf("expected 32-byte secrets")
	}
	if len(keys.ClientKey) != 16 || len(keys.ClientIV) != 12 || len(keys.ClientHPKey) != 16 {
		t.Fatalf("expected 16-byte key, 12-byte IV, 16-byte HP")
	}
	if len(keys.ServerKey) != 16 || len(keys.ServerIV) != 12 || len(keys.ServerHPKey) != 16 {
		t.Fatalf("expected 16-byte key, 12-byte IV, 16-byte HP")
	}
}

func TestHeaderProtectionAES(t *testing.T) {
	hpKey := fromHex(t, "9f50449e04a0e810283a1e9933adedd2")
	sample := fromHex(t, "d1b1c98dd7689fb8ec11d242b123dc9b")

	hp, err := NewAESHeaderProtector(hpKey, true)
	if err != nil {
		t.Fatalf("failed to create AES header protector: %v", err)
	}

	firstByte := byte(0xc3)
	pn := fromHex(t, "00000002")

	// Encrypt
	hp.EncryptHeader(sample, &firstByte, pn)

	expectedFirstByte := byte(0xc0)
	expectedPN := fromHex(t, "7b9aec34")

	if firstByte != expectedFirstByte {
		t.Errorf("encrypted first byte: got 0x%02x, want 0x%02x", firstByte, expectedFirstByte)
	}
	if !bytes.Equal(pn, expectedPN) {
		t.Errorf("encrypted PN: got %x, want %x", pn, expectedPN)
	}

	// Decrypt
	hp.DecryptHeader(sample, &firstByte, pn)

	if firstByte != byte(0xc3) {
		t.Errorf("decrypted first byte: got 0x%02x, want 0xc3", firstByte)
	}
	if !bytes.Equal(pn, fromHex(t, "00000002")) {
		t.Errorf("decrypted PN: got %x, want 00000002", pn)
	}
}

func TestHeaderProtectionChaCha20(t *testing.T) {
	hpKey := make([]byte, 32)
	for i := range hpKey {
		hpKey[i] = byte(i + 1)
	}
	sample := make([]byte, 16)
	for i := range sample {
		sample[i] = byte(i + 10)
	}

	hp, err := NewChaChaHeaderProtector(hpKey, false)
	if err != nil {
		t.Fatalf("failed to create ChaCha20 header protector: %v", err)
	}

	origFirst := byte(0x40)
	origPN := []byte{0x01, 0x02, 0x03}

	firstByte := origFirst
	pn := bytes.Clone(origPN)

	hp.EncryptHeader(sample, &firstByte, pn)
	if firstByte == origFirst && bytes.Equal(pn, origPN) {
		t.Fatalf("expected header protection to modify header")
	}

	hp.DecryptHeader(sample, &firstByte, pn)
	if firstByte != origFirst {
		t.Errorf("restored first byte mismatch: got 0x%02x, want 0x%02x", firstByte, origFirst)
	}
	if !bytes.Equal(pn, origPN) {
		t.Errorf("restored PN mismatch: got %x, want %x", pn, origPN)
	}
}
