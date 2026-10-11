// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package compress implements HTTP payload compression and decompression codecs.
//
// # Package compress
//
// Layer 7 Architecture:
// In the Layer 7 protocol stack, compress handles payload content coding and
// decoding (RFC 9110 §8.4) for HTTP/1.1, HTTP/2, and HTTP/3 transactions. It provides
// stream-oriented and buffer-oriented codecs for Deflate, Gzip, Brotli, and Zstandard.
//
// Goroutines executing stream compression are decoupled from large stack allocations
// using stackless worker pools via [AcquireStacklessGzipWriter] and [AcquireStacklessDeflateWriter].
// Buffer operations recycle internal resources through per-core byte buffer pools.
//
// Relevant Specifications:
//   - RFC 1950: ZLIB Compressed Data Format Specification
//   - RFC 1951: DEFLATE Compressed Data Format Specification
//   - RFC 1952: GZIP File Format Specification
//   - RFC 7932: Brotli Compressed Data Format
//   - RFC 8878: Zstandard Compression and the 'application/zstd' Media Type
//   - RFC 9110: HTTP Semantics (Content-Encoding)
package compress
