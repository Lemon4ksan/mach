// Package http implements high-performance, zero-allocation HTTP/1.x wire parsing,
// message serialization, header manipulation, and streaming body decoders conforming to RFC 9110 and RFC 9112.
//
// # Architecture
//
// The package exposes foundational data-in-motion structures for HTTP transactions:
//
//   - Request & Response Representations ([Request], [Response]):
//     Structured message models providing in-place wire deserialization, zero-copy URI and header views,
//     and fast status serialization.
//   - Header Arena & Field Management ([Header], [HeaderFields], [HeaderTrailers]):
//     Specialized header storage supporting multi-value lookups, case-insensitive canonicalization,
//     cookie parsing, and trailer sections without per-header heap allocations.
//   - Streaming Body Decoders:
//     Configures streaming body pipelines handling identity framing, chunked transfer coding,
//     and transparent content decompression (gzip, deflate, brotli, zstd) with bounded memory reuse.
//   - Zero-Copy Wire Codecs:
//     Parses start lines, header blocks, and chunk boundaries directly across network buffers,
//     enforcing strict RFC 9112 syntax validation to guard against request smuggling.
//
// # RFC Specifications
//
//   - RFC 9110: HTTP Semantics (Fields, Status Codes, Content Negotiation)
//   - RFC 9112: HTTP/1.1 (Message Syntax, Chunked Transfer Coding, Connection Management)
//
// # Compared to the standard library
//
// Stdlib counterpart: [net/http]
//
// Standard library HTTP message abstractions allocate strings for every header name and value,
// duplicate buffers during body reads, and rely on garbage-collected readers. This package provides
// pooled, zero-allocation message structures with in-place byte slicing and bounded buffer reuse.
//
// Allocations: 0 allocs/op during active message parsing.
package http
