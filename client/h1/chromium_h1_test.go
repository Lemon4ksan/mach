// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/require"
	"github.com/lemon4ksan/mach/client/h1"
	"github.com/lemon4ksan/mach/proto/http"
)

// oneByteConnClient wraps net.Conn to deliver bytes 1 byte per Read call.
type oneByteConnClient struct {
	net.Conn
}

func (c *oneByteConnClient) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return c.Conn.Read(p[:1])
}

// Port of Chromium net/http/http_stream_parser_unittest.cc: NullFails
// Test that null byte in response status line causes an error.
func TestChromium_Client_NullInStatusLine(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		br := bufio.NewReader(c2)
		req := http.AcquireRequest()
		defer http.ReleaseRequest(req)
		_ = req.Read(br)

		// Inject null byte into status line reason phrase
		_, _ = c2.Write([]byte("HTTP/1.1 200 OK\x00Injected\r\nContent-Length: 0\r\n\r\n"))
	}()

	cc := h1.NewClientConn(c1)
	defer cc.Close()

	req := http.AcquireRequest()
	resp := http.AcquireResponse()
	defer http.ReleaseRequest(req)
	defer http.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/")
	req.Header.SetHost("localhost")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cc.Do(ctx, req, resp)
	if err == nil {
		t.Fatal("Chromium NullFails violation: response with null byte in status line was accepted")
	}
}

// Port of Chromium status line tests: Bare CR in status line reason phrase
func TestChromium_Client_BareCRInReasonPhrase(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		br := bufio.NewReader(c2)
		req := http.AcquireRequest()
		defer http.ReleaseRequest(req)
		_ = req.Read(br)

		_, _ = c2.Write([]byte("HTTP/1.1 200 OK\rInjected\r\nContent-Length: 0\r\n\r\n"))
	}()

	cc := h1.NewClientConn(c1)
	defer cc.Close()

	req := http.AcquireRequest()
	resp := http.AcquireResponse()
	defer http.ReleaseRequest(req)
	defer http.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/")
	req.Header.SetHost("localhost")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cc.Do(ctx, req, resp)
	if err == nil {
		t.Fatal("RFC 9112 §2.2 violation: bare CR in status line reason phrase must be rejected")
	}
}

// Port of Chromium net/http/http_stream_parser_unittest.cc: MixedResponseHeaders
// Test reading informational (100 Continue) packed with final response in same stream.
func TestChromium_Client_100Continue_Mixed(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		br := bufio.NewReader(c2)
		req := http.AcquireRequest()
		defer http.ReleaseRequest(req)
		_ = req.Read(br)

		// 100 Continue followed immediately by 200 OK
		rawResp := "HTTP/1.1 100 Continue\r\n\r\n" +
			"HTTP/1.1 200 OK\r\n" +
			"Content-Length: 5\r\n\r\n" +
			"hello"
		_, _ = c2.Write([]byte(rawResp))
	}()

	cc := h1.NewClientConn(c1)
	defer cc.Close()

	req := http.AcquireRequest()
	resp := http.AcquireResponse()
	defer http.ReleaseRequest(req)
	defer http.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/")
	req.Header.SetHost("localhost")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cc.Do(ctx, req, resp)
	require.NoError(t, err)

	if resp.StatusCode() != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	if string(resp.Body()) != "hello" {
		t.Fatalf("expected body 'hello', got %q", string(resp.Body()))
	}
}

// Port of Chromium net/http/http_stream_parser_unittest.cc: ReceiveOneByteAtATime
// Test receiving 100 Continue and final 200 response with 1-byte read fragmentation.
func TestChromium_Client_100Continue_1ByteFragmentation(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		br := bufio.NewReader(c2)
		req := http.AcquireRequest()
		defer http.ReleaseRequest(req)
		_ = req.Read(br)

		rawResp := "HTTP/1.1 100 Continue\r\n\r\n" +
			"HTTP/1.1 200 OK\r\n" +
			"Content-Length: 5\r\n\r\n" +
			"hello"
		_, _ = c2.Write([]byte(rawResp))
	}()

	cc := h1.NewClientConn(&oneByteConnClient{Conn: c1})
	defer cc.Close()

	req := http.AcquireRequest()
	resp := http.AcquireResponse()
	defer http.ReleaseRequest(req)
	defer http.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/")
	req.Header.SetHost("localhost")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cc.Do(ctx, req, resp)
	require.NoError(t, err)

	if resp.StatusCode() != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	if string(resp.Body()) != "hello" {
		t.Fatalf("expected body 'hello', got %q", string(resp.Body()))
	}
}

// Port of Chromium net/http/http_response_headers_unittest.cc: Do not add missing status text ("HTTP/1.1 201\n")
func TestChromium_Client_MissingReasonPhrase(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		br := bufio.NewReader(c2)
		req := http.AcquireRequest()
		defer http.ReleaseRequest(req)
		_ = req.Read(br)

		_, _ = c2.Write([]byte("HTTP/1.1 201\r\nContent-Length: 0\r\n\r\n"))
	}()

	cc := h1.NewClientConn(c1)
	defer cc.Close()

	req := http.AcquireRequest()
	resp := http.AcquireResponse()
	defer http.ReleaseRequest(req)
	defer http.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/")
	req.Header.SetHost("localhost")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cc.Do(ctx, req, resp)
	require.NoError(t, err)

	if resp.StatusCode() != 201 {
		t.Fatalf("expected status 201, got %d", resp.StatusCode())
	}
}

// Port of Chromium net/http/http_response_headers_unittest.cc: Normalize whitespace ("HTTP/1.1    202   Accepted  \n")
func TestChromium_Client_MultipleSpacesInStatusLine(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		br := bufio.NewReader(c2)
		req := http.AcquireRequest()
		defer http.ReleaseRequest(req)
		_ = req.Read(br)

		_, _ = c2.Write([]byte("HTTP/1.1    202   Accepted\r\nContent-Length: 0\r\n\r\n"))
	}()

	cc := h1.NewClientConn(c1)
	defer cc.Close()

	req := http.AcquireRequest()
	resp := http.AcquireResponse()
	defer http.ReleaseRequest(req)
	defer http.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/")
	req.Header.SetHost("localhost")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cc.Do(ctx, req, resp)
	require.NoError(t, err)

	if resp.StatusCode() != 202 {
		t.Fatalf("expected status 202, got %d", resp.StatusCode())
	}
	if string(resp.Header.StatusMessage()) != "Accepted" {
		t.Fatalf("expected normalized status message 'Accepted', got %q", string(resp.Header.StatusMessage()))
	}
}
