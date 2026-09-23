// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"io"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/net/quic"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

// acceptUniStreams continuously accepts client-initiated unidirectional streams
// and dispatches them to handleUniStream in background goroutines (RFC 9114 §6.2).
func (sc *ServerConn) acceptUniStreams() {
	ctx := sc.quicConn.Context()
	for {
		stream, err := sc.quicConn.AcceptUniStream(ctx)
		if err != nil {
			return
		}

		go sc.handleUniStream(stream)
	}
}

// handleUniStream routes an incoming unidirectional stream according to its stream type
// (RFC 9114 §6.2): Control (0x00), QPACK Encoder (0x02), or QPACK Decoder (0x03).
//
// Inbound streams are checked to enforce that only a single instance of each stream type exists per
// connection; receipt of a duplicate stream results in connection termination with H3_STREAM_CREATION_ERROR.
// Unknown stream types are gracefully drained and ignored per RFC 9114 §6.2.
func (sc *ServerConn) handleUniStream(stream *quic.ReceiveStream) {
	defer stream.CancelRead(0)

	qr := varint.NewReader(stream)

	streamType, err := varint.Read(qr)
	if err != nil {
		return
	}

	switch streamType {
	case coreh3.StreamTypeControl:
		// RFC 9114 §6.2.1: Only one control stream per peer is permitted
		if sc.hasControlIn.Swap(true) {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate control stream (RFC 9114 §6.2.1)",
			)

			return
		}

		// Read peer settings frame (RFC 9114 §6.2.1 & §7.2.4)
		frameType, err := varint.Read(qr)
		if err != nil || frameType != coreh3.FrameTypeSettings {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3MissingSettings),
				"missing SETTINGS on control stream (RFC 9114 §6.2.1)",
			)

			return
		}

		frameLen, err := varint.Read(qr)
		if err != nil {
			return
		}

		if frameLen > 0 {
			lr := io.LimitReader(stream, int64(frameLen)) //nolint:gosec // bounds checked
			_, _ = io.Copy(io.Discard, lr)
		}

	case coreh3.StreamTypeQPACKEncoder:
		if sc.hasQPACKEncoder.Swap(true) {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate QPACK encoder stream",
			)

			return
		}

		_, _ = io.Copy(io.Discard, stream)

	case coreh3.StreamTypeQPACKDecoder:
		if sc.hasQPACKDecoder.Swap(true) {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate QPACK decoder stream",
			)

			return
		}

		_, _ = io.Copy(io.Discard, stream)

	default:
		// RFC 9114 §6.2: Unknown unidirectional stream types MUST either be discarded or cancelled
		_, _ = io.Copy(io.Discard, stream)
	}
}
