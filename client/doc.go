// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package client provides high-performance client transport engines, connection pooling,
// multi-protocol session abstractions, and zero-allocation proxy integration across HTTP/1.1, HTTP/2, and HTTP/3.
//
// # Architecture
//
// The central coordinator [Engine] manages end-to-end request dispatching, protocol negotiation,
// connection reuse, and tunnel establishment:
//
//   - Request Execution ([Engine.Do], [Engine.RoundTrip]):
//     Dispatches standard requests across host-partitioned connection pools, selecting optimal
//     transports according to protocol availability and cached alternative service routes.
//   - Connection Pooling ([PoolManager]):
//     Maintains host-keyed pools with stripped lock contention and generational idle evictions,
//     minimizing allocation drag during connection acquisition.
//   - Transparent Alternative Services ([AltSvcManager]):
//     Parses and caches RFC 7838 Alt-Svc headers from ingress responses, automatically migrating
//     subsequent requests from TCP-based HTTP/1.1 or HTTP/2 to zero-RTT HTTP/3 over QUIC.
//   - Proxy Integration ([WithProxy]):
//     Integrates SOCKS5 and HTTP CONNECT tunnels directly into the dialing pipeline, negotiating
//     forwarding paths before initiating TLS handshakes or protocol frames.
//   - Multi-Protocol WebSocket Dialing ([Engine.DialWebSocket]):
//     Establishes full-duplex WebSocket sessions across protocol generations, negotiating standard
//     RFC 6455 upgrades on HTTP/1.1, or RFC 8441 / RFC 9220 Extended CONNECT tunnels on HTTP/2 and HTTP/3 streams.
//
// # Transport Selection & Protocol Negotiation
//
// Outbound connections determine protocol capability via TLS Application-Layer Protocol
// Negotiation (ALPN):
//   - "h2": Directs the transport to initialize binary frame multiplexers and HPACK contexts.
//   - "http/1.1": Retains single-stream or pipelined connection pipelines.
//   - QUIC / UDP: Initiated directly when Alt-Svc records advertise HTTP/3 availability on the remote target.
//
// # RFC Specifications
//
//   - RFC 9110: HTTP Semantics
//   - RFC 9112: HTTP/1.1
//   - RFC 9113: HTTP/2 (Multiplexed Transport)
//   - RFC 9114: HTTP/3 over QUIC
//   - RFC 7838: HTTP Alternative Services (Alt-Svc)
//   - RFC 6455: The WebSocket Protocol
//   - RFC 8441: Bootstrapping WebSockets with HTTP/2 (Extended CONNECT)
//   - RFC 9220: Bootstrapping WebSockets with HTTP/3 (Extended CONNECT)
//
// # Compared to the standard library
//
// Stdlib counterpart: [net/http.Client] and [net/http.Transport]
//
// In contrast to standard library transports that rely on reflection-based configuration, separate
// package hierarchies for HTTP/2, and unbounded goroutine spawning per connection, this engine
// unifies HTTP/1.1, HTTP/2, and HTTP/3 behind a single configuration surface with host-partitioned
// connection pooling and direct proxy dialer chaining.
//
// Allocations: amortized zero on established persistent connections.
package client
