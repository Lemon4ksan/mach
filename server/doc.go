// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package server provides the unified multi-protocol L7 application server for Mach.
//
// It binds the underlying HTTP/1.1, HTTP/2, and HTTP/3 protocol engines under a single
// universal request/response Context and Handler interface. It provides transparent
// TLS ALPN protocol negotiation across HTTP/1.1 and HTTP/2 on a single TCP listener,
// and automated Alt-Svc advertisement for HTTP/3 over QUIC.
package server
