// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package testutils provides packet injection and MITM simulation tools for QUIC.
//
// This package contains utilities to serialize and intercept QUIC frames and packets
// for rigorous transport testing. It is not intended for production use.
//
// # Compared to the standard library
//
// Stdlib counterpart: none - fills gap: QUIC protocol test harnesses
//
// Rejected compromise: none.
//
// Accepted cost: the API is unstable and freely allocates memory for ease of test writing.
//
// Allocations: unconstrained
package testutils
