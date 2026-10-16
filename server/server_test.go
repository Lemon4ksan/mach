// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server_test

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
	"golang.org/x/net/http2"

	h3client "github.com/lemon4ksan/mach/client/h3"
	"github.com/lemon4ksan/mach/net/quic"
	machhttp "github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/proto/semantic/status"
	"github.com/lemon4ksan/mach/server"
)

func generateTestTLS(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"mach-server-test"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"h2", "http/1.1"},
	}

	clientTLS := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
		NextProtos:         []string{"h2", "http/1.1"},
	}

	return serverTLS, clientTLS
}

func TestServer_PlaintextH1(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	srv := server.NewWithHandlerFunc(func(ctx *server.Context) error {
		assert.Equal(t, server.ProtoHTTP11, ctx.Proto())
		assert.True(t, ctx.IsH1())
		assert.False(t, ctx.IsH2())
		assert.False(t, ctx.IsH3())
		assert.Equal(t, "GET", ctx.Method())
		assert.Equal(t, "/test/path", ctx.Path())
		assert.Equal(t, "query=1", ctx.Query())

		ctx.SetStatusCode(status.OK)
		ctx.SetHeader("X-Custom", "mach-v1")
		ctx.SetBodyString("Hello HTTP/1.1 Unified")
		return nil
	})

	go func() {
		_ = srv.Serve(ln)
	}()
	defer srv.Close()

	// Send standard HTTP/1.1 request
	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	req := "GET /test/path?query=1 HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
	_, err = conn.Write([]byte(req))
	require.NoError(t, err)

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, status.OK, resp.StatusCode)
	assert.Equal(t, "mach-v1", resp.Header.Get("X-Custom"))
	assert.Equal(t, "Hello HTTP/1.1 Unified", string(body))
}

func TestServer_PlaintextH2C(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	srv := server.NewWithHandlerFunc(func(ctx *server.Context) error {
		assert.Equal(t, server.ProtoHTTP2, ctx.Proto())
		assert.True(t, ctx.IsH2())
		assert.Equal(t, "POST", ctx.Method())
		assert.Equal(t, "/h2c-endpoint", ctx.Path())
		assert.Equal(t, "client-payload", string(ctx.Body()))

		ctx.SetStatusCode(status.Created)
		ctx.SetHeader("X-H2C", "active")
		ctx.SetBodyString("H2C Prior Knowledge Handled")
		return nil
	})

	go func() {
		_ = srv.Serve(ln)
	}()
	defer srv.Close()

	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		},
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"http://"+ln.Addr().String()+"/h2c-endpoint",
		strings.NewReader("client-payload"),
	)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, status.Created, resp.StatusCode)
	assert.Equal(t, "HTTP/2.0", resp.Proto)
	assert.Equal(t, "active", resp.Header.Get("X-H2C"))
	assert.Equal(t, "H2C Prior Knowledge Handled", string(body))
}

func TestServer_TLS_ALPN_TransparentNegotiation(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLS(t)

	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer tcpLn.Close()

	tlsLn := tls.NewListener(tcpLn, serverTLS)

	var requestProtocols sync.Map

	srv := server.New(
		server.WithHandlerFunc(func(ctx *server.Context) error {
			requestProtocols.Store(ctx.Proto(), true)

			ctx.SetStatusCode(status.OK)
			ctx.SetHeader("X-Served-Proto", ctx.Proto())
			ctx.SetBodyString("Unified: " + ctx.Proto())
			return nil
		}),
		server.WithAltSvcPort(8443),
		server.WithAltSvcMaxAge(3600*time.Second),
	)

	go func() {
		_ = srv.Serve(tlsLn)
	}()
	defer srv.Close()

	addr := tcpLn.Addr().String()

	// 1. Client explicitly negotiating HTTP/2 via ALPN
	h2ClientTLS := clientTLS.Clone()
	h2ClientTLS.NextProtos = []string{"h2"}

	h2Client := &http.Client{
		Transport: &http2.Transport{
			TLSClientConfig: h2ClientTLS,
		},
		Timeout: 5 * time.Second,
	}

	reqH2, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+addr+"/h2", nil)
	require.NoError(t, err)

	respH2, err := h2Client.Do(reqH2)
	require.NoError(t, err)
	bodyH2, _ := io.ReadAll(respH2.Body)
	_ = respH2.Body.Close()

	assert.Equal(t, status.OK, respH2.StatusCode)
	assert.Equal(t, "HTTP/2.0", respH2.Proto)
	assert.Equal(t, "HTTP/2", respH2.Header.Get("X-Served-Proto"))
	assert.Equal(t, "Unified: HTTP/2", string(bodyH2))

	// Verify Alt-Svc emission on H2 response
	altSvcH2 := respH2.Header.Get("Alt-Svc")
	assert.True(t, strings.Contains(altSvcH2, `h3=":8443"`))
	assert.True(t, strings.Contains(altSvcH2, `ma=3600`))

	// 2. Client explicitly negotiating HTTP/1.1 via ALPN on the EXACT SAME listener!
	h1ClientTLS := clientTLS.Clone()
	h1ClientTLS.NextProtos = []string{"http/1.1"}

	h1Client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: h1ClientTLS,
		},
		Timeout: 5 * time.Second,
	}

	reqH1, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+addr+"/h1", nil)
	require.NoError(t, err)

	respH1, err := h1Client.Do(reqH1)
	require.NoError(t, err)
	bodyH1, _ := io.ReadAll(respH1.Body)
	_ = respH1.Body.Close()

	assert.Equal(t, status.OK, respH1.StatusCode)
	assert.Equal(t, "HTTP/1.1", respH1.Proto)
	assert.Equal(t, "HTTP/1.1", respH1.Header.Get("X-Served-Proto"))
	assert.Equal(t, "Unified: HTTP/1.1", string(bodyH1))

	// Verify Alt-Svc emission on H1 response
	altSvcH1 := respH1.Header.Get("Alt-Svc")
	assert.True(t, strings.Contains(altSvcH1, `h3=":8443"`))
	assert.True(t, strings.Contains(altSvcH1, `ma=3600`))

	// Ensure both protocols were served by the single unified handler
	_, hasH1 := requestProtocols.Load(server.ProtoHTTP11)
	_, hasH2 := requestProtocols.Load(server.ProtoHTTP2)
	assert.True(t, hasH1)
	assert.True(t, hasH2)
}

func TestServer_AltSvc_EmissionAndSuppression(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	srv := server.New(
		server.WithHandlerFunc(func(ctx *server.Context) error {
			switch ctx.Path() {
			case "/suppressed":
				ctx.DisableAltSvc()
				ctx.SetBodyString("suppressed")
			case "/custom":
				ctx.SetHeader("Alt-Svc", `h3=":9999"; ma=7200`)
				ctx.SetBodyString("custom")
			default:
				ctx.SetBodyString("default")
			}
			return nil
		}),
		server.WithAltSvcPort(8443),
	)

	go func() {
		_ = srv.Serve(ln)
	}()
	defer srv.Close()

	addr := ln.Addr().String()

	// 1. Default request should have Alt-Svc
	c1, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer c1.Close()
	_, _ = c1.Write([]byte("GET /default HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"))
	r1, err := http.ReadResponse(bufio.NewReader(c1), nil)
	require.NoError(t, err)
	_ = r1.Body.Close()
	assert.Equal(t, `h3=":8443"`, r1.Header.Get("Alt-Svc"))

	// 2. Suppressed request should NOT have Alt-Svc
	c2, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer c2.Close()
	_, _ = c2.Write([]byte("GET /suppressed HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"))
	r2, err := http.ReadResponse(bufio.NewReader(c2), nil)
	require.NoError(t, err)
	_ = r2.Body.Close()
	assert.Equal(t, "", r2.Header.Get("Alt-Svc"))

	// 3. Custom Alt-Svc set by handler should NOT be overridden
	c3, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer c3.Close()
	_, _ = c3.Write([]byte("GET /custom HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"))
	r3, err := http.ReadResponse(bufio.NewReader(c3), nil)
	require.NoError(t, err)
	b3, _ := io.ReadAll(r3.Body)
	_ = r3.Body.Close()
	t.Logf("r3.Body: %q, Alt-Svc: %q", string(b3), r3.Header.Get("Alt-Svc"))
	assert.Equal(t, `h3=":9999"; ma=7200`, r3.Header.Get("Alt-Svc"))
}

func TestServer_MultiProtocol_H1_H2_H3(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLS(t)

	// Setup TCP TLS listener
	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer tcpLn.Close()

	tlsTCP := tls.NewListener(tcpLn, serverTLS)

	// Setup QUIC listener with h3 in ALPN
	quicServerTLS := serverTLS.Clone()
	quicServerTLS.NextProtos = []string{"h3"}

	quicLn, err := quic.ListenAddr("127.0.0.1:0", quicServerTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicLn.Close()

	udpAddr := quicLn.Addr().(*net.UDPAddr)

	var (
		mu     sync.Mutex
		seenH1 bool
		seenH2 bool
		seenH3 bool
	)

	srv := server.New(
		server.WithHandlerFunc(func(ctx *server.Context) error {
			mu.Lock()
			switch ctx.Proto() {
			case server.ProtoHTTP11:
				seenH1 = true
			case server.ProtoHTTP2:
				seenH2 = true
			case server.ProtoHTTP3:
				seenH3 = true
			}
			mu.Unlock()

			ctx.SetStatusCode(status.OK)
			ctx.SetHeader("X-Protocol-Witness", ctx.Proto())
			ctx.SetBodyString(fmt.Sprintf("Served via %s", ctx.Proto()))
			return nil
		}),
		server.WithAltSvcPort(udpAddr.Port),
	)

	go func() {
		_ = srv.ServeMultiProtocol(tlsTCP, quicLn)
	}()
	defer srv.Close()

	// 1. Verify H1 over TLS
	h1TLS := clientTLS.Clone()
	h1TLS.NextProtos = []string{"http/1.1"}
	h1Client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: h1TLS},
		Timeout:   3 * time.Second,
	}
	r1, err := h1Client.Get("https://" + tcpLn.Addr().String() + "/h1-probe")
	require.NoError(t, err)
	b1, _ := io.ReadAll(r1.Body)
	_ = r1.Body.Close()
	assert.Equal(t, "HTTP/1.1", r1.Header.Get("X-Protocol-Witness"))
	assert.Equal(t, fmt.Sprintf(`h3=":%d"`, udpAddr.Port), r1.Header.Get("Alt-Svc"))
	assert.Equal(t, "Served via HTTP/1.1", string(b1))

	// 2. Verify H2 over TLS
	h2TLS := clientTLS.Clone()
	h2TLS.NextProtos = []string{"h2"}
	h2Client := &http.Client{
		Transport: &http2.Transport{TLSClientConfig: h2TLS},
		Timeout:   3 * time.Second,
	}
	r2, err := h2Client.Get("https://" + tcpLn.Addr().String() + "/h2-probe")
	require.NoError(t, err)
	b2, _ := io.ReadAll(r2.Body)
	_ = r2.Body.Close()
	assert.Equal(t, "HTTP/2", r2.Header.Get("X-Protocol-Witness"))
	assert.Equal(t, fmt.Sprintf(`h3=":%d"`, udpAddr.Port), r2.Header.Get("Alt-Svc"))
	assert.Equal(t, "Served via HTTP/2", string(b2))

	// 3. Verify H3 over QUIC
	h3ClientTLS := clientTLS.Clone()
	h3ClientTLS.NextProtos = []string{"h3"}

	ctxH3, cancelH3 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelH3()

	quicConn, err := quic.DialAddr(ctxH3, quicLn.Addr().String(), h3ClientTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicConn.CloseWithError(0, "")

	cc, err := h3client.NewClientConn(quicConn, nil)
	require.NoError(t, err)
	defer cc.Close()

	reqH3 := machhttp.AcquireRequest()
	respH3 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(reqH3)
	defer machhttp.ReleaseResponse(respH3)

	reqH3.Header.SetMethod("GET")
	reqH3.SetRequestURI("/h3-probe")
	reqH3.Header.SetHost("localhost")

	_, err = cc.Do(ctxH3, reqH3, respH3, nil)
	require.NoError(t, err)

	assert.Equal(t, status.OK, respH3.StatusCode())
	assert.Equal(t, "HTTP/3", string(respH3.Header.Peek("X-Protocol-Witness")))
	assert.Equal(t, "Served via HTTP/3", string(respH3.Body()))

	// Assert that all 3 protocols were received and processed by the single universal handler!
	mu.Lock()
	defer mu.Unlock()
	assert.True(t, seenH1, "HTTP/1.1 handled")
	assert.True(t, seenH2, "HTTP/2 handled")
	assert.True(t, seenH3, "HTTP/3 handled")
}

func TestServer_ShutdownAndClose(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := server.NewWithHandlerFunc(func(ctx *server.Context) error {
		time.Sleep(100 * time.Millisecond)
		ctx.SetStatusCode(status.OK)
		return nil
	})

	go func() {
		_ = srv.Serve(ln)
	}()

	// Start a concurrent request
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"))
		_, _ = http.ReadResponse(bufio.NewReader(conn), nil)
	}()

	time.Sleep(20 * time.Millisecond)

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = srv.Shutdown(ctx)
	require.NoError(t, err)

	wg.Wait()
}

func TestServer_EXP1201_ShortCleartextRequest_NoDeadlock(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	srv := server.NewWithHandlerFunc(func(ctx *server.Context) error {
		assert.Equal(t, server.ProtoHTTP11, ctx.Proto())
		assert.Equal(t, "/short", ctx.Path())
		ctx.SetStatusCode(status.OK)
		ctx.SetBodyString("exp1201-resolved")
		return nil
	})

	go func() {
		_ = srv.Serve(ln)
	}()
	defer srv.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	// 18 bytes: shorter than 24 bytes, without closing connection immediately
	shortReq := []byte("GET /short HTTP/1.0\r\n\r\n")
	assert.True(t, len(shortReq) < 24)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, wErr := conn.Write(shortReq)
		assert.NoError(t, wErr)

		resp, rErr := http.ReadResponse(bufio.NewReader(conn), nil)
		assert.NoError(t, rErr)
		assert.Equal(t, status.OK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		assert.Equal(t, "exp1201-resolved", string(body))
	}()

	select {
	case <-done:
		// Succeeded immediately without deadlock
	case <-time.After(2 * time.Second):
		t.Fatal("DEADLOCK DETECTED: serveCleartextConn blocked waiting for 24 bytes on an 18-byte request (EXP-1201)")
	}
}

func TestServer_EXP1210_SingleBytePrefixDivergence(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	srv := server.NewWithHandlerFunc(func(ctx *server.Context) error {
		assert.Equal(t, server.ProtoHTTP11, ctx.Proto())
		assert.Equal(t, "/", ctx.Path())
		assert.Equal(t, "POST", ctx.Method())
		ctx.SetStatusCode(status.OK)
		ctx.SetBodyString("exp1210-resolved")
		return nil
	})

	go func() {
		_ = srv.Serve(ln)
	}()
	defer srv.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Write 1st byte: 'P' (matches h2Preface[:1])
		_, wErr := conn.Write([]byte("P"))
		assert.NoError(t, wErr)

		// Small delay to ensure server reads byte 0 in serveCleartextConn
		time.Sleep(10 * time.Millisecond)

		// Write remaining 18 bytes: "OST / HTTP/1.0\r\n\r\n"
		// Total request = 19 bytes (< 24 bytes). Client stops sending and waits for response.
		_, wErr = conn.Write([]byte("OST / HTTP/1.0\r\n\r\n"))
		assert.NoError(t, wErr)

		resp, rErr := http.ReadResponse(bufio.NewReader(conn), nil)
		assert.NoError(t, rErr)
		assert.Equal(t, status.OK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		assert.Equal(t, "exp1210-resolved", string(body))
	}()

	select {
	case <-done:
		// Succeeded immediately without deadlock
	case <-time.After(2 * time.Second):
		t.Fatal(
			"DEADLOCK DETECTED: serveCleartextConn blocked waiting for 24 bytes after single-byte prefix divergence (EXP-1210)",
		)
	}
}
