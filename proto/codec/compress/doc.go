// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package compress implements HTTP payload compression and decompression codecs conforming to RFC 9110 §8.4.
//
// # Architecture
//
// The package handles content coding negotiation, streaming compression, and buffer-oriented codecs
// across HTTP/1.1, HTTP/2, and HTTP/3:
//
//   - Supported Encodings:
//     Gzip (RFC 1952), Deflate (RFC 1951), Brotli (RFC 7932), and Zstandard (RFC 8878).
//   - Resource Recycling:
//     Streaming compressors and decompressors utilize per-core recycling pools ([AcquireGzipWriter],
//     [AcquireDeflateWriter], [ReleaseGzipWriter], etc.), recycling internal state without GC churn on active streams.
//
// # RFC Specifications
//
//   - RFC 1950: ZLIB Compressed Data Format
//   - RFC 1951: DEFLATE Compressed Data Format
//   - RFC 1952: GZIP File Format
//   - RFC 7932: Brotli Compressed Data Format
//   - RFC 8878: Zstandard Compression
//   - RFC 9110 §8.4: HTTP Semantics (Content Coding)
package compress
