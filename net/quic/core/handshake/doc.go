// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package handshake implements a modern, clean-room TLS 1.3 cryptographic handshake
// session driver for QUIC (RFC 9001 and RFC 9369) directly on the Go standard library
// crypto/tls.QUICConn API without legacy assembly or internal compiler shims.
//
// # Design Principles
//
// 1. Native Go Standard Library crypto/tls:
// Handshake session state is driven directly via standard library crypto/tls.QUICConn
// and crypto/tls.QUICConfig for both Client and Server roles.
//
// 2. Pure Event-Driven Processing:
// Comprehensive event loop handling QUICSetReadSecret, QUICSetWriteSecret,
// QUICWriteData, QUICTransportParameters, QUICTransportParametersRequired,
// and QUICHandshakeDone across Initial, Handshake, 1-RTT, and Early (0-RTT) levels.
//
// 3. Zero-Allocation Transport Parameters (RFC 9000 §18.2):
// Full support for all 17 standard parameters plus RFC 9221 datagram extension,
// operating with 0 allocations on both encoding and parsing hot paths.
//
// 4. Dual-Version RFC 9001 & RFC 9369 Initial Key Derivation:
// Bit-for-bit parity with RFC 9001 Appendix A and RFC 9369 QUIC v2 golden vectors
// for Initial secrets, packet protection keys, IVs, and header protectors.
package handshake
