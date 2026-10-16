// Copyright (c) 2016 the quic-go authors. All rights reserved.
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package quic provides an RFC 9000 compliant, zero-allocation QUIC transport protocol implementation.
//
// # Clean-Room Core Integration & Architecture
//
// The transport architecture integrates clean-room core engines
// designed for line-rate packet processing, zero allocations on steady-state hot paths,
// and strict asymmetric isolation between L7 application handlers and L4/L5 transport congestion:
//
//   - Zero-Alloc Wire Codec ([net/quic/core/wire]):
//     Zero-heap-allocation packet header parsing/serialization (RFC 9000 §17, RFC 9369 §4),
//     in-place header protection unmasking/application (AES-ECB, ChaCha20), zero-alloc packet
//     number reconstruction, and push-iterator frame parsing ([wire.Frames]) yielding STREAM,
//     ACK, RESET_STREAM, STOP_SENDING, CRYPTO, and flow control frames with zero allocations.
//
//   - Native TLS 1.3 Handshake Driver ([net/quic/core/handshake]):
//     Direct integration with Go stdlib [crypto/tls.QUICConn] (RFC 9001), managing state transitions
//     across Initial, Handshake, 0-RTT, and 1-RTT encryption levels. Automates HKDF-SHA256/SHA384
//     traffic key derivation, transport parameter encoding/decoding, and TLS session events.
//
//   - Clean Stream Engine & Multiplexer ([net/quic/core/stream]):
//     Stream state machines conforming strictly to RFC 9000 §3, featuring multi-tier flow control
//     with BDP auto-tuning, zero-alloc out-of-order frame reassembly buffers with strict FinalSize
//     validation, and unified multiplexing for bidirectional and unidirectional streams.
//
// # Public Stream Types & Core Aliases
//
// Top-level stream abstractions in package quic cleanly expose the exact public methods
// and interfaces required for 100% interoperability with client/h3 and server/h3:
//
//   - [Conn]: QUIC connection session implementing [stream.ConnI], exposing OpenStream,
//     OpenStreamSync, OpenUniStream, OpenUniStreamSync, AcceptStream, AcceptUniStream,
//     CloseWithError, Close, Context, LocalAddr, and RemoteAddr.
//   - [Stream]: Bidirectional stream integrating [stream.Stream], exposing Read, Write,
//     Close, CloseWrite, CloseRead, CancelWrite, CancelRead, StreamID, Context, SetPriority,
//     and FinalSize.
//   - [SendStream]: Outbound unidirectional stream integrating [stream.SendStream], exposing
//     Write, Close, CancelWrite, StreamID, Context, and SetPriority.
//   - [ReceiveStream]: Inbound unidirectional stream integrating [stream.ReceiveStream], exposing
//     Read, Close, CancelRead, StreamID, and FinalSize.
//   - [StreamID]: 62-bit QUIC stream identifier conforming to [stream.StreamID].
//   - [StreamErrorCode]: Stream termination code conforming to [stream.StreamErrorCode].
//   - [ApplicationErrorCode]: Application closure code conforming to [stream.ApplicationErrorCode].
//
// # Transport & Dispatch Model
//
//   - [Transport]: Central management point handling incoming and outgoing QUIC connections over a
//     shared [net.PacketConn] socket. Features Connection ID (CID) demultiplexing to route packets
//     to active connection sessions.
//   - [Listener] & [ListenAddr]: Accepts incoming QUIC connections spawned from Initial packets.
//   - [Dial] & [DialAddr]: Establishes outbound QUIC sessions to remote servers.
//   - Asymmetric Isolation: L7 server handler execution is fully isolated from packet fragmentation,
//     malformed frames, and transport congestion. Malformed frames are cleanly rejected at the wire
//     parser without propagating invalid state or panicking server handlers.
//
// # RFC Specifications
//
// This transport engine conforms to the following standards:
//
//   - RFC 9000: QUIC: A UDP-Based Multiplexed and Secure Transport
//   - RFC 9001: Using TLS to Secure QUIC
//   - RFC 9002: QUIC Loss Detection and Congestion Control
//   - RFC 8899: Packetization Layer Path MTU Discovery for Datagram Transports (DPLPMTUD)
//   - RFC 9114: HTTP/3
//   - RFC 9218: Extensible Prioritization Scheme for HTTP (Urgency and Incremental Multiplexing)
//   - RFC 9221: An Unreliable Datagram Extension to QUIC
//   - RFC 9369: QUIC Version 2
//
// Allocations: amortized zero on hot path
package quic
