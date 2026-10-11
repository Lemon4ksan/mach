// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hpack implements HTTP/2 header compression.
//
// # Package hpack
//
// Layer 7 Architecture:
// In the Layer 7 protocol stack, hpack serves as the header serialization and
// compression engine for HTTP/2 frames. It translates HTTP headers to and from
// compact wire representations using static indexing, a stateful dynamic table,
// and static Huffman coding.
//
// Dynamic table entries and header structures are managed with zero heap allocations
// on active streams via per-core recycling pools.
//
// Relevant Specifications:
//   - RFC 7541: HPACK - Header Compression for HTTP/2
//   - RFC 9113: HTTP/2
package hpack
