// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"time"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"

	"github.com/lemon4ksan/mach"
	"github.com/lemon4ksan/mach/proto/headkit"
	"github.com/lemon4ksan/mach/proto/semantic/status"
)

var (
	readerStorage = pool.NewPerCorePool(func() *bufio.Reader {
		return bufio.NewReaderSize(nil, 4096)
	})
	writerStorage = pool.NewPerCorePool(func() *bytesconv.ByteBuffer {
		return &bytesconv.ByteBuffer{B: make([]byte, 0, 4096)}
	})
	reqStorage = pool.NewPerCorePool(func() *Request {
		r := &Request{
			Body:      make([]byte, 0, 1024),
			headerBuf: make([]byte, 0, 1024),
			Headers:   headkit.NewWithCapacity(16),
		}
		r.initClosures()

		return r
	})
	resStorage = pool.NewPerCorePool(func() *Response {
		return &Response{
			Body: make([]byte, 0, 1024),
		}
	})
	streamStorage = pool.NewPerCorePool(func() *ServerStream {
		return &ServerStream{}
	})
)

// HandlerFunc is the core callback for dispatching an incoming H1 request to the server router (RFC 9110 §3).
// Handlers must not retain references to req, res, or their buffers after HandlerFunc returns.
type HandlerFunc func(req *Request, res *Response) error

// ConnHandler manages the lifecycle of a single incoming TCP or TLS connection (RFC 9112 §9).
type ConnHandler struct {
	// ReadTimeout specifies the maximum duration for reading the entire request.
	ReadTimeout time.Duration
	// WriteTimeout specifies the maximum duration for writing the complete response.
	WriteTimeout time.Duration
	// IdleTimeout specifies the maximum duration to wait for the next request on a keep-alive connection.
	IdleTimeout time.Duration
	// MaxBodySize sets the maximum permitted body payload size in bytes (RFC 9110 §8.6).
	MaxBodySize int64
	// Handler is the callback invoked for each received HTTP request.
	Handler HandlerFunc
	// StreamHandler is the universal protocol-agnostic callback (mach.Handler).
	StreamHandler mach.Handler
}

// ServeConn processes HTTP/1.1 requests sequentially on conn until closed, timed out, or error occurs (RFC 9112 §9.3).
// Mitigates request smuggling by terminating the connection if conflicting framing headers are detected (RFC 9112 §11.2).
func (ch *ConnHandler) ServeConn(conn net.Conn) error {
	br := readerStorage.Get()
	br.Reset(conn)

	bw := writerStorage.Get()
	bw.ResetWriter(conn)

	var isHijacked bool
	defer func() {
		if !isHijacked {
			_ = bw.Flush()
			_ = conn.Close()

			br.Reset(nil)
			readerStorage.Put(br)

			if cap(bw.B) > 64*1024 {
				bw.B = make([]byte, 0, 4096)
			}

			bw.Reset()
			writerStorage.Put(bw)
		}
	}()

	req := reqStorage.Get()
	defer func() {
		if cap(req.Body) > 64*1024 {
			req.Body = make([]byte, 0, 1024)
		}

		req.Reset()
		reqStorage.Put(req)
	}()

	res := resStorage.Get()
	defer func() {
		if cap(res.Body) > 64*1024 {
			res.Body = make([]byte, 0, 1024)
		}

		res.Reset()
		resStorage.Put(res)
	}()

	var tlsState *tls.ConnectionState
	if tlsConn, ok := conn.(*tls.Conn); ok {
		state := tlsConn.ConnectionState()
		tlsState = &state
	}

	maxBody := ch.MaxBodySize
	if maxBody <= 0 {
		maxBody = 32 << 20 // 32MB default
	}

	for {
		req.Reset()
		res.Reset()

		req.Conn = conn
		req.bw = bw
		req.br = br
		req.TLS = tlsState

		// Set read timeout
		readDeadline := ch.ReadTimeout
		if readDeadline <= 0 {
			readDeadline = ch.IdleTimeout
		}

		if readDeadline > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(readDeadline))
		}

		err := req.ReadRequest(br, bw, maxBody)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil
			}

			return err
		}

		keepAlive := req.Headers.IsKeepAlive(req.Proto)
		// RFC 9112 §6.3 Item 3 & §11.2: To mitigate Request Smuggling when both Transfer-Encoding
		// and Content-Length were received, or when forced by CloseConnection, close the connection.
		if req.CloseConnection || (req.Headers.Has(headkit.TransferEncoding) && req.Headers.Has(headkit.ContentLength)) {
			keepAlive = false
		}

		// Set write timeout
		if ch.WriteTimeout > 0 {
			_ = conn.SetWriteDeadline(time.Now().Add(ch.WriteTimeout))
		}

		if ch.Handler != nil {
			if err := ch.Handler(req, res); err != nil {
				res.StatusCode = 500
				res.Body = []byte(`{"error":"INTERNAL_ERROR","message":"Internal Server Error"}`)
			}
		} else if ch.StreamHandler != nil {
			s := streamStorage.Get()
			s.req = req
			s.res = res
			s.br = br
			s.bw = bw
			s.conn = conn
			err := ch.StreamHandler.Serve(s)
			s.req = nil
			s.res = nil
			s.br = nil
			s.bw = nil
			s.conn = nil
			streamStorage.Put(s)

			if err != nil {
				res.StatusCode = 500
				res.Body = []byte(`{"error":"INTERNAL_ERROR","message":"Internal Server Error"}`)
			}
		}

		if req.CloseConnection {
			keepAlive = false
		}

		if res.StatusCode == status.SwitchingProtocols {
			keepAlive = false
			isHijacked = true
		}

		if res.Headers.Has(headkit.Connection) &&
			bytesconv.EqualFoldASCII(res.Headers.Get(headkit.Connection), headkit.ValueClose) {
			keepAlive = false
		}

		if req.isHijacked {
			isHijacked = true
			return nil
		}

		// High-Throughput HTTP Pipelining Coalescing:
		// If more unread request bytes already reside in br and connection is keep-alive,
		// defer bw.Flush() to combine multiple pipelined responses into a single TCP write.
		hasMorePipelined := keepAlive && br.Buffered() > 0

		if err := res.WriteTo(bw, keepAlive, false); err != nil {
			return err
		}

		if !hasMorePipelined {
			if _, err := bw.WriteTo(conn); err != nil {
				return err
			}

			bw.ResetWriter(conn)
		}

		if !keepAlive {
			return nil
		}
	}
}

// ServerStream adapts an H1 server request/response exchange into a mach.Stream.
type ServerStream struct {
	req   *Request
	res   *Response
	br    *bufio.Reader
	bw    *bytesconv.ByteBuffer
	conn  net.Conn
	frame StreamFrame
}

var _ mach.Stream = (*ServerStream)(nil)

func (s *ServerStream) Read(p []byte) (int, error) {
	if s.req != nil && len(s.req.Body) > 0 {
		n := copy(p, s.req.Body)
		s.req.Body = s.req.Body[n:]
		return n, nil
	}

	if s.br != nil {
		return s.br.Read(p)
	}

	return 0, io.EOF
}

func (s *ServerStream) Write(p []byte) (int, error) {
	if s.bw != nil {
		return s.bw.Write(p)
	}

	if s.conn != nil {
		return s.conn.Write(p)
	}

	return 0, io.ErrClosedPipe
}

func (s *ServerStream) Close() error {
	if s.bw != nil && s.conn != nil {
		_, err := s.bw.WriteTo(s.conn)
		return err
	}

	return nil
}

func (s *ServerStream) ReadFrame() (mach.Frame, error) {
	if s.br == nil {
		return nil, io.EOF
	}

	line, err := s.br.ReadSlice('\n')
	if err != nil {
		return nil, err
	}

	s.frame.frameType = mach.FrameData
	s.frame.payload = line

	return &s.frame, nil
}

func (s *ServerStream) Request() *Request {
	return s.req
}

func (s *ServerStream) Response() *Response {
	return s.res
}

// StreamFrame implements mach.Frame for ServerStream.
type StreamFrame struct {
	frameType uint8
	payload   []byte
}

func (f *StreamFrame) Type() uint8 {
	return f.frameType
}

func (f *StreamFrame) Payload() []byte {
	return f.payload
}
