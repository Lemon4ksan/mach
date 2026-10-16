// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package stream implements high-performance, zero-allocation stream multiplexing,
// flow control, and reassembly for QUIC connections (RFC 9000 §2, §3, and §4).
//
// # Design Principles
//
// 1. Strict RFC 9000 §3 Stream State Machine:
// Separate, rigorous state machines for Send (Ready, Send, DataSent, DataRecvd, ResetSent, ResetRecvd)
// and Receive (Recv, SizeKnown, DataRecvd, DataRead, ResetRead) stream states with
// cancellation, deadlines, and graceful completion tracking.
//
// 2. High-Throughput Sparse Reassembly:
// Efficient chunk reassembly buffer supporting out-of-order, overlapping, duplicate,
// and reverse arrival of data chunks, with strict FinalSizeError validation and 0-alloc Peek/Read.
//
// 3. Credit-Based Dual Flow Control:
// Window-based flow enforcement for per-stream and connection-level byte limits,
// featuring RTT-based auto-tuning window growth and limit violation detection.
//
// 4. HTTP/3 Drop-In Compatibility:
// Minimal, zero-drag drop-in quic.Conn and quic.Stream interfaces designed specifically
// for mach/client/h3 and mach/server/h3 protocol codecs.
package stream
