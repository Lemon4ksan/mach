// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package h3 implements the HTTP/3 application protocol layer, binary variable-length integer framing,
// unidirectional control stream state tracking, and QPACK field compression conforming to RFC 9114 and RFC 9204.
//
// # Architecture
//
// HTTP/3 maps HTTP semantics directly onto the multiplexed QUIC transport protocol (RFC 9000).
// By assigning each request-response transaction to an independent bidirectional QUIC stream,
// HTTP/3 eliminates transport-level head-of-line blocking, while control state and header compression
// metadata flow over dedicated unidirectional streams.
//
// The protocol engine is organized into distinct functional tiers:
//
//   - Frame Engine ([ReadFrameHeader], [AppendHeadersHeader], [AppendDataHeader]):
//     Decodes and serializes binary HTTP/3 variable-length integer frame headers (RFC 9114 §7.1)
//     directly over byte slices without heap escape.
//   - Control & Settings Negotiation ([Settings], [DecodeSettings]):
//     Encodes and validates unidirectional control stream parameters, rejecting prohibited
//     HTTP/2 settings (RFC 9114 §7.2.4 & Appendix A.3).
//   - QPACK Field Compression Engine ([QPACKCodec], [QPACKStreamError]):
//     Dual-pipeline field section encoder and decoder with deterministic error reporting,
//     ordered field prioritization, and progressive stream decoding (RFC 9204).
//   - Transport Stream Abstractions ([Stream], [ReceiveStream], [SendStream]):
//     Decouples L7 HTTP/3 wire parsing and serialization from underlying L4 QUIC transport
//     mechanics, enforcing strict layer separation.
//   - Error Hierarchy ([ErrorCode]):
//     Typed RFC 9114 §8.1 application error codes mapped onto QUIC stream resets and connection
//     teardown events.
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
// # Failure Boundaries & Error Codes
//
// Protocol violations—such as receipt of prohibited frames on control streams, malformed QPACK
// instructions, or stream state violations—terminate streams or connections using standard [ErrorCode]
// constants ([ErrCodeGeneralProtocolError], [ErrCodeFrameUnexpected], [ErrCodeExcessiveLoad]).
// Stream resets abort specific transactions without affecting concurrent transfers on the same QUIC connection.
//
// # RFC Specifications
//
//   - RFC 9114: HTTP/3 (Transport, Framing, and Message Semantics)
//   - RFC 9204: QPACK - Field Compression for HTTP/3
//   - RFC 9218: Extensible Prioritization Scheme for HTTP
//   - RFC 9220: Bootstrapping WebSockets with HTTP/3 (Extended CONNECT)
//   - RFC 9297: HTTP Datagrams and the Capsule Protocol (0x33 Parameter)
//   - RFC 9000: QUIC: A UDP-Based Multiplexed and Secure Transport
//
// # Compared to the standard library
//
// Stdlib counterpart: none - Go standard library provides no native HTTP/3 protocol engine
//
// While experimental packages rely on channel-heavy pipelines and reflection-based serialization,
// this package implements zero-allocation variable-length integer parsing, slab-pooled memory structures,
// and clean stream decoupling for high-throughput HTTP/3 servers and clients.
//
// Allocations: amortized zero on active streams.
package h3
