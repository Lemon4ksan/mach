// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package h2 implements the HTTP/2 binary framing layer, stream flow control state machines,
// and slab-allocated frame pooling conforming to RFC 9113.
//
// # Architecture
//
// HTTP/2 multiplexes discrete bidirectional request-response streams over a single transport
// connection using structured 9-octet binary frame envelopes (RFC 9113 §4.1).
//
// Protocol operations organize into distinct processing layers:
//
//   - Frame Header Pipeline ([FrameHeader], [ReadFrameFrom], [ReadFrameFromWithSize]):
//     Decodes 24-bit payload lengths, 8-bit frame types, 8-bit flag sets, and 31-bit stream IDs
//     with strict payload boundary enforcement and automatic unknown frame skipping (RFC 9113 §4.1).
//   - Frame Specialization Hierarchy ([Frame], [Data], [Headers], [Priority], [RstStream],
//     [Settings], [PushPromise], [Ping], [GoAway], [WindowUpdate], [Continuation]):
//     Specialized serialization and deserialization implementations adhering to RFC 9113 §6.
//   - Slab Allocation & Frame Pooling ([ConnectionFramePool], [AcquireFrame], [ReleaseFrame]):
//     Bitmap-backed slab allocators for fixed-size control frames (Ping, Priority, RstStream,
//     WindowUpdate), recycling buffers without heap escape or GC tracking on active streams.
//   - Error Hierarchy ([ErrorCode], [Error], [NewGoAwayError], [NewResetStreamError]):
//     Typed 32-bit protocol error classification supporting connection-level GOAWAY teardown
//     and stream-level RST_STREAM generation (RFC 9113 §7).
//
// # Transport and Stream Multiplexing Model
//
// HTTP/2 operates over a reliable byte stream (TCP or TLS) initiated by the 24-octet client connection
// preface "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n" followed by an initial SETTINGS exchange (RFC 9113 §3.4).
//
// Streams progress through distinct states (idle, reserved, open, half-closed, closed) governed
// by frame flags:
//   - END_STREAM (0x1): Concludes payload transmission from the sender on DATA and HEADERS frames.
//   - END_HEADERS (0x4): Concludes a field block without requiring intervening CONTINUATION frames.
//   - ACK (0x1): Acknowledges receipt of SETTINGS and PING frames.
//   - PADDED (0x8): Demarcates padding octets applied for traffic side-channel mitigation.
//
// # Failure Boundaries & Protocol Errors
//
// Framing violations, window overflows, or invalid stream state transitions yield typed [ErrorCode]
// values. Severe protocol violations trigger connection-level termination via [GoAway] frames,
// whereas stream-specific violations terminate individual streams via [RstStream] without
// disrupting neighboring concurrent streams on the same connection.
//
// # RFC Specifications
//
//   - RFC 9113: HTTP/2 (Frames, Stream Multiplexing, Flow Control, Error Codes)
//   - RFC 7541: HPACK - Header Compression for HTTP/2
//   - RFC 8441: Bootstrapping WebSockets with HTTP/2 (Extended CONNECT Protocol)
//
// # Compared to the standard library
//
// Stdlib counterpart: golang.org/x/net/http2
//
// Standard library HTTP/2 implementations allocate heap memory per frame, hide background goroutines
// inside opaque framers, and perform untyped buffer copies. This package replaces heap-allocated frame
// structs with slab-pooled memory and zero-copy header slicing, allowing callers to sustain line-rate
// multiplexing with amortized zero allocations.
//
// Allocations: amortized zero on active streams.
package h2
