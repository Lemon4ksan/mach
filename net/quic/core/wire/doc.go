// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package wire provides an ultra-lightweight, zero-allocation wire and packet engine
// for QUIC (RFC 9000, RFC 9001, and RFC 9369).
//
// # Design Principles
//
// 1. Zero Allocations on Hot Path:
// All frame encoding/decoding, packet header parsing/serialization, header protection
// mask generation, and AEAD packet seal/open operations are strictly zero-allocation.
// Testing verifies 0 allocs/op using testing.AllocsPerRun.
//
// 2. Dual-Version RFC 9000 & RFC 9369 Support:
// Full support for QUIC Version 1 (RFC 9000) and QUIC Version 2 (RFC 9369), including
// version-specific packet type mappings and Initial salt vectors.
//
// 3. Complete RFC 9000 Frame Taxonomy:
// Exhaustive codecs for all RFC 9000 frame types (PADDING, PING, ACK, RESET_STREAM,
// STOP_SENDING, CRYPTO, NEW_TOKEN, STREAM, MAX_DATA, MAX_STREAM_DATA, MAX_STREAMS,
// DATA_BLOCKED, STREAM_DATA_BLOCKED, STREAMS_BLOCKED, NEW_CONNECTION_ID,
// RETIRE_CONNECTION_ID, PATH_CHALLENGE, PATH_RESPONSE, CONNECTION_CLOSE, HANDSHAKE_DONE)
// and RFC 9221 DATAGRAM frames.
//
// 4. Clean Cryptographic Core:
// Hardware-accelerated AEAD packet protection (AES-128-GCM, AES-256-GCM, and
// ChaCha20-Poly1305) and zero-alloc header protection (AES-ECB and ChaCha20).
package wire
