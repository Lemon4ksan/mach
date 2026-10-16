// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"io"
)

// Stream is the transport abstraction representing a bidirectional QUIC stream (RFC 9000 §2.1, RFC 9114 §6.1).
//
// In HTTP/3, client-initiated bidirectional streams are used exclusively for request/response
// exchanges carrying HEADERS and DATA frames. This interface enforces L4/L7 separation by allowing
// the L7 wire codec to consume streams without depending on concrete L4 transport structures.
type Stream interface {
	io.Reader
	io.Writer
	io.Closer
}

// ReceiveStream is the transport abstraction representing an inbound unidirectional QUIC stream (RFC 9000 §2.1, RFC 9114 §6.2).
//
// HTTP/3 unidirectional streams carry critical connection state: Control Stream (0x00),
// Push Stream (0x01), QPACK Encoder Stream (0x02), and QPACK Decoder Stream (0x03).
type ReceiveStream interface {
	io.Reader
}

// SendStream is the transport abstraction representing an outbound unidirectional QUIC stream (RFC 9000 §2.1, RFC 9114 §6.2).
//
// Endpoints open unidirectional streams to transmit local control settings and QPACK table
// instructions to the remote peer.
type SendStream interface {
	io.Writer
	io.Closer
}
