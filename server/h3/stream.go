// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"

	"github.com/lemon4ksan/mach/net/quic"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	"github.com/lemon4ksan/mach/proto/headkit"
)

// ServerHandlerFunc is the callback signature for dispatching an incoming H3 stream request (RFC 9114 §4.1).
// The req and res pointers are pooled and must not be retained after the handler returns.
type ServerHandlerFunc func(req *ServerRequest, res *ServerResponse) error

// ServerRequest represents a parsed incoming HTTP/3 request over QUIC (RFC 9114 §4.1, RFC 9204).
type ServerRequest struct {
	StreamID          uint64          // StreamID is the client-initiated bidirectional stream ID (RFC 9000 §2.1, RFC 9114 §6.1).
	Method            string          // Method is the :method pseudo-header (RFC 9114 §4.1.2).
	Path              string          // Path is the :path pseudo-header (RFC 9114 §4.1.2).
	Scheme            string          // Scheme is the :scheme pseudo-header (RFC 9114 §4.1.2).
	Authority         string          // Authority is the :authority pseudo-header (RFC 9114 §4.1.2).
	Protocol          string          // Protocol is the :protocol pseudo-header (RFC 9220 §4).
	Headers           headkit.Headers // Headers contains QPACK-decoded request header fields (RFC 9204).
	Body              []byte          // Body contains payload bytes from DATA frames (RFC 9114 §7.2.1).
	RemoteAddr        string          // RemoteAddr is the peer network address.
	Ctx               context.Context // Ctx is the stream context.
	Stream            io.ReadWriteCloser
	UpgradeFn         func(statusCode int, header headkit.Headers) error
	IsExtendedConnect bool
}

// Reset clears the ServerRequest structure for reuse.
func (r *ServerRequest) Reset() {
	r.StreamID = 0
	r.Method = ""
	r.Path = ""
	r.Scheme = ""
	r.Authority = ""
	r.Protocol = ""
	r.Headers.Reset()
	r.Stream = nil
	r.UpgradeFn = nil
	r.IsExtendedConnect = false

	r.Body = r.Body[:0]
	if cap(r.Body) > 64*1024 {
		r.Body = nil
	}

	r.RemoteAddr = ""
	r.Ctx = nil
}

// ServerResponse represents an outgoing HTTP/3 response (RFC 9114 §4.1.2).
type ServerResponse struct {
	StatusCode int             // StatusCode is the HTTP response status code (RFC 9114 §4.1.2 :status).
	Headers    headkit.Headers // Headers contains outgoing headers to be QPACK encoded (RFC 9204).
	Body       []byte          // Body contains response payload framed into DATA frames (RFC 9114 §7.2.1).
}

// Reset clears the ServerResponse structure for reuse.
func (r *ServerResponse) Reset() {
	r.StatusCode = http.StatusOK
	r.Headers.Reset()

	r.Body = r.Body[:0]
	if cap(r.Body) > 64*1024 {
		r.Body = nil
	}
}

var (
	serverReqStorage = pool.NewPerCorePool(func() *ServerRequest {
		return &ServerRequest{
			Headers: headkit.NewWithCapacity(16),
		}
	})

	serverResStorage = pool.NewPerCorePool(func() *ServerResponse {
		return &ServerResponse{
			Headers: headkit.NewWithCapacity(16),
		}
	})

	h3HeaderBlockStorage = pool.NewPerCorePool(func() *[]byte {
		b := make([]byte, 0, 16384)
		return &b
	})

	h3ReaderStorage = pool.NewPerCorePool(func() *bufio.Reader {
		return bufio.NewReaderSize(nil, 4096)
	})

	h3BodyBufferStorage = pool.NewPerCorePool(func() *bytesconv.ByteBuffer {
		return &bytesconv.ByteBuffer{}
	})
)

// h3DataStreamAdapter encapsulates reading and writing HTTP/3 DATA frames over a QUIC stream (RFC 9114 §7.2.1, RFC 9220 §4).
type h3DataStreamAdapter struct {
	stream     *quic.Stream
	br         *bufio.Reader
	readRemain int64
	closed     atomic.Bool
}

func (a *h3DataStreamAdapter) Read(p []byte) (int, error) {
	for a.readRemain == 0 {
		frameType, frameLen, err := coreh3.ReadFrameHeader(a.br)
		if err != nil {
			return 0, err
		}

		if frameType == coreh3.FrameTypeData {
			a.readRemain = int64(frameLen)
			if a.readRemain == 0 {
				continue
			}

			break
		}

		if frameLen > 0 {
			lr := io.LimitReader(a.br, int64(frameLen))
			_, _ = io.Copy(io.Discard, lr)
		}
	}

	toRead := int64(len(p))
	if toRead > a.readRemain {
		toRead = a.readRemain
	}

	n, err := a.br.Read(p[:toRead])
	if n > 0 {
		a.readRemain -= int64(n)
	}

	return n, err
}

func (a *h3DataStreamAdapter) Write(p []byte) (int, error) {
	if a.closed.Load() {
		return 0, io.ErrClosedPipe
	}

	if len(p) == 0 {
		return 0, nil
	}

	var hdr [16]byte

	b := varint.Append(hdr[:0], coreh3.FrameTypeData)

	b = varint.Append(b, uint64(len(p)))
	if _, err := a.stream.Write(b); err != nil {
		return 0, err
	}

	return a.stream.Write(p)
}

func (a *h3DataStreamAdapter) Close() error {
	if a.closed.CompareAndSwap(false, true) {
		return a.stream.Close()
	}

	return nil
}

// handleRequestStream processes an incoming client bidirectional request stream (RFC 9114 §4.1 & §7.1).
func (sc *ServerConn) handleRequestStream(stream *quic.Stream) {
	defer func() { _ = stream.Close() }()

	br := h3ReaderStorage.Get()
	br.Reset(stream)

	req := serverReqStorage.Get()
	res := serverResStorage.Get()
	bodyBuf := h3BodyBufferStorage.Get()
	bodyBuf.Reset()

	var heapHeaderBuf *[]byte

	defer func() {
		br.Reset(nil)
		h3ReaderStorage.Put(br)

		if heapHeaderBuf != nil {
			*heapHeaderBuf = (*heapHeaderBuf)[:0]
			h3HeaderBlockStorage.Put(heapHeaderBuf)
		}

		req.Reset()
		serverReqStorage.Put(req)

		res.Reset()
		serverResStorage.Put(res)

		bodyBuf.Reset()
		h3BodyBufferStorage.Put(bodyBuf)
	}()

	var (
		headerBlock    []byte
		stackHeaderBuf [4096]byte
		hasSeenHeaders bool
		hasSeenTrailer bool
	)

	for {
		frameType, frameLen, err := coreh3.ReadFrameHeader(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

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

			if frameLen > 16*1024*1024 {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3ExcessiveLoad),
					"HEADERS frame payload too large (RFC 9114 §4.2)",
				)

				return
			}

			if hasSeenHeaders {
				hasSeenTrailer = true

				if frameLen > 0 {
					lr := io.LimitReader(br, int64(frameLen)) //nolint:gosec
					_, _ = io.Copy(io.Discard, lr)
				}

				break
			}

			hasSeenHeaders = true

			if frameLen <= uint64(len(stackHeaderBuf)) {
				headerBlock = stackHeaderBuf[:frameLen]
			} else {
				heapHeaderBuf = h3HeaderBlockStorage.Get()

				b := (*heapHeaderBuf)[:0]
				if uint64(cap(b)) < frameLen {
					b = make([]byte, frameLen)
				} else {
					b = b[:frameLen]
				}

				headerBlock = b
			}

			if _, err := io.ReadFull(br, headerBlock); err != nil {
				return
			}

			streamID := uint64(stream.StreamID()) //nolint:gosec

			m, p, s, a, decErr := sc.qpack.DecodeRequestHeaders(
				streamID,
				headerBlock,
				&req.Headers,
			)
			if decErr != nil {
				stream.CancelRead(quic.StreamErrorCode(coreh3.ErrCodeH3MessageError))
				stream.CancelWrite(quic.StreamErrorCode(coreh3.ErrCodeH3MessageError))
				return
			}

			req.StreamID = streamID
			req.Method = m
			req.Path = p
			req.Scheme = s
			req.Authority = a
			req.Protocol = req.Headers.Get(":protocol")

			if req.Method == "CONNECT" && req.Protocol != "" {
				if !sc.EnableConnectProtocol || req.Scheme == "" || req.Path == "" || req.Authority == "" {
					stream.CancelRead(quic.StreamErrorCode(coreh3.ErrCodeH3GeneralProtocolError))
					stream.CancelWrite(quic.StreamErrorCode(coreh3.ErrCodeH3GeneralProtocolError))
					return
				}

				req.IsExtendedConnect = true
				req.RemoteAddr = sc.quicConn.RemoteAddr().String()
				req.Ctx = stream.Context()

				adapter := &h3DataStreamAdapter{
					stream: stream,
					br:     br,
				}
				req.Stream = adapter

				var isUpgraded bool

				req.UpgradeFn = func(statusCode int, resHdrs headkit.Headers) error {
					isUpgraded = true
					respBlock := sc.qpack.EncodeResponseHeaders(streamID, statusCode, resHdrs, -1)

					var initialBuf [256]byte
					b := varint.Append(initialBuf[:0], coreh3.FrameTypeHeaders)
					b = varint.Append(b, uint64(len(respBlock)))
					b = append(b, respBlock...)
					_, err := stream.Write(b)
					return err
				}

				if sc.handler != nil {
					_ = sc.handler(req, res)
				}

				if isUpgraded {
					_ = adapter.Close()
					return
				}

				respBlock := sc.qpack.EncodeResponseHeaders(streamID, res.StatusCode, res.Headers, len(res.Body))

				var initialBuf [256]byte
				b := varint.Append(initialBuf[:0], coreh3.FrameTypeHeaders)
				b = varint.Append(b, uint64(len(respBlock)))
				b = append(b, respBlock...)
				_, _ = stream.Write(b)

				return
			}

		case coreh3.FrameTypeData:
			if !hasSeenHeaders || hasSeenTrailer {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
					"DATA frame unexpected (RFC 9114 §4.1)",
				)

				return
			}

			if frameLen > 0 {
				lr := io.LimitReader(br, int64(frameLen)) //nolint:gosec
				if _, err := bodyBuf.ReadFrom(lr); err != nil {
					return
				}
			}

		default:
			if coreh3.IsReservedH2FrameType(frameType) {
				// RFC 9114 §7.2.8 & §11.2.1: Reserved HTTP/2 frame types MUST cause H3_FRAME_UNEXPECTED
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
					"reserved H2 frame type on request stream (RFC 9114 §7.2.8)",
				)

				return
			}

			// Control-only frames received on request stream (RFC 9114 Table 1, §7.2.3, §7.2.4, §7.2.6, §7.2.7)
			if frameType == coreh3.FrameTypeSettings ||
				frameType == coreh3.FrameTypeGoAway ||
				frameType == coreh3.FrameTypeCancelPush ||
				frameType == coreh3.FrameTypeMaxPushID {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
					"control frame received on request stream (RFC 9114 §7.2 Table 1)",
				)

				return
			}

			if frameLen > 0 {
				lr := io.LimitReader(br, int64(frameLen)) //nolint:gosec
				_, _ = io.Copy(io.Discard, lr)
			}
		}
	}

	if !hasSeenHeaders || len(headerBlock) == 0 {
		stream.CancelRead(quic.StreamErrorCode(coreh3.ErrCodeH3RequestIncomplete))
		stream.CancelWrite(quic.StreamErrorCode(coreh3.ErrCodeH3RequestIncomplete))
		return
	}

	streamID := uint64(stream.StreamID()) //nolint:gosec
	req.Body = bodyBuf.B
	req.RemoteAddr = sc.quicConn.RemoteAddr().String()
	req.Ctx = stream.Context()

	if sc.handler != nil {
		_ = sc.handler(req, res)
	}

	respBlock := sc.qpack.EncodeResponseHeaders(streamID, res.StatusCode, res.Headers, len(res.Body))

	respBufPtr := h3HeaderBlockStorage.Get()
	b := (*respBufPtr)[:0]
	b = varint.Append(b, coreh3.FrameTypeHeaders)
	b = varint.Append(b, uint64(len(respBlock)))
	b = append(b, respBlock...)

	if len(res.Body) > 0 {
		b = varint.Append(b, coreh3.FrameTypeData)
		b = varint.Append(b, uint64(len(res.Body)))
		b = append(b, res.Body...)
	}

	_, _ = stream.Write(b)

	*respBufPtr = (*respBufPtr)[:0]
	h3HeaderBlockStorage.Put(respBufPtr)
}
