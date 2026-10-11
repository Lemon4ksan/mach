// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package zerocopy provides zero-allocation representations and parsers for HTTP primitives.
//
// # Package zerocopy
//
// Layer 7 Architecture:
// In the Layer 7 protocol stack, zerocopy provides core data structures and
// byte-level decoders for HTTP request components including URIs, query parameters,
// headers, and cookies across HTTP/1.1, HTTP/2, and HTTP/3. It avoids data copying
// by maintaining direct slice references into connection buffers.
//
// Mutable structures such as [Args], [URI], and [Cookie] are pooled and recycled
// via per-core pools to ensure zero heap allocations during stream and message processing.
//
// Relevant Specifications:
//   - RFC 3986: Uniform Resource Identifier (URI): Generic Syntax
//   - RFC 6265: HTTP State Management Mechanism (Cookies)
//   - RFC 9110: HTTP Semantics
//   - RFC 9112: HTTP/1.1
package zerocopy
