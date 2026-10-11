// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package h3 provides zero-allocation HTTP/3 binary frame parsing, frame encoding,
// connection settings negotiation, and QPACK field section compression for Go.
//
// # Architecture
//
// HTTP/3 maps HTTP semantics directly onto the multiplexed QUIC transport protocol (RFC 9000).
// Unlike HTTP/1.x (byte-stream) and HTTP/2 (single-stream TCP multiplexing), HTTP/3 eliminates
// transport-level head-of-line blocking by assigning each request-response exchange to an
// independent bidirectional QUIC stream, while control state and header compression metadata
// flow over dedicated unidirectional streams.
//
// The package is structured into four primary architectural tiers:
//
//   - Frame Engine ([ReadFrameHeader], [AppendHeadersHeader], [AppendDataHeader]):
//     Decodes and serializes binary HTTP/3 variable-length integer frame headers (RFC 9114 §7.1).
//   - Control & Settings Negotiation ([Settings], [DecodeSettings]):
//     Encodes and validates unidirectional control stream parameters, rejecting prohibited
//     HTTP/2 settings (RFC 9114 §7.2.4 & Appendix A.3).
//   - QPACK Field Compression Engine ([QPACKCodec], [QPACKStreamError]):
//     Dual-pipeline field section encoder and decoder with deterministic error reporting,
//     ordered field prioritization, and progressive stream decoding (RFC 9204).
//   - Error Hierarchy ([ErrorCode]):
//     Strict RFC 9114 §8.1 application error codes transmitted across QUIC stream resets
//     and connection closure frames.
//
// # Transport Model
//
// HTTP/3 utilizes multiple QUIC stream types with distinct lifecycle characteristics:
//
//   - Control Stream (0x00, RFC 9114 §6.2.1):
//     Unidirectional stream opened once per endpoint to exchange initial SETTINGS and GOAWAY frames.
//   - Push Stream (0x01, RFC 9114 §6.2.2):
//     Server-initiated unidirectional stream delivering promised server push responses.
//   - QPACK Encoder Stream (0x02, RFC 9204 §4.2):
//     Unidirectional stream emitting table modifications from encoder to peer decoder.
//   - QPACK Decoder Stream (0x03, RFC 9204 §4.2):
//     Unidirectional stream emitting dynamic table acknowledgments to peer encoder.
//   - Request Streams (Bidirectional, RFC 9114 §6.1):
//     Client-initiated streams conveying HEADERS and DATA frames for requests and responses.
//
// # RFC Specifications
//
// This package strictly conforms to the following standards:
//
//   - RFC 9114: HTTP/3 (Transport, Framing, and Message Semantics)
//   - RFC 9204: QPACK - Field Compression for HTTP/3
//   - RFC 9218: Extensible Prioritization Scheme for HTTP
//   - RFC 9220: Bootstrapping WebSockets with HTTP/3 (Extended CONNECT)
//   - RFC 9297: HTTP Datagrams and the Capsule Protocol (0x33 Parameter)
//
// # Compared to the standard library
//
// Stdlib counterpart: none - fills gap: HTTP/3 protocol engine
//
// Rejected compromise: Reflection-based serialization and channel allocations.
//
// Accepted cost: Callers manage byte buffers and wire QUIC stream listeners directly.
//
// Allocations: amortized zero
package h3
