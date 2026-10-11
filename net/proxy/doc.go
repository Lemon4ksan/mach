// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package proxy provides support for a variety of protocols to proxy network data.
//
// This package implements proxy dialers that route network connections through
// intermediary servers. It supports environment-based proxy configuration, SOCKS5,
// and per-host routing rules. Callers can compose proxy dialers to chain connections
// or bypass proxies for specific networks. Note that this package includes HTTP
// proxy support, which operates at layer 7.
//
// # Compared to the standard library
//
// Stdlib counterpart: golang.org/x/net/proxy
//
// Rejected compromise: standard library proxy packages often lack deep context propagation or active proxy probing.
//
// Accepted cost: proxy chains increase connection setup latency and require manual management of fallback dialers.
//
// Allocations: unconstrained
package proxy
