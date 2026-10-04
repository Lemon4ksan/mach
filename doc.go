// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mach provides silicon-speed, zero-allocation L7 protocol engines and transport codecs for Go.
//
// # Architectural Scope & Guidelines
//
// mach is built as the foundational L7 protocol layer on top of foundation. It delivers
// wire-speed codecs and state machines for HTTP/1.1 (RFC 9112), HTTP/2 (RFC 9113), HTTP/3 (RFC 9114),
// HPACK (RFC 7541), and QPACK (RFC 9204).
//
//   - Zero-Allocation Hot Paths: Request and response parsing, framing, and header decoding
//     must execute with 0 allocations on active streams.
//   - Asymmetric Isolation: Server handler logic is strictly isolated from malformed wire traffic
//     and network congestion.
//   - Sans-IO & FSM: Protocol decoding operates via event-driven state machines without
//     goroutine-per-stream overhead or blocking I/O calls on the hot path.
//   - Clean Encapsulation: Codecs and protocol definitions (proto, hpack, qpack) are public,
//     independent building blocks; runtime workers and internals reside in internal/.
package mach
