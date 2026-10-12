// Package h3 implements the HTTP/3 server protocol engine conforming to RFC 9114,
// featuring QUIC stream demultiplexing, QPACK field compression, and asymmetric isolation.
//
// # Architecture
//
// The server engine maps HTTP/3 transactions directly onto QUIC transport streams (RFC 9000):
//
//   - Connection State Machine ([ServerConn]):
//     Dispatches incoming bidirectional request streams, manages unidirectional control streams,
//     and coordinates QPACK dynamic table synchronization across encoder and decoder streams.
//   - Request Processing & Header Decompression:
//     Decodes QPACK field sections without heap escape, enforcing HTTP/3 pseudo-header validation
//     and progressive body streaming.
//   - Extended CONNECT WebSocket Upgrades:
//     Accepts RFC 9220 Extended CONNECT requests over bidirectional HTTP/3 streams, transitioning
//     individual streams into full-duplex WebSocket tunnels without affecting neighboring streams.
//   - Asymmetric Isolation:
//     Isolates application handlers from QUIC transport congestion and framing anomalies. Protocol
//     violations emit standard HTTP/3 error codes to reset offending streams or close the QUIC connection.
//
// # RFC Specifications
//
//   - RFC 9110: HTTP Semantics
//   - RFC 9114: HTTP/3 (Transport, Framing, Semantics)
//   - RFC 9204: QPACK - Field Compression for HTTP/3
//   - RFC 9220: Bootstrapping WebSockets with HTTP/3 (Extended CONNECT)
//   - RFC 9000: QUIC: A UDP-Based Multiplexed and Secure Transport
//
// # Compared to the standard library
//
// Stdlib counterpart: none - Go standard library provides no native HTTP/3 server
//
// This engine delivers zero-allocation variable-length integer decoding, deterministic QPACK table
// management, and seamless Extended CONNECT integration for high-performance HTTP/3 servers.
//
// Allocations: amortized zero per transaction on active streams.
package h3
