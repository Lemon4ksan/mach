// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package client provides high-performance client transport engines, connection pooling,
// multi-protocol session abstractions, and zero-allocation proxy integration across HTTP/1.1, HTTP/2, and HTTP/3.
//
// It delivers lock-minimized, host-partitioned connection reuse and lifecycle management
// to ensure zero allocation drag during client session acquisition, request dispatch,
// and proxy tunnel negotiation. Callers can configure proxy routing across transports via [WithProxy].
package client
