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
	"net/http"
	"time"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"

	"github.com/lemon4ksan/mach/proto/headkit"
	"github.com/lemon4ksan/mach/proto/http/header"
)

var (
	readerStorage = pool.NewPerPStorage(func() *bufio.Reader {
		return bufio.NewReaderSize(nil, 4096)
	})
	writerStorage = pool.NewPerPStorage(func() *bytesconv.ByteBuffer {
		return &bytesconv.ByteBuffer{}
	})
	reqStorage = pool.NewPerPStorage(func() *Request {
		return &Request{
			Body:    make([]byte, 0, 1024),
			Headers: headkit.NewWithCapacity(16),
		}
	})
	resStorage = pool.NewPerPStorage(func() *Response {
		return &Response{
			Body: make([]byte, 0, 1024),
		}
	})
)

// HandlerFunc is the core callback for dispatching an incoming H1 request to the server router (RFC 9110 §3).
//
// Lifecycle:
//   - Both req and res are recycled into Per-P storage after the handler returns.
//   - Handlers MUST NOT retain references to req, res, or their buffers beyond the return of HandlerFunc.
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

			bw.Reset()
			writerStorage.Put(bw)
		}
	}()

	earlyHintsFn := func(h http.Header) error {
		if len(h) == 0 {
			return nil
		}

		_, _ = bw.WriteString("HTTP/1.1 103 Early Hints\r\n")
		for k, vv := range h {
			for _, v := range vv {
				_, _ = bw.WriteString(k)
				_, _ = bw.WriteString(": ")
				_, _ = bw.WriteString(v)
				_, _ = bw.WriteString("\r\n")
			}
		}

		_, _ = bw.WriteString("\r\n")
		if _, err := bw.WriteTo(conn); err != nil {
			return err
		}

		bw.ResetWriter(conn)

		return nil
	}

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

	remoteAddr := conn.RemoteAddr().String()

	var tlsState *tls.ConnectionState
	if tlsConn, ok := conn.(*tls.Conn); ok {
		state := tlsConn.ConnectionState()
		tlsState = &state
	}

	maxBody := ch.MaxBodySize
	if maxBody <= 0 {
		maxBody = 32 << 20 // 32MB default
	}

	hijackFn := func() (net.Conn, *bufio.ReadWriter, error) {
		if isHijacked {
			return nil, nil, errors.New("h1: connection already hijacked")
		}

		isHijacked = true
		rw := bufio.NewReadWriter(br, bufio.NewWriter(conn))

		return conn, rw, nil
	}

	for {
		req.Reset()
		res.Reset()

		req.Conn = conn
		req.RemoteAddr = remoteAddr
		req.TLS = tlsState
		req.HijackFn = hijackFn
		req.EarlyHintsFn = earlyHintsFn

		// Set read timeout
		if ch.ReadTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(ch.ReadTimeout))
		} else if ch.IdleTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(ch.IdleTimeout))
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
		if req.CloseConnection || (req.Headers.Has(header.TransferEncoding) && req.Headers.Has(header.ContentLength)) {
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
		}

		if req.CloseConnection {
			keepAlive = false
		}

		if res.Headers.Has(header.Connection) &&
			bytesconv.EqualFoldASCII(res.Headers.Get(header.Connection), header.ValueClose) {
			keepAlive = false
		}

		if isHijacked {
			// Handler took over the raw connection
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
