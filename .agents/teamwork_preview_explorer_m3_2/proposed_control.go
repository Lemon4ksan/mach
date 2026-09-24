// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"context"
	"errors"
	"io"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/net/quic"

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
				"duplicate control stream",
			)

			return
		}

		cc.handleControlStream(r)

	case coreh3.StreamTypeQPACKEncoder:
		if cc.hasQPACKEncoder.Swap(true) {
			_ = cc.conn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate QPACK encoder stream",
			)

			return
		}

		_, _ = io.Copy(io.Discard, str)

	case coreh3.StreamTypeQPACKDecoder:
		if cc.hasQPACKDecoder.Swap(true) {
			_ = cc.conn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate QPACK decoder stream",
			)

			return
		}

		_, _ = io.Copy(io.Discard, str)

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
			return
		}

		if firstFrame {
			if frameType != coreh3.FrameTypeSettings {
				_ = cc.conn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3MissingSettings),
					"H3_MISSING_SETTINGS: first frame must be SETTINGS (RFC 9114 §7.2.4)",
				)

				return
			}

			firstFrame = false
		}

		switch frameType {
		case coreh3.FrameTypeSettings:
			st, err := coreh3.DecodeSettings(r, payloadLen)
			if err != nil {
				if errors.Is(err, coreh3.ErrH3SettingsError) {
					_ = cc.conn.CloseWithError(
						quic.ApplicationErrorCode(coreh3.ErrCodeH3SettingsError),
						"H3_SETTINGS_ERROR: reserved H2 setting ID (RFC 9114 §7.2.4.1)",
					)
				} else {
					_ = cc.conn.CloseWithError(
						quic.ApplicationErrorCode(coreh3.ErrCodeH3SettingsError),
						"H3_SETTINGS_ERROR: invalid settings payload (RFC 9114 §7.2.4)",
					)
				}

				return
			}

			cc.settings = *st

		case coreh3.FrameTypeGoAway:
			cc.handleGoAway(r, payloadLen)

			return

		default:
			if _, err := io.CopyN(io.Discard, r, int64(payloadLen)); err != nil { //nolint:gosec
				return
			}
		}
	}
}

// handleGoAway processes a GOAWAY frame received from the server, reads the shutdown stream ID,
// and initiates client connection termination (RFC 9114 §7.2.6).
func (cc *ClientConn) handleGoAway(r varint.Reader, payloadLen uint64) {
	if payloadLen > 0 {
		_, _ = varint.Read(r)
	}

	_ = cc.Close()
}
