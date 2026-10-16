// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package h1 implements the HTTP/1.1 server protocol engine conforming to RFC 9112,
// featuring asymmetric isolation, zero-allocation request parsing, and pipelined execution.
//
// # Architecture
//
// The server engine isolates L4 network connection management from L7 application handlers:
//
//   - Connection State Machine ([Conn]):
//     Processes incoming requests sequentially or via pipelining over active TCP connections,
//     recycling request contexts and header buffers to eliminate allocation overhead.
//   - Request Parsing & Pipelining:
//     Parses start lines and headers in-place using SWAR vector acceleration, supporting persistent
//     connections (keep-alive) and chunked request bodies.
//   - Asymmetric Isolation:
//     Enforces strict RFC 9112 syntax validation at the protocol boundary, discarding malformed
//     frames, detecting request smuggling vectors, and cleanly aborting corrupted connections
//     without compromising server stability.
//
// # RFC Specifications
//
//   - RFC 9110: HTTP Semantics
//   - RFC 9112: HTTP/1.1 (Message Syntax, Pipelining, Connection Management)
//
// # Compared to the standard library
//
// Stdlib counterpart: [net/http.Server] (HTTP/1.1 engine)
//
// Standard library HTTP/1.1 servers allocate per-request buffers and reader structures on every
// transaction. This implementation maintains bounded buffer arenas per connection, driving steady-state
// execution with zero heap allocations per request.
//
// Allocations: amortized zero per transaction.
package h1
