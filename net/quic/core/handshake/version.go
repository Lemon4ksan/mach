// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import "fmt"

// Version is the QUIC version number as defined in RFC 9000 and RFC 9369.
type Version uint32

const (
	// Version1 is QUIC version 1 (RFC 9000).
	Version1 Version = 0x00000001
	// Version2 is QUIC version 2 (RFC 9369).
	Version2 Version = 0x6b3343cf
	// VersionDraft29 is draft-ietf-quic-transport-29.
	VersionDraft29 Version = 0xff00001d
)

func (v Version) String() string {
	switch v {
	case Version1:
		return "v1"
	case Version2:
		return "v2"
	case VersionDraft29:
		return "draft-29"
	default:
		return fmt.Sprintf("0x%08x", uint32(v))
	}
}

// Perspective represents client or server role in QUIC.
type Perspective uint8

const (
	PerspectiveClient Perspective = 1
	PerspectiveServer Perspective = 2
)

func (p Perspective) String() string {
	switch p {
	case PerspectiveClient:
		return "client"
	case PerspectiveServer:
		return "server"
	default:
		return "unknown"
	}
}

// Opposite returns the peer's perspective.
func (p Perspective) Opposite() Perspective {
	if p == PerspectiveClient {
		return PerspectiveServer
	}
	return PerspectiveClient
}
