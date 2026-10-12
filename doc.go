// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mach defines foundational Layer 7 protocol engines, wire codecs, and stream multiplexing abstractions for Go.
//
// Root contracts unify transport mechanics across HTTP/1.1, HTTP/2, and HTTP/3 over QUIC,
// establishing protocol-agnostic interfaces for connection lifecycles, zero-allocation framing,
// and request dispatching.
//
// # Unified Protocol Model
//
// Protocol engines expose a cohesive hierarchy decoupling network I/O from protocol state machines:
//
//   - [Engine]: Protocol connection factory establishing outbound transports or yielding protocol-specific multiplexers.
//   - [Conn]: Active transport carrier managing one or more concurrent request-response streams.
//   - [Stream]: Isolated data-in-motion stream supporting zero-copy frame ingestion ([Stream.ReadFrame]) and standard streaming I/O.
//   - [Frame]: Protocol-agnostic representation of serialized wire frames ([Frame.Type], [Frame.Payload]), encompassing standard control, header, and data primitives.
//   - [Handler] and [HandlerFunc]: Universal request processing abstraction serving streams across HTTP generations without transport coupling.
//
// # Architectural Invariants
//
//   - Zero-Allocation Steady State: Wire framing, header parsing, payload masking, and stream demultiplexing execute without heap allocations on active streams.
//   - 64-Bit SWAR Acceleration: Vectorized bit-manipulation kernels scan header delimiters and apply WebSocket masking masks at memory bandwidth limits.
//   - Asymmetric Isolation: Protocol parsers reject malformed ingress frames at the wire boundary, safeguarding application handlers from malformed traffic and network congestion.
//   - Sans-I/O State Machines: Event-driven parsing pipelines decouple protocol transitions from thread scheduling and blocking socket operations.
//
// # Supported Protocols & Standards
//
//   - RFC 9110: HTTP Semantics
//   - RFC 9112: HTTP/1.1 (Chunked Transfer, Pipelining)
//   - RFC 9113: HTTP/2 (Binary Framing, Stream Multiplexing, Flow Control)
//   - RFC 9114: HTTP/3 (QUIC-Mapped Transport, Control Streams)
//   - RFC 7541: HPACK - Header Compression for HTTP/2
//   - RFC 9204: QPACK - Field Compression for HTTP/3
//   - RFC 6455: The WebSocket Protocol
//   - RFC 8441: Bootstrapping WebSockets with HTTP/2 (Extended CONNECT)
//   - RFC 9220: Bootstrapping WebSockets with HTTP/3 (Extended CONNECT)
//   - RFC 7838: HTTP Alternative Services (Alt-Svc)
package mach
