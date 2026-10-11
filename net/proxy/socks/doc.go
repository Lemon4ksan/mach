// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package socks provides a high-performance, zero-allocation SOCKS version 5 client implementation.
//
// SOCKS protocol version 5 is defined in RFC 1928, and Username/Password
// authentication is defined in RFC 1929. This package handles connection setup
// and proxy handshakes before handing off the raw socket for data transfer.
//
// # Compared to the standard library
//
// Stdlib counterpart: golang.org/x/net/proxy
//
// Rejected compromise: legacy vendored proxy implementations suffered from goroutine sprawl
// and unconstrained heap allocations on every connection attempt.
//
// Accepted cost: fixed stack buffers limit raw handshake headers to 512 bytes, fully compliant
// with maximum RFC 1928 address envelopes.
//
// Allocations: 0 allocs/op on handshake hot paths.
package socks
