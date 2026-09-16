// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"github.com/lemon4ksan/mach/core/bytesutil"
	coreheaders "github.com/lemon4ksan/mach/core/headers"

	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/silicon/pool"
)

var (
	readerStorage = pool.NewPerPStorage(func() *bufio.Reader {
		return bufio.NewReaderSize(nil, 4096)
	})
	writerStorage = pool.NewPerPStorage(func() *bytesutil.ByteBuffer {
		return bytesutil.AcquireByteBuffer()
	})
	reqStorage = pool.NewPerPStorage(func() *Request {
		return &Request{
			Body:    make([]byte, 0, 1024),
			Headers: coreheaders.NewWithCapacity(16),
		}
	})
	resStorage = pool.NewPerPStorage(func() *Response {
		return &Response{
			Body: make([]byte, 0, 1024),
		}
	})
)

// HandlerFunc is the core callback for dispatching an incoming H1 request to the server router.
type HandlerFunc func(req *Request, res *Response) error

// ConnHandler manages the lifecycle of a single incoming TCP or TLS connection.
type ConnHandler struct {
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	MaxBodySize  int64
	Handler      HandlerFunc
}

// ServeConn processes HTTP/1.1 requests sequentially on conn until closed or error occurs.
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
			readerStorage.Put(br)
			bytesutil.ReleaseByteBuffer(bw)
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
	defer reqStorage.Put(req)

	res := resStorage.Get()
	defer resStorage.Put(res)

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
		// and Content-Length were received, the server MUST close the connection after responding.
		if req.Headers.Has(header.TransferEncoding) && req.Headers.Has(header.ContentLength) {
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
