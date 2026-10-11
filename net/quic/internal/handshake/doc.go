// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package handshake implements TLS 1.3 cryptographic key establishment, header protection,
// and token authentication for QUIC transports (RFC 9001).
//
// # Architecture
//
// QUIC delegates cryptographic negotiation to TLS 1.3 while managing its own record framing,
// packet protection, and key updates (RFC 9001 §1).
//
// The handshake package provides:
//
//   - Crypto Setup ([CryptoSetup]):
//     Bridges QUIC packet processing with the Go TLS 1.3 engine. Dispatches secrets for Initial,
//     Handshake, 0-RTT, and 1-RTT encryption levels.
//   - Key Derivation & HKDF ([hkdfExpandLabel]):
//     Derives encryption keys, IVs, and header protection keys using QUIC HKDF labels (RFC 9001 §5.1 & §5.2).
//   - Header Protection ([HeaderProtector]):
//     Applies and removes header protection masks over packet number fields using 16-byte ciphertext samples
//     to prevent observer packet number correlation (RFC 9001 §5.4).
//   - Updatable AEAD ([UpdatableAEAD]):
//     Manages 1-RTT application traffic keys and executes coordinated key phase updates (RFC 9001 §6).
//   - Address Validation Tokens ([TokenGenerator], [TokenProtector]):
//     Generates and authenticates encrypted Retry tokens and stateless reset tokens (RFC 9000 §8.1 & §10.3).
//
// # Cryptographic Suites
//
// The package supports the mandatory QUIC cipher suites (RFC 9001 §5.3):
//   - TLS_AES_128_GCM_SHA256
//   - TLS_AES_256_GCM_SHA384
//   - TLS_CHACHA20_POLY1305_SHA256
//
// # RFC Specifications
//
//   - RFC 9001: Using TLS to Secure QUIC
//   - RFC 9000 §8: Address Validation and Retry
//   - RFC 9000 §10.3: Stateless Reset
//   - RFC 8446: The Transport Layer Security (TLS) Protocol Version 1.3
//
// # Compared to the standard library
//
// Stdlib counterpart: crypto/tls (fills gap: QUIC TLS 1.3 record layer integration)
//
// Rejected compromise: Standard TLS record layer framing which conflicts with QUIC packet boundaries.
//
// Accepted cost: Callers provide TLS configurations and certificates.
//
// Allocations: amortized zero during steady-state 1-RTT operation
package handshake
