// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mach provides silicon-speed, zero-allocation L7 protocol engines and transport codecs for Go.
//
// # Architectural Scope & Guidelines
//
// mach is built as the foundational L7 protocol layer on top of foundation. It delivers
// wire-speed codecs and state machines for:
//   - HTTP/1.1 (RFC 9112)
//   - HTTP/2 (RFC 9113)
//   - HTTP/3 over QUIC (RFC 9114)
//   - HPACK (RFC 7541) and QPACK (RFC 9204) Header Compression
//   - WebSocket Framing & Protocol Engine (RFC 6455)
//   - Bootstrapping WebSockets with HTTP/2 (RFC 8441) and HTTP/3 (RFC 9220) Extended CONNECT
//
// # Core Invariants
//
//   - Zero-Allocation Hot Paths: Request/response parsing, wire framing, header decoding, and
//     WebSocket masking must execute with 0 heap allocations on active streams.
//   - 64-bit SWAR Acceleration: Hardware-agnostic SIMD/SWAR vector kernels accelerate header
//     scanning and WebSocket frame masking at wire speeds (39+ GB/s).
//   - Multi-Protocol Unified Server: Server handlers process requests transparently across
//     HTTP/1.1, HTTP/2, and HTTP/3, and can perform seamless WebSocket upgrades via
//     Context.UpgradeWebSocket().
//   - Resilient Client Engine: Client engine provides automatic ALPN negotiation, SOCKS5/HTTP
//     CONNECT proxy tunneling, Alt-Svc connection pooling, and multi-protocol WebSocket dialing
//     via Engine.DialWebSocket().
//   - Asymmetric Isolation: Server handler logic is strictly isolated from malformed wire traffic
//     and network congestion.
//   - Sans-IO & FSM: Protocol decoding operates via event-driven state machines without
//     goroutine-per-stream overhead or blocking I/O calls on the hot path.
package mach
