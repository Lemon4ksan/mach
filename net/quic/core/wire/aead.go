// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// CipherSuite identifies a QUIC AEAD cipher suite (RFC 9001 §5.3).
type CipherSuite uint16

const (
	// TLS_AES_128_GCM_SHA256 represents AES-128-GCM.
	TLS_AES_128_GCM_SHA256 CipherSuite = 0x1301
	// TLS_AES_256_GCM_SHA384 represents AES-256-GCM.
	TLS_AES_256_GCM_SHA384 CipherSuite = 0x1302
	// TLS_CHACHA20_POLY1305_SHA256 represents ChaCha20-Poly1305.
	TLS_CHACHA20_POLY1305_SHA256 CipherSuite = 0x1303
)

// String returns the name of the cipher suite.
func (cs CipherSuite) String() string {
	switch cs {
	case TLS_AES_128_GCM_SHA256:
		return "TLS_AES_128_GCM_SHA256"
	case TLS_AES_256_GCM_SHA384:
		return "TLS_AES_256_GCM_SHA384"
	case TLS_CHACHA20_POLY1305_SHA256:
		return "TLS_CHACHA20_POLY1305_SHA256"
	default:
		return fmt.Sprintf("CipherSuite(0x%04x)", uint16(cs))
	}
}

// ComputeNonce computes the 12-byte AEAD nonce for a given packet number and IV (RFC 9001 §5.3).
func ComputeNonce(iv []byte, pn uint64, nonce *[12]byte) {
	copy(nonce[:], iv[:12])
	var pnBytes [8]byte
	binary.BigEndian.PutUint64(pnBytes[:], pn)
	for i := 0; i < 8; i++ {
		nonce[4+i] ^= pnBytes[i]
	}
}

// PacketProtector encrypts packet payloads and applies header protection without allocations.
type PacketProtector struct {
	suite CipherSuite
	aead  cipher.AEAD
	iv    [12]byte
	nonce [12]byte
	hp    HeaderProtector
}

// NewPacketProtector creates a PacketProtector for the given cipher suite, key, IV, and header protection key.
func NewPacketProtector(suite CipherSuite, key, iv, hpKey []byte) (*PacketProtector, error) {
	if len(iv) < 12 {
		return nil, errors.New("wire: IV must be at least 12 bytes")
	}

	var aead cipher.AEAD
	var hp HeaderProtector
	var err error

	switch suite {
	case TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384:
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		aead, err = cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		hp, err = NewAESHeaderProtector(hpKey)
		if err != nil {
			return nil, err
		}

	case TLS_CHACHA20_POLY1305_SHA256:
		aead, err = chacha20poly1305.New(key)
		if err != nil {
			return nil, err
		}
		hp, err = NewChaChaHeaderProtector(hpKey)
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("wire: unsupported cipher suite: %s", suite)
	}

	p := &PacketProtector{
		suite: suite,
		aead:  aead,
		hp:    hp,
	}
	copy(p.iv[:], iv[:12])
	return p, nil
}

// SealPacket encodes the header, encrypts the payload, and applies header protection into dst.
// The resulting fully protected packet is appended to dst. Zero allocations on hot path.
func (p *PacketProtector) SealPacket(dst []byte, h *Header, payload []byte) ([]byte, error) {
	startLen := len(dst)

	// Update length for Long Headers if not already set:
	// length = packetNumberLen + len(payload) + aead.Overhead()
	if h.IsLong && h.Type != PacketTypeRetry {
		h.Length = uint64(h.PacketNumberLen + len(payload) + p.aead.Overhead())
	}

	var pnOffset int
	var err error
	dst, pnOffset, err = AppendHeader(dst, h)
	if err != nil {
		return nil, err
	}
	aad := dst[startLen:]

	ComputeNonce(p.iv[:], h.PacketNumber, &p.nonce)

	// Encrypt payload directly into dst
	dst = p.aead.Seal(dst, p.nonce[:], payload, aad)

	// Apply header protection
	if err := ApplyHeaderProtection(dst[startLen:], pnOffset-startLen, h.PacketNumberLen, p.hp); err != nil {
		return nil, err
	}

	return dst, nil
}

// PacketOpener unprotects packet headers and decrypts payloads without allocations.
type PacketOpener struct {
	suite CipherSuite
	aead  cipher.AEAD
	iv    [12]byte
	nonce [12]byte
	hp    HeaderProtector
}

// NewPacketOpener creates a PacketOpener for the given cipher suite, key, IV, and header protection key.
func NewPacketOpener(suite CipherSuite, key, iv, hpKey []byte) (*PacketOpener, error) {
	if len(iv) < 12 {
		return nil, errors.New("wire: IV must be at least 12 bytes")
	}

	var aead cipher.AEAD
	var hp HeaderProtector
	var err error

	switch suite {
	case TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384:
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		aead, err = cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		hp, err = NewAESHeaderProtector(hpKey)
		if err != nil {
			return nil, err
		}

	case TLS_CHACHA20_POLY1305_SHA256:
		aead, err = chacha20poly1305.New(key)
		if err != nil {
			return nil, err
		}
		hp, err = NewChaChaHeaderProtector(hpKey)
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("wire: unsupported cipher suite: %s", suite)
	}

	o := &PacketOpener{
		suite: suite,
		aead:  aead,
		hp:    hp,
	}
	copy(o.iv[:], iv[:12])
	return o, nil
}

// OpenPacket removes header protection from packet in-place, decodes the header,
// and decrypts the payload into dst. Zero heap allocations.
func (o *PacketOpener) OpenPacket(dst []byte, packet []byte, destConnIDLen int, largestAcked uint64) (Header, []byte, error) {
	var h Header

	pnOffset, _, _, _, _, _, err := ParseHeaderOffsets(packet, destConnIDLen)
	if err != nil {
		return h, nil, err
	}

	pnLen, err := RemoveHeaderProtection(packet, pnOffset, o.hp)
	if err != nil {
		return h, nil, err
	}

	h, err = ParseHeader(packet, destConnIDLen, largestAcked)
	if err != nil {
		return h, nil, err
	}
	h.PacketNumberLen = pnLen

	ComputeNonce(o.iv[:], h.PacketNumber, &o.nonce)

	aad := packet[:h.HeaderLen]
	ciphertext := packet[h.HeaderLen : h.HeaderLen+h.PayloadLen]

	decrypted, err := o.aead.Open(dst, o.nonce[:], ciphertext, aad)
	if err != nil {
		return h, nil, ErrDecryptionFailed
	}

	return h, decrypted, nil
}
