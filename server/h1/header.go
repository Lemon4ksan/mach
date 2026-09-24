// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import "github.com/lemon4ksan/mach/proto/headkit"

// Headers represents a high-performance HTTP/1.1 header block backed by foundation/net/headkit (RFC 9110 §6.3, RFC 9112).
//
// Concurrency:
//   - Not safe for concurrent use across multiple goroutines.
type (
	Headers     = headkit.Headers
	HeaderEntry = headkit.HeaderEntry
)

// NewHeadersWithCapacity allocates a new Headers table with preallocated entry capacity (RFC 9110 §6.3).
var NewHeadersWithCapacity = headkit.NewWithCapacity
