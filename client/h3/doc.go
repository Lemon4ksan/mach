// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package h3 implements the HTTP/3 client protocol engine conforming to RFC 9114,
// featuring QUIC stream multiplexing, QPACK field compression, and asymmetric isolation.
//
// # Architecture
//
// The client engine establishes and coordinates HTTP/3 sessions over QUIC connections (RFC 9000):
//
//   - Engine Pipeline ([Engine]):
//     Implements [mach.Engine], dialing QUIC endpoints, opening unidirectional control streams,
//     and negotiating initial SETTINGS.
//   - Client Connection ([ClientConn]):
//     Coordinates concurrent bidirectional request streams alongside dedicated QPACK encoder
//     and decoder streams. Decodes response headers via QPACK and routes streaming body chunks.
//   - Asymmetric Isolation:
//     Validates incoming HTTP/3 frames and QPACK instructions at the wire boundary, terminating
//     individual streams upon error without disrupting other concurrent transactions on the QUIC session.
//
// # RFC Specifications
//
//   - RFC 9110: HTTP Semantics
//   - RFC 9114: HTTP/3 (QUIC-Based Semantics)
//   - RFC 9204: QPACK - Field Compression for HTTP/3
//   - RFC 9220: Bootstrapping WebSockets with HTTP/3 (Extended CONNECT)
//   - RFC 9000: QUIC: A UDP-Based Multiplexed and Secure Transport
//
// # Compared to the standard library
//
// Stdlib counterpart: none - Go standard library provides no native HTTP/3 client
//
// This package delivers a clean HTTP/3 client transport with zero-allocation frame encoding,
// deterministic QPACK field compression, and native QUIC stream lifecycle management.
//
// Allocations: amortized zero on active streams.
package h3
