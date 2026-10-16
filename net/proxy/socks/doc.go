// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package socks implements a zero-allocation SOCKS version 5 client conforming to RFC 1928 and RFC 1929.
//
// # Architecture
//
// The client engine negotiates proxy handshakes over established stream connections before handing
// off raw sockets for application data transfer:
//
//   - Authentication Methods:
//     Supports No Authentication (0x00) and Username/Password Authentication (0x02, RFC 1929).
//   - Target Addressing:
//     Encodes IPv4 (0x01), IPv6 (0x04), and fully qualified domain names (0x03) without heap escapes.
//   - Handshake Framing:
//     Fixed 512-byte stack buffers accommodate standard address envelopes while eliminating GC pressure.
//
// # RFC Specifications
//
//   - RFC 1928: SOCKS Protocol Version 5
//   - RFC 1929: Username/Password Authentication for SOCKS V5
//
// # Compared to the standard library
//
// Stdlib counterpart: golang.org/x/net/proxy
//
// Standard library proxy implementations allocate per-dial state and spawn background goroutines
// during authentication. This package executes handshakes with zero heap allocations and prompt socket closure.
//
// Allocations: 0 allocs/op on handshake hot paths.
package socks
