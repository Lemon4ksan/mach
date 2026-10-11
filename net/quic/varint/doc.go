// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package varint implements QUIC-style variable-length integers (RFC 9000).
//
// The QUIC wire format extensively uses variable-length integers. This package
// provides fast, zero-allocation encoding and decoding routines using Go 1.23
// push iterators for sequential reading directly from packet payloads.
//
// # Compared to the standard library
//
// Stdlib counterpart: encoding/binary
//
// Rejected compromise: encoding/binary.Read uses reflection and generic io.Reader interfaces which are too slow for per-packet line-rate processing.
//
// Accepted cost: callers must buffer packets into byte slices rather than reading streams on the fly.
//
// Allocations: zero
package varint
