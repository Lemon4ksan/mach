// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package qpack implements HTTP/3 header compression.
//
// # Package qpack
//
// Layer 7 Architecture:
// In the Layer 7 protocol stack, qpack delivers header serialization and
// field block compression for HTTP/3 over QUIC. Unlike HPACK in HTTP/2, QPACK
// is designed for out-of-order stream multiplexing, eliminating head-of-line
// blocking through independent unidirectional encoder and decoder control streams,
// base index tracking, and a dynamic table with eviction tracking.
//
// State machines, table entries, and stream buffers are managed with zero heap
// allocations on active connections via per-core recycling pools.
//
// Relevant Specifications:
//   - RFC 9204: QPACK - Field Compression for HTTP/3
//   - RFC 9114: HTTP/3
//   - RFC 9000: QUIC: A UDP-Based Multiplexed and Secure Transport
package qpack
