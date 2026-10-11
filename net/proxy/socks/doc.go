// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package socks provides a SOCKS version 5 client implementation.
//
// SOCKS protocol version 5 is defined in RFC 1928, and Username/Password
// authentication is defined in RFC 1929. This package handles connection setup
// and proxy handshakes before handing off the raw socket for data transfer.
//
// # Compared to the standard library
//
// Stdlib counterpart: golang.org/x/net/proxy
//
// Rejected compromise: none (this package primarily maintains compatibility with standard proxy dialers).
//
// Accepted cost: the handshake relies on blocking I/O and creates per-connection allocations.
//
// Allocations: unconstrained
package socks
