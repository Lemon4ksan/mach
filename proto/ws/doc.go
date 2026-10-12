// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ws implements the RFC 6455 WebSocket wire protocol, vectorized payload masking,
// and HTTP/1.1 opening handshake negotiation.
//
// # Architecture
//
// The package provides wire framing, streaming message parsing, and full-duplex session handling:
//
//   - Connection Engine ([Conn]):
//     Manages bidirectional message streaming ([Conn.ReadMessage], [Conn.WriteMessage], [Conn.NextReader],
//     [Conn.NextWriter]) over raw network connections or protocol streams. Handles interleaved control
//     frames (Ping, Pong, Close) transparently without disturbing fragmented data streams.
//   - Wire Framing ([Header], [ReadHeader], [WriteHeader]):
//     Serializes and decodes standard 2-to-14 byte RFC 6455 frame headers with zero allocations,
//     enforcing payload length encoding rules (7-bit, 16-bit, and 64-bit variants).
//   - Vectorized Masking ([MaskPayload]):
//     Accelerates client-to-server payload masking with 64-bit SWAR (SIMD Within A Register) XOR
//     vector kernels, processing payloads at wire speed (39+ GB/s) while falling back to scalar byte
//     loops for unaligned boundaries.
//   - Streaming Frame Parser ([Parser]):
//     Decodes segmented wire inputs without intermediate buffer allocations, validating RSV bit
//     invariants, UTF-8 text compliance, and fragmented message sequences.
//   - Handshake Negotiation ([ValidateClientHandshake], [BuildServerHandshakeResponse]):
//     Validates HTTP/1.1 upgrade headers, computes Sec-WebSocket-Accept keys via SHA-1 hashes (RFC 6455 §1.3),
//     and negotiates requested subprotocols.
//   - Permessage-Deflate Compression:
//     Implements RFC 7692 permessage-deflate extension framing with sliding window renegotiation
//     and context takeover controls.
//
// # Failure Boundaries & Protocol Closures
//
// Violations of RFC 6455 framing invariants—such as unmasked client frames, control frames exceeding
// 125 octets, non-zero RSV bits without negotiated extensions, or invalid UTF-8 payload in text frames—result
// in protocol closure frames emitting typed status codes ([StatusProtocolError], [StatusInvalidFramePayloadData]).
//
// # RFC Specifications
//
//   - RFC 6455: The WebSocket Protocol
//   - RFC 7692: Compression Extensions for WebSocket (permessage-deflate)
//   - RFC 8441: Bootstrapping WebSockets with HTTP/2 (Extended CONNECT)
//   - RFC 9220: Bootstrapping WebSockets with HTTP/3 (Extended CONNECT)
//
// # Compared to the standard library
//
// Stdlib counterpart: none - Go standard library provides no RFC 6455 WebSocket implementation
//
// Popular third-party alternatives incur multiple allocations per frame, copy payloads during masking,
// and lack native integration with HTTP/2 or HTTP/3 multiplexed streams. This package provides zero-allocation
// streaming framing, line-rate SWAR masking, and seamless multi-protocol Extended CONNECT bridging.
//
// Allocations: 0 allocs/op during active streaming and masking.
package ws
