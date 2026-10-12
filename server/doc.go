// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package server provides a multi-protocol Layer 7 application server binding HTTP/1.1,
// HTTP/2, and HTTP/3 engines under a unified handler architecture.
//
// # Architecture
//
// The server decouples transport framing and socket multiplexing from application handlers,
// presenting an identical execution lifecycle regardless of underlying protocol:
//
//   - Universal Execution Context ([Context]):
//     Wraps incoming requests and outgoing responses across HTTP/1.1, HTTP/2 streams, and HTTP/3
//     QUIC streams. Provides zero-allocation header lookups, streaming request bodies, and fast
//     status serialization.
//   - Handler Abstraction ([Handler], [HandlerFunc]):
//     Standard contract processing requests across all supported HTTP versions without protocol branching.
//   - ALPN & Multi-Protocol Dispatch ([Server.Serve], [Server.ServeTLS]):
//     Negotiates HTTP/1.1 and HTTP/2 on a shared TCP listener using TLS ALPN tokens ("h2", "http/1.1").
//     Automatically emits Alt-Svc headers directing compatible clients to companion HTTP/3 QUIC endpoints.
//   - Extended CONNECT WebSocket Upgrades ([Context.UpgradeWebSocket]):
//     Upgrades incoming requests to full-duplex WebSocket connections across protocol
//     generations, servicing standard RFC 6455 handshakes alongside RFC 8441 and RFC 9220 Extended CONNECT
//     streams on HTTP/2 and HTTP/3.
//   - Asymmetric Isolation:
//     Ingress decoders strictly enforce RFC grammar and stream limits at the protocol boundary.
//     Malformed frames, invalid pseudo-headers, and stream resets are dropped or handled in-engine,
//     preventing connection degradation from bubbling into handler logic.
//
// # RFC Specifications
//
//   - RFC 9110: HTTP Semantics
//   - RFC 9112: HTTP/1.1
//   - RFC 9113: HTTP/2 (Binary Framing and Multiplexing)
//   - RFC 9114: HTTP/3 (QUIC-Based Semantics)
//   - RFC 7838: HTTP Alternative Services (Alt-Svc)
//   - RFC 6455: The WebSocket Protocol
//   - RFC 8441: Bootstrapping WebSockets with HTTP/2 (Extended CONNECT)
//   - RFC 9220: Bootstrapping WebSockets with HTTP/3 (Extended CONNECT)
//
// # Compared to the standard library
//
// Stdlib counterpart: [net/http.Server]
//
// Standard library servers allocate per-request context structs, spawn dedicated goroutines per stream,
// and require external packages or separate servers to manage HTTP/3 and modern WebSocket upgrades.
// This server unifies HTTP/1.1, HTTP/2, and HTTP/3 within a single handler pipeline, recycling stream
// contexts to eliminate steady-state heap allocations.
//
// Allocations: amortized zero per transaction on active streams.
package server
