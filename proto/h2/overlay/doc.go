// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package overlay provides in-situ, zero-copy HTTP/2 frame inspection and payload
// extraction directly over raw byte buffers (RFC 9113).
//
// # Architecture
//
// Traditional HTTP/2 frame deserializers decode incoming network streams into heap-allocated
// or pooled Go structs. For high-throughput proxying, packet inspection, and firewalling, this
// creates garbage collection overhead and memory movement.
//
// The overlay package eliminates decoding by treating raw byte slices as in-situ frames:
//
//   - Frame Overlay ([Frame]): Slices the fixed 9-octet HTTP/2 wire header using Bounds Check
//     Elimination (BCE) hints to extract Length (24-bit), Type (8-bit), Flags (8-bit), and
//     StreamID (31-bit) with zero memory allocations and zero copies.
//   - Specialized In-Situ Frames ([DataFrame], [HeadersFrame]): Typed views offering zero-copy
//     padding stripping ([DataFrame.Data]), end-of-stream detection ([DataFrame.IsEndStream]),
//     and header block fragment access ([HeadersFrame.HeaderBlockFragment]).
//
// # Memory and Performance Model
//
// Every operation in this package operates directly on caller-provided wire buffers:
//
//   - Zero Heap Allocations: Frame views are type-aliased byte slices (`type Frame []byte`).
//   - Single-Instruction BCE: Compiler bounds check elimination hints allow multi-byte integer assembly
//     without branch instructions.
//   - In-Place Slicing: Unpadded application data is returned as a sub-slice of the input buffer.
//
// # RFC Specifications
//
//   - RFC 9113 §4.1: Frame Format (9-octet header, 24-bit length, 31-bit stream ID)
//   - RFC 9113 §4.2: Frame Size Constraints
//   - RFC 9113 §6.1: DATA Frame Parsing and Padding Removal
//   - RFC 9113 §6.2: HEADERS Frame Parsing and Header Block Access
//
// # Compared to the standard library
//
// Stdlib counterpart: golang.org/x/net/http2 (Framer)
//
// Rejected compromise: Struct allocation and payload copying during packet decoding.
//
// Accepted cost: Callers must verify buffer validity ([Frame.IsValid]) prior to payload access.
//
// Allocations: zero
package overlay
