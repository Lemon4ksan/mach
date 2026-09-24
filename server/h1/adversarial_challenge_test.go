// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/proto/http/status"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/server/h1"
)

// TestAdversarial_H1_Smuggling_PipelinedDataNeverAnswered verifies that when a client sends
// conflicting framing headers (dual Transfer-Encoding and Content-Length) followed immediately
// by pipelined requests, the server MUST close the TCP socket and the pipelined data is NEVER
// processed or answered (RFC 9112 §6.3 & §11.2).
func TestAdversarial_H1_Smuggling_PipelinedDataNeverAnswered(t *testing.T) {
	testCases := []struct {
		name       string
		requestRaw string
	}{
		{
			name: "TE_Before_CL_Chunked",
			requestRaw: "POST /smuggle-te-first HTTP/1.1\r\n" +
				"Host: 127.0.0.1\r\n" +
				"Transfer-Encoding: chunked\r\n" +
				"Content-Length: 5\r\n" +
				"\r\n" +
				"5\r\nhello\r\n0\r\n\r\n" +
				"POST /pipelined-evil HTTP/1.1\r\n" +
				"Host: 127.0.0.1\r\n" +
				"Content-Length: 4\r\n" +
				"\r\n" +
				"evil",
		},
		{
			name: "CL_Before_TE_Chunked",
			requestRaw: "POST /smuggle-cl-first HTTP/1.1\r\n" +
				"Host: 127.0.0.1\r\n" +
				"Content-Length: 5\r\n" +
				"Transfer-Encoding: chunked\r\n" +
				"\r\n" +
				"5\r\nworld\r\n0\r\n\r\n" +
				"POST /pipelined-evil HTTP/1.1\r\n" +
				"Host: 127.0.0.1\r\n" +
				"Content-Length: 4\r\n" +
				"\r\n" +
				"evil",
		},
		{
			name: "Mixed_Case_Headers",
			requestRaw: "POST /smuggle-mixed HTTP/1.1\r\n" +
				"Host: 127.0.0.1\r\n" +
				"transfer-encoding: chunked\r\n" +
				"content-length: 5\r\n" +
				"\r\n" +
				"5\r\nalive\r\n0\r\n\r\n" +
				"GET /pipelined-evil HTTP/1.1\r\n" +
				"Host: 127.0.0.1\r\n\r\n",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var (
				invokedMu sync.Mutex
				pathsSeen []string
			)

			ch := &h1.ConnHandler{
				Handler: func(req *h1.Request, res *h1.Response) error {
					invokedMu.Lock()

					pathsSeen = append(pathsSeen, req.URI)
					invokedMu.Unlock()

					res.StatusCode = status.OK
					res.Body = []byte("smuggling-handled")

					return nil
				},
			}

			var lc net.ListenConfig

			ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
			require.NoError(t, err)

			defer ln.Close()

			go func() {
				c, aErr := ln.Accept()
				if aErr != nil {
					return
				}

				_ = ch.ServeConn(c)
			}()

			var d net.Dialer

			conn, err := d.DialContext(context.Background(), "tcp", ln.Addr().String())
			require.NoError(t, err)

			defer conn.Close()

			// Send smuggling request + pipelined request in a single TCP write
			_, err = conn.Write([]byte(tc.requestRaw))
			require.NoError(t, err)

			br := bufio.NewReader(conn)

			resp, err := http.ReadResponse(br, nil)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			// RFC 9112 §11.2: Server MUST emit Connection: close indicator
			assert.True(t, resp.Close || resp.Header.Get("Connection") == "close")

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, "smuggling-handled", string(body))

			// Socket MUST be closed by server immediately following the response
			_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			_, err = br.ReadByte()
			assert.True(t, errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed),
				fmt.Sprintf("expected EOF/ErrClosed indicating socket teardown, got %v", err))

			// Oracle verification: pipelined /pipelined-evil MUST NEVER have reached the handler
			invokedMu.Lock()
			defer invokedMu.Unlock()

			for _, p := range pathsSeen {
				assert.NotEqual(t, "/pipelined-evil", p, "smuggled pipelined request was processed by handler!")
			}

			assert.Equal(t, 1, len(pathsSeen), "handler should only have been invoked exactly once")
		})
	}
}

// TestAdversarial_H1_Smuggling_PerPStorage_NoCrossContamination verifies that
// when a request triggers CloseConnection, the Per-P recycled request struct has its
// CloseConnection state completely cleared, ensuring subsequent connections are not falsely closed.
func TestAdversarial_H1_Smuggling_PerPStorage_NoCrossContamination(t *testing.T) {
	ch := &h1.ConnHandler{
		Handler: func(req *h1.Request, res *h1.Response) error {
			res.StatusCode = status.OK
			res.Body = []byte("ok")

			return nil
		},
	}

	var lc net.ListenConfig

	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	defer ln.Close()

	var activeConns atomic.Int32

	go func() {
		for {
			c, aErr := ln.Accept()
			if aErr != nil {
				return
			}

			activeConns.Add(1)

			go func(conn net.Conn) {
				defer activeConns.Add(-1)

				_ = ch.ServeConn(conn)
			}(c)
		}
	}()

	addr := ln.Addr().String()

	var d net.Dialer

	// 1. Send 10 smuggling requests across 10 connections to dirty Per-P pools
	for i := 0; i < 10; i++ {
		c, dErr := d.DialContext(context.Background(), "tcp", addr)
		require.NoError(t, dErr)

		raw := "POST /dirty HTTP/1.1\r\n" +
			"Host: 127.0.0.1\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Content-Length: 4\r\n\r\n" +
			"4\r\ntest\r\n0\r\n\r\n"
		_, _ = c.Write([]byte(raw))

		br := bufio.NewReader(c)

		resp, rErr := http.ReadResponse(br, nil)
		if rErr == nil {
			_ = resp.Body.Close()
		}

		_ = c.Close()
	}

	// Wait for connections to close and recycle into Per-P
	time.Sleep(50 * time.Millisecond)

	// 2. Open a persistent connection and send 5 sequential keep-alive requests
	c, err := d.DialContext(context.Background(), "tcp", addr)
	require.NoError(t, err)

	defer c.Close()

	br := bufio.NewReader(c)

	for seq := 0; seq < 5; seq++ {
		reqWire := fmt.Sprintf("GET /seq-%d HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: keep-alive\r\n\r\n", seq)
		_, err = c.Write([]byte(reqWire))
		require.NoError(t, err)

		resp, err := http.ReadResponse(br, nil)
		require.NoErrorf(t, err, "sequential keep-alive request %d failed: connection was prematurely closed!", seq)

		_ = resp.Body.Close()

		assert.False(t, resp.Close, "keep-alive connection falsely marked Close!")
		assert.NotEqual(t, "close", resp.Header.Get("Connection"), "unexpected Connection: close header")
	}
}
