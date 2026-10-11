// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"context"
	"errors"
	"io"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/mach/net/quic"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

// setupControlStream opens the client's outbound unidirectional control stream, writes the stream
// type identifier (0x00), and transmits the initial SETTINGS frame (RFC 9114 §6.2.1 & §7.2.4).
func (cc *ClientConn) setupControlStream() error {
	str, err := cc.conn.OpenUniStream()
	if err != nil {
		return err
	}

	var stackBuf [64]byte

	buf := varint.Append(stackBuf[:0], coreh3.StreamTypeControl)
	buf = append(buf, cc.settings.Encode()...)

	_, err = str.Write(buf)

	return err
}

// readUnidirectionalStreams accepts inbound unidirectional streams opened by the server and
// dispatches each to handleUnidirectionalStream in a separate goroutine (RFC 9114 §6.2).
func (cc *ClientConn) readUnidirectionalStreams() {
	for {
		str, err := cc.conn.AcceptUniStream(context.Background())
		if err != nil {
			return
		}

		go cc.handleUnidirectionalStream(str)
	}
}

// handleUnidirectionalStream decodes the stream type varint of an incoming unidirectional stream
// and routes it according to RFC 9114 §6.2: Control (0x00), QPACK Encoder (0x02), or QPACK Decoder (0x03).
//
// Inbound streams are checked to enforce that only a single instance of each stream type exists per
// connection; receipt of a duplicate stream results in connection termination with H3_STREAM_CREATION_ERROR.
// Unknown stream types are gracefully drained and ignored per RFC 9114 §6.2.
func (cc *ClientConn) handleUnidirectionalStream(str *quic.ReceiveStream) {
	r := varint.NewReader(str)

	streamType, err := varint.Read(r)
	if err != nil {
		return
	}

	switch streamType {
	case coreh3.StreamTypeControl:
		if cc.hasControlIn.Swap(true) {
			_ = cc.conn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate control stream (RFC 9114 §6.2.1)",
			)

			return
		}

		cc.handleControlStream(r)

	case coreh3.StreamTypeQPACKEncoder:
		if cc.hasQPACKEncoder.Swap(true) {
			_ = cc.conn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate QPACK encoder stream (RFC 9114 §6.2 & RFC 9204 §4.2)",
			)

			return
		}

		_, err := io.Copy(io.Discard, str)
		if err == nil || errors.Is(err, io.EOF) {
			_ = cc.conn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3ClosedCriticalStream),
				"QPACK encoder stream closed (RFC 9114 §6.2.1 & RFC 9204 §4.2)",
			)
		}

	case coreh3.StreamTypeQPACKDecoder:
		if cc.hasQPACKDecoder.Swap(true) {
			_ = cc.conn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate QPACK decoder stream (RFC 9114 §6.2 & RFC 9204 §4.2)",
			)

			return
		}

		_, err := io.Copy(io.Discard, str)
		if err == nil || errors.Is(err, io.EOF) {
			_ = cc.conn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3ClosedCriticalStream),
				"QPACK decoder stream closed (RFC 9114 §6.2.1 & RFC 9204 §4.2)",
			)
		}

	default:
		// Unknown unidirectional stream: RFC 9114 §6.2 specifies unknown stream types must be ignored.
		_, _ = io.Copy(io.Discard, str)
	}
}

// handleControlStream decodes frames received on the inbound server control stream (RFC 9114 §7.2).
//
// Invariant: The first frame on the control stream MUST be a SETTINGS frame (RFC 9114 §7.2.4); receipt of
// any other frame causes connection closure with H3_MISSING_SETTINGS.
// Receipt of reserved HTTP/2 settings causes connection closure with H3_SETTINGS_ERROR (RFC 9114 §7.2.4.1).
func (cc *ClientConn) handleControlStream(r varint.Reader) {
	firstFrame := true

	for {
		frameType, payloadLen, err := coreh3.ReadFrameHeader(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				_ = cc.conn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3ClosedCriticalStream),
					"control stream closed unexpectedly (RFC 9114 §6.2.1)",
				)
			}

			return
		}

		if firstFrame && frameType != coreh3.FrameTypeSettings {
			_ = cc.conn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3MissingSettings),
				"H3_MISSING_SETTINGS: first frame must be SETTINGS (RFC 9114 §7.2.4)",
			)

			return
		}
		if !firstFrame && frameType == coreh3.FrameTypeSettings {
			_ = cc.conn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
				"H3_FRAME_UNEXPECTED: second SETTINGS frame on control stream (RFC 9114 §7.2.4)",
			)

			return
		}
		firstFrame = false

		switch frameType {
		case coreh3.FrameTypeSettings:
			st, err := coreh3.DecodeSettings(r, payloadLen)
			if err != nil {
				var (
					msg  string
					code = coreh3.ErrCodeH3SettingsError
				)
				if errors.Is(err, coreh3.ErrH3SettingsError) {
					msg = "H3_SETTINGS_ERROR: reserved H2 setting ID (RFC 9114 §7.2.4.1)"
				} else {
					code = coreh3.ErrCodeH3FrameError
					msg = "H3_FRAME_ERROR: truncated settings payload (RFC 9114 §7.1)"
				}
				_ = cc.conn.CloseWithError(
					quic.ApplicationErrorCode(code),
					msg,
				)

				return
			}

			cc.settings = *st

		case coreh3.FrameTypeGoAway:
			if err := cc.handleGoAway(r, payloadLen); err != nil {
				_ = cc.conn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameError),
					"H3_FRAME_ERROR: malformed GOAWAY payload (RFC 9114 §7.1 & §7.2.6)",
				)

				return
			}

			return

		case coreh3.FrameTypeCancelPush:
			// RFC 9114 §7.2.3: CANCEL_PUSH payload is a single Push ID varint.
			if payloadLen == 0 {
				_ = cc.conn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameError),
					"H3_FRAME_ERROR: empty CANCEL_PUSH payload (RFC 9114 §7.1 & §7.2.3)",
				)

				return
			}

			lr := io.LimitReader(r, int64(payloadLen)) //nolint:gosec
			vReader := varint.NewReader(lr)
			if _, err := varint.Read(vReader); err != nil {
				_ = cc.conn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameError),
					"H3_FRAME_ERROR: malformed CANCEL_PUSH push ID (RFC 9114 §7.1 & §7.2.3)",
				)

				return
			}

			var extra [1]byte
			if n, _ := lr.Read(extra[:]); n > 0 {
				_ = cc.conn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameError),
					"H3_FRAME_ERROR: excess payload in CANCEL_PUSH frame (RFC 9114 §7.1 & §7.2.3)",
				)

				return
			}

		case coreh3.FrameTypeMaxPushID:
			// RFC 9114 §7.2.7: A server MUST NOT send a MAX_PUSH_ID frame. A client MUST treat the
			// receipt of a MAX_PUSH_ID frame as a connection error of type H3_FRAME_UNEXPECTED.
			_ = cc.conn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
				"H3_FRAME_UNEXPECTED: server sent MAX_PUSH_ID frame (RFC 9114 §7.2.7)",
			)

			return

		case coreh3.FrameTypeData, coreh3.FrameTypeHeaders, coreh3.FrameTypePushPromise:
			// RFC 9114 §7.2.1, §7.2.2, §7.2.5: DATA, HEADERS, PUSH_PROMISE MUST NOT be received on control stream
			_ = cc.conn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
				"H3_FRAME_UNEXPECTED: prohibited frame received on control stream (RFC 9114 §7.2)",
			)

			return

		default:
			if coreh3.IsReservedH2FrameType(frameType) {
				// RFC 9114 §7.2.8 & §11.2.1: Reserved HTTP/2 frame types MUST cause H3_FRAME_UNEXPECTED
				_ = cc.conn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
					"H3_FRAME_UNEXPECTED: reserved H2 frame received on control stream (RFC 9114 §7.2.8)",
				)

				return
			}

			if _, err := io.CopyN(io.Discard, r, int64(payloadLen)); err != nil { //nolint:gosec
				return
			}
		}
	}
}

// handleGoAway processes a GOAWAY frame received from the server, reads the shutdown stream ID,
// and initiates client connection termination (RFC 9114 §7.2.6).
func (cc *ClientConn) handleGoAway(r varint.Reader, payloadLen uint64) error {
	if payloadLen == 0 {
		return errors.New("empty GOAWAY payload")
	}

	lr := io.LimitReader(r, int64(payloadLen)) //nolint:gosec
	vReader := varint.NewReader(lr)
	if _, err := varint.Read(vReader); err != nil {
		return err
	}

	var extra [1]byte
	if n, _ := lr.Read(extra[:]); n > 0 {
		return errors.New("excess payload in GOAWAY")
	}

	_ = cc.Close()

	return nil
}
