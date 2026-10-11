// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package qerr defines QUIC transport error codes, cryptographic alert mappings,
// and connection termination error structures (RFC 9000 §20).
//
// # Architecture
//
// QUIC error handling distinguishes between transport-level failures (which close the
// connection or reset individual streams) and application-defined failures (RFC 9000 §10).
//
// The qerr package provides:
//
//   - Transport Error Codes ([TransportErrorCode]):
//     Standardized 62-bit error codes (NO_ERROR, PROTOCOL_VIOLATION, FLOW_CONTROL_ERROR, etc.)
//     transmitted inside CONNECTION_CLOSE frames (RFC 9000 §20.1).
//   - Crypto Alerts ([TransportErrorCode.IsCryptoError]):
//     Maps TLS 1.3 alert codes into the reserved QUIC error code range [0x0100, 0x01ff] (RFC 9000 §20.1).
//   - Structured Error Types ([TransportError], [ApplicationError], [VersionNegotiationError], [StatelessResetError]):
//     Rich diagnostics encapsulating triggering frame types and human-readable reason phrases.
//
// # RFC Specifications
//
//   - RFC 9000 §10: Connection Termination
//   - RFC 9000 §20: Error Codes
//   - RFC 8446: The Transport Layer Security (TLS) Protocol Version 1.3 (Alert Protocol)
//
// # Compared to the standard library
//
// Stdlib counterpart: none - fills gap: QUIC transport error taxonomy
//
// Rejected compromise: Untyped string errors or generic net.OpError wrapping.
//
// Accepted cost: Strict adherence to RFC 9000 62-bit wire integer codes.
//
// Allocations: zero for error codes; minimal for structured error instances
package qerr
