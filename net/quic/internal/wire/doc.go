// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package wire provides low-level binary frame parsing, frame serialization, packet header
// encoding, and transport parameter negotiation for QUIC (RFC 9000).
//
// # Architecture
//
// The wire package constitutes the wire-protocol serialization tier of the QUIC engine.
// It converts between raw UDP payload bytes and structured Go representations with zero
// unnecessary heap allocations via pooled frame buffers.
//
// The package is structured into three primary subsystems:
//
//   - Packet Headers ([Header], [ExtendedHeader], [ParseShortHeader], [AppendShortHeader]):
//     Decodes and serializes Long Headers (Initial, 0-RTT, Handshake, Retry) and Short Headers
//     (1-RTT) with bounds check validation and reserved bit integrity checks (RFC 9000 §17).
//   - Frame Engine ([FrameParser], [Frame]):
//     Zero-allocation parsing pipeline decoding frame types ([ParseType]), common stream frames
//     ([ParseStreamFrame]), ACK frames ([ParseAckFrame]), datagrams, and control frames (RFC 9000 §19).
//   - Transport Parameters ([TransportParameters]):
//     Encodes and validates TLS extension parameters exchanged during the cryptographic
//     handshake (RFC 9000 §18).
//
// # Frame Taxonomy and Encoding Model
//
// QUIC packets convey one or more frames with variable-length integer type identifiers (RFC 9000 §19):
//
//   - PADDING (0x00, RFC 9000 §19.1): Expands packets to minimum size constraints (e.g. 1200 bytes for Initial).
//   - PING (0x01, RFC 9000 §19.2): Verifies path liveness and solicits ACK frames.
//   - ACK / ACK_ECN (0x02-0x03, RFC 9000 §19.3): Reports received packet number ranges and ECN counters.
//   - RESET_STREAM (0x04, RFC 9000 §19.4) & STOP_SENDING (0x05, RFC 9000 §19.5): Immediate stream termination.
//   - CRYPTO (0x06, RFC 9000 §19.6): Carries cryptographic handshake records for TLS 1.3.
//   - NEW_TOKEN (0x07, RFC 9000 §19.7): Issues future connection validation tokens to clients.
//   - STREAM (0x08-0x0f, RFC 9000 §19.8): Carries arbitrary application data with optional FIN, LEN, and OFF flags.
//   - MAX_DATA (0x10, RFC 9000 §19.9) & MAX_STREAM_DATA (0x11, RFC 9000 §19.10): Flow control credit grants.
//   - MAX_STREAMS (0x12-0x13, RFC 9000 §19.11): Advertises maximum cumulative bidirectional or unidirectional streams.
//   - DATA_BLOCKED (0x14, RFC 9000 §19.12) & STREAM_DATA_BLOCKED (0x15, RFC 9000 §19.13): Signals flow credit exhaustion.
//   - STREAMS_BLOCKED (0x16-0x17, RFC 9000 §19.14): Signals stream concurrency exhaustion.
//   - NEW_CONNECTION_ID (0x18, RFC 9000 §19.15) & RETIRE_CONNECTION_ID (0x19, RFC 9000 §19.16): Connection migration management.
//   - PATH_CHALLENGE (0x1a, RFC 9000 §19.17) & PATH_RESPONSE (0x1b, RFC 9000 §19.18): Path validation round-trips.
//   - CONNECTION_CLOSE (0x1c-0x1d, RFC 9000 §19.19): Signals transport or application level connection termination.
//   - HANDSHAKE_DONE (0x1e, RFC 9000 §19.20): Confirms handshake completion from server to client.
//   - DATAGRAM (0x30-0x31, RFC 9221): Unreliable, out-of-order application datagrams.
//
// # RFC Specifications
//
//   - RFC 9000: QUIC: A UDP-Based Multiplexed and Secure Transport
//   - RFC 9001: Using TLS to Secure QUIC (Header Protection and Sample Extraction)
//   - RFC 9221: An Unreliable Datagram Extension to QUIC
//
// # Compared to the standard library
//
// Stdlib counterpart: none - fills gap: QUIC binary wire protocol codecs
//
// Rejected compromise: Reflection-based binary decoding and io.Reader interface allocations.
//
// Accepted cost: Frame parsers operate on contiguous byte slices and require explicit buffer lifetime management.
//
// Allocations: amortized zero
package wire
