// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package protocol defines core types, invariants, packet layouts, and numerical limits
// for the QUIC transport protocol (RFC 9000).
//
// # Architecture
//
// This package centralizes protocol invariants and constants shared across wire parsing,
// packet processing, stream multiplexing, and loss recovery engines:
//
//   - Packet Numbers ([PacketNumber], [PacketNumberLen]):
//     62-bit monotonically increasing sequence numbers and variable-length integer representations (RFC 9000 §12.3).
//   - Stream IDs & Types ([StreamID], [StreamNum]):
//     32-bit stream identifiers encoding stream initiator (client/server) and directionality
//     (bidirectional/unidirectional) in the least significant 2 bits (RFC 9000 §2.1).
//   - Encryption Levels ([EncryptionLevel]):
//     Four cryptographic tiers (Initial, Handshake, 0-RTT, 1-RTT) isolating packet number spaces
//     and key schedules (RFC 9001 §4).
//   - Connection IDs ([ConnectionID]):
//     Variable-length endpoint routing identifiers (0 to 20 octets) enabling multi-path routing
//     and connection migration (RFC 9000 §5.1).
//   - ECN & Network Feedback ([ECN]):
//     Explicit Congestion Notification codepoints (Not-ECT, ECT(0), ECT(1), CE) parsed from IP headers (RFC 3168).
//
// # RFC Specifications
//
//   - RFC 9000: QUIC: A UDP-Based Multiplexed and Secure Transport
//   - RFC 9001: Using TLS to Secure QUIC
//   - RFC 9002: QUIC Loss Detection and Congestion Control
//   - RFC 9369: QUIC Version 2
//   - RFC 3168: The Addition of Explicit Congestion Notification (ECN) to IP
//
// # Compared to the standard library
//
// Stdlib counterpart: none - fills gap: QUIC protocol primitives
//
// Rejected compromise: Floating-point or interface-based protocol abstractions.
//
// Accepted cost: Strongly typed bitmask and integer types optimized for BCE and register allocation.
//
// Allocations: zero
package protocol
