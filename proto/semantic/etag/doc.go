// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package etag implements RFC 9110 (§8.8.1-§8.8.3) and RFC 9111 HTTP Entity Tags,
// conditional request matching, and 304 response reconstruction.
//
// It provides zero-allocation generic matching algorithms across strings and byte slices,
// bounded automaton caching, and optional type integrations.
package etag
