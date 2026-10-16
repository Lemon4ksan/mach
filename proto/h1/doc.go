// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package h1 implements low-level HTTP/1.1 wire framing, SWAR-accelerated header scanning,
// and chunked transfer encoding conforming to RFC 9112.
//
// # Architecture
//
// The package provides high-throughput wire primitives optimized for zero-copy parsing
// and silicon-speed stream validation:
//
//   - Vectorized Header Scanner ([HeaderScanner]):
//     Scans request and response header blocks using 64-bit SWAR (SIMD Within A Register)
//     vector algorithms. Locates CRLF boundaries, colon delimiters, and header field sections
//     at line rate without heap allocation.
//   - Chunked Transfer Coding ([ChunkedReader], [ChunkedWriter]):
//     Implements RFC 9112 §7.1 streaming chunked decoding and serialization. Decodes hex chunk
//     sizes, skips chunk extensions, enforces terminating CRLF invariants, and surfaces trailing
//     field sections without buffering whole message payloads into memory.
//   - RFC 9112 Validation & Smuggling Protections:
//     Strictly enforces HTTP/1.1 syntax constraints to defeat request smuggling vulnerabilities.
//     Rejects obsolete line folding (obs-fold, RFC 9112 §5.2), invalid whitespace preceding header
//     names ([ErrInvalidHeaderLeadingWhitespace]), and malformed chunk delimiters.
//
// # RFC Specifications
//
//   - RFC 9110: HTTP Semantics
//   - RFC 9112: HTTP/1.1 (Message Syntax, Chunked Transfer Coding, Header Fields)
//
// # Compared to the standard library
//
// Stdlib counterpart: [bufio.Reader] and internal `net/http` wire parsers
//
// Standard library HTTP/1.1 parsing incurs multiple string allocations per header line,
// lacks SIMD/SWAR delimiter scanning, and permits ambiguous whitespace that exposes downstream
// servers to HTTP request smuggling. This package parses headers and chunked streams in-place
// over wire buffers with zero heap allocations.
//
// Allocations: 0 allocs/op on active streams.
package h1
