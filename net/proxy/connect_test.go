// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/net/proxy"
)

func startMockHTTPProxy(t *testing.T, expectedAuth string, statusCode int) (net.Listener, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan struct{})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)

				// Read request line
				reqLine, err := br.ReadString('\n')
				if err != nil {
					return
				}

				if !strings.HasPrefix(reqLine, "CONNECT ") {
					_, _ = c.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
					return
				}

				authHeader := ""
				for {
					line, err := br.ReadString('\n')
					if err != nil || line == "\r\n" {
						break
					}
					if strings.HasPrefix(strings.ToLower(line), "proxy-authorization:") {
						authHeader = strings.TrimSpace(line)
					}
				}

				if expectedAuth != "" && !strings.EqualFold(authHeader, expectedAuth) {
					_, _ = c.Write(
						[]byte("HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic\r\n\r\n"),
					)
					return
				}

				if statusCode != 200 {
					_, _ = c.Write([]byte(fmt.Sprintf("HTTP/1.1 %d Tunnel Failed\r\n\r\n", statusCode)))
					return
				}

				// 200 OK
				_, _ = c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))

				// Echo loop
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	return ln, func() {
		_ = ln.Close()
		close(done)
	}
}

func TestHTTPConnect_Success(t *testing.T) {
	t.Parallel()

	ln, cleanup := startMockHTTPProxy(t, "", 200)
	defer cleanup()

	dialer, err := proxy.HTTPConnect("tcp", ln.Addr().String(), nil, nil)
	require.NoError(t, err)

	conn, err := dialer.Dial("tcp", "target.example.com:443")
	require.NoError(t, err)
	defer conn.Close()

	// Verify bidirectional tunnel
	msg := []byte("hello through tunnel")
	_, err = conn.Write(msg)
	require.NoError(t, err)

	reply := make([]byte, len(msg))
	_, err = io.ReadFull(conn, reply)
	require.NoError(t, err)
	assert.Equal(t, msg, reply)
}

func TestHTTPConnect_BasicAuth(t *testing.T) {
	t.Parallel()

	// Expected: "Proxy-Authorization: Basic dXNlcjpwYXNz" (user:pass base64)
	ln, cleanup := startMockHTTPProxy(t, "proxy-authorization: Basic dXNlcjpwYXNz", 200)
	defer cleanup()

	auth := &proxy.Auth{User: "user", Password: "pass"}
	dialer, err := proxy.HTTPConnect("tcp", ln.Addr().String(), auth, nil)
	require.NoError(t, err)

	conn, err := dialer.Dial("tcp", "secure.example.com:8443")
	require.NoError(t, err)
	defer conn.Close()

	msg := []byte("authenticated tunnel data")
	_, err = conn.Write(msg)
	require.NoError(t, err)

	reply := make([]byte, len(msg))
	_, err = io.ReadFull(conn, reply)
	require.NoError(t, err)
	assert.Equal(t, msg, reply)
}

func TestHTTPConnect_AuthFailure(t *testing.T) {
	t.Parallel()

	ln, cleanup := startMockHTTPProxy(t, "proxy-authorization: Basic dXNlcjpwYXNz", 200)
	defer cleanup()

	badAuth := &proxy.Auth{User: "wrong", Password: "password"}
	dialer, err := proxy.HTTPConnect("tcp", ln.Addr().String(), badAuth, nil)
	require.NoError(t, err)

	conn, err := dialer.Dial("tcp", "secure.example.com:8443")
	assert.Error(t, err)
	if conn != nil {
		conn.Close()
	}
	assert.Contains(t, err.Error(), "407")
}

func TestHTTPConnect_ServerFailure(t *testing.T) {
	t.Parallel()

	ln, cleanup := startMockHTTPProxy(t, "", 502)
	defer cleanup()

	dialer, err := proxy.HTTPConnect("tcp", ln.Addr().String(), nil, nil)
	require.NoError(t, err)

	conn, err := dialer.Dial("tcp", "bad.example.com:80")
	assert.Error(t, err)
	if conn != nil {
		conn.Close()
	}
	assert.Contains(t, err.Error(), "502")
}

func TestHTTPConnect_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Blackhole listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	dialer, err := proxy.HTTPConnect("tcp", ln.Addr().String(), nil, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cd, ok := dialer.(proxy.ContextDialer)
	require.True(t, ok)

	conn, err := cd.DialContext(ctx, "tcp", "target.example.com:443")
	if conn != nil {
		conn.Close()
	}
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "canceled"))
}

func TestFromURL_HTTPAndHTTPS(t *testing.T) {
	t.Parallel()

	// 1. Plain HTTP proxy URL
	u, err := url.Parse("http://127.0.0.1:8080")
	require.NoError(t, err)
	d, err := proxy.FromURL(u, nil)
	require.NoError(t, err)
	require.NotNil(t, d)

	hd, ok := d.(*proxy.HTTPConnectDialer)
	require.True(t, ok)
	assert.Equal(t, "127.0.0.1:8080", hd.ProxyAddress)
	assert.Nil(t, hd.TLSConfig)

	// 2. HTTPS proxy URL
	uHTTPS, err := url.Parse("https://proxy.example.com:8443")
	require.NoError(t, err)
	dHTTPS, err := proxy.FromURL(uHTTPS, nil)
	require.NoError(t, err)
	require.NotNil(t, dHTTPS)

	hdHTTPS, ok := dHTTPS.(*proxy.HTTPConnectDialer)
	require.True(t, ok)
	assert.Equal(t, "proxy.example.com:8443", hdHTTPS.ProxyAddress)
	require.NotNil(t, hdHTTPS.TLSConfig)
	assert.Equal(t, "proxy.example.com", hdHTTPS.TLSConfig.ServerName)
}

func TestHTTPConnect_EXP1203_TrailingBytesPreserved(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	trailingData := []byte("PRESERVED_PIPELINED_DATA_12345")

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		br := bufio.NewReader(conn)
		for {
			line, err := br.ReadString('\n')
			if err != nil || line == "\r\n" {
				break
			}
		}

		resp := append([]byte("HTTP/1.1 200 Connection established\r\n\r\n"), trailingData...)
		_, _ = conn.Write(resp)
	}()

	dialer, err := proxy.HTTPConnect("tcp", ln.Addr().String(), nil, nil)
	require.NoError(t, err)

	conn, err := dialer.Dial("tcp", "backend.local:80")
	require.NoError(t, err)
	defer conn.Close()

	buf := make([]byte, len(trailingData))
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	assert.Equal(t, trailingData, buf)
}

func TestHTTPConnect_EXP1204_MultipleSpacesInStatusLine(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		br := bufio.NewReader(conn)
		for {
			line, err := br.ReadString('\n')
			if err != nil || line == "\r\n" {
				break
			}
		}

		// Two spaces between HTTP/1.1 and 200
		_, _ = conn.Write([]byte("HTTP/1.1  200  Connection Established\r\n\r\n"))
	}()

	dialer, err := proxy.HTTPConnect("tcp", ln.Addr().String(), nil, nil)
	require.NoError(t, err)

	conn, err := dialer.Dial("tcp", "backend.local:80")
	require.NoError(t, err)
	conn.Close()
}
