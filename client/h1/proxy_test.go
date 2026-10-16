// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/client/h1"
	"github.com/lemon4ksan/mach/net/proxy/socks/sockstest"
	"github.com/lemon4ksan/mach/proto/http"
)

func startMockHTTPBackend(t *testing.T) (net.Listener, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				reqLine, err := br.ReadString('\n')
				if err != nil {
					return
				}
				for {
					line, err := br.ReadString('\n')
					if err != nil || line == "\r\n" {
						break
					}
				}

				if strings.Contains(reqLine, "/hello") {
					resp := "HTTP/1.1 200 OK\r\nContent-Length: 13\r\n\r\nHello Proxied"
					_, _ = c.Write([]byte(resp))
				} else {
					resp := "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n"
					_, _ = c.Write([]byte(resp))
				}
			}(conn)
		}
	}()

	return ln, func() { _ = ln.Close() }
}

func startMockHTTPTunnel(t *testing.T) (net.Listener, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)

				reqLine, err := br.ReadString('\n')
				if err != nil {
					return
				}

				// Consume headers until CRLF
				for {
					line, err := br.ReadString('\n')
					if err != nil || line == "\r\n" {
						break
					}
				}

				if !strings.HasPrefix(reqLine, "CONNECT ") {
					_, _ = c.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
					return
				}

				parts := strings.Split(reqLine, " ")
				target := parts[1]

				// Connect to target
				targetConn, err := net.Dial("tcp", target)
				if err != nil {
					_, _ = c.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
					return
				}
				defer targetConn.Close()

				_, _ = c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))

				// Relay
				errCh := make(chan error, 2)
				go func() {
					_, err := io.Copy(targetConn, c)
					errCh <- err
				}()
				go func() {
					_, err := io.Copy(c, targetConn)
					errCh <- err
				}()

				<-errCh
			}(conn)
		}
	}()

	return ln, func() { _ = ln.Close() }
}

func TestH1_Engine_WithHTTPProxy(t *testing.T) {
	t.Parallel()

	backendLn, backendCleanup := startMockHTTPBackend(t)
	defer backendCleanup()

	proxyLn, proxyCleanup := startMockHTTPTunnel(t)
	defer proxyCleanup()

	proxyURL := "http://" + proxyLn.Addr().String()

	// Test with h1.WithProxy
	engine := h1.NewEngine(h1.WithProxy(proxyURL))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	machConn, err := engine.Dial(ctx, backendLn.Addr().String())
	require.NoError(t, err)
	defer machConn.Close()

	clientConn, ok := machConn.(*h1.ClientConn)
	require.True(t, ok)

	req := http.AcquireRequest()
	defer http.ReleaseRequest(req)
	req.Header.SetMethod("GET")
	req.SetRequestURI("/hello")
	req.Header.SetHost(backendLn.Addr().String())

	res := http.AcquireResponse()
	defer http.ReleaseResponse(res)

	err = clientConn.Do(ctx, req, res)
	require.NoError(t, err)

	assert.Equal(t, 200, res.StatusCode())
	assert.Equal(t, "Hello Proxied", string(res.Body()))
}

func TestH1_Engine_WithSOCKS5Proxy(t *testing.T) {
	t.Parallel()

	ss, err := sockstest.NewServer(sockstest.NoAuthRequired, sockstest.NoProxyRequired)
	require.NoError(t, err)
	defer ss.Close()

	socksURL := "socks5://" + ss.Addr().String()

	// Test with h1.WithProxy
	engine := h1.NewEngine(h1.WithProxy(socksURL))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// SOCKS server in sockstest responds to ss.TargetAddr()
	machConn, err := engine.Dial(ctx, ss.TargetAddr().String())
	require.NoError(t, err)
	defer machConn.Close()

	require.NotNil(t, machConn)
}
