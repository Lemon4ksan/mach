// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package headkit provides a zero-allocation, high-performance HTTP header,
// directive, content-negotiation, and MIME media-type toolkit for Go.
//
// # Architectural Manifesto
//
// Standard [net/http.Header] is a simple map[string][]string that allocates heap memory on
// every lookup and lacks understanding of RFC header grammar (directives, quality factors,
// parameters, and sensitivity). headkit bridges this gap with zero-allocation iterators,
// SIMD-accelerated lookups, and RFC-compliant parsers:
//
//   - Directive Parsing (RFC 9110 §5.6 & RFC 9111 §5.2): Parses comma- and semicolon-separated
//     directives (e.g., Cache-Control, Alt-Svc, Forwarded, TE) without heap slice allocations.
//   - Content Negotiation & q-values (RFC 9110 §12.4.2 & §12.5): Quality-weighted media type,
//     charset, and language matching.
//   - Media Type & MIME (RFC 2045 & RFC 9110 §8.3): Fast extraction of media types, charsets,
//     and multipart boundaries.
//   - Sensitive Header Redaction (RFC 9110 §15): High-speed detection and masking of credentials,
//     session cookies, and authorization tokens for HAR logs and telemetry.
//   - Modern Go Iterators: Idiomatic [iter.Seq] and [iter.Seq2] range-over-func adapters for
//     streaming header evaluation with zero heap overhead.
package headkit
