// Package h2 implements the HTTP/2 client protocol engine conforming to RFC 9113,
// featuring stream multiplexing, HPACK header compression, and asymmetric isolation.
//
// # Architecture
//
// The client engine establishes and coordinates multiplexed HTTP/2 sessions over TLS or cleartext TCP:
//
//   - Engine Pipeline ([Engine]):
//     Implements [mach.Engine], transmitting the 24-octet connection preface, exchanging initial SETTINGS,
//     and yielding active multiplexed connections.
//   - Multiplexed Connection ([Conn]):
//     Coordinates concurrent bidirectional streams over a single transport connection. Serializes egress
//     frames, dispatches incoming DATA and HEADERS frames to active streams, and updates flow control windows.
//   - Asymmetric Isolation:
//     Decodes and validates ingress frames at the wire parser, discarding malformed frames and resetting
//     faulty streams without corrupting session multiplexing state.
//
// # RFC Specifications
//
//   - RFC 9110: HTTP Semantics
//   - RFC 9113: HTTP/2 (Framing, Streams, Flow Control)
//   - RFC 7541: HPACK - Header Compression for HTTP/2
//   - RFC 8441: Bootstrapping WebSockets with HTTP/2 (Extended CONNECT)
//
// # Compared to the standard library
//
// Stdlib counterpart: [golang.org/x/net/http2.Transport]
//
// Unlike standard library HTTP/2 clients that allocate frame objects on every network read,
// this implementation uses slab-pooled frame buffers and zero-copy header decoding pipelines.
//
// Allocations: amortized zero on active streams.
package h2
