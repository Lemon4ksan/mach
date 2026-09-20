// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/lemon4ksan/foundation/encoding/varint"

	coreheaders "github.com/lemon4ksan/foundation/net/headkit"
	"github.com/lemon4ksan/foundation/net/quic"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

// ServerHandlerFunc is the callback signature for dispatching an incoming H3 stream request.
type ServerHandlerFunc func(req *ServerRequest, res *ServerResponse) error

// ServerRequest represents a parsed incoming HTTP/3 request.
type ServerRequest struct {
	StreamID   uint64
	Method     string
	Path       string
	Scheme     string
	Authority  string
	Headers    coreheaders.Headers
	Body       []byte
	RemoteAddr string
	Ctx        context.Context
}

// ServerResponse represents an outgoing HTTP/3 response.
type ServerResponse struct {
	StatusCode int
	Headers    coreheaders.Headers
	Body       []byte
}

// ServerConn manages an active HTTP/3 server connection over an underlying QUIC connection (RFC 9114).
type ServerConn struct {
	quicConn        *quic.Conn
	handler         ServerHandlerFunc
	qpack           *coreh3.QPACKCodec
	isClosed        atomic.Bool
	closeErr        error
	controlOut      *quic.SendStream
	hasControlIn    atomic.Bool
	hasQPACKEncoder atomic.Bool
	hasQPACKDecoder atomic.Bool
}

// NewServerConn creates a new HTTP/3 server connection wrapping a QUIC connection.
func NewServerConn(quicConn *quic.Conn, handler ServerHandlerFunc) *ServerConn {
	return &ServerConn{
		quicConn: quicConn,
		handler:  handler,
		qpack:    coreh3.NewQPACKCodec(),
	}
}

// Serve initializes control streams and handles incoming bidirectional request streams.
func (sc *ServerConn) Serve() error {
	ctx := sc.quicConn.Context()

	// 1. Open and initialize server unidirectional control stream (RFC 9114 §6.2.1)
	ctrlStream, err := sc.quicConn.OpenUniStreamSync(ctx)
	if err != nil {
		return err
	}

	sc.controlOut = ctrlStream

	// Write Control Stream Type (0x00)
	var typeBuf [8]byte

	n := varint.Append(typeBuf[:0], coreh3.StreamTypeControl)
	if _, err := ctrlStream.Write(n); err != nil {
		return err
	}

	// Write initial server SETTINGS frame (RFC 9114 §7.2.4)
	st := &coreh3.Settings{
		MaxFieldSectionSize: 64 * 1024,
	}
	if _, err := ctrlStream.Write(st.Encode()); err != nil {
		return err
	}

	// 2. Accept client-initiated unidirectional streams (Client control & QPACK streams) in background
	go sc.acceptUniStreams()

	// 3. Main loop: accept client-initiated bidirectional request streams (RFC 9114 §6.1)
	for {
		if sc.isClosed.Load() {
			return sc.closeErr
		}

		stream, err := sc.quicConn.AcceptStream(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return nil
			}

			return err
		}

		go sc.handleRequestStream(stream)
	}
}

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

func (sc *ServerConn) handleUniStream(stream *quic.ReceiveStream) {
	defer stream.CancelRead(0)

	// Read stream type varint (RFC 9114 §6.2)
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

		settingsPayload := make([]byte, frameLen)
		if _, err := io.ReadFull(stream, settingsPayload); err != nil {
			return
		}

	case coreh3.StreamTypeQPACKEncoder:
		if sc.hasQPACKEncoder.Swap(true) {
			_ = sc.quicConn.CloseWithError(quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError), "duplicate QPACK encoder stream")
			return
		}
		_, _ = io.Copy(io.Discard, stream)
	case coreh3.StreamTypeQPACKDecoder:
		if sc.hasQPACKDecoder.Swap(true) {
			_ = sc.quicConn.CloseWithError(quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError), "duplicate QPACK decoder stream")
			return
		}
		_, _ = io.Copy(io.Discard, stream)
	default:
		// RFC 9114 §6.2: Unknown unidirectional stream types MUST either be discarded or cancelled
		_, _ = io.Copy(io.Discard, stream)
	}
}

func (sc *ServerConn) handleRequestStream(stream *quic.Stream) {
	defer func() { _ = stream.Close() }()

	qr := varint.NewReader(stream)

	var (
		headerBlock    []byte
		bodyBuf        bytes.Buffer
		hasSeenHeaders bool
		hasSeenTrailer bool
	)

	// Read frames on request stream (RFC 9114 §4.1 & §7.1)
	for {
		frameType, err := varint.Read(qr)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return
		}

		frameLen, err := varint.Read(qr)
		if err != nil {
			return
		}

		switch frameType {
		case coreh3.FrameTypeHeaders:
			if hasSeenTrailer {
				// RFC 9114 §4.1: Frame after trailer section is invalid
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
					"frame after trailing headers (RFC 9114 §4.1)",
				)

				return
			}

			if !hasSeenHeaders {
				hasSeenHeaders = true

				headerBlock = make([]byte, frameLen)
				if _, err := io.ReadFull(stream, headerBlock); err != nil {
					return
				}
			} else {
				// Trailing headers
				hasSeenTrailer = true

				trailerBlock := make([]byte, frameLen)
				if _, err := io.ReadFull(stream, trailerBlock); err != nil {
					return
				}
			}

		case coreh3.FrameTypeData:
			// RFC 9114 §4.1: DATA frame before HEADERS or after trailing HEADERS is invalid
			if !hasSeenHeaders || hasSeenTrailer {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
					"DATA frame unexpected (RFC 9114 §4.1)",
				)

				return
			}

			if frameLen > 0 {
				lr := io.LimitReader(stream, int64(frameLen))
				if _, err := io.Copy(&bodyBuf, lr); err != nil {
					return
				}
			}

		default:
			// Skip unknown frame (RFC 9114 §7.2.8 & §9)
			if frameLen > 0 {
				lr := io.LimitReader(stream, int64(frameLen))
				_, _ = io.Copy(io.Discard, lr)
			}
		}
	}

	if !hasSeenHeaders || len(headerBlock) == 0 {
		// RFC 9114 §4.1: Incomplete request stream termination
		stream.CancelRead(quic.StreamErrorCode(coreh3.ErrCodeH3RequestIncomplete))
		return
	}

	// Decode QPACK headers (RFC 9114 §4.1.2)
	var parsedHeaders coreheaders.Headers
	parsedHeaders.Reset()

	method, path, scheme, authority, err := sc.qpack.DecodeRequestHeaders(uint64(stream.StreamID()), headerBlock, &parsedHeaders)
	if err != nil {
		stream.CancelRead(quic.StreamErrorCode(coreh3.ErrCodeH3MessageError))
		return
	}

	req := &ServerRequest{
		StreamID:   uint64(stream.StreamID()),
		Method:     method,
		Path:       path,
		Scheme:     scheme,
		Authority:  authority,
		Headers:    parsedHeaders,
		Body:       bodyBuf.Bytes(),
		RemoteAddr: sc.quicConn.RemoteAddr().String(),
		Ctx:        stream.Context(),
	}

	res := &ServerResponse{
		StatusCode: http.StatusOK,
		Headers:    coreheaders.NewWithCapacity(16),
	}

	if sc.handler != nil {
		_ = sc.handler(req, res)
	}

	// Write response HEADERS frame
	respBlock := sc.qpack.EncodeResponseHeaders(uint64(stream.StreamID()), res.StatusCode, res.Headers, len(res.Body))

	var frameHdr [16]byte

	hdrBytes := varint.Append(frameHdr[:0], coreh3.FrameTypeHeaders)
	hdrBytes = varint.Append(hdrBytes, uint64(len(respBlock)))

	if _, err := stream.Write(hdrBytes); err != nil {
		return
	}

	if _, err := stream.Write(respBlock); err != nil {
		return
	}

	// Write response DATA frame
	if len(res.Body) > 0 {
		dataHdrBytes := varint.Append(frameHdr[:0], coreh3.FrameTypeData)
		dataHdrBytes = varint.Append(dataHdrBytes, uint64(len(res.Body)))

		if _, err := stream.Write(dataHdrBytes); err != nil {
			return
		}

		if _, err := stream.Write(res.Body); err != nil {
			return
		}
	}
}

// Close gracefully closes the HTTP/3 connection.
func (sc *ServerConn) Close() error {
	sc.isClosed.Store(true)
	return sc.quicConn.CloseWithError(0x0100, "h3 normal closure")
}
