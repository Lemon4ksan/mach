// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package e2e_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/net/http/status"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	machhttp "github.com/lemon4ksan/mach/proto/http"
	h1server "github.com/lemon4ksan/mach/server/h1"
)

// ============================================================================
// Tier 1: Feature Coverage (RFC 9112 / RFC 9110)
// ============================================================================

func TestH1_Tier1_GetRequestAndQueryParsing(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		assert.Equal(t, "GET", req.Method)
		assert.Equal(t, "/api/v1/resource", req.Path)
		assert.Equal(t, "filter=active&limit=25", req.Query)
		assert.Equal(t, "trace-12345", req.Headers.Get("X-Test-Trace"))

		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "application/json")
		res.Body = []byte(`{"status":"ok","items":[1,2,3]}`)

		return nil
	})
	defer cleanup()

	client, rawConn := dialH1Client(t, addr)
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/api/v1/resource?filter=active&limit=25")
	req.Header.SetHost(addr)
	req.Header.Set("X-Test-Trace", "trace-12345")

	err := client.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, `{"status":"ok","items":[1,2,3]}`, string(resp.Body()))
	assert.Equal(t, "application/json", string(resp.Header.Peek("Content-Type")))
}

func TestH1_Tier1_PostWithContentLengthBody(t *testing.T) {
	t.Parallel()

	expectedBody := `{"name":"mach","role":"high-performance-engine"}`

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		assert.Equal(t, "POST", req.Method)
		assert.Equal(t, "/submit", req.Path)
		assert.Equal(t, expectedBody, string(req.Body))

		res.StatusCode = status.Created
		res.Headers.Set("Content-Type", "application/json")
		res.Body = []byte(`{"id":"gen-42","created":true}`)

		return nil
	})
	defer cleanup()

	client, rawConn := dialH1Client(t, addr)
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("/submit")
	req.Header.SetHost(addr)
	req.Header.Set("Content-Type", "application/json")
	req.SetBody([]byte(expectedBody))

	err := client.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode())
	assert.Equal(t, `{"id":"gen-42","created":true}`, string(resp.Body()))
}

func TestH1_Tier1_KeepAliveConnectionReuse(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		res.StatusCode = status.OK
		res.Body = []byte("response-for-" + req.Path)

		return nil
	})
	defer cleanup()

	client, rawConn := dialH1Client(t, addr)
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Execute 5 sequential requests on the same connection
	for i := 1; i <= 5; i++ {
		req := machhttp.AcquireRequest()
		resp := machhttp.AcquireResponse()

		path := fmt.Sprintf("/req-%d", i)
		req.Header.SetMethod("GET")
		req.SetRequestURI(path)
		req.Header.SetHost(addr)

		err := client.Do(ctx, req, resp)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode())
		assert.Equal(t, "response-for-"+path, string(resp.Body()))

		machhttp.ReleaseRequest(req)
		machhttp.ReleaseResponse(resp)
	}
}

func TestH1_Tier1_ChunkedTransferEncoding(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "text/plain")
		res.StreamWriter = func(w io.Writer) error {
			chunks := []string{"chunk-alpha-", "chunk-beta-", "chunk-gamma"}
			for _, c := range chunks {
				if _, err := w.Write([]byte(c)); err != nil {
					return err
				}
			}

			return nil
		}

		return nil
	})
	defer cleanup()

	client, rawConn := dialH1Client(t, addr)
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/stream")
	req.Header.SetHost(addr)

	err := client.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "chunk-alpha-chunk-beta-chunk-gamma", string(resp.Body()))
}

func TestH1_Tier1_Expect100ContinueNegotiation(t *testing.T) {
	t.Parallel()

	bodyData := "payload-verified-after-continue"

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		assert.Equal(t, bodyData, string(req.Body))
		res.StatusCode = status.OK
		res.Body = []byte("continue-flow-success")

		return nil
	})
	defer cleanup()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	bw := bufio.NewWriter(conn)
	br := bufio.NewReader(conn)

	// Send headers with Expect: 100-continue
	hdr := fmt.Sprintf("POST /upload HTTP/1.1\r\nHost: %s\r\nExpect: 100-continue\r\nContent-Length: %d\r\n\r\n",
		addr, len(bodyData))
	_, err = bw.WriteString(hdr)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Read 100 Continue response line
	line1, err := br.ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, line1, "100 Continue")

	line2, err := br.ReadString('\n') // empty CRLF after 100 Continue
	require.NoError(t, err)
	assert.Equal(t, "\r\n", line2)

	// Transmit body now
	_, err = bw.WriteString(bodyData)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Read final 200 OK response
	finalResp := machhttp.AcquireResponse()
	defer machhttp.ReleaseResponse(finalResp)
	err = finalResp.Read(br)
	require.NoError(t, err)
	assert.Equal(t, 200, finalResp.StatusCode())
	assert.Equal(t, "continue-flow-success", string(finalResp.Body()))
}

func TestH1_Tier1_ConnectionHijacking(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		if req.Path == "/upgrade" {
			netConn, rw, err := req.HijackFn()
			if err != nil {
				return err
			}

			_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: test-raw\r\n\r\n")
			_ = rw.Flush()

			_, _ = rw.WriteString("HIJACKED_RAW_STREAM_DATA")
			_ = rw.Flush()
			_ = netConn.Close()

			return nil
		}

		res.StatusCode = status.NotFound
		return nil
	})
	defer cleanup()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	bw := bufio.NewWriter(conn)
	br := bufio.NewReader(conn)

	reqWire := fmt.Sprintf("GET /upgrade HTTP/1.1\r\nHost: %s\r\nUpgrade: test-raw\r\nConnection: Upgrade\r\n\r\n", addr)
	_, err = bw.WriteString(reqWire)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Read status line
	statusLine, err := br.ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, statusLine, "101 Switching Protocols")

	// Read until end of headers (\r\n\r\n)
	for {
		line, rErr := br.ReadString('\n')
		require.NoError(t, rErr)
		if line == "\r\n" {
			break
		}
	}

	// Read hijacked raw payload
	rawPayload := make([]byte, len("HIJACKED_RAW_STREAM_DATA"))
	_, err = io.ReadFull(br, rawPayload)
	require.NoError(t, err)
	assert.Equal(t, "HIJACKED_RAW_STREAM_DATA", string(rawPayload))
}

func TestH1_Tier1_EarlyHints103(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		hints := http.Header{}
		hints.Set("Link", "</assets/style.css>; rel=preload; as=style")
		if err := req.EarlyHintsFn(hints); err != nil {
			return err
		}

		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "text/html")
		res.Body = []byte("<html><body>Hello with Early Hints</body></html>")

		return nil
	})
	defer cleanup()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	bw := bufio.NewWriter(conn)
	br := bufio.NewReader(conn)

	reqWire := fmt.Sprintf("GET /index.html HTTP/1.1\r\nHost: %s\r\n\r\n", addr)
	_, err = bw.WriteString(reqWire)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// 1. First response is 103 Early Hints
	line1, err := br.ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, line1, "103 Early Hints")

	// Read 103 headers
	var linkHeader string
	for {
		line, rErr := br.ReadString('\n')
		require.NoError(t, rErr)
		if line == "\r\n" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "link:") {
			linkHeader = strings.TrimSpace(line)
		}
	}
	assert.Contains(t, linkHeader, "</assets/style.css>; rel=preload; as=style")

	// 2. Final response is 200 OK
	finalResp := machhttp.AcquireResponse()
	defer machhttp.ReleaseResponse(finalResp)
	err = finalResp.Read(br)
	require.NoError(t, err)
	assert.Equal(t, 200, finalResp.StatusCode())
	assert.Contains(t, string(finalResp.Body()), "Hello with Early Hints")
}

// ============================================================================
// Tier 2: Boundary & Corner Cases (RFC 9112 / RFC 9110)
// ============================================================================

func TestH1_Tier2_MaxBodySizeEnforcement(t *testing.T) {
	t.Parallel()

	// Server configured with strict 256 bytes MaxBodySize
	ch := h1server.ConnHandler{
		MaxBodySize: 256,
		Handler: func(req *h1server.Request, res *h1server.Response) error {
			res.StatusCode = status.OK
			return nil
		},
	}

	addr, cleanup := startH1ServerWithOpts(t, ch)
	defer cleanup()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	oversizedPayload := strings.Repeat("A", 1024)
	reqWire := fmt.Sprintf("POST /oversized HTTP/1.1\r\nHost: %s\r\nContent-Length: %d\r\n\r\n%s",
		addr, len(oversizedPayload), oversizedPayload)

	_, err = conn.Write([]byte(reqWire))
	require.NoError(t, err)

	// Server should close connection due to ErrBodyTooLarge
	br := bufio.NewReader(conn)
	buf := make([]byte, 128)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := br.Read(buf)
	assert.True(t, n == 0 || err != nil)
}

func TestH1_Tier2_MissingHostHeader(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	// RFC 9112 §3.2 violation: HTTP/1.1 without Host header
	reqWire := "GET /no-host HTTP/1.1\r\nUser-Agent: test\r\n\r\n"
	_, err = conn.Write([]byte(reqWire))
	require.NoError(t, err)

	// Server must reject with error and terminate connection
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 128)
	n, rErr := conn.Read(buf)
	assert.True(t, n == 0 || rErr != nil)
}

func TestH1_Tier2_LeadingCRLFTolerance(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		assert.Equal(t, "/ping", req.Path)
		res.StatusCode = status.OK
		res.Body = []byte("pong")

		return nil
	})
	defer cleanup()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	// RFC 9112 §2.2: Server MUST tolerate leading CRLFs before request-line
	reqWire := fmt.Sprintf("\r\n\r\nGET /ping HTTP/1.1\r\nHost: %s\r\n\r\n", addr)
	_, err = conn.Write([]byte(reqWire))
	require.NoError(t, err)

	br := bufio.NewReader(conn)
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseResponse(resp)

	err = resp.Read(br)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "pong", string(resp.Body()))
}

func TestH1_Tier2_RequestSmugglingMitigation(t *testing.T) {
	t.Parallel()

	// RFC 9112 §6.3 Item 3: If both Transfer-Encoding and Content-Length are present,
	// Transfer-Encoding overrides Content-Length to mitigate Request Smuggling.
	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		assert.Equal(t, "hello", string(req.Body))
		res.StatusCode = status.OK
		res.Body = []byte("smuggling-checked")

		return nil
	})
	defer cleanup()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	// Dual TE + CL
	reqWire := fmt.Sprintf("POST /smuggle HTTP/1.1\r\nHost: %s\r\nTransfer-Encoding: chunked\r\nContent-Length: 5\r\n\r\n5\r\nhello\r\n0\r\n\r\n", addr)
	_, err = conn.Write([]byte(reqWire))
	require.NoError(t, err)

	br := bufio.NewReader(conn)
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseResponse(resp)

	err = resp.Read(br)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "smuggling-checked", string(resp.Body()))
}

func TestH1_Tier2_ZeroLengthBodies(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		assert.Equal(t, 0, len(req.Body))
		res.StatusCode = status.NoContent

		return nil
	})
	defer cleanup()

	client, rawConn := dialH1Client(t, addr)
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("/empty")
	req.Header.SetHost(addr)
	req.Header.Set("Content-Length", "0")

	err := client.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 204, resp.StatusCode())
	assert.Equal(t, 0, len(resp.Body()))
}

func TestH1_Tier2_LargeHeaderBlocks(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		for i := 1; i <= 10; i++ {
			k := fmt.Sprintf("X-Test-Hdr-%02d", i)
			assert.NotEmpty(t, req.Headers.Get(k))
		}
		res.StatusCode = status.OK
		res.Body = []byte("large-headers-ok")

		return nil
	})
	defer cleanup()

	client, rawConn := dialH1Client(t, addr)
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/large-headers")
	req.Header.SetHost(addr)

	// Add 10 headers each with ~200 chars to test large header block decoding
	pad := strings.Repeat("h", 200)
	for i := 1; i <= 10; i++ {
		req.Header.Set(fmt.Sprintf("X-Test-Hdr-%02d", i), pad)
	}

	err := client.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "large-headers-ok", string(resp.Body()))
}

// ============================================================================
// Tier 3: Cross-Feature Combinations
// ============================================================================

func TestH1_Tier3_ChunkedKeepAlivePipelining(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		res.StatusCode = status.OK
		path := req.Path
		res.StreamWriter = func(w io.Writer) error {
			_, err := fmt.Fprintf(w, "chunked-payload-for-%s", path)
			return err
		}

		return nil
	})
	defer cleanup()

	client, rawConn := dialH1Client(t, addr)
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 1; i <= 3; i++ {
		req := machhttp.AcquireRequest()
		resp := machhttp.AcquireResponse()

		path := fmt.Sprintf("/stream-pipe-%d", i)
		req.Header.SetMethod("GET")
		req.SetRequestURI(path)
		req.Header.SetHost(addr)

		err := client.Do(ctx, req, resp)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode())
		assert.Equal(t, "chunked-payload-for-"+path, string(resp.Body()))

		machhttp.ReleaseRequest(req)
		machhttp.ReleaseResponse(resp)
	}
}

func TestH1_Tier3_Expect100ContinueLargeBodyKeepAlive(t *testing.T) {
	t.Parallel()

	largeBody := strings.Repeat("B", 32768)

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		if req.Path == "/large-upload" {
			assert.Equal(t, len(largeBody), len(req.Body))
			res.StatusCode = status.OK
			res.Body = []byte("large-upload-done")

			return nil
		}

		res.StatusCode = status.OK
		res.Body = []byte("second-request-ok")

		return nil
	})
	defer cleanup()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	bw := bufio.NewWriter(conn)
	br := bufio.NewReader(conn)

	// 1. Send Expect 100 Continue request
	hdr := fmt.Sprintf("POST /large-upload HTTP/1.1\r\nHost: %s\r\nExpect: 100-continue\r\nContent-Length: %d\r\n\r\n",
		addr, len(largeBody))
	_, err = bw.WriteString(hdr)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Read 100 Continue
	line1, err := br.ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, line1, "100 Continue")
	_, err = br.ReadString('\n') // discard CRLF
	require.NoError(t, err)

	// Send large body
	_, err = bw.WriteString(largeBody)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Read first 200 response
	resp1 := machhttp.AcquireResponse()
	defer machhttp.ReleaseResponse(resp1)
	err = resp1.Read(br)
	require.NoError(t, err)
	assert.Equal(t, 200, resp1.StatusCode())
	assert.Equal(t, "large-upload-done", string(resp1.Body()))

	// 2. Send second request on the SAME keep-alive connection
	secondReq := fmt.Sprintf("GET /second HTTP/1.1\r\nHost: %s\r\n\r\n", addr)
	_, err = bw.WriteString(secondReq)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	resp2 := machhttp.AcquireResponse()
	defer machhttp.ReleaseResponse(resp2)
	err = resp2.Read(br)
	require.NoError(t, err)
	assert.Equal(t, 200, resp2.StatusCode())
	assert.Equal(t, "second-request-ok", string(resp2.Body()))
}

func TestH1_Tier3_EarlyHintsWithConnectionClose(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		hints := http.Header{}
		hints.Set("Link", "</critical.js>; rel=preload")
		_ = req.EarlyHintsFn(hints)

		res.StatusCode = status.OK
		res.Headers.Set("Connection", "close")
		res.Body = []byte("final-with-close")

		return nil
	})
	defer cleanup()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	bw := bufio.NewWriter(conn)
	br := bufio.NewReader(conn)

	reqWire := fmt.Sprintf("GET /hints-close HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", addr)
	_, err = bw.WriteString(reqWire)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// 1. Read 103 Early Hints
	line1, err := br.ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, line1, "103 Early Hints")

	for {
		line, rErr := br.ReadString('\n')
		require.NoError(t, rErr)
		if line == "\r\n" {
			break
		}
	}

	// 2. Read final 200 response
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseResponse(resp)
	err = resp.Read(br)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "final-with-close", string(resp.Body()))

	// Connection closed
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	extra := make([]byte, 16)
	_, err = br.Read(extra)
	assert.ErrorIs(t, err, io.EOF)
}

func TestH1_Tier3_BidirectionalChunkedStreaming(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		// Server verifies chunked body received
		assert.Equal(t, "hello-from-chunked-client", string(req.Body))

		// Server echoes back chunked response
		res.StatusCode = status.OK
		res.StreamWriter = func(w io.Writer) error {
			_, err := w.Write([]byte("echo:" + string(req.Body)))
			return err
		}

		return nil
	})
	defer cleanup()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	bw := bufio.NewWriter(conn)
	br := bufio.NewReader(conn)

	// Send chunked request
	chunkedReq := fmt.Sprintf("POST /bidi-chunked HTTP/1.1\r\nHost: %s\r\nTransfer-Encoding: chunked\r\n\r\n"+
		"B\r\nhello-from-\r\n"+
		"E\r\nchunked-client\r\n"+
		"0\r\n\r\n", addr)

	_, err = bw.WriteString(chunkedReq)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseResponse(resp)

	err = resp.Read(br)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "echo:hello-from-chunked-client", string(resp.Body()))
}

// ============================================================================
// Tier 4: Real-World Application Scenarios
// ============================================================================

func TestH1_Tier4_HighThroughputKeepAlivePipeline(t *testing.T) {
	t.Parallel()

	counter := 0
	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		counter++
		res.StatusCode = status.OK
		res.Body = []byte(fmt.Sprintf("pipeline-ack-%d", counter))

		return nil
	})
	defer cleanup()

	client, rawConn := dialH1Client(t, addr)
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	// 100 sequential requests on the same connection exercising Per-P storage reuse
	for i := 1; i <= 100; i++ {
		req.Reset()
		resp.Reset()

		req.Header.SetMethod("GET")
		req.SetRequestURI(fmt.Sprintf("/item/%d", i))
		req.Header.SetHost(addr)

		err := client.Do(ctx, req, resp)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode())
		assert.Equal(t, fmt.Sprintf("pipeline-ack-%d", i), string(resp.Body()))
	}
}

func TestH1_Tier4_MultiMegabytePayloadTransfer(t *testing.T) {
	t.Parallel()

	// 2MB deterministic payload
	payloadSize := 2 * 1024 * 1024
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	expectedHash := sha256.Sum256(payload)

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		reqHash := sha256.Sum256(req.Body)
		assert.Equal(t, expectedHash, reqHash)

		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "application/octet-stream")
		res.Body = req.Body

		return nil
	})
	defer cleanup()

	client, rawConn := dialH1Client(t, addr)
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("/large-transfer")
	req.Header.SetHost(addr)
	req.SetBody(payload)

	err := client.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())

	respHash := sha256.Sum256(resp.Body())
	assert.Equal(t, expectedHash, respHash)
}
