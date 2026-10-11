// Copyright (c) 2016 the quic-go authors. All rights reserved.
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package quic provides an RFC 9000 compliant, zero-allocation QUIC transport protocol implementation.
//
// # Architecture
//
// Derived from quic-go and stripped of redundant abstractions, this package is designed
// exclusively for line-rate packet processing over UDP. The engine manages packet packing,
// unpacking, stream multiplexing, loss detection, congestion control, and connection state.
//
// Key architectural subsystems include:
//
//   - Packet Pipeline ([packetPacker], [packetUnpacker]):
//     Constructs coalesced datagrams (Initial, Handshake, 0-RTT/1-RTT) and unpacks incoming
//     datagrams with constant-time side-channel protection during header unmasking and AEAD opening.
//   - Frame Sorter & Reassembly ([frameSorter]):
//     Reconstructs contiguous stream byte sequences from out-of-order STREAM frames using a
//     hybrid 16-slot ringbuffer and linked-list gap tracker without heap churn on in-order arrivals.
//   - Priority Framer ([framer]):
//     Dual-engine stream scheduling engine combining RFC 9218 urgency tiers with round-robin
//     fair queuing (incremental streams) and min-heap priority drains (non-incremental streams).
//   - Flow Control Engine ([connectionFlowController], [streamFlowController]):
//     Multi-tier connection-level and stream-level flow credit tracking featuring Bandwidth-Delay
//     Product (BDP) auto-tuning epochs.
//
// # Protocol and Transport Model
//
// QUIC replaces TCP's single byte-stream abstraction with an encrypted, multiplexed transport
// running over UDP (RFC 9000 §1):
//
//   - Packet Spaces: Independent packet number spaces for Initial, Handshake, and Application Data
//     (0-RTT and 1-RTT), preventing cross-encryption-level ACK confusion.
//   - Connection Migration & Multi-Path Validation: Active Connection IDs allow seamless endpoint
//     migration across IP/port changes without terminating existing streams (RFC 9000 §9).
//   - Loss Detection & Recovery: Packet numbers increase strictly monotonically, eliminating
//     TCP retransmission ambiguity and enabling precise RTT calculation (RFC 9002 §5).
//
// # RFC Specifications
//
// This transport engine conforms to the following standards:
//
//   - RFC 9000: QUIC: A UDP-Based Multiplexed and Secure Transport
//   - RFC 9001: Using TLS to Secure QUIC
//   - RFC 9002: QUIC Loss Detection and Congestion Control
//   - RFC 8899: Packetization Layer Path MTU Discovery for Datagram Transports (DPLPMTUD)
//   - RFC 9218: Extensible Prioritization Scheme for HTTP (Urgency and Incremental Multiplexing)
//   - RFC 9221: An Unreliable Datagram Extension to QUIC
//   - RFC 9369: QUIC Version 2
//
// # Compared to the standard library
//
// Stdlib counterpart: none - fills gap: QUIC transport protocol
//
// Rejected compromise: Standard io.Reader / io.Writer interfaces on packet hot paths in favor
// of zero-allocation frame iterators and off-heap memory reuse.
//
// Accepted cost: Callers provide initialized network sockets ([net.PacketConn]) and TLS configuration.
//
// Allocations: amortized zero
package quic
