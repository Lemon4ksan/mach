// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package congestion provides congestion control, packet pacing, and bandwidth estimation
// algorithms for QUIC transports (RFC 9002).
//
// # Architecture
//
// Congestion control regulates the transmission rate of an endpoint to avoid saturating
// bottleneck routers along the network path.
//
// The congestion package provides the following components:
//
//   - CUBIC Congestion Control ([Cubic], [CubicSender]):
//     Fixed-point implementation of the CUBIC polynomial window growth function (RFC 8312)
//     with TCP-friendly Reno emulation and aggressive recovery in high-bandwidth-delay networks.
//   - Hybrid Slow Start ([HybridSlowStart]):
//     Detects incipient queuing delay during slow start to exit before packet loss occurs (RFC 9002 §7.3.1).
//   - Packet Pacer ([Pacer]):
//     Smooths bursty transmissions across time intervals to prevent buffer overflows in intermediate
//     network switches (RFC 9002 §7.7).
//   - Bandwidth Estimator ([Bandwidth]):
//     Line-rate bandwidth calculation based on delivered bytes across round-trip time samples.
//
// # RFC Specifications
//
//   - RFC 9002: QUIC Loss Detection and Congestion Control (Section 7 Congestion Control)
//   - RFC 8312: CUBIC for Fast Long-Distance Networks
//   - RFC 6582: The NewReno Modification to TCP's Fast Recovery Algorithm
//
// # Compared to the standard library
//
// Stdlib counterpart: none - fills gap: transport congestion control and packet pacing
//
// Rejected compromise: Dynamic allocations during window adjustment and RTT calculation.
//
// Accepted cost: Fixed-point arithmetic and bitwise scaling factors for fast integer evaluation.
//
// Allocations: zero
package congestion
