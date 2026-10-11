// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package qpack implements HTTP/3 field compression (RFC 9204).
//
// # Package qpack
//
// Layer 7 Architecture:
// In the Layer 7 protocol stack, qpack delivers header serialization and
// field block compression for HTTP/3 over QUIC (RFC 9114 §4.2). Unlike HPACK in HTTP/2,
// QPACK is designed for out-of-order stream multiplexing, eliminating head-of-line
// blocking through independent unidirectional encoder (RFC 9204 §2.1 & §4.3) and
// decoder (RFC 9204 §2.2 & §4.4) control streams, base index tracking (RFC 9204 §3.2.5),
// and a dynamic table with eviction tracking (RFC 9204 §2.1, §3.2, §4.3.1).
//
// State machines, table entries, and stream buffers are managed with zero heap
// allocations on active connections via per-core recycling pools.
//
// Relevant Specifications:
//   - RFC 9204 §2.1 & §4.3: Encoder Stream and Instructions (Set Capacity §4.3.1, Insert Name Ref §4.3.2, Insert Literal §4.3.3, Duplicate §4.3.4)
//   - RFC 9204 §2.2 & §4.4: Decoder Stream and Instructions (Section Ack §4.4.1, Stream Cancellation §4.4.2, Insert Count Increment §4.4.3)
//   - RFC 9204 §3.2: Dynamic Table Management and Eviction
//   - RFC 9204 §4.1: Primitive Representations (Varint §4.1.1, String Literal & Huffman §4.1.2)
//   - RFC 9204 §4.5: Field Section Prefix (§4.5.1) and Field Line Representations (§4.5.2-§4.5.6)
//   - RFC 9114 §4.2: HTTP/3 Field Section Encoding
//   - RFC 9000: QUIC: A UDP-Based Multiplexed and Secure Transport
package qpack
