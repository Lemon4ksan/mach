// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/tls"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// EncryptionLevel represents a QUIC encryption level (RFC 9001 Section 4).
type EncryptionLevel uint8

const (
	// EncryptionLevelInitial is for Initial packets.
	EncryptionLevelInitial EncryptionLevel = 0
	// EncryptionLevelEarly is for 0-RTT packets.
	EncryptionLevelEarly EncryptionLevel = 1
	// EncryptionLevelHandshake is for Handshake packets.
	EncryptionLevelHandshake EncryptionLevel = 2
	// EncryptionLevel1RTT is for 1-RTT (Application) packets.
	EncryptionLevel1RTT EncryptionLevel = 3
)

func (l EncryptionLevel) String() string {
	switch l {
	case EncryptionLevelInitial:
		return "Initial"
	case EncryptionLevelEarly:
		return "0-RTT"
	case EncryptionLevelHandshake:
		return "Handshake"
	case EncryptionLevel1RTT:
		return "1-RTT"
	default:
		return fmt.Sprintf("EncryptionLevel(%d)", uint8(l))
	}
}

// ToTLS converts EncryptionLevel to crypto/tls.QUICEncryptionLevel.
func (l EncryptionLevel) ToTLS() tls.QUICEncryptionLevel {
	switch l {
	case EncryptionLevelInitial:
		return tls.QUICEncryptionLevelInitial
	case EncryptionLevelEarly:
		return tls.QUICEncryptionLevelEarly
	case EncryptionLevelHandshake:
		return tls.QUICEncryptionLevelHandshake
	case EncryptionLevel1RTT:
		return tls.QUICEncryptionLevelApplication
	default:
		return tls.QUICEncryptionLevelInitial
	}
}

// FromTLSEncryptionLevel converts crypto/tls.QUICEncryptionLevel to EncryptionLevel.
func FromTLSEncryptionLevel(l tls.QUICEncryptionLevel) EncryptionLevel {
	switch l {
	case tls.QUICEncryptionLevelInitial:
		return EncryptionLevelInitial
	case tls.QUICEncryptionLevelEarly:
		return EncryptionLevelEarly
	case tls.QUICEncryptionLevelHandshake:
		return EncryptionLevelHandshake
	case tls.QUICEncryptionLevelApplication:
		return EncryptionLevel1RTT
	default:
		return EncryptionLevelInitial
	}
}

// TrafficKeys stores keying material for a QUIC encryption level.
type TrafficKeys struct {
	Suite  uint16
	Secret []byte
	Key    []byte
	IV     []byte
	HPKey  []byte
}

// DeriveTrafficKeys derives packet protection keys from a traffic secret and TLS cipher suite ID.
func DeriveTrafficKeys(secret []byte, suite uint16, v Version) (*TrafficKeys, error) {
	var (
		hash     crypto.Hash
		keyLen   int
		hpKeyLen int
	)

	switch suite {
	case tls.TLS_AES_128_GCM_SHA256:
		hash = crypto.SHA256
		keyLen = 16
		hpKeyLen = 16
	case tls.TLS_AES_256_GCM_SHA384:
		hash = crypto.SHA384
		keyLen = 32
		hpKeyLen = 32
	case tls.TLS_CHACHA20_POLY1305_SHA256:
		hash = crypto.SHA256
		keyLen = 32
		hpKeyLen = 32
	default:
		return nil, fmt.Errorf("quic: unsupported cipher suite: 0x%04x", suite)
	}

	keyLabel := labelKeyV1
	ivLabel := labelIVV1

	hpLabel := labelHPV1
	if v == Version2 {
		keyLabel = labelKeyV2
		ivLabel = labelIVV2
		hpLabel = labelHPV2
	}

	key := HKDFExpandLabel(hash, secret, nil, keyLabel, keyLen)
	iv := HKDFExpandLabel(hash, secret, nil, ivLabel, 12)
	hpKey := HKDFExpandLabel(hash, secret, nil, hpLabel, hpKeyLen)

	return &TrafficKeys{
		Suite:  suite,
		Secret: secret,
		Key:    key,
		IV:     iv,
		HPKey:  hpKey,
	}, nil
}

// CreateAEAD creates a cipher.AEAD instance from packet protection keys and cipher suite ID.
func CreateAEAD(key []byte, suite uint16) (cipher.AEAD, error) {
	switch suite {
	case tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384:
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("quic: failed to create AES block cipher: %w", err)
		}

		return cipher.NewGCM(block)

	case tls.TLS_CHACHA20_POLY1305_SHA256:
		return chacha20poly1305.New(key)
	default:
		return nil, fmt.Errorf("quic: unsupported cipher suite: 0x%04x", suite)
	}
}

// CreateHeaderProtector creates a HeaderProtector from header protection key and cipher suite ID.
func CreateHeaderProtector(hpKey []byte, suite uint16, isLongHeader bool) (HeaderProtector, error) {
	switch suite {
	case tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384:
		return NewAESHeaderProtector(hpKey, isLongHeader)
	case tls.TLS_CHACHA20_POLY1305_SHA256:
		return NewChaChaHeaderProtector(hpKey, isLongHeader)
	default:
		return nil, fmt.Errorf("quic: unsupported cipher suite: 0x%04x", suite)
	}
}
