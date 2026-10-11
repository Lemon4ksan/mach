// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package proxy provides high-performance, zero-allocation proxy dialers and tunneling engines.
//
// This package implements zero-allocation proxy dialers that route network connections through
// intermediary servers, supporting SOCKS5 (RFC 1928, RFC 1929) and HTTP CONNECT tunneling (RFC 9110 §9.3.6).
// It supports environment-based proxy configuration, automatic URL parsing, credential authentication,
// and per-host routing rules. Callers can compose proxy dialers to chain connections or bypass proxies
// for specific networks.
//
// # Compared to the standard library
//
// Stdlib counterpart: golang.org/x/net/proxy
//
// Rejected compromise: standard library proxy packages suffered from goroutine leaks on cancelled dials,
// lacked first-class HTTP CONNECT tunneling, and incurred unconstrained heap allocations on every connection.
//
// Accepted cost: fixed stack buffers limit raw handshake headers to 1KB for HTTP CONNECT and 512B for SOCKS5,
// strictly adhering to protocol limits without heap allocation.
//
// Allocations: 0 allocs/op on handshake hot paths.
package proxy
