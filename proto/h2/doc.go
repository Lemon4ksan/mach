// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package h2 provides high-performance, zero-allocation HTTP/2 binary frame parsing,
// frame serialization, stream flow-control, and off-heap slab pooling for Go.
//
// # Architecture
//
// HTTP/2 enables full multiplexing of request and response messages over a single TCP
// connection using 9-octet binary framed streams (RFC 9113 §4.1).
//
// The h2 package implements line-rate HTTP/2 wire protocol operations divided into:
//
//   - Frame Header Pipeline ([FrameHeader], [ReadFrameFrom], [ReadFrameFromWithSize]):
//     Decodes 24-bit payload lengths, 8-bit frame types, 8-bit flag sets, and 31-bit stream IDs
//     with strict payload boundary enforcement and automatic unknown frame skipping (RFC 9113 §4.1).
//   - Frame Specialization Hierarchy ([Frame], [Data], [Headers], [Priority], [RstStream],
//     [Settings], [PushPromise], [Ping], [GoAway], [WindowUpdate], [Continuation]):
//     Specialized serialization and deserialization implementations adhering to RFC 9113 §6.
//   - Off-Heap Slab Allocation ([ConnectionFramePool]):
//     Direct bitmap-backed slab allocators for plain-old-data (POD) frame types (Ping, Priority,
//     RstStream, WindowUpdate) eliminating GC tracking and stop-the-world spikes on hot paths.
//   - Error Hierarchy ([ErrorCode], [Error], [NewGoAwayError], [NewResetStreamError]):
//     Comprehensive 32-bit protocol error classification supporting connection-level GOAWAY
//     and stream-level RST_STREAM generation (RFC 9113 §7).
//
// # Transport and Stream Multiplexing Model
//
// HTTP/2 operates over a reliable transport (TCP or TLS) initiated by the 24-octet client connection
// preface "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n" followed by a SETTINGS frame (RFC 9113 §3.4).
//
// Streams progress through distinct states (idle, reserved, open, half-closed, closed) controlled
// by frame flags:
//   - END_STREAM (0x1): Concludes data transmission from the sender on DATA and HEADERS frames.
//   - END_HEADERS (0x4): Signals completion of a field section without intervening CONTINUATION frames.
//   - ACK (0x1): Confirms receipt of SETTINGS and PING frames.
//   - PADDED (0x8): Encapsulates security padding octets for side-channel mitigation.
//
// # RFC Specifications
//
//   - RFC 9113: HTTP/2 (Frames, Stream Multiplexing, Flow Control, Error Codes)
//   - RFC 7541: HPACK - Header Compression for HTTP/2
//   - RFC 8441: Bootstrapping WebSockets with HTTP/2 (Extended CONNECT Protocol, 0x8)
//
// # Compared to the standard library
//
// Stdlib counterpart: golang.org/x/net/http2
//
// Rejected compromise: Heap allocations per frame, hidden internal goroutines, and untyped buffer copies.
//
// Accepted cost: Manual frame pooling lifecycle management ([AcquireFrame], [ReleaseFrame]).
//
// Allocations: amortized zero
package h2
