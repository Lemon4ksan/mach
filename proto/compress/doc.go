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
//   - RFC 1950 §2.2: ZLIB Compressed Data Format (CMF/FLG headers, FCHECK, Adler-32 trailer)
//   - RFC 1951 §3.1.1 & §3.2: DEFLATE Compressed Data Format (BFINAL, BTYPE, fixed & dynamic Huffman)
//   - RFC 1952 §2.2 & §2.3.1: GZIP File Format (ID1/ID2 magic, FLG flags, CRC-32 and ISIZE trailers)
//   - RFC 7932 §2 & §9: Brotli Compressed Data Format (meta-block framing, WBITS sliding window)
//   - RFC 8878 §3.1.1: Zstandard Compression (Magic Number 0xFD2FB528, Frame Header Descriptor, Block format, XXH64 checksum)
//   - RFC 9110 §8.4: HTTP Semantics (Content-Coding: "gzip" §8.4.1.3, "deflate" §8.4.1.2, "br" §8.4.1.4, "zstd" §8.4.1.5)
package compress
