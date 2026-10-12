// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package proxy implements zero-allocation proxy dialers and tunnel negotiation engines
// supporting SOCKS5 and HTTP CONNECT protocols.
//
// # Architecture
//
// Proxy routing coordinates transport establishment across intermediary nodes before handing
// off clean byte streams to application callers:
//
//   - Dialer Contracts ([Dialer], [ContextDialer]):
//     Universal interfaces providing standard and context-bound connection establishment through
//     proxy chains.
//   - Dialing Pipelines ([FromURL], [FromEnvironment], [Direct]):
//     Resolves proxy topologies automatically from environment variables (HTTP_PROXY, HTTPS_PROXY,
//     ALL_PROXY, NO_PROXY) or parsed URLs (socks5://, socks5h://, http://, https://).
//   - HTTP CONNECT Tunneling ([HTTPConnect], [ConnectOption]):
//     Establishes end-to-end TCP and TLS tunnels through HTTP forward proxies conforming to RFC 9110 §9.3.6,
//     supporting Basic authentication ([WithConnectAuth]), arbitrary proxy headers ([WithConnectHeaders]),
//     and upstream TLS handshakes ([WithConnectTLS]).
//   - Host-Based Routing ([PerHost]):
//     Directs traffic selectively through proxies or directly to destination targets based on
//     CIDR blocks, loopback designations, and domain pattern matches.
//
// # Memory and Buffer Boundaries
//
// Handshake framing operates entirely on fixed stack buffers (1KB for HTTP CONNECT headers,
// 512B for SOCKS5 greeting and authentication frames). Dials execute with zero heap allocations
// on the connection establishment path.
//
// # RFC Specifications
//
//   - RFC 1928: SOCKS Protocol Version 5
//   - RFC 1929: Username/Password Authentication for SOCKS V5
//   - RFC 9110 §9.3.6: HTTP CONNECT Method
//
// # Compared to the standard library
//
// Stdlib counterpart: golang.org/x/net/proxy
//
// Standard library proxy utilities lack native HTTP CONNECT support, leak background goroutines
// upon context cancellation during handshakes, and incur multiple heap allocations per dial.
// This implementation provides unified SOCKS5 and HTTP CONNECT engines with zero heap allocations
// and prompt socket termination on cancellation.
//
// Allocations: 0 allocs/op on handshake hot paths.
package proxy
