// Copyright (c) 2016 the quic-go authors. All rights reserved.
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package quic provides an RFC 9000 compliant QUIC transport protocol implementation.
//
// Derived from quic-go, this package strips high-level abstractions to focus exclusively on
// line-rate packet processing. The caller must provide an initialized network socket
// (usually a [net.PacketConn]) and handle their own TLS configuration. Steady-state packet
// reading and frame processing are heavily optimized to run without heap allocations.
// Connection state, multiplexing, and stream control are fully managed by the engine.
//
// # Compared to the standard library
//
// Stdlib counterpart: none - fills gap: QUIC transport protocol
//
// Rejected compromise: none (the standard library does not implement QUIC, but this package avoids standard io.Reader interfaces on the hot packet path in favor of zero-allocation frame iterators).
//
// Accepted cost: callers must manually wire the underlying UDP sockets and TLS handshakes.
//
// Allocations: amortized
package quic
