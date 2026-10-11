// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import "fmt"

// Version represents a QUIC version number (RFC 9000 §14).
type Version uint32

const (
	// VersionNegotiation is the version number reserved for Version Negotiation packets (RFC 9000 §17.2.1).
	VersionNegotiation Version = 0x00000000

	// Version1 is QUIC version 1 (RFC 9000).
	Version1 Version = 0x00000001

	// Version2 is QUIC version 2 (RFC 9369).
	Version2 Version = 0x709a50c4
)

// Initial salt values.
var (
	// initialSaltV1 is the salt for QUIC Version 1 (RFC 9001 §5.2).
	initialSaltV1 = [20]byte{
		0x38, 0x7b, 0x2c, 0x36, 0x05, 0x65, 0x89, 0x07, 0x36, 0x95,
		0xb0, 0xa3, 0xb9, 0x0f, 0x64, 0xdd, 0x39, 0xd2, 0xb7, 0x9e,
	}

	// initialSaltV2 is the salt for QUIC Version 2 (RFC 9369 §5.2).
	initialSaltV2 = [20]byte{
		0x0d, 0xed, 0xe3, 0xde, 0xf7, 0x00, 0xa6, 0xdb, 0x81, 0x93,
		0x81, 0xbe, 0x6e, 0x26, 0x9d, 0xcb, 0xf9, 0xbd, 0x2e, 0xd9,
	}
)

// IsSupportedVersion reports whether v is a recognized and supported QUIC version.
func IsSupportedVersion(v Version) bool {
	return v == Version1 || v == Version2
}

// InitialSalt returns the initial HKDF salt for version v.
func InitialSalt(v Version) []byte {
	switch v {
	case Version2:
		return initialSaltV2[:]
	default:
		return initialSaltV1[:]
	}
}

// String returns a human-readable representation of v.
func (v Version) String() string {
	switch v {
	case VersionNegotiation:
		return "Version Negotiation"
	case Version1:
		return "v1 (RFC 9000)"
	case Version2:
		return "v2 (RFC 9369)"
	default:
		return fmt.Sprintf("0x%08x", uint32(v))
	}
}
