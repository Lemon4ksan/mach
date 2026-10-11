// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"io"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

// Settings represents HTTP/3 configuration parameters exchanged on the control stream (RFC 9114 §7.2.4).
type Settings = coreh3.Settings

// QPACKCodec encapsulates QPACK compression state for HTTP/3 headers (RFC 9204).
type QPACKCodec = coreh3.QPACKCodec

// NewQPACKCodec initializes a new QPACKCodec with default table capacities (RFC 9204 §4).
func NewQPACKCodec() *QPACKCodec { return coreh3.NewQPACKCodec() }

// FrameTypeHeaders identifies the HTTP/3 HEADERS frame type (0x01) (RFC 9114 §7.2.2).
const FrameTypeHeaders = coreh3.FrameTypeHeaders

// ReadResponseFromForTest exports readResponseFrom for package tests and e2e boundary validation.
func (cc *ClientConn) ReadResponseFromForTest(
	reader io.Reader,
	resp *h1.Response,
	streamID uint64,
) (map[string][]string, error) {
	return cc.readResponseFrom(reader, resp, streamID)
}

// NewClientConnWithCodec initializes an offline ClientConn with specified QPACKCodec for testing.
func NewClientConnWithCodec(codec *QPACKCodec) *ClientConn {
	return &ClientConn{
		qpack: codec,
	}
}

