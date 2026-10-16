// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package e2e_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
	"golang.org/x/net/http2"

	h3client "github.com/lemon4ksan/mach/client/h3"
	"github.com/lemon4ksan/mach/net/quic"
	machhttp "github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/proto/semantic/altsvc"
	"github.com/lemon4ksan/mach/proto/semantic/status"
	"github.com/lemon4ksan/mach/server"
)

func TestUnifiedServer_E2E_MultiProtocolMatrix(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)

	// 1. Setup TCP TLS listener (ALPN h2, http/1.1)
	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer tcpLn.Close()

	tlsTCP := tls.NewListener(tcpLn, serverTLS)

	// 2. Setup QUIC UDP listener (ALPN h3)
	quicServerTLS := serverTLS.Clone()
	quicServerTLS.NextProtos = []string{"h3"}

	quicLn, err := quic.ListenAddr("127.0.0.1:0", quicServerTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicLn.Close()

	udpAddr := quicLn.Addr().(*net.UDPAddr)

	// 3. Universal Handler
	srv := server.New(
		server.WithHandlerFunc(func(ctx *server.Context) error {
			ctx.SetStatusCode(status.OK)
			ctx.SetHeader("X-Unified-Protocol", ctx.Proto())
			ctx.SetHeader("X-Echo-Method", ctx.Method())
			ctx.SetHeader("X-Echo-Path", ctx.Path())
			ctx.SetBodyString(fmt.Sprintf("Unified response via %s for %s", ctx.Proto(), ctx.Path()))
			return nil
		}),
		server.WithAltSvcPort(udpAddr.Port),
		server.WithAltSvcMaxAge(3600*time.Second),
	)

	go func() {
		_ = srv.ServeMultiProtocol(tlsTCP, quicLn)
	}()
	defer srv.Close()

	tcpAddr := tcpLn.Addr().String()

	// --- Phase 1: HTTP/1.1 client over TLS ---
	h1TLS := clientTLS.Clone()
	h1TLS.NextProtos = []string{"http/1.1"}
	h1Client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: h1TLS},
		Timeout:   5 * time.Second,
	}

	reqH1, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+tcpAddr+"/probe-h1", nil)
	require.NoError(t, err)
	respH1, err := h1Client.Do(reqH1)
	require.NoError(t, err)

	bodyH1, _ := io.ReadAll(respH1.Body)
	_ = respH1.Body.Close()

	assert.Equal(t, status.OK, respH1.StatusCode)
	assert.Equal(t, "HTTP/1.1", respH1.Proto)
	assert.Equal(t, "HTTP/1.1", respH1.Header.Get("X-Unified-Protocol"))
	assert.Equal(t, "GET", respH1.Header.Get("X-Echo-Method"))
	assert.Equal(t, "/probe-h1", respH1.Header.Get("X-Echo-Path"))
	assert.Equal(t, "Unified response via HTTP/1.1 for /probe-h1", string(bodyH1))

	// Verify Alt-Svc advertisement on HTTP/1.1 response
	altSvcH1 := respH1.Header.Get("Alt-Svc")
	require.True(t, altSvcH1 != "", "Alt-Svc header must be present on H1 response")
	servicesH1 := altsvc.Parse(altSvcH1)
	require.True(t, len(servicesH1) > 0, "Alt-Svc services parsed successfully")
	assert.Equal(t, "h3", servicesH1[0].ProtocolID)
	assert.Equal(t, udpAddr.Port, servicesH1[0].Port)

	// --- Phase 2: HTTP/2 client over TLS on the SAME TCP socket ---
	h2TLS := clientTLS.Clone()
	h2TLS.NextProtos = []string{"h2"}
	h2Client := &http.Client{
		Transport: &http2.Transport{TLSClientConfig: h2TLS},
		Timeout:   5 * time.Second,
	}

	reqH2, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+tcpAddr+"/probe-h2", nil)
	require.NoError(t, err)
	respH2, err := h2Client.Do(reqH2)
	require.NoError(t, err)

	bodyH2, _ := io.ReadAll(respH2.Body)
	_ = respH2.Body.Close()

	assert.Equal(t, status.OK, respH2.StatusCode)
	assert.Equal(t, "HTTP/2.0", respH2.Proto)
	assert.Equal(t, "HTTP/2", respH2.Header.Get("X-Unified-Protocol"))
	assert.Equal(t, "GET", respH2.Header.Get("X-Echo-Method"))
	assert.Equal(t, "/probe-h2", respH2.Header.Get("X-Echo-Path"))
	assert.Equal(t, "Unified response via HTTP/2 for /probe-h2", string(bodyH2))

	// Verify Alt-Svc advertisement on HTTP/2 response
	altSvcH2 := respH2.Header.Get("Alt-Svc")
	require.True(t, altSvcH2 != "", "Alt-Svc header must be present on H2 response")
	servicesH2 := altsvc.Parse(altSvcH2)
	require.True(t, len(servicesH2) > 0, "Alt-Svc services parsed successfully")
	assert.Equal(t, "h3", servicesH2[0].ProtocolID)
	assert.Equal(t, udpAddr.Port, servicesH2[0].Port)

	// --- Phase 3: HTTP/3 client over QUIC on the UDP listener ---
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
	reqH3.SetRequestURI("/probe-h3")
	reqH3.Header.SetHost("localhost")

	_, err = cc.Do(ctxH3, reqH3, respH3, nil)
	require.NoError(t, err)

	assert.Equal(t, status.OK, respH3.StatusCode())
	assert.Equal(t, "HTTP/3", string(respH3.Header.Peek("X-Unified-Protocol")))
	assert.Equal(t, "GET", string(respH3.Header.Peek("X-Echo-Method")))
	assert.Equal(t, "/probe-h3", string(respH3.Header.Peek("X-Echo-Path")))
	assert.Equal(t, "Unified response via HTTP/3 for /probe-h3", string(respH3.Body()))
}

func TestUnifiedServer_E2E_AltSvcDiscoveryAndTransition(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)

	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer tcpLn.Close()

	tlsTCP := tls.NewListener(tcpLn, serverTLS)

	quicServerTLS := serverTLS.Clone()
	quicServerTLS.NextProtos = []string{"h3"}

	quicLn, err := quic.ListenAddr("127.0.0.1:0", quicServerTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicLn.Close()

	srv := server.New(
		server.WithHandlerFunc(func(ctx *server.Context) error {
			if ctx.IsH3() {
				ctx.SetHeader("X-Service-Tier", "http3-quic-accelerated")
				ctx.SetBodyString("Upgraded to HTTP/3 successfully!")
			} else {
				ctx.SetHeader("X-Service-Tier", "legacy-tcp")
				ctx.SetBodyString("Connect over HTTP/3 for acceleration")
			}
			return nil
		}),
	)

	go func() {
		_ = srv.ServeMultiProtocol(tlsTCP, quicLn)
	}()
	defer srv.Close()

	// Step 1: Initial client connection over HTTP/1.1 via TCP
	h1TLS := clientTLS.Clone()
	h1TLS.NextProtos = []string{"http/1.1"}
	h1Client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: h1TLS},
		Timeout:   3 * time.Second,
	}

	initialResp, err := h1Client.Get("https://" + tcpLn.Addr().String() + "/resource")
	require.NoError(t, err)
	_ = initialResp.Body.Close()

	assert.Equal(t, "legacy-tcp", initialResp.Header.Get("X-Service-Tier"))

	// Step 2: Extract Alt-Svc advertisement pointing to HTTP/3 over QUIC
	altSvcRaw := initialResp.Header.Get("Alt-Svc")
	require.True(t, altSvcRaw != "", "Alt-Svc header must be advertised")

	services := altsvc.Parse(altSvcRaw)
	require.True(t, len(services) > 0, "Discovered alternative services")
	h3Port := services[0].Port
	require.True(t, h3Port > 0, "Discovered HTTP/3 port must be positive")

	// Step 3: Transparently transition to HTTP/3 over QUIC on discovered port
	discoveredAddr := fmt.Sprintf("127.0.0.1:%d", h3Port)
	h3ClientTLS := clientTLS.Clone()
	h3ClientTLS.NextProtos = []string{"h3"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	quicConn, err := quic.DialAddr(ctx, discoveredAddr, h3ClientTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicConn.CloseWithError(0, "")

	cc, err := h3client.NewClientConn(quicConn, nil)
	require.NoError(t, err)
	defer cc.Close()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/resource")
	req.Header.SetHost("localhost")

	_, err = cc.Do(ctx, req, resp, nil)
	require.NoError(t, err)

	assert.Equal(t, status.OK, resp.StatusCode())
	assert.Equal(t, "http3-quic-accelerated", string(resp.Header.Peek("X-Service-Tier")))
	assert.Equal(t, "Upgraded to HTTP/3 successfully!", string(resp.Body()))
}

func TestUnifiedServer_E2E_PayloadTransfer_AcrossAllProtocols(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)

	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer tcpLn.Close()

	tlsTCP := tls.NewListener(tcpLn, serverTLS)

	quicServerTLS := serverTLS.Clone()
	quicServerTLS.NextProtos = []string{"h3"}

	quicLn, err := quic.ListenAddr("127.0.0.1:0", quicServerTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicLn.Close()

	payloadSize := 64 * 1024 // 64 KB test payload
	testPayload := make([]byte, payloadSize)
	for i := range testPayload {
		testPayload[i] = byte(i % 256)
	}
	expectedHash := fmt.Sprintf("%x", sha256.Sum256(testPayload))

	srv := server.New(
		server.WithHandlerFunc(func(ctx *server.Context) error {
			body := ctx.Body()
			receivedHash := fmt.Sprintf("%x", sha256.Sum256(body))
			if receivedHash != expectedHash {
				ctx.SetStatusCode(status.BadRequest)
				ctx.SetBodyString("payload hash mismatch")
				return nil
			}

			ctx.SetStatusCode(status.OK)
			ctx.SetHeader("X-Payload-Hash", receivedHash)
			ctx.SetBodyString("payload-verified")
			return nil
		}),
	)

	go func() {
		_ = srv.ServeMultiProtocol(tlsTCP, quicLn)
	}()
	defer srv.Close()

	tcpAddr := tcpLn.Addr().String()

	// 1. POST via HTTP/1.1
	h1TLS := clientTLS.Clone()
	h1TLS.NextProtos = []string{"http/1.1"}
	h1Client := &http.Client{Transport: &http.Transport{TLSClientConfig: h1TLS}, Timeout: 5 * time.Second}

	respH1, err := h1Client.Post("https://"+tcpAddr+"/upload-h1", "application/octet-stream", strings.NewReader(string(testPayload)))
	require.NoError(t, err)
	bodyH1, _ := io.ReadAll(respH1.Body)
	_ = respH1.Body.Close()
	assert.Equal(t, status.OK, respH1.StatusCode)
	assert.Equal(t, expectedHash, respH1.Header.Get("X-Payload-Hash"))
	assert.Equal(t, "payload-verified", string(bodyH1))

	// 2. POST via HTTP/2
	h2TLS := clientTLS.Clone()
	h2TLS.NextProtos = []string{"h2"}
	h2Client := &http.Client{Transport: &http2.Transport{TLSClientConfig: h2TLS}, Timeout: 5 * time.Second}

	respH2, err := h2Client.Post("https://"+tcpAddr+"/upload-h2", "application/octet-stream", strings.NewReader(string(testPayload)))
	require.NoError(t, err)
	bodyH2, _ := io.ReadAll(respH2.Body)
	_ = respH2.Body.Close()
	assert.Equal(t, status.OK, respH2.StatusCode)
	assert.Equal(t, expectedHash, respH2.Header.Get("X-Payload-Hash"))
	assert.Equal(t, "payload-verified", string(bodyH2))

	// 3. POST via HTTP/3
	h3ClientTLS := clientTLS.Clone()
	h3ClientTLS.NextProtos = []string{"h3"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	quicConn, err := quic.DialAddr(ctx, quicLn.Addr().String(), h3ClientTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicConn.CloseWithError(0, "")

	cc, err := h3client.NewClientConn(quicConn, nil)
	require.NoError(t, err)
	defer cc.Close()

	reqH3 := machhttp.AcquireRequest()
	respH3 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(reqH3)
	defer machhttp.ReleaseResponse(respH3)

	reqH3.Header.SetMethod("POST")
	reqH3.SetRequestURI("/upload-h3")
	reqH3.Header.SetHost("localhost")
	reqH3.SetBody(testPayload)

	_, err = cc.Do(ctx, reqH3, respH3, nil)
	require.NoError(t, err)

	assert.Equal(t, status.OK, respH3.StatusCode())
	assert.Equal(t, expectedHash, string(respH3.Header.Peek("X-Payload-Hash")))
	assert.Equal(t, "payload-verified", string(respH3.Body()))
}

func TestUnifiedServer_E2E_EXP1201_ShortRequest_PreservesFraming(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	srv := server.NewWithHandlerFunc(func(ctx *server.Context) error {
		assert.Equal(t, server.ProtoHTTP11, ctx.Proto())
		assert.Equal(t, "/exp1201", ctx.Path())
		ctx.SetStatusCode(status.OK)
		ctx.SetBodyString("exp1201-protected")
		return nil
	})

	go func() {
		_ = srv.Serve(ln)
	}()
	defer srv.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	// Short request: 20 bytes (< 24 bytes)
	req := "GET /exp1201 HTTP/1.0\r\n\r\n"
	_, err = conn.Write([]byte(req))
	require.NoError(t, err)

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	assert.Equal(t, status.OK, resp.StatusCode)
	assert.Equal(t, "exp1201-protected", string(body))
}
