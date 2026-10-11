// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package varint implements QUIC variable-length integer encoding and decoding routines (RFC 9000).
//
// # Architecture
//
// The QUIC wire format extensively utilizes variable-length integers to encode frame types,
// packet numbers, stream identifiers, offsets, error codes, and configuration parameters.
//
// This package delivers zero-allocation encoding ([EncodeSlice], [Encode]) and decoding ([Decode])
// routines alongside Go 1.23 push iterators ([DecodeSeq], [Values]) for streaming inspection
// directly over packet buffers without intermediate slice allocations.
//
// # Encoding Specification
//
// Variable-length integers are encoded in 1, 2, 4, or 8 octets, identified by the two most
// significant bits (MSBs) of the initial octet (RFC 9000 §16, Table 4):
//
//   - 2MSB 00: 1 octet length, maximum value 63 (2^6 - 1)
//   - 2MSB 01: 2 octet length, maximum value 16,383 (2^14 - 1)
//   - 2MSB 10: 4 octet length, maximum value 1,073,741,823 (2^30 - 1)
//   - 2MSB 11: 8 octet length, maximum value 4,611,686,018,427,387,903 (2^62 - 1)
//
// # RFC Specifications
//
//   - RFC 9000 §16: Variable-Length Integer Encoding
//
// # Compared to the standard library
//
// Stdlib counterpart: encoding/binary
//
// Rejected compromise: Reflection and generic io.Reader interfaces in encoding/binary that induce heap allocations.
//
// Accepted cost: Callers operate directly on memory buffers rather than generic streaming readers.
//
// Allocations: zero
package varint
