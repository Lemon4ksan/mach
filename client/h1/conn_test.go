// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/client/h1"
	"github.com/lemon4ksan/mach/proto/http"
)

func runMockH1Server(t *testing.T, handler func(net.Conn)) (net.Listener, string) {
	var lc net.ListenConfig

	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}

			go handler(c)
		}
	}()

	return ln, ln.Addr().String()
}

func TestClientConn_BasicRoundTrip(t *testing.T) {
	t.Parallel()

	ln, addr := runMockH1Server(t, func(c net.Conn) {
		defer c.Close()

		br := bufio.NewReader(c)
		bw := bufio.NewWriter(c)

		req := http.AcquireRequest()

		defer http.ReleaseRequest(req)

		if err := req.Read(br); err != nil {
			return
		}

		resp := http.AcquireResponse()

		defer http.ReleaseResponse(resp)

		resp.SetStatusCode(200)
		resp.SetBodyString("hello from h1 server")

		_ = resp.Write(bw)
		_ = bw.Flush()
	})

	defer ln.Close()

	var dialer net.Dialer

	c, err := dialer.DialContext(context.Background(), "tcp", addr)
	require.NoError(t, err)

	cc := h1.NewClientConn(c)

	defer cc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

	defer cancel()

	req := http.AcquireRequest()
	resp := http.AcquireResponse()

	defer http.ReleaseRequest(req)
	defer http.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/test")
	req.Header.SetHost(addr)

	err = cc.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "hello from h1 server", string(resp.Body()))
}

func TestClientConn_ContextCancellation(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})

	ln, addr := runMockH1Server(t, func(c net.Conn) {
		defer c.Close()

		close(started)
		// Stall indefinitely without sending response
		time.Sleep(5 * time.Second)
	})

	defer ln.Close()

	var dialer net.Dialer

	c, err := dialer.DialContext(context.Background(), "tcp", addr)
	require.NoError(t, err)

	cc := h1.NewClientConn(c)

	defer cc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)

	defer cancel()

	req := http.AcquireRequest()
	resp := http.AcquireResponse()

	defer http.ReleaseRequest(req)
	defer http.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/hang")
	req.Header.SetHost(addr)

	err = cc.Do(ctx, req, resp)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestClientConn_Close(t *testing.T) {
	t.Parallel()

	ln, addr := runMockH1Server(t, func(c net.Conn) {
		defer c.Close()
	})

	defer ln.Close()

	var dialer net.Dialer

	c, err := dialer.DialContext(context.Background(), "tcp", addr)
	require.NoError(t, err)

	cc := h1.NewClientConn(c)
	err = cc.Close()
	require.NoError(t, err)

	// Subsequent Do on closed connection should error
	ctx := context.Background()
	req := http.AcquireRequest()
	resp := http.AcquireResponse()

	defer http.ReleaseRequest(req)
	defer http.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/test")
	req.Header.SetHost(addr)

	err = cc.Do(ctx, req, resp)
	assert.Error(t, err)
}

func BenchmarkClientConn_RoundTrip(b *testing.B) {
	var lc net.ListenConfig

	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}

	defer ln.Close()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}

			go func(conn net.Conn) {
				defer conn.Close()

				br := bufio.NewReader(conn)
				bw := bufio.NewWriter(conn)

				req := http.AcquireRequest()
				resp := http.AcquireResponse()

				defer http.ReleaseRequest(req)
				defer http.ReleaseResponse(resp)

				resp.SetStatusCode(200)
				resp.SetBodyString("bench-pong")

				for {
					if err := req.Read(br); err != nil {
						return
					}

					if err := resp.Write(bw); err != nil {
						return
					}

					if err := bw.Flush(); err != nil {
						return
					}
				}
			}(c)
		}
	}()

	var dialer net.Dialer

	c, err := dialer.DialContext(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		b.Fatal(err)
	}

	cc := h1.NewClientConn(c)

	defer cc.Close()

	ctx := context.Background()
	req := http.AcquireRequest()
	resp := http.AcquireResponse()

	defer http.ReleaseRequest(req)
	defer http.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/bench")
	req.Header.SetHost(ln.Addr().String())

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := cc.Do(ctx, req, resp); err != nil {
			b.Fatal(fmt.Errorf("Do failed at iteration %d: %w", i, err))
		}
	}
}
