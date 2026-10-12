// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package qpack implements HTTP/3 field section compression conforming to RFC 9204.
//
// # Architecture
//
// Designed for out-of-order stream multiplexing over QUIC (RFC 9000), qpack eliminates
// transport head-of-line blocking by separating field block representations from dynamic
// table updates:
//
//   - Unidirectional Control Streams:
//     Encoder (RFC 9204 §4.3) and decoder (RFC 9204 §4.4) streams communicate table updates
//     and acknowledgments asynchronously.
//   - Base Index & Relative Addressing (RFC 9204 §3.2.5):
//     Decouples field section references from eviction ordering across concurrent streams.
//   - Dynamic Table Management (RFC 9204 §3.2):
//     Maintains strict insertion and eviction bounds to prevent resource exhaustion.
//
// State machines, table entries, and stream buffers operate without heap allocations
// on active connections via per-core recycling pools.
//
// # RFC Specifications
//
//   - RFC 9204: QPACK - Field Compression for HTTP/3
//   - RFC 9114 §4.2: HTTP/3 Field Section Encoding
//   - RFC 9000: QUIC: A UDP-Based Multiplexed and Secure Transport
package qpack
