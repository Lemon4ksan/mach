// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
)

const (
	// HeaderProtectionSampleLen is the required sample length in bytes (RFC 9001 §5.4.2).
	HeaderProtectionSampleLen = 16

	// HeaderProtectionMaskLen is the length of the mask applied to header and packet number.
	HeaderProtectionMaskLen = 5
)

// HeaderProtector computes header protection masks (RFC 9001 §5.4).
type HeaderProtector interface {
	// Mask computes a 5-byte mask from the 16-byte sample without heap allocations.
	Mask(sample []byte) [5]byte
}

// AESHeaderProtector implements AES-ECB header protection for AES-128 and AES-256 (RFC 9001 §5.4.3).
type AESHeaderProtector struct {
	block   cipher.Block
	maskBuf [16]byte
}

// NewAESHeaderProtector creates an AESHeaderProtector from a 16-byte or 32-byte header protection key.
func NewAESHeaderProtector(hpKey []byte) (*AESHeaderProtector, error) {
	block, err := aes.NewCipher(hpKey)
	if err != nil {
		return nil, err
	}
	return &AESHeaderProtector{block: block}, nil
}

// Mask encrypts the 16-byte sample with AES-ECB and outputs the first 5 bytes.
func (hp *AESHeaderProtector) Mask(sample []byte) [5]byte {
	hp.block.Encrypt(hp.maskBuf[:], sample[:16])
	var mask [5]byte
	copy(mask[:], hp.maskBuf[:5])
	return mask
}

// ChaChaHeaderProtector implements ChaCha20 header protection (RFC 9001 §5.4.4).
type ChaChaHeaderProtector struct {
	key [8]uint32
}

// NewChaChaHeaderProtector creates a ChaChaHeaderProtector from a 32-byte key.
func NewChaChaHeaderProtector(hpKey []byte) (*ChaChaHeaderProtector, error) {
	if len(hpKey) != 32 {
		return nil, errors.New("wire: ChaCha20 header protection key must be 32 bytes")
	}
	hp := &ChaChaHeaderProtector{}
	for i := 0; i < 8; i++ {
		hp.key[i] = binary.LittleEndian.Uint32(hpKey[i*4 : (i+1)*4])
	}
	return hp, nil
}

// Mask runs the ChaCha20 block function with the sample as counter and nonce,
// and extracts the first 5 bytes of the keystream. Zero heap allocations.
func (hp *ChaChaHeaderProtector) Mask(sample []byte) [5]byte {
	var state [16]uint32
	// Constants "expand 32-byte k"
	state[0] = 0x61707865
	state[1] = 0x3320646e
	state[2] = 0x79622d32
	state[3] = 0x6b206574

	// Key
	for i := 0; i < 8; i++ {
		state[4+i] = hp.key[i]
	}

	// Counter (first 4 bytes of sample, little endian)
	state[12] = binary.LittleEndian.Uint32(sample[0:4])

	// Nonce (remaining 12 bytes of sample, little endian)
	state[13] = binary.LittleEndian.Uint32(sample[4:8])
	state[14] = binary.LittleEndian.Uint32(sample[8:12])
	state[15] = binary.LittleEndian.Uint32(sample[12:16])

	orig := state

	// 10 double rounds (20 rounds total)
	for i := 0; i < 10; i++ {
		// Column rounds
		quarterRound(&state[0], &state[4], &state[8], &state[12])
		quarterRound(&state[1], &state[5], &state[9], &state[13])
		quarterRound(&state[2], &state[6], &state[10], &state[14])
		quarterRound(&state[3], &state[7], &state[11], &state[15])
		// Diagonal rounds
		quarterRound(&state[0], &state[5], &state[10], &state[15])
		quarterRound(&state[1], &state[6], &state[11], &state[12])
		quarterRound(&state[2], &state[7], &state[8], &state[13])
		quarterRound(&state[3], &state[4], &state[9], &state[14])
	}

	for i := 0; i < 16; i++ {
		state[i] += orig[i]
	}

	// First 5 bytes of output
	var mask [5]byte
	binary.LittleEndian.PutUint32(mask[0:4], state[0])
	mask[4] = byte(state[1])
	return mask
}

func quarterRound(a, b, c, d *uint32) {
	*a += *b
	*d ^= *a
	*d = (*d << 16) | (*d >> 16)
	*c += *d
	*b ^= *c
	*b = (*b << 12) | (*b >> 20)
	*a += *b
	*d ^= *a
	*d = (*d << 8) | (*d >> 24)
	*c += *d
	*b ^= *c
	*b = (*b << 7) | (*b >> 25)
}

// ApplyHeaderProtection applies header protection in-place to packet (RFC 9001 §5.4.1).
// pnOffset is the byte offset of the Packet Number field.
// pnLen is the length of the packet number (1 to 4 bytes).
func ApplyHeaderProtection(packet []byte, pnOffset int, pnLen int, hp HeaderProtector) error {
	sampleStart := pnOffset + 4
	if len(packet) < sampleStart+HeaderProtectionSampleLen {
		return ErrSampleOutOfBounds
	}
	if pnLen < 1 || pnLen > 4 {
		return ErrInvalidPacketNumber
	}

	mask := hp.Mask(packet[sampleStart : sampleStart+HeaderProtectionSampleLen])

	isLong := (packet[0] & 0x80) != 0
	if isLong {
		packet[0] ^= mask[0] & 0x0f
	} else {
		packet[0] ^= mask[0] & 0x1f
	}

	for i := 0; i < pnLen; i++ {
		packet[pnOffset+i] ^= mask[1+i]
	}
	return nil
}

// RemoveHeaderProtection removes header protection in-place from packet (RFC 9001 §5.4.1).
// pnOffset is the byte offset of the Packet Number field.
// It returns the decoded packet number length (1 to 4 bytes).
func RemoveHeaderProtection(packet []byte, pnOffset int, hp HeaderProtector) (int, error) {
	sampleStart := pnOffset + 4
	if len(packet) < sampleStart+HeaderProtectionSampleLen {
		return 0, ErrSampleOutOfBounds
	}

	mask := hp.Mask(packet[sampleStart : sampleStart+HeaderProtectionSampleLen])

	isLong := (packet[0] & 0x80) != 0
	if isLong {
		packet[0] ^= mask[0] & 0x0f
	} else {
		packet[0] ^= mask[0] & 0x1f
	}

	pnLen := int(packet[0]&0x03) + 1
	if len(packet) < pnOffset+pnLen {
		return 0, ErrTruncatedPacket
	}

	for i := 0; i < pnLen; i++ {
		packet[pnOffset+i] ^= mask[1+i]
	}
	return pnLen, nil
}
