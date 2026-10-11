// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

// Settings represents HTTP/3 configuration parameters exchanged on the control stream (RFC 9114 §7.2.4).
type Settings = coreh3.Settings

// QPACKCodec encapsulates QPACK compression state for HTTP/3 headers (RFC 9204).
type QPACKCodec = coreh3.QPACKCodec

// NewQPACKCodec initializes a new QPACKCodec with default table capacities (RFC 9204 §4).
func NewQPACKCodec() *QPACKCodec { return coreh3.NewQPACKCodec() }

// FrameTypeHeaders identifies the HTTP/3 HEADERS frame type (0x01) (RFC 9114 §7.2.2).
const FrameTypeHeaders = coreh3.FrameTypeHeaders

// ReadFrameHeader parses an HTTP/3 frame type and payload length from a varint reader (RFC 9114 §7.1).
var ReadFrameHeader = coreh3.ReadFrameHeader

