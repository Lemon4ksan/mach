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

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/mach/proto/headkit"
	"github.com/lemon4ksan/foundation/net/quic"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

// ServerHandlerFunc is the callback signature for dispatching an incoming H3 stream request (RFC 9114 §4.1).
//
// Concurrency: Invoked concurrently across independent goroutines handling separate request streams.
// Lifecycle: The req and res pointers are pooled and must not be accessed after the handler returns.
type ServerHandlerFunc func(req *ServerRequest, res *ServerResponse) error

// ServerRequest represents a parsed incoming HTTP/3 request over QUIC (RFC 9114 §4.1, RFC 9204).
//
// Concurrency:
// A ServerRequest instance is owned by a single request stream goroutine and is not safe
// for concurrent access without external synchronization.
type ServerRequest struct {
	StreamID   uint64          // StreamID is the client-initiated bidirectional stream ID (RFC 9000 §2.1, RFC 9114 §6.1).
	Method     string          // Method is the :method pseudo-header (RFC 9114 §4.1.2).
	Path       string          // Path is the :path pseudo-header (RFC 9114 §4.1.2).
	Scheme     string          // Scheme is the :scheme pseudo-header (RFC 9114 §4.1.2).
	Authority  string          // Authority is the :authority pseudo-header (RFC 9114 §4.1.2).
	Headers    headkit.Headers // Headers contains QPACK-decoded request header fields (RFC 9204).
	Body       []byte          // Body contains payload bytes from DATA frames (RFC 9114 §7.2.1).
	RemoteAddr string          // RemoteAddr is the peer network address.
	Ctx        context.Context // Ctx is the stream context.
}

// Reset clears the ServerRequest structure for reuse in Per-P storage pools.
func (r *ServerRequest) Reset() {
	r.StreamID = 0
	r.Method = ""
	r.Path = ""
	r.Scheme = ""
	r.Authority = ""
	r.Headers.Reset()

	if cap(r.Body) > 64*1024 {
		r.Body = nil
	} else {
		r.Body = r.Body[:0]
	}

	r.RemoteAddr = ""
	r.Ctx = nil
}

// ServerResponse represents an outgoing HTTP/3 response (RFC 9114 §4.1.2).
//
// Concurrency:
// ServerResponse is populated by the stream handler and serialized directly to the QUIC stream.
type ServerResponse struct {
	StatusCode int             // StatusCode is the HTTP response status code (RFC 9114 §4.1.2 :status).
	Headers    headkit.Headers // Headers contains outgoing headers to be QPACK encoded (RFC 9204).
	Body       []byte          // Body contains response payload framed into DATA frames (RFC 9114 §7.2.1).
}

// Reset clears the ServerResponse structure for reuse in Per-P storage pools.
func (res *ServerResponse) Reset() {
	res.StatusCode = http.StatusOK
	res.Headers.Reset()

	if cap(res.Body) > 64*1024 {
		res.Body = nil
	} else {
		res.Body = res.Body[:0]
	}
}

var (
	serverReqStorage = pool.NewPerPStorage(func() *ServerRequest {
		return &ServerRequest{
			Headers: headkit.NewWithCapacity(16),
		}
	})

	serverResStorage = pool.NewPerPStorage(func() *ServerResponse {
		return &ServerResponse{
			Headers: headkit.NewWithCapacity(16),
		}
	})

	h3HeaderBlockStorage = pool.NewPerPStorage(func() *[]byte {
		b := make([]byte, 0, 16384)
		return &b
	})

	h3ReaderStorage = pool.NewPerPStorage(func() *bufio.Reader {
		return bufio.NewReaderSize(nil, 4096)
	})

	h3BodyBufferStorage = pool.NewPerPStorage(func() *bytesconv.ByteBuffer {
		return &bytesconv.ByteBuffer{}
	})
)

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

			if !hasSeenHeaders {
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
			} else {
				hasSeenTrailer = true

				if frameLen > 0 {
					lr := io.LimitReader(br, int64(frameLen)) //nolint:gosec
					_, _ = io.Copy(io.Discard, lr)
				}
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
			if frameLen > 0 {
				lr := io.LimitReader(br, int64(frameLen)) //nolint:gosec
				_, _ = io.Copy(io.Discard, lr)
			}
		}
	}

	if !hasSeenHeaders || len(headerBlock) == 0 {
		stream.CancelRead(quic.StreamErrorCode(coreh3.ErrCodeH3RequestIncomplete))
		return
	}

	streamID := uint64(stream.StreamID()) //nolint:gosec

	method, path, scheme, authority, err := sc.qpack.DecodeRequestHeaders(
		streamID,
		headerBlock,
		&req.Headers,
	)
	if err != nil {
		stream.CancelRead(quic.StreamErrorCode(coreh3.ErrCodeH3MessageError))
		return
	}

	req.StreamID = streamID
	req.Method = method
	req.Path = path
	req.Scheme = scheme
	req.Authority = authority
	req.Body = bodyBuf.B
	req.RemoteAddr = sc.quicConn.RemoteAddr().String()
	req.Ctx = stream.Context()

	if sc.handler != nil {
		_ = sc.handler(req, res)
	}

	respBlock := sc.qpack.EncodeResponseHeaders(streamID, res.StatusCode, res.Headers, len(res.Body))

	var frameHdr [16]byte

	hdrBytes := varint.Append(frameHdr[:0], coreh3.FrameTypeHeaders)
	hdrBytes = varint.Append(hdrBytes, uint64(len(respBlock)))

	if _, err := stream.Write(hdrBytes); err != nil {
		return
	}

	if _, err := stream.Write(respBlock); err != nil {
		return
	}

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
