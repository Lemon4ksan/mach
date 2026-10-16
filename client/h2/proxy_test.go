// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2_test

import (
	"crypto/tls"
	"net"
	"net/url"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/client/h2"
	"github.com/lemon4ksan/mach/net/proxy"
)

func TestH2_Dialer_WithProxy_Options(t *testing.T) {
	t.Parallel()

	// 1. With string socks5 URL using h2.WithProxy
	d1 := h2.NewDialer("example.com:443", h2.WithProxy("socks5://127.0.0.1:1080"))
	require.NotNil(t, d1.ProxyDialer)

	// 2. With string http URL using h2.WithProxy
	d2 := h2.NewDialer("example.com:443", h2.WithProxy("http://127.0.0.1:8080"))
	require.NotNil(t, d2.ProxyDialer)
	_, ok := d2.ProxyDialer.(*proxy.HTTPConnectDialer)
	assert.True(t, ok)

	// 3. With explicit proxy.Dialer
	pDialer, err := proxy.SOCKS5("tcp", "127.0.0.1:1080", nil, nil)
	require.NoError(t, err)
	d3 := h2.NewDialer("example.com:443", h2.WithProxy(pDialer))
	assert.Equal(t, pDialer, d3.ProxyDialer)

	// 4. With *url.URL
	u, err := url.Parse("http://proxy.corp:3128")
	require.NoError(t, err)
	d4 := h2.NewDialer("example.com:443", h2.WithProxy(u))
	require.NotNil(t, d4.ProxyDialer)
}

func TestH2_Dialer_WithProxy_MockTunnel(t *testing.T) {
	t.Parallel()

	// Start a mock tunnel that verifies the CONNECT request is made
	tunnelLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer tunnelLn.Close()

	connectReceived := make(chan string, 1)

	go func() {
		c, err := tunnelLn.Accept()
		if err != nil {
			return
		}
		defer c.Close()

		buf := make([]byte, 1024)
		n, err := c.Read(buf)
		if err != nil {
			return
		}
		connectReceived <- string(buf[:n])

		// Send 200 OK
		_, _ = c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	}()

	proxyURL := "http://" + tunnelLn.Addr().String()
	targetAddr := "securedomain.internal:443"

	dialer := h2.NewDialer(targetAddr, h2.WithProxy(proxyURL))
	dialer.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	// We test tryDial: it will connect via proxy tunnel, send CONNECT, receive 200 OK,
	// and then attempt TLS handshake (which will fail because mock doesn't do TLS,
	// but the tunnel handshake must succeed!).
	ctx := t.Context()
	_, dialErr := dialer.DialContext(ctx, h2.ConnOpts{})

	// Verify CONNECT request was received by the mock proxy
	select {
	case req := <-connectReceived:
		assert.Contains(t, req, "CONNECT securedomain.internal:443 HTTP/1.1")
		assert.Contains(t, req, "Host: securedomain.internal:443")
	default:
		t.Fatal("expected mock proxy to receive CONNECT tunnel request")
	}

	// TLS failed as expected since mock closed after 200 OK
	assert.Error(t, dialErr)
}

func TestH2_EXP1211_SharedTLSConfigNotMutated(t *testing.T) {
	t.Parallel()

	sharedTLS := &tls.Config{
		InsecureSkipVerify: true,
	}

	dialer := h2.NewDialer("example.com:443", h2.WithTLSConfig(sharedTLS))

	// Perform multiple dials using failing NetDial to trigger tryDial setup
	dialer.NetDial = func(addr string) (net.Conn, error) {
		return nil, net.ErrClosed
	}

	for i := 0; i < 5; i++ {
		_, _ = dialer.Dial(h2.ConnOpts{})
	}

	// Verify sharedTLS was never mutated in place
	assert.Equal(t, 0, len(sharedTLS.NextProtos), "shared TLSConfig.NextProtos must not be mutated in place")
	assert.Equal(t, "", sharedTLS.ServerName, "shared TLSConfig.ServerName must not be mutated in place")
}
