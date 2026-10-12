// Package h1 implements the HTTP/1.1 client protocol engine conforming to RFC 9112,
// featuring asymmetric isolation and zero-allocation persistent connection reuse.
//
// # Architecture
//
// The client transport coordinates HTTP/1.1 request-response round-trips over persistent TCP connections:
//
//   - Engine Pipeline ([Engine]):
//     Implements [mach.Engine], dialing target endpoints and configuring socket options, TLS handshakes,
//     and proxy routing.
//   - Connection Lifecycle ([ClientConn]):
//     Manages single-stream and pipelined transactions over active network connections. Reusable buffers
//     eliminate heap churn across successive requests on persistent connections.
//   - Asymmetric Isolation:
//     Protects client state machines by validating server status lines, transfer encodings, and chunk
//     framing at the wire boundary. Upgraded connections transition cleanly out of standard HTTP mode ([ErrConnUpgraded]).
//
// # RFC Specifications
//
//   - RFC 9110: HTTP Semantics
//   - RFC 9112: HTTP/1.1 (Message Syntax, Connection Management)
//
// # Compared to the standard library
//
// Stdlib counterpart: [net/http.Transport] (HTTP/1.1 mode)
//
// In contrast to standard library clients that hide internal buffer allocations and background read loops,
// this engine executes transactions with direct buffer recycling and zero steady-state heap allocations.
//
// Allocations: zero per request on persistent connections.
package h1
