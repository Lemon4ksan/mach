// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mach

import (
	"context"
	"io"
)

// Engine defines the universal contract for protocol engines (H1/H2/H3/QUIC).
// It acts as a factory for protocol connections.
type Engine interface {
	// Dial establishes a connection or yields a protocol-specific multiplexer.
	Dial(ctx context.Context, addr string) (Conn, error)
}

// Conn represents an active protocol connection.
// For multiplexed protocols (H2, H3), it manages multiple Streams.
// For H1, it acts as a single-stream carrier.
type Conn interface {
	io.Closer

	// OpenStream creates a new request-response stream (Zero-alloc where possible).
	OpenStream(ctx context.Context) (Stream, error)
}

// Stream represents an isolated request/response lifecycle.
// It bypasses standard io.Reader/io.Writer allocations by using Zero-copy methods
// or integrating directly with the foundation/bufkit arenas.
type Stream interface {
	io.Reader
	io.Writer
	io.Closer

	// ReadFrame parses the next protocol-specific frame without heap allocations.
	ReadFrame() (Frame, error)
}

// Frame represents a Data-In-Motion abstraction of a protocol frame (H2/H3/QUIC).
type Frame interface {
	// Type returns the protocol-specific frame type identifier.
	Type() uint8
	// Payload yields the zero-copy slice of the frame body.
	Payload() []byte
}
