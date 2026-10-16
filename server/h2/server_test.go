// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
	"golang.org/x/net/http2"

	h2client "github.com/lemon4ksan/mach/client/h2"
	"github.com/lemon4ksan/mach/proto/codec/hpack"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	machhttp "github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/proto/semantic/status"
	"github.com/lemon4ksan/mach/server/h2"
)

func TestH2Server_EndToEnd(t *testing.T) {
	var lc net.ListenConfig

	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	defer ln.Close() //nolint:errcheck

	handler := func(req *h2.ServerRequest, res *h2.ServerResponse) error {
		switch req.Path {
		case "/hello":
			res.StatusCode = status.OK
			res.Headers.Set("Content-Type", "text/plain")
			res.Body = []byte("Hello HTTP/2 World!")

			return nil

		case "/echo":
			res.StatusCode = status.OK
			res.Headers.Set("Content-Type", "application/octet-stream")

			res.Body = append([]byte("Echo: "), req.Body...)

			return nil

		default:
			res.StatusCode = status.NotFound
			res.Body = []byte("404 Not Found")
			return nil
		}
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				sc := h2.NewServerConn(c, handler)
				defer sc.Release()

				_ = sc.Serve()
			}(conn)
		}
	}()

	// Connect using Go's official HTTP/2 client transport over cleartext TCP
	client := &http.Client{
		//nolint:staticcheck
		Transport: &http2.Transport{
			//nolint:staticcheck
			AllowHTTP: true,
			//nolint:staticcheck
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		},
		Timeout: 5 * time.Second,
	}

	addr := ln.Addr().String()

	// 1. Test GET /hello
	reqHello, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr+"/hello", nil)
	require.NoError(t, err)
	resp, err := client.Do(reqHello)
	require.NoError(t, err)
	assert.Equal(t, status.OK, resp.StatusCode)
	assert.Equal(t, "HTTP/2.0", resp.Proto)

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	require.NoError(t, err)
	assert.Equal(t, "Hello HTTP/2 World!", string(body))

	// 2. Test POST /echo
	postData := "Multiplexed Stream Payload 2026"
	reqPost, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"http://"+addr+"/echo",
		strings.NewReader(postData),
	)
	reqPost.Header.Set("Content-Type", "text/plain")
	require.NoError(t, err)
	respPost, err := client.Do(reqPost)
	require.NoError(t, err)
	assert.Equal(t, status.OK, respPost.StatusCode)

	postBody, err := io.ReadAll(respPost.Body)
	_ = respPost.Body.Close()

	require.NoError(t, err)
	assert.Equal(t, "Echo: "+postData, string(postBody))

	// 3. Test High-Concurrency Streams
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			reqStream, sErr := http.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"http://"+addr+"/hello",
				nil,
			)
			if sErr != nil {
				t.Errorf("stream %d failed to create req: %v", id, sErr)
				return
			}

			r, err := client.Do(reqStream)
			if err != nil {
				t.Errorf("stream %d failed: %v", id, err)
				return
			}

			b, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()

			if string(b) != "Hello HTTP/2 World!" {
				t.Errorf("unexpected body on stream %d: %s", id, string(b))
			}
		}(i)
	}

	wg.Wait()
}

// TestAdversarial_H2_StreamTeardownRace_UnderAbruptDisconnect stress tests Escalation 2
// by dispatching bursts of concurrent streams, abruptly severing the TCP connection
// while stream goroutines are active, and validating under the Go race detector that
// ServerConn.Release() with clear(sc.streams) never races with in-flight stream deletion.
func TestAdversarial_H2_StreamTeardownRace_UnderAbruptDisconnect(t *testing.T) {
	const (
		iterations  = 5
		concurrency = 40
	)

	for range iterations {
		var lc net.ListenConfig

		ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
		require.NoError(t, err)

		handler := func(req *h2.ServerRequest, res *h2.ServerResponse) error {
			// Simulate in-flight processing latency
			select {
			case <-time.After(50 * time.Millisecond):
				res.StatusCode = status.OK
				res.Body = []byte("processed")

			case <-req.Ctx.Done():
				// Context cancelled upon connection close or stream reset
				return req.Ctx.Err()
			}

			return nil
		}

		serverDone := make(chan struct{})

		go func() {
			defer close(serverDone)

			for {
				c, aErr := ln.Accept()
				if aErr != nil {
					return
				}

				go func(conn net.Conn) {
					sc := h2.NewServerConn(conn, handler)
					defer sc.Release() // Stress tests Release() and clear(sc.streams)

					_ = sc.Serve()
				}(c)
			}
		}()

		var d net.Dialer

		rawConn, err := d.DialContext(context.Background(), "tcp", ln.Addr().String())
		require.NoError(t, err)

		client := h2client.NewConn(rawConn, h2client.ConnOpts{})
		require.NoError(t, client.Handshake())

		var wg sync.WaitGroup

		for i := range concurrency {
			wg.Add(1)

			go func(streamIdx int) {
				defer wg.Done()

				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()

				req := machhttp.AcquireRequest()
				resp := machhttp.AcquireResponse()

				defer machhttp.ReleaseRequest(req)
				defer machhttp.ReleaseResponse(resp)

				req.Header.SetMethod("GET")
				req.SetRequestURI("/teardown-stress")
				req.Header.SetHost(ln.Addr().String())

				_ = client.Do(ctx, req, resp)
			}(i)
		}

		// Let some streams start executing in server/h2 goroutines
		time.Sleep(15 * time.Millisecond)

		// Abruptly sever the underlying TCP connection while streams are in-flight
		_ = rawConn.Close()
		_ = client.Close()

		// Wait for all client callers to unblock
		wg.Wait()

		// Stop listener and wait for server
		_ = ln.Close()

		<-serverDone
	}
}

// TestAdversarial_H2_StreamTeardown_IncompleteStreams verifies that when client disconnects
// while streams are only partially opened (e.g. without EndStream), ServerConn.Release()
// cleans up safely without deadlocking or racing.
func TestAdversarial_H2_StreamTeardown_IncompleteStreams(t *testing.T) {
	var lc net.ListenConfig

	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	defer ln.Close()

	scReleased := make(chan struct{})

	go func() {
		c, aErr := ln.Accept()
		if aErr != nil {
			return
		}

		sc := h2.NewServerConn(c, func(req *h2.ServerRequest, res *h2.ServerResponse) error {
			res.StatusCode = status.OK

			return nil
		})

		defer func() {
			sc.Release()
			close(scReleased)
		}()

		_ = sc.Serve()
	}()

	var d net.Dialer

	conn, err := d.DialContext(context.Background(), "tcp", ln.Addr().String())
	require.NoError(t, err)

	// Send standard HTTP/2 client preface
	preface := "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
	_, err = conn.Write([]byte(preface))
	require.NoError(t, err)

	// Send empty SETTINGS frame: 9-byte header [0,0,0, 4, 0, 0,0,0,0]
	settingsFrame := []byte{0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00}
	_, err = conn.Write(settingsFrame)
	require.NoError(t, err)

	// Give the server time to process preface and SETTINGS
	time.Sleep(20 * time.Millisecond)

	// Abruptly close connection without finishing streams or sending GOAWAY
	_ = conn.Close()

	// Ensure ServerConn.Release() completes within 2 seconds without hanging
	select {
	case <-scReleased:
		// Success: Release completed cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ServerConn.Release() to complete on abrupt disconnect")
	}
}

func TestH2Server_RFC9113_MalformedHeaders_StreamReset(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	handler := func(req *h2.ServerRequest, res *h2.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("OK")
		return nil
	}

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		sc := h2.NewServerConn(conn, handler)
		defer sc.Release()
		_ = sc.Serve()
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer clientConn.Close()

	// Send preface + empty SETTINGS
	_, err = clientConn.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n\x00\x00\x00\x04\x00\x00\x00\x00\x00"))
	require.NoError(t, err)

	enc := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(enc)

	// Stream 1: Send request with uppercase header name (RFC 9113 §8.2.1)
	var headerPayload []byte
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	for _, kv := range [][2]string{
		{":method", "GET"},
		{":scheme", "http"},
		{":path", "/test"},
		{"x-Uppercase-Header", "value"},
	} {
		hf.Set(kv[0], kv[1])
		headerPayload = enc.AppendHeader(headerPayload, hf, false)
	}

	fr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(fr)

	hFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	hFrame.SetEndHeaders(true)
	hFrame.SetEndStream(true)
	hFrame.SetHeaders(headerPayload)

	fr.SetStream(1)
	fr.SetFlags(coreh2.FlagEndHeaders | coreh2.FlagEndStream)
	fr.SetBody(hFrame)

	bw := bufio.NewWriter(clientConn)
	_, err = fr.WriteTo(bw)
	require.NoError(t, err)
	_ = bw.Flush()

	// Read frames from server until RST_STREAM on stream 1 is observed
	br := bufio.NewReader(clientConn)
	foundRst := false
	for range 10 {
		respFr, rErr := coreh2.ReadFrameFrom(br)
		if rErr != nil {
			break
		}
		if respFr.Type() == coreh2.FrameResetStream && respFr.Stream() == 1 {
			rst := respFr.Body().(*coreh2.RstStream)
			assert.Equal(t, coreh2.ProtocolError, rst.Code())
			foundRst = true
			coreh2.ReleaseFrameHeader(respFr)
			break
		}
		coreh2.ReleaseFrameHeader(respFr)
	}
	assert.True(t, foundRst, "expected RST_STREAM(1, PROTOCOL_ERROR) for uppercase header name (RFC 9113 §8.2.1)")
}
