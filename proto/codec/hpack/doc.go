// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hpack implements HTTP/2 header compression conforming to RFC 7541.
//
// # Architecture
//
// As the header serialization and compression engine for HTTP/2 frames (RFC 9113 §4.3),
// hpack translates HTTP header field sections to and from compact wire representations using:
//
//   - Static Table Indexing (RFC 7541 §2.3.1 & Appendix A):
//     Predefined 61-entry mapping for common HTTP headers and pseudo-headers.
//   - Stateful Dynamic Table (RFC 7541 §2.3.2 & §4):
//     FIFO header cache with configurable capacity and eviction accounting.
//   - Static Huffman Coding (RFC 7541 §5.2 & Appendix B):
//     Canonical Huffman code trees accelerating ASCII literal compression.
//
// Dynamic table entries and header structures are recycled across active streams
// using per-core pools to sustain zero heap allocations on the hot path.
//
// # RFC Specifications
//
//   - RFC 7541: HPACK - Header Compression for HTTP/2
//   - RFC 9113 §4.3: HTTP/2 Header Compression and Decompression
package hpack
