// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hpack implements HTTP/2 header compression (RFC 7541).
//
// # Package hpack
//
// Layer 7 Architecture:
// In the Layer 7 protocol stack, hpack serves as the header serialization and
// compression engine for HTTP/2 frames (RFC 9113 §4.3). It translates HTTP headers
// to and from compact wire representations using static indexing (RFC 7541 §2.3.1 &
// Appendix A), a stateful dynamic table (RFC 7541 §2.3.2, §2.3.3, §4.1-§4.4),
// and static Huffman coding (RFC 7541 §5.2 & Appendix B).
//
// Dynamic table entries and header structures are managed with zero heap allocations
// on active streams via per-core recycling pools.
//
// Relevant Specifications:
//   - RFC 7541 §2.3: Indexing Tables (Static Table §2.3.1, Dynamic Table §2.3.2, Index Address Space §2.3.3)
//   - RFC 7541 §4: Dynamic Table Management (Calculating Size §4.1, Maximum Size §4.2, Eviction §4.3-§4.4)
//   - RFC 7541 §5: Primitive Type Representations (Integer §5.1, String Literal & Huffman Padding §5.2)
//   - RFC 7541 §6: Binary Format (Indexed §6.1, Literal §6.2, Dynamic Table Size Update §6.3)
//   - RFC 7541 Appendix A: Static Table Definition
//   - RFC 7541 Appendix B: Huffman Code Table
//   - RFC 9113 §4.3: HTTP/2 Header Compression and Decompression
package hpack
