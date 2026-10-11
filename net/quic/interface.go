// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quic

import (
	"crypto/tls"
	"iter"
	"net"

	"github.com/lemon4ksan/mach/net/quic/core/handshake"
	"github.com/lemon4ksan/mach/net/quic/core/stream"
	"github.com/lemon4ksan/mach/net/quic/core/wire"
)

type (
	// StreamID is the 62-bit QUIC stream identifier.
	StreamID = stream.StreamID

	// Version is a QUIC version number.
	Version = wire.Version

	// ConnectionID is a stack-allocated zero-heap QUIC connection ID.
	ConnectionID = handshake.ConnectionID
)

const (
	// Version1 is RFC 9000.
	Version1 = wire.Version1
	// Version2 is RFC 9369.
	Version2 = wire.Version2
)

// SupportedVersions returns the supported QUIC versions.
func SupportedVersions() []Version {
	return []Version{Version1, Version2}
}

// SupportedVersionsSeq returns an iterator over supported QUIC versions with 0 heap allocations.
func SupportedVersionsSeq() iter.Seq[Version] {
	return func(yield func(Version) bool) {
		if !yield(Version1) {
			return
		}
		yield(Version2)
	}
}

// ConnectionIDFromBytes converts a byte slice into a ConnectionID.
func ConnectionIDFromBytes(b []byte) ConnectionID {
	return handshake.NewConnectionID(b)
}

// ClientInfo contains metadata about an incoming client connection.
type ClientInfo struct {
	RemoteAddr   net.Addr
	AddrVerified bool
}

// ConnectionState records active QUIC connection parameters.
type ConnectionState struct {
	TLS                                tls.ConnectionState
	SupportsDatagrams                  struct{ Remote, Local bool }
	SupportsStreamResetPartialDelivery struct{ Remote, Local bool }
	Used0RTT                           bool
	Version                            Version
	GSO                                bool
}
