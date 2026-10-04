// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package zerocopy provides high-performance, zero-allocation data structures
// and parsers for HTTP query parameters, URIs, and cookies (RFC 9110, RFC 9112, RFC 6265).
//
// All mutable structures (Args, URI, Cookie) are backed by recycled sync.Pool instances
// to guarantee zero heap allocations during hot-path request and response processing.
package zerocopy
