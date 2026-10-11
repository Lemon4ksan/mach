// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ackhandler manages QUIC packet acknowledgment tracking, loss detection,
// probe timeout (PTO) scheduling, and Explicit Congestion Notification (ECN) accounting (RFC 9002).
//
// # Architecture
//
// Reliable delivery in QUIC requires separate tracking for packets sent into the network
// and packets received from the peer across three distinct packet number spaces (Initial,
// Handshake, and Application Data).
//
// The ackhandler package is partitioned into two cooperating subsystems:
//
//   - Sent Packet Handler ([SentPacketHandler], [sentPacketHistory]):
//     Maintains an array-indexed sliding ring buffer of outstanding packets. Computes smoothed
//     round-trip time (SRTT), RTT variation (RTTVAR), loss detection thresholds (kTimeThreshold,
//     kPacketThreshold), and PTO timer deadlines (RFC 9002 §5 & §6).
//   - Received Packet Handler ([ReceivedPacketHandler], [receivedPacketHistory]):
//     Tracks received packet numbers within bounded interval slices (`[4]interval` inline storage).
//     Calculates unacknowledged gaps, suppresses duplicate packets, and formats ACK and ACK_ECN
//     frames (RFC 9000 §13.2 & RFC 9002 §6).
//
// # Loss Detection Model
//
// Packets are declared lost when:
//   - Time Threshold (RFC 9002 §6.1.2): A packet was sent prior to a packet that was acknowledged
//     at least `timeThreshold * max(SRTT, latestRTT)` earlier.
//   - Packet Threshold (RFC 9002 §6.1.1): A packet was sent at least `kPacketThreshold` (3) packets
//     before the largest acknowledged packet.
//
// # RFC Specifications
//
//   - RFC 9000: QUIC: A UDP-Based Multiplexed and Secure Transport (Section 13 Packet Handling)
//   - RFC 9002: QUIC Loss Detection and Congestion Control
//   - RFC 3168: The Addition of Explicit Congestion Notification (ECN) to IP
//
// # Compared to the standard library
//
// Stdlib counterpart: none - fills gap: QUIC loss detection and acknowledgment engine
//
// Rejected compromise: Generic heap-allocated timer queues and slice allocations on packet reception.
//
// Accepted cost: Fixed internal bounds (e.g. MaxNumAckRanges = 64) for bounded packet history memory.
//
// Allocations: amortized zero on in-order network paths
package ackhandler
