// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package h2 implements the HTTP/2 server protocol engine conforming to RFC 9113,
// featuring stream multiplexing, flow control state machines, and asymmetric isolation.
//
// # Architecture
//
// The server engine multiplexes concurrent HTTP/2 streams over persistent TCP connections:
//
//   - Connection State Machine ([ServerConn]):
//     Processes incoming binary frames, updates connection and stream-level flow control windows,
//     and serializes egress responses across active multiplexed streams.
//   - Header & Frame Processing:
//     Decodes compressed field sections via HPACK with zero heap allocations on steady-state streams,
//     validating pseudo-header order, pseudo-header presence, and trailer boundaries.
//   - Extended CONNECT Support:
//     Processes RFC 8441 Extended CONNECT requests (`:method = CONNECT`, `:protocol = websocket`),
//     bootstrapping full-duplex WebSocket tunnels directly over multiplexed HTTP/2 streams.
//   - Asymmetric Isolation:
//     Enforces RFC 9113 stream state machine invariants. Malformed frames, window overflows, and
//     protocol violations trigger typed RST_STREAM or GOAWAY frames at the framing boundary, isolating
//     application handlers from transport corruption.
//
// # RFC Specifications
//
//   - RFC 9110: HTTP Semantics
//   - RFC 9113: HTTP/2 (Framing, Multiplexing, Flow Control)
//   - RFC 7541: HPACK - Header Compression for HTTP/2
//   - RFC 8441: Bootstrapping WebSockets with HTTP/2 (Extended CONNECT)
//
// # Compared to the standard library
//
// Stdlib counterpart: [golang.org/x/net/http2.Server]
//
// While standard library HTTP/2 servers spawn background goroutines per active stream and allocate
// frame structures on the heap, this engine uses slab-pooled frame memory and event-driven stream
// state machines to deliver amortized zero-allocation multiplexing.
//
// Allocations: amortized zero per transaction on active streams.
package h2
