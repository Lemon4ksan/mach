// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/lemon4ksan/mach/server/h1"
)

// oneByteConn wraps net.Conn to deliver bytes 1 byte per Read call.
type oneByteConn struct {
	net.Conn
}

func (c *oneByteConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return c.Conn.Read(p[:1])
}

// Port of Chromium HTTP request line tests: Double space before URI ("GET  / HTTP/1.1")
func TestChromium_RequestLine_DoubleSpaceBeforeURI(t *testing.T) {
	raw := "GET  / HTTP/1.1\r\nHost: localhost\r\n\r\n"
	br := bufio.NewReader(strings.NewReader(raw))
	var req h1.Request
	err := req.ReadRequest(br, nil, 1024)
	if err == nil {
		t.Fatal("RFC 9112 §3.1.1 violation: double space between method and URI must be rejected")
	}
}

// Port of Chromium HTTP request line tests: Double space before Protocol ("GET /  HTTP/1.1")
func TestChromium_RequestLine_DoubleSpaceBeforeProto(t *testing.T) {
	raw := "GET /  HTTP/1.1\r\nHost: localhost\r\n\r\n"
	br := bufio.NewReader(strings.NewReader(raw))
	var req h1.Request
	err := req.ReadRequest(br, nil, 1024)
	if err == nil {
		t.Fatal("RFC 9112 §3.1.1 violation: double space between URI and protocol must be rejected")
	}
}

// Port of Chromium HTTP request line tests: Trailing space in request line ("GET / HTTP/1.1 ")
func TestChromium_RequestLine_TrailingSpaceInProto(t *testing.T) {
	raw := "GET / HTTP/1.1 \r\nHost: localhost\r\n\r\n"
	br := bufio.NewReader(strings.NewReader(raw))
	var req h1.Request
	err := req.ReadRequest(br, nil, 1024)
	if err == nil {
		t.Fatal("RFC 9112 §3.1.1 violation: trailing whitespace in request line must be rejected")
	}
}

// Port of Chromium HTTP request line tests: Unsupported HTTP version ("GET / HTTP/9.9")
func TestChromium_RequestLine_UnsupportedProtocol(t *testing.T) {
	raw := "GET / HTTP/9.9\r\nHost: localhost\r\n\r\n"
	br := bufio.NewReader(strings.NewReader(raw))
	var req h1.Request
	err := req.ReadRequest(br, nil, 1024)
	if err == nil {
		t.Fatal("RFC 9112 §2.3 violation: unsupported protocol HTTP/9.9 must be rejected")
	}
	if !errors.Is(err, h1.ErrUnsupportedProtocol) {
		t.Fatalf("expected ErrUnsupportedProtocol, got %v", err)
	}
}

// Port of Chromium HTTP request line tests: Bogus protocol token ("GET / BOGUS")
func TestChromium_RequestLine_BogusProtocol(t *testing.T) {
	raw := "GET / BOGUS\r\nHost: localhost\r\n\r\n"
	br := bufio.NewReader(strings.NewReader(raw))
	var req h1.Request
	err := req.ReadRequest(br, nil, 1024)
	if err == nil {
		t.Fatal("RFC 9112 §2.3 violation: non-HTTP protocol BOGUS must be rejected")
	}
}

// Port of Chromium HTTP request line tests: Bare CR in request line ("GET /foo\rbar HTTP/1.1")
func TestChromium_RequestLine_BareCR(t *testing.T) {
	raw := "GET /foo\rbar HTTP/1.1\r\nHost: localhost\r\n\r\n"
	br := bufio.NewReader(strings.NewReader(raw))
	var req h1.Request
	err := req.ReadRequest(br, nil, 1024)
	if err == nil {
		t.Fatal("RFC 9112 §2.2 violation: bare CR in request line must be rejected")
	}
}

// Port of Chromium HTTP request line tests: Bare CR in protocol ("GET / HTTP/1.1\r\r\n")
func TestChromium_RequestLine_BareCRInProto(t *testing.T) {
	raw := "GET / HTTP/1.1\r\r\nHost: localhost\r\n\r\n"
	br := bufio.NewReader(strings.NewReader(raw))
	var req h1.Request
	err := req.ReadRequest(br, nil, 1024)
	if err == nil {
		t.Fatal("RFC 9112 §2.2 violation: bare CR in protocol must be rejected")
	}
}

// Port of Chromium net/http/http_stream_parser_unittest.cc: NullFails
// Test inserting null bytes into request line and headers.
func TestChromium_NullFails(t *testing.T) {
	baseReq := "GET /index.html HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"User-Agent: test\r\n" +
		"Content-Length: 4\r\n\r\n" +
		"body"

	// Insert null byte at every position inside header section (before \r\n\r\n)
	headerEnd := strings.Index(baseReq, "\r\n\r\n")
	for i := 0; i < headerEnd; i++ {
		corrupted := baseReq[:i] + "\x00" + baseReq[i:]
		br := bufio.NewReader(strings.NewReader(corrupted))
		var req h1.Request
		err := req.ReadRequest(br, nil, 1024)
		if err == nil {
			t.Fatalf("Chromium NullFails violation: request with null byte at position %d was accepted: %q", i, corrupted[:i+20])
		}
	}
}

// Port of Chromium obsolete line folding (obs-fold) tests:
// RFC 9112 §5.2: Header lines beginning with SP or HTAB are obs-fold and MUST be rejected with 400.
func TestChromium_Headers_ObsFold(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{
			name: "ObsFold_NewHeaderWithLeadingSpace",
			raw: "GET / HTTP/1.1\r\n" +
				"Host: localhost\r\n" +
				" X-Smuggled: evil\r\n\r\n",
		},
		{
			name: "ObsFold_ContinuationLine",
			raw: "GET / HTTP/1.1\r\n" +
				"Host: localhost\r\n" +
				"X-Header: value1\r\n" +
				" value2\r\n\r\n",
		},
		{
			name: "ObsFold_LeadingTab",
			raw: "GET / HTTP/1.1\r\n" +
				"Host: localhost\r\n" +
				"\tX-Header: value\r\n\r\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			br := bufio.NewReader(strings.NewReader(tc.raw))
			var req h1.Request
			err := req.ReadRequest(br, nil, 1024)
			if err == nil {
				t.Fatalf("RFC 9112 §5.2 violation: %s must be rejected with 400", tc.name)
			}
		})
	}
}

// Port of Chromium malformed header tests: Header line without colon
func TestChromium_Headers_NoColon(t *testing.T) {
	raw := "GET / HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"CorruptedLineWithoutColon\r\n\r\n"

	br := bufio.NewReader(strings.NewReader(raw))
	var req h1.Request
	err := req.ReadRequest(br, nil, 1024)
	if err == nil {
		t.Fatal("RFC 9112 §5 violation: header line without colon must be rejected")
	}
}

// Port of Chromium malformed header tests: Bare CR in header line
func TestChromium_Headers_BareCR(t *testing.T) {
	raw := "GET / HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"X-Custom: foo\rbar\r\n\r\n"

	br := bufio.NewReader(strings.NewReader(raw))
	var req h1.Request
	err := req.ReadRequest(br, nil, 1024)
	if err == nil {
		t.Fatal("RFC 9112 §2.2 violation: bare CR in header value must be rejected")
	}
}

// Port of Chromium net/http/http_stream_parser_unittest.cc: ReceiveOneByteAtATime & Pipelined Requests
// Pipelined chunked requests processed with 1-byte socket fragmentation.
func TestChromium_Pipelined_SplitPacketChunked(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ch := &h1.ConnHandler{
		Handler: func(req *h1.Request, res *h1.Response) error {
			res.StatusCode = 200
			res.Body = append([]byte(nil), req.Body...)
			return nil
		},
	}

	go func() {
		_ = ch.ServeConn(&oneByteConn{Conn: serverConn})
	}()

	// Send 2 pipelined requests: 1st with chunked encoding, 2nd with Content-Length
	req1 := "POST /first HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Transfer-Encoding: chunked\r\n\r\n" +
		"5\r\nhello\r\n" +
		"6\r\n world\r\n" +
		"0\r\n\r\n"

	req2 := "POST /second HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Content-Length: 9\r\n\r\n" +
		"pipelined"

	go func() {
		_, _ = clientConn.Write([]byte(req1 + req2))
	}()

	br := bufio.NewReader(clientConn)

	// Read response 1
	resp1Line, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("failed reading resp1 status: %v", err)
	}
	if !strings.Contains(resp1Line, "200") {
		t.Fatalf("expected 200 for resp1, got %q", resp1Line)
	}

	// Drain headers for resp1
	for {
		l, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("failed reading resp1 headers: %v", err)
		}
		if l == "\r\n" {
			break
		}
	}
	// Resp1 body has Content-Length 11 ("hello world")
	buf1 := make([]byte, 11)
	if _, err := io.ReadFull(br, buf1); err != nil {
		t.Fatalf("failed reading resp1 body: %v", err)
	}
	if string(buf1) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(buf1))
	}

	// Read response 2
	resp2Line, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("failed reading resp2 status: %v", err)
	}
	if !strings.Contains(resp2Line, "200") {
		t.Fatalf("expected 200 for resp2, got %q", resp2Line)
	}

	for {
		l, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("failed reading resp2 headers: %v", err)
		}
		if l == "\r\n" {
			break
		}
	}
	buf2 := make([]byte, 9)
	if _, err := io.ReadFull(br, buf2); err != nil {
		t.Fatalf("failed reading resp2 body: %v", err)
	}
	if string(buf2) != "pipelined" {
		t.Fatalf("expected 'pipelined', got %q", string(buf2))
	}
}

// Port of Chromium 100-Continue tests:
// Request with Expect: 100-continue receives 100 Continue before chunked body is received.
func TestChromium_Expect100Continue_ChunkedBody(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ch := &h1.ConnHandler{
		Handler: func(req *h1.Request, res *h1.Response) error {
			res.StatusCode = 200
			res.Body = append([]byte(nil), req.Body...)
			return nil
		},
	}

	go func() {
		_ = ch.ServeConn(serverConn)
	}()

	// 1. Client sends headers with Expect: 100-continue
	headers := "POST /upload HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Expect: 100-continue\r\n" +
		"Transfer-Encoding: chunked\r\n\r\n"

	go func() {
		_, _ = clientConn.Write([]byte(headers))
	}()

	br := bufio.NewReader(clientConn)

	// 2. Server must emit 100 Continue
	interimLine, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("failed reading interim response: %v", err)
	}
	if !strings.Contains(interimLine, "100 Continue") {
		t.Fatalf("expected 100 Continue, got %q", interimLine)
	}
	// Drain interim empty CRLF
	_, _ = br.ReadString('\n')

	// 3. Client sends chunked body
	chunkedBody := "5\r\nhello\r\n0\r\n\r\n"
	_, _ = clientConn.Write([]byte(chunkedBody))

	// 4. Server receives body and responds with 200 OK
	finalLine, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("failed reading final response: %v", err)
	}
	if !strings.Contains(finalLine, "200") {
		t.Fatalf("expected 200 OK, got %q", finalLine)
	}
}

// Zero allocation verification on server request reading hot path.
func TestChromium_ServerH1_ZeroAllocs(t *testing.T) {
	raw := "GET /api/v1/resource HTTP/1.1\r\nHost: localhost\r\nAccept: text/plain\r\n\r\n"
	r := strings.NewReader(raw)
	br := bufio.NewReader(r)
	req := h1.AcquireRequest()
	defer h1.ReleaseRequest(req)

	allocs := testing.AllocsPerRun(10, func() {
		r.Reset(raw)
		br.Reset(r)
		req.Reset()
		err := req.ReadRequest(br, nil, 1024)
		if err != nil {
			panic(fmt.Sprintf("unexpected error: %v", err))
		}
	})

	if allocs != 0 {
		t.Fatalf("expected 0 allocs on ReadRequest hot path, got %v", allocs)
	}
}
