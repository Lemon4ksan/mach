// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"errors"
	"io"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/mach/net/quic"

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
	qr := varint.NewReader(stream)

	streamType, err := varint.Read(qr)
	if err != nil {
		stream.CancelRead(0)
		return
	}

	switch streamType {
	case coreh3.StreamTypeControl:
		// RFC 9114 §6.2.1: Only one control stream per peer is permitted; receipt of a second
		// stream claiming to be a control stream MUST be treated as H3_STREAM_CREATION_ERROR.
		if sc.hasControlIn.Swap(true) {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate control stream (RFC 9114 §6.2.1)",
			)

			return
		}

		sc.handleControlStream(stream, qr)

	case coreh3.StreamTypePush:
		// RFC 9114 §6.2.2: A client MUST NOT initiate a push stream; a server MUST treat receipt
		// of a stream header with type 0x01 on a client-initiated unidirectional stream as a connection
		// error of type H3_STREAM_CREATION_ERROR.
		_ = sc.quicConn.CloseWithError(
			quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
			"client cannot initiate push stream (RFC 9114 §6.2.2)",
		)

		return

	case coreh3.StreamTypeQPACKEncoder:
		if sc.hasQPACKEncoder.Swap(true) {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate QPACK encoder stream (RFC 9114 §6.2 & RFC 9204 §4.2)",
			)

			return
		}

		_, err := sc.qpack.ReadEncoderStream(stream)
		if sc.qpack.Err() == nil && (err == nil || errors.Is(err, io.EOF)) {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3ClosedCriticalStream),
				"QPACK encoder stream closed (RFC 9114 §6.2.1 & RFC 9204 §4.2)",
			)
		}

	case coreh3.StreamTypeQPACKDecoder:
		if sc.hasQPACKDecoder.Swap(true) {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate QPACK decoder stream (RFC 9114 §6.2 & RFC 9204 §4.2)",
			)

			return
		}

		_, err := sc.qpack.ReadDecoderStream(stream)
		if sc.qpack.Err() == nil && (err == nil || errors.Is(err, io.EOF)) {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3ClosedCriticalStream),
				"QPACK decoder stream closed (RFC 9114 §6.2.1 & RFC 9204 §4.2)",
			)
		}

	default:
		// RFC 9114 §6.2: Unknown unidirectional stream types MUST either be discarded or cancelled.
		defer stream.CancelRead(0)
		_, _ = io.Copy(io.Discard, stream)
	}
}

func (sc *ServerConn) handleControlStream(stream *quic.ReceiveStream, qr varint.Reader) {
	firstFrame := true

	for {
		frameType, payloadLen, err := coreh3.ReadFrameHeader(qr)
		if err != nil {
			if errors.Is(err, io.EOF) {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3ClosedCriticalStream),
					"control stream closed unexpectedly (RFC 9114 §6.2.1)",
				)
			}

			return
		}

		if firstFrame && frameType != coreh3.FrameTypeSettings {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3MissingSettings),
				"missing SETTINGS on control stream (RFC 9114 §6.2.1)",
			)

			return
		}
		if !firstFrame && frameType == coreh3.FrameTypeSettings {
			// RFC 9114 §7.2.4: A second SETTINGS frame MUST cause H3_FRAME_UNEXPECTED.
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
				"second SETTINGS frame received on control stream (RFC 9114 §7.2.4)",
			)

			return
		}
		firstFrame = false

		switch frameType {
		case coreh3.FrameTypeSettings:
			st, err := coreh3.DecodeSettings(qr, payloadLen)
			if err != nil {
				var (
					msg  string
					code = coreh3.ErrCodeH3SettingsError
				)
				if errors.Is(err, coreh3.ErrH3SettingsError) {
					msg = "reserved H2 setting ID in SETTINGS (RFC 9114 §7.2.4.1)"
				} else {
					code = coreh3.ErrCodeH3FrameError
					msg = "truncated SETTINGS frame (RFC 9114 §7.1)"
				}
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(code),
					msg,
				)

				return
			}

			sc.qpack.ApplySettings(st.QpackMaxTableCap, st.QpackBlockedStreams)

		case coreh3.FrameTypeGoAway:
			// RFC 9114 §7.2.6: GOAWAY carries a single variable-length integer (Stream ID/Push ID).
			// Redundant or excess payload is H3_FRAME_ERROR (RFC 9114 §7.1).
			if payloadLen == 0 {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameError),
					"empty GOAWAY frame payload (RFC 9114 §7.1 & §7.2.6)",
				)

				return
			}

			lr := io.LimitReader(stream, int64(payloadLen)) //nolint:gosec
			vReader := varint.NewReader(lr)
			if _, err := varint.Read(vReader); err != nil {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameError),
					"malformed GOAWAY stream ID (RFC 9114 §7.1 & §7.2.6)",
				)

				return
			}

			var extra [1]byte
			if n, _ := lr.Read(extra[:]); n > 0 {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameError),
					"excess payload in GOAWAY frame (RFC 9114 §7.1 & §7.2.6)",
				)

				return
			}

			return

		case coreh3.FrameTypeCancelPush:
			// RFC 9114 §7.2.3: CANCEL_PUSH payload is a single Push ID varint.
			if payloadLen == 0 {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameError),
					"empty CANCEL_PUSH payload (RFC 9114 §7.1 & §7.2.3)",
				)

				return
			}

			lr := io.LimitReader(stream, int64(payloadLen)) //nolint:gosec
			vReader := varint.NewReader(lr)
			if _, err := varint.Read(vReader); err != nil {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameError),
					"malformed CANCEL_PUSH push ID (RFC 9114 §7.1 & §7.2.3)",
				)

				return
			}

			var extra [1]byte
			if n, _ := lr.Read(extra[:]); n > 0 {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameError),
					"excess payload in CANCEL_PUSH frame (RFC 9114 §7.1 & §7.2.3)",
				)

				return
			}

		case coreh3.FrameTypeMaxPushID:
			// RFC 9114 §7.2.7: MAX_PUSH_ID payload is a single Push ID varint.
			if payloadLen == 0 {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameError),
					"empty MAX_PUSH_ID payload (RFC 9114 §7.1 & §7.2.7)",
				)

				return
			}

			lr := io.LimitReader(stream, int64(payloadLen)) //nolint:gosec
			vReader := varint.NewReader(lr)
			if _, err := varint.Read(vReader); err != nil {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameError),
					"malformed MAX_PUSH_ID push ID (RFC 9114 §7.1 & §7.2.7)",
				)

				return
			}

			var extra [1]byte
			if n, _ := lr.Read(extra[:]); n > 0 {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameError),
					"excess payload in MAX_PUSH_ID frame (RFC 9114 §7.1 & §7.2.7)",
				)

				return
			}

		case coreh3.FrameTypeData, coreh3.FrameTypeHeaders, coreh3.FrameTypePushPromise:
			// RFC 9114 §7.2.1, §7.2.2, §7.2.5: DATA, HEADERS, PUSH_PROMISE MUST NOT be received on control stream
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
				"prohibited frame received on control stream (RFC 9114 §7.2)",
			)

			return

		default:
			if coreh3.IsReservedH2FrameType(frameType) {
				// RFC 9114 §7.2.8 & §11.2.1: Reserved HTTP/2 frame types MUST cause H3_FRAME_UNEXPECTED
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
					"reserved H2 frame received on control stream (RFC 9114 §7.2.8)",
				)

				return
			}

			// Unknown or GREASE frames: RFC 9114 §7.2.8 & §9: ignore payload
			if payloadLen > 0 {
				lr := io.LimitReader(stream, int64(payloadLen)) //nolint:gosec
				_, _ = io.Copy(io.Discard, lr)
			}
		}
	}
}
