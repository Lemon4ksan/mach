// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"

	"golang.org/x/crypto/chacha20"
)

// Salt values for Initial secret derivation.
var (
	// SaltV1 is the salt defined in RFC 9001 Section 5.2 for QUIC version 1.
	SaltV1 = []byte{
		0x38, 0x76, 0x2c, 0xf7, 0xf5, 0x59, 0x34, 0xb3,
		0x4d, 0x17, 0x9a, 0xe6, 0xa4, 0xc8, 0x0c, 0xad,
		0xcc, 0xbb, 0x7f, 0x0a,
	}

	// SaltV2 is the version 2 salt (0x0d3e709f25fa60728d85121cd6010f74f34f6a0d).
	SaltV2 = []byte{
		0x0d, 0x3e, 0x70, 0x9f, 0x25, 0xfa, 0x60, 0x72,
		0x8d, 0x85, 0x12, 0x1c, 0xd6, 0x01, 0x0f, 0x74,
		0xf3, 0x4f, 0x6a, 0x0d,
	}

	// SaltV2RFC9369 is the version 2 salt as published in RFC 9369 Section 5.2.
	SaltV2RFC9369 = []byte{
		0x0d, 0xed, 0xe3, 0xde, 0xf7, 0x00, 0xa6, 0xdb,
		0x81, 0x93, 0x81, 0xbe, 0x6e, 0x26, 0x9d, 0xcb,
		0xf9, 0xbd, 0x2e, 0xd9,
	}
)

const (
	labelClientIn = "client in"
	labelServerIn = "server in"

	labelKeyV1 = "quic key"
	labelIVV1  = "quic iv"
	labelHPV1  = "quic hp"

	labelKeyV2 = "quicv2 key"
	labelIVV2  = "quicv2 iv"
	labelHPV2  = "quicv2 hp"
)

// HeaderProtector encrypts and decrypts QUIC packet header protection.
type HeaderProtector interface {
	EncryptHeader(sample []byte, firstByte *byte, pnBytes []byte)
	DecryptHeader(sample []byte, firstByte *byte, pnBytes []byte)
}

// InitialKeys contains all derived keys and secrets for QUIC Initial packets.
type InitialKeys struct {
	ClientSecret []byte
	ServerSecret []byte

	ClientKey   []byte
	ClientIV    []byte
	ClientHPKey []byte

	ServerKey   []byte
	ServerIV    []byte
	ServerHPKey []byte
}

// DeriveInitialSecrets derives client and server initial secrets from destConnID using the default salt for v.
func DeriveInitialSecrets(destConnID []byte, v Version) (clientSecret, serverSecret []byte) {
	salt := SaltV1
	if v == Version2 {
		salt = SaltV2
	}
	return DeriveInitialSecretsWithSalt(destConnID, salt, v)
}

// DeriveInitialSecretsWithSalt derives client and server initial secrets using a specified salt.
func DeriveInitialSecretsWithSalt(destConnID []byte, salt []byte, v Version) (clientSecret, serverSecret []byte) {
	initialSecret := HKDFExtract(crypto.SHA256, destConnID, salt)
	clientSecret = HKDFExpandLabel(crypto.SHA256, initialSecret, nil, labelClientIn, crypto.SHA256.Size())
	serverSecret = HKDFExpandLabel(crypto.SHA256, initialSecret, nil, labelServerIn, crypto.SHA256.Size())
	return clientSecret, serverSecret
}

// DeriveInitialPacketKeys derives the AEAD packet protection key (16 bytes for AES-128) and IV (12 bytes).
func DeriveInitialPacketKeys(secret []byte, v Version) (key, iv []byte) {
	keyLabel := labelKeyV1
	ivLabel := labelIVV1
	if v == Version2 {
		keyLabel = labelKeyV2
		ivLabel = labelIVV2
	}
	key = HKDFExpandLabel(crypto.SHA256, secret, nil, keyLabel, 16)
	iv = HKDFExpandLabel(crypto.SHA256, secret, nil, ivLabel, 12)
	return key, iv
}

// DeriveInitialHeaderProtectionKey derives the header protection key (16 bytes for AES-128).
func DeriveInitialHeaderProtectionKey(secret []byte, v Version) []byte {
	hpLabel := labelHPV1
	if v == Version2 {
		hpLabel = labelHPV2
	}
	return HKDFExpandLabel(crypto.SHA256, secret, nil, hpLabel, 16)
}

// NewInitialKeys derives all Initial secrets and keys for both client and server from destConnID.
func NewInitialKeys(destConnID []byte, v Version) *InitialKeys {
	salt := SaltV1
	if v == Version2 {
		salt = SaltV2
	}
	return NewInitialKeysWithSalt(destConnID, salt, v)
}

// NewInitialKeysWithSalt derives all Initial secrets and keys using the specified salt.
func NewInitialKeysWithSalt(destConnID []byte, salt []byte, v Version) *InitialKeys {
	clientSecret, serverSecret := DeriveInitialSecretsWithSalt(destConnID, salt, v)

	clientKey, clientIV := DeriveInitialPacketKeys(clientSecret, v)
	clientHP := DeriveInitialHeaderProtectionKey(clientSecret, v)

	serverKey, serverIV := DeriveInitialPacketKeys(serverSecret, v)
	serverHP := DeriveInitialHeaderProtectionKey(serverSecret, v)

	return &InitialKeys{
		ClientSecret: clientSecret,
		ServerSecret: serverSecret,
		ClientKey:    clientKey,
		ClientIV:     clientIV,
		ClientHPKey:  clientHP,
		ServerKey:    serverKey,
		ServerIV:     serverIV,
		ServerHPKey:  serverHP,
	}
}

// aesHeaderProtector implements HeaderProtector for AES-128 / AES-256 (RFC 9001 Section 5.4.3).
type aesHeaderProtector struct {
	block        cipher.Block
	isLongHeader bool
}

// NewAESHeaderProtector creates an AES-based header protector.
func NewAESHeaderProtector(hpKey []byte, isLongHeader bool) (HeaderProtector, error) {
	block, err := aes.NewCipher(hpKey)
	if err != nil {
		return nil, fmt.Errorf("quic: failed to create AES cipher for header protection: %w", err)
	}
	return &aesHeaderProtector{
		block:        block,
		isLongHeader: isLongHeader,
	}, nil
}

func (p *aesHeaderProtector) apply(sample []byte, firstByte *byte, pnBytes []byte) {
	if len(sample) < 16 || firstByte == nil || len(pnBytes) == 0 {
		return
	}
	var mask [16]byte
	p.block.Encrypt(mask[:], sample[:16])

	var maskFirst byte
	if p.isLongHeader {
		maskFirst = mask[0] & 0x0f
	} else {
		maskFirst = mask[0] & 0x1f
	}
	*firstByte ^= maskFirst
	for i := 0; i < len(pnBytes) && i < 4; i++ {
		pnBytes[i] ^= mask[1+i]
	}
}

func (p *aesHeaderProtector) EncryptHeader(sample []byte, firstByte *byte, pnBytes []byte) {
	p.apply(sample, firstByte, pnBytes)
}

func (p *aesHeaderProtector) DecryptHeader(sample []byte, firstByte *byte, pnBytes []byte) {
	p.apply(sample, firstByte, pnBytes)
}

// chachaHeaderProtector implements HeaderProtector for ChaCha20 (RFC 9001 Section 5.4.3).
type chachaHeaderProtector struct {
	hpKey        []byte
	isLongHeader bool
}

// NewChaChaHeaderProtector creates a ChaCha20-based header protector.
func NewChaChaHeaderProtector(hpKey []byte, isLongHeader bool) (HeaderProtector, error) {
	if len(hpKey) != 32 {
		return nil, fmt.Errorf("quic: invalid ChaCha20 HP key length: %d (expected 32)", len(hpKey))
	}
	return &chachaHeaderProtector{
		hpKey:        hpKey,
		isLongHeader: isLongHeader,
	}, nil
}

func (p *chachaHeaderProtector) apply(sample []byte, firstByte *byte, pnBytes []byte) {
	if len(sample) < 16 || firstByte == nil || len(pnBytes) == 0 {
		return
	}
	counter := binary.LittleEndian.Uint32(sample[0:4])
	nonce := sample[4:16]

	c, err := chacha20.NewUnauthenticatedCipher(p.hpKey, nonce)
	if err != nil {
		return
	}
	c.SetCounter(counter)

	var mask [5]byte
	c.XORKeyStream(mask[:], mask[:])

	var maskFirst byte
	if p.isLongHeader {
		maskFirst = mask[0] & 0x0f
	} else {
		maskFirst = mask[0] & 0x1f
	}
	*firstByte ^= maskFirst
	for i := 0; i < len(pnBytes) && i < 4; i++ {
		pnBytes[i] ^= mask[1+i]
	}
}

func (p *chachaHeaderProtector) EncryptHeader(sample []byte, firstByte *byte, pnBytes []byte) {
	p.apply(sample, firstByte, pnBytes)
}

func (p *chachaHeaderProtector) DecryptHeader(sample []byte, firstByte *byte, pnBytes []byte) {
	p.apply(sample, firstByte, pnBytes)
}
