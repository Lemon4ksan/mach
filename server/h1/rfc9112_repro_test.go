// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/semantic/status"
	"github.com/lemon4ksan/mach/server/h1"
)

// TestRepro_DuplicateConflictingContentLength demonstrates that multiple conflicting
// Content-Length headers are silently accepted (RFC 9112 §6.3 Item 5).
func TestRepro_DuplicateConflictingContentLength(t *testing.T) {
	raw := "POST /cl-cl HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Content-Length: 5\r\n" +
		"Content-Length: 10\r\n\r\n" +
		"1234567890"

	br := bufio.NewReader(strings.NewReader(raw))
	var req h1.Request
	err := req.ReadRequest(br, nil, 1024)
	if err == nil {
		t.Fatal("RFC 9112 §6.3 violation: conflicting duplicate Content-Length headers must be rejected")
	}
}

// TestRepro_NonDigitContentLength demonstrates that Content-Length with a leading plus sign '+5'
// is illegally accepted (RFC 9112 §6.3).
func TestRepro_NonDigitContentLength(t *testing.T) {
	raw := "POST /test HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Content-Length: +5\r\n\r\n" +
		"12345"

	br := bufio.NewReader(strings.NewReader(raw))
	var req h1.Request
	err := req.ReadRequest(br, nil, 1024)
	if err == nil {
		t.Fatal("RFC 9112 §6.3 violation: non-digit Content-Length '+5' must be rejected")
	}
}

// TestRepro_MultipleHostHeaders demonstrates that multiple Host headers
// are silently overwritten instead of rejected with 400 (RFC 9112 §3.2).
func TestRepro_MultipleHostHeaders(t *testing.T) {
	raw := "GET / HTTP/1.1\r\n" +
		"Host: alpha.com\r\n" +
		"Host: beta.com\r\n\r\n"

	br := bufio.NewReader(strings.NewReader(raw))
	var req h1.Request
	err := req.ReadRequest(br, nil, 1024)
	if err == nil {
		t.Fatal("RFC 9112 §3.2 violation: multiple Host headers must be rejected")
	}
}

// TestRepro_MultipleOrInvalidTransferEncoding demonstrates that multiple Transfer-Encoding headers
// or duplicate 'chunked' are accepted (RFC 9112 §6.1).
func TestRepro_MultipleOrInvalidTransferEncoding(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{
			name: "MultipleTEHeaders_ChunkedThenIdentity",
			raw: "POST /te HTTP/1.1\r\n" +
				"Host: localhost\r\n" +
				"Transfer-Encoding: chunked\r\n" +
				"Transfer-Encoding: identity\r\n\r\n" +
				"5\r\nhello\r\n0\r\n\r\n",
		},
		{
			name: "DuplicateChunkedApplied",
			raw: "POST /te HTTP/1.1\r\n" +
				"Host: localhost\r\n" +
				"Transfer-Encoding: chunked, chunked\r\n\r\n" +
				"5\r\nhello\r\n0\r\n\r\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			br := bufio.NewReader(strings.NewReader(tc.raw))
			var req h1.Request
			err := req.ReadRequest(br, nil, 1024)
			if err == nil {
				t.Fatalf("RFC 9112 §6.1 violation: %s must be rejected", tc.name)
			}
		})
	}
}

// TestRepro_Response1xxMustNotHaveContentLength demonstrates that responses with 1xx status code
// (e.g. 101 Switching Protocols) illegally emit Content-Length: 0 (RFC 9112 §6.3 Item 1).
func TestRepro_Response1xxMustNotHaveContentLength(t *testing.T) {
	var res h1.Response
	res.StatusCode = status.SwitchingProtocols // 101

	var buf bytes.Buffer
	bw := &bytesconv.ByteBuffer{B: make([]byte, 0, 1024)}
	bw.ResetWriter(&buf)

	err := res.WriteTo(bw, true, true)
	require.NoError(t, err)
	_ = bw.Flush()

	out := buf.String()
	if strings.Contains(out, "Content-Length:") {
		t.Fatalf("RFC 9112 §6.3 Item 1 violation: 1xx response must not have Content-Length header, got:\n%s", out)
	}
	if strings.Contains(out, "Connection: keep-alive") {
		t.Fatalf("RFC 9110 §7.8 violation: 101 Switching Protocols must not send Connection: keep-alive, got:\n%s", out)
	}
}

// TestRepro_Expect100ContinueOnHTTP10 demonstrates that HTTP/1.0 requests with Expect: 100-continue
// illegally receive a 100 Continue response (RFC 9110 §10.1.1).
func TestRepro_Expect100ContinueOnHTTP10(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ch := &h1.ConnHandler{
		Handler: func(req *h1.Request, res *h1.Response) error {
			res.StatusCode = 200
			res.Body = []byte("done")
			return nil
		},
	}

	go func() {
		_ = ch.ServeConn(serverConn)
	}()

	// Send HTTP/1.0 request with Expect: 100-continue
	raw := "POST /test HTTP/1.0\r\nContent-Length: 5\r\nExpect: 100-continue\r\n\r\nhello"
	go func() {
		_, _ = clientConn.Write([]byte(raw))
	}()

	br := bufio.NewReader(clientConn)
	line, err := br.ReadString('\n')
	require.NoError(t, err)

	if strings.Contains(line, "100 Continue") {
		t.Fatalf("RFC 9110 §10.1.1 violation: server sent 100 Continue to HTTP/1.0 client: %q", line)
	}
}

// TestRepro_EarlyHintsOnHTTP10 demonstrates that 103 Early Hints are emitted to HTTP/1.0 clients (RFC 8297).
func TestRepro_EarlyHintsOnHTTP10(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	ch := &h1.ConnHandler{
		Handler: func(req *h1.Request, res *h1.Response) error {
			_ = req.WriteEarlyHints(http.Header{"Link": []string{"</style.css>; rel=preload"}})
			res.StatusCode = 200
			return nil
		},
	}

	go func() {
		_ = ch.ServeConn(c2)
	}()

	_, _ = c1.Write([]byte("GET / HTTP/1.0\r\n\r\n"))

	br := bufio.NewReader(c1)
	line, err := br.ReadString('\n')
	require.NoError(t, err)

	if strings.Contains(line, "103 Early Hints") {
		t.Fatalf("RFC 8297 violation: Early Hints emitted to HTTP/1.0 request: %s", line)
	}
}

// TestRepro_UpgradeTransitionsLoopInServeConn demonstrates that a 101 Switching Protocols response
// terminates the HTTP/1.1 request-response loop without hanging or trying to parse upgraded frames.
func TestRepro_UpgradeTransitionsLoopInServeConn(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ch := &h1.ConnHandler{
		Handler: func(req *h1.Request, res *h1.Response) error {
			res.StatusCode = status.SwitchingProtocols // 101
			return nil
		},
	}

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- ch.ServeConn(serverConn)
	}()

	// Send Upgrade request followed immediately by raw non-HTTP websocket frame
	rawReq := "GET /ws HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n" +
		"\x81\x05hello" // raw websocket frame
	_, _ = clientConn.Write([]byte(rawReq))

	br := bufio.NewReader(clientConn)
	resp, err := http.ReadResponse(br, nil)
	require.NoError(t, err)
	_ = resp.Body.Close()

	if resp.StatusCode != 101 {
		t.Fatalf("expected 101, got %d", resp.StatusCode)
	}

	// ServeConn should cleanly return nil, having completed the HTTP phase of the connection
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("ServeConn failed trying to parse upgraded stream as HTTP: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("ServeConn did not exit after 101 Switching Protocols transition")
	}
}
