// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package e2e_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/client"
	"github.com/lemon4ksan/mach/net/quic"
	"github.com/lemon4ksan/mach/proto/codec/hpack"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	"github.com/lemon4ksan/mach/proto/ws"
	"github.com/lemon4ksan/mach/server"
	h2server "github.com/lemon4ksan/mach/server/h2"
)

// setupEchoServer starts a unified server that upgrades incoming requests
// across HTTP/1.1, HTTP/2, and HTTP/3 and runs a full-duplex WebSocket echo loop.
func setupEchoServer(t *testing.T, upgradeOpts ...server.WebSocketUpgradeOption) (tcpAddr string, quicAddr string, clientTLS *tls.Config, cleanup func()) {
	t.Helper()

	serverTLS, cTLS := generateTestTLSConfig(t)

	// 1. Setup TCP TLS listener (ALPN: h2, http/1.1)
	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	tlsTCP := tls.NewListener(tcpLn, serverTLS)

	// 2. Setup QUIC UDP listener (ALPN: h3)
	quicServerTLS := serverTLS.Clone()
	quicServerTLS.NextProtos = []string{"h3"}

	quicLn, err := quic.ListenAddr("127.0.0.1:0", quicServerTLS, quic.WithDatagrams(true))
	require.NoError(t, err)

	srv := server.New(
		server.WithHandlerFunc(func(ctx *server.Context) error {
			if ctx.Path() == "/reject" {
				ctx.SetStatusCode(403)
				ctx.SetBodyString("Forbidden")
				return nil
			}

			wsConn, err := ctx.UpgradeWebSocket(upgradeOpts...)
			if err != nil {
				return err
			}
			defer wsConn.Close()

			for {
				msgType, payload, err := wsConn.ReadMessage()
				if err != nil {
					if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
						return nil
					}
					var closeErr *ws.CloseError
					if errors.As(err, &closeErr) {
						return nil
					}
					return err
				}

				if err := wsConn.WriteMessage(msgType, payload); err != nil {
					return err
				}
			}
		}),
	)

	go func() {
		_ = srv.ServeMultiProtocol(tlsTCP, quicLn)
	}()

	cleanupFn := func() {
		_ = srv.Close()
		_ = tlsTCP.Close()
		_ = quicLn.Close()
	}

	return tcpLn.Addr().String(), quicLn.Addr().String(), cTLS, cleanupFn
}

func TestWebSocket_E2E_HTTP1_Plaintext_Echo(t *testing.T) {
	t.Parallel()

	// Cleartext TCP listener
	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer tcpLn.Close()

	srv := server.New(
		server.WithHandlerFunc(func(ctx *server.Context) error {
			wsConn, err := ctx.UpgradeWebSocket()
			if err != nil {
				return err
			}
			defer wsConn.Close()

			for {
				msgType, payload, err := wsConn.ReadMessage()
				if err != nil {
					return nil
				}
				if err := wsConn.WriteMessage(msgType, payload); err != nil {
					return err
				}
			}
		}),
	)

	go func() {
		_ = srv.Serve(tcpLn)
	}()
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	urlStr := fmt.Sprintf("ws://%s/chat", tcpLn.Addr().String())
	conn, resp, err := client.DialWebSocket(ctx, urlStr, client.WithForceProto("http/1.1"))
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.NotNil(t, resp)
	assert.Equal(t, 101, resp.StatusCode())
	defer conn.Close()

	// Test Text Echo
	for i := 0; i < 5; i++ {
		msg := fmt.Sprintf("hello plaintext h1 message #%d", i)
		err = conn.WriteText(msg)
		require.NoError(t, err)

		op, data, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, ws.OpText, op)
		assert.Equal(t, msg, string(data))
	}

	// Test Binary Echo
	binPayload := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03, 0x04}
	err = conn.WriteBinary(binPayload)
	require.NoError(t, err)

	op, data, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, ws.OpBinary, op)
	assert.Equal(t, binPayload, data)
}

func TestWebSocket_E2E_HTTP1_TLS_Echo(t *testing.T) {
	t.Parallel()

	tcpAddr, _, clientTLS, cleanup := setupEchoServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	urlStr := fmt.Sprintf("wss://%s/chat", tcpAddr)
	conn, resp, err := client.DialWebSocket(ctx, urlStr,
		client.WithWebSocketTLSConfig(clientTLS),
		client.WithForceProto("http/1.1"),
	)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.NotNil(t, resp)
	assert.Equal(t, 101, resp.StatusCode())
	defer conn.Close()

	for i := 0; i < 5; i++ {
		msg := fmt.Sprintf("hello TLS H1 message #%d", i)
		err = conn.WriteText(msg)
		require.NoError(t, err)

		op, data, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, ws.OpText, op)
		assert.Equal(t, msg, string(data))
	}
}

func TestWebSocket_E2E_HTTP2_ExtendedConnect_Echo(t *testing.T) {
	t.Parallel()

	tcpAddr, _, clientTLS, cleanup := setupEchoServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	urlStr := fmt.Sprintf("wss://%s/chat", tcpAddr)
	conn, resp, err := client.DialWebSocket(ctx, urlStr,
		client.WithWebSocketTLSConfig(clientTLS),
		client.WithForceProto("h2"),
	)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode())
	defer conn.Close()

	// Text echo over RFC 8441 stream
	for i := 0; i < 5; i++ {
		msg := fmt.Sprintf("RFC 8441 H2 Extended CONNECT frame #%d", i)
		err = conn.WriteText(msg)
		require.NoError(t, err)

		op, data, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, ws.OpText, op)
		assert.Equal(t, msg, string(data))
	}

	// Binary echo over RFC 8441 stream
	bin := make([]byte, 1024)
	_, _ = rand.Read(bin)
	err = conn.WriteBinary(bin)
	require.NoError(t, err)

	op, data, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, ws.OpBinary, op)
	assert.Equal(t, bin, data)
}

func TestWebSocket_E2E_HTTP3_ExtendedConnect_Echo(t *testing.T) {
	t.Parallel()

	_, quicAddr, clientTLS, cleanup := setupEchoServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	urlStr := fmt.Sprintf("https://%s/chat", quicAddr)
	conn, resp, err := client.DialWebSocket(ctx, urlStr,
		client.WithWebSocketTLSConfig(clientTLS),
		client.WithForceProto("h3"),
	)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode())
	defer conn.Close()

	// Text echo over RFC 9220 H3 stream
	for i := 0; i < 5; i++ {
		msg := fmt.Sprintf("RFC 9220 H3 Extended CONNECT frame #%d", i)
		err = conn.WriteText(msg)
		require.NoError(t, err)

		op, data, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, ws.OpText, op)
		assert.Equal(t, msg, string(data))
	}

	// Binary echo over RFC 9220 H3 stream
	bin := make([]byte, 2048)
	_, _ = rand.Read(bin)
	err = conn.WriteBinary(bin)
	require.NoError(t, err)

	op, data, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, ws.OpBinary, op)
	assert.Equal(t, bin, data)
}

func TestWebSocket_E2E_Automatic_ALPN_Negotiation(t *testing.T) {
	t.Parallel()

	tcpAddr, _, clientTLS, cleanup := setupEchoServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect without WithForceProto: ALPN negotiates "h2", server has SETTINGS_ENABLE_CONNECT_PROTOCOL,
	// so client transparently selects HTTP/2 Extended CONNECT.
	urlStr := fmt.Sprintf("wss://%s/chat", tcpAddr)
	conn, resp, err := client.DialWebSocket(ctx, urlStr,
		client.WithWebSocketTLSConfig(clientTLS),
	)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode())
	defer conn.Close()

	msg := "auto-negotiated websocket over ALPN h2"
	err = conn.WriteText(msg)
	require.NoError(t, err)

	op, data, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, ws.OpText, op)
	assert.Equal(t, msg, string(data))
}

func TestWebSocket_E2E_PingPong_Heartbeats(t *testing.T) {
	t.Parallel()

	tcpAddr, quicAddr, clientTLS, cleanup := setupEchoServer(t)
	defer cleanup()

	protocols := []struct {
		name       string
		addr       string
		scheme     string
		forceProto string
	}{
		{name: "H1", addr: tcpAddr, scheme: "wss", forceProto: "http/1.1"},
		{name: "H2", addr: tcpAddr, scheme: "wss", forceProto: "h2"},
		{name: "H3", addr: quicAddr, scheme: "https", forceProto: "h3"},
	}

	for _, tc := range protocols {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			urlStr := fmt.Sprintf("%s://%s/chat", tc.scheme, tc.addr)
			conn, _, err := client.DialWebSocket(ctx, urlStr,
				client.WithWebSocketTLSConfig(clientTLS),
				client.WithForceProto(tc.forceProto),
			)
			require.NoError(t, err)
			defer conn.Close()

			pongReceived := make(chan string, 1)
			conn.SetPongHandler(func(appData []byte) error {
				pongReceived <- string(appData)
				return nil
			})

			pingPayload := fmt.Sprintf("ping-%s-%d", tc.name, time.Now().UnixNano())
			err = conn.WritePing([]byte(pingPayload))
			require.NoError(t, err)

			// Also write a text message so ReadMessage reads until pong and then text
			textMsg := "hello after ping"
			err = conn.WriteText(textMsg)
			require.NoError(t, err)

			op, data, err := conn.ReadMessage()
			require.NoError(t, err)
			assert.Equal(t, ws.OpText, op)
			assert.Equal(t, textMsg, string(data))

			select {
			case pong := <-pongReceived:
				assert.Equal(t, pingPayload, pong)
			case <-time.After(2 * time.Second):
				t.Fatalf("%s: timed out waiting for pong", tc.name)
			}
		})
	}
}

func TestWebSocket_E2E_Subprotocol_Negotiation(t *testing.T) {
	t.Parallel()

	// Server configured to accept "graphql-transport-ws" or "wamp.2.json"
	tcpAddr, quicAddr, clientTLS, cleanup := setupEchoServer(t, server.WithSubprotocols("graphql-transport-ws", "wamp.2.json"))
	defer cleanup()

	protocols := []struct {
		name       string
		addr       string
		scheme     string
		forceProto string
	}{
		{name: "H1", addr: tcpAddr, scheme: "wss", forceProto: "http/1.1"},
		{name: "H2", addr: tcpAddr, scheme: "wss", forceProto: "h2"},
		{name: "H3", addr: quicAddr, scheme: "https", forceProto: "h3"},
	}

	for _, tc := range protocols {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			urlStr := fmt.Sprintf("%s://%s/chat", tc.scheme, tc.addr)
			conn, _, err := client.DialWebSocket(ctx, urlStr,
				client.WithWebSocketTLSConfig(clientTLS),
				client.WithForceProto(tc.forceProto),
				client.WithWebSocketSubprotocols("custom.protocol", "graphql-transport-ws"),
			)
			require.NoError(t, err)
			defer conn.Close()

			assert.Equal(t, "graphql-transport-ws", conn.Subprotocol())
		})
	}
}

func TestWebSocket_E2E_LargePayload(t *testing.T) {
	t.Parallel()

	tcpAddr, quicAddr, clientTLS, cleanup := setupEchoServer(t)
	defer cleanup()

	protocols := []struct {
		name       string
		addr       string
		scheme     string
		forceProto string
	}{
		{name: "H1", addr: tcpAddr, scheme: "wss", forceProto: "http/1.1"},
		{name: "H2", addr: tcpAddr, scheme: "wss", forceProto: "h2"},
		{name: "H3", addr: quicAddr, scheme: "https", forceProto: "h3"},
	}

	const payloadSize = 512 * 1024 // 512KB
	largeData := make([]byte, payloadSize)
	_, _ = rand.Read(largeData)
	expectedHash := sha256.Sum256(largeData)

	for _, tc := range protocols {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			urlStr := fmt.Sprintf("%s://%s/chat", tc.scheme, tc.addr)
			conn, _, err := client.DialWebSocket(ctx, urlStr,
				client.WithWebSocketTLSConfig(clientTLS),
				client.WithForceProto(tc.forceProto),
			)
			require.NoError(t, err)
			defer conn.Close()

			err = conn.WriteBinary(largeData)
			require.NoError(t, err)

			op, data, err := conn.ReadMessage()
			require.NoError(t, err)
			assert.Equal(t, ws.OpBinary, op)
			assert.Equal(t, len(largeData), len(data))

			actualHash := sha256.Sum256(data)
			assert.Equal(t, expectedHash, actualHash)
		})
	}
}

func TestWebSocket_E2E_Chromium_BoundaryConditions(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)

	// Boundary 1: Server lacks SETTINGS_ENABLE_CONNECT_PROTOCOL (RFC 8441 §3 / Chromium websocket_http2_handshake_stream.cc)
	t.Run("H2_ServerLacksEnableConnect_Rejection", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()

		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				tlsConn := tls.Server(conn, serverTLS)
				if err := tlsConn.Handshake(); err != nil {
					_ = conn.Close()
					continue
				}
				sc := h2server.NewServerConn(tlsConn, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
					return nil
				})
				// Explicitly disable SETTINGS_ENABLE_CONNECT_PROTOCOL
				sc.EnableConnectProtocol = false
				go func(c *h2server.ServerConn) {
					_ = c.Serve()
				}(sc)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		urlStr := fmt.Sprintf("wss://%s/chat", ln.Addr().String())
		_, _, err = client.DialWebSocket(ctx, urlStr,
			client.WithWebSocketTLSConfig(clientTLS),
			client.WithForceProto("h2"),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SETTINGS_ENABLE_CONNECT_PROTOCOL")
	})

	// Boundary 2: Non-200 handshake response rejects upgrade
	t.Run("Non200_HandshakeResponse_Rejected", func(t *testing.T) {
		tcpAddr, quicAddr, cTLS, cleanup := setupEchoServer(t)
		defer cleanup()

		protos := []struct {
			name   string
			addr   string
			scheme string
			proto  string
		}{
			{name: "H1", addr: tcpAddr, scheme: "wss", proto: "http/1.1"},
			{name: "H2", addr: tcpAddr, scheme: "wss", proto: "h2"},
			{name: "H3", addr: quicAddr, scheme: "https", proto: "h3"},
		}

		for _, p := range protos {
			t.Run(p.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()

				urlStr := fmt.Sprintf("%s://%s/reject", p.scheme, p.addr)
				_, resp, err := client.DialWebSocket(ctx, urlStr,
					client.WithWebSocketTLSConfig(cTLS),
					client.WithForceProto(p.proto),
				)
				require.Error(t, err)
				if resp != nil {
					assert.Equal(t, 403, resp.StatusCode())
				}
			})
		}
	})

	createTCPPair := func(t *testing.T) (net.Conn, net.Conn) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()

		ch := make(chan net.Conn, 1)
		go func() {
			c, _ := ln.Accept()
			ch <- c
		}()

		c1, err := net.Dial("tcp", ln.Addr().String())
		require.NoError(t, err)
		c2 := <-ch
		return c1, c2
	}

	// Boundary 3: RFC 6455 §5.1 Client-to-Server Masking Violation
	t.Run("Server_Rejects_Unmasked_ClientFrame", func(t *testing.T) {
		clientConn, serverConn := createTCPPair(t)
		defer serverConn.Close()
		defer clientConn.Close()

		// Server side expecting masked client frames (isServer = true)
		serverWS := ws.NewConn(serverConn, true)

		// Rogue client sending unmasked frame
		go func() {
			// Opcode 1 (Text), unmasked (mask bit 0), length 5, payload "hello"
			rawFrame := []byte{0x81, 0x05, 'h', 'e', 'l', 'l', 'o'}
			_, _ = clientConn.Write(rawFrame)
			_, _ = io.Copy(io.Discard, clientConn)
		}()

		_, _, err := serverWS.ReadMessage()
		require.Error(t, err)
		assert.Equal(t, ws.ErrUnmaskedClientFrame, err)
	})

	// Boundary 4: RFC 6455 §5.1 Server-to-Client Masking Violation
	t.Run("Client_Rejects_Masked_ServerFrame", func(t *testing.T) {
		serverConn, clientConn := createTCPPair(t)
		defer serverConn.Close()
		defer clientConn.Close()

		// Client side expecting unmasked server frames (isServer = false)
		clientWS := ws.NewConn(clientConn, false)

		// Rogue server sending masked frame (mask bit = 1)
		go func() {
			rawFrame := []byte{0x81, 0x85, 0x01, 0x02, 0x03, 0x04, 'h', 'e', 'l', 'l', 'o'}
			_, _ = serverConn.Write(rawFrame)
			_, _ = io.Copy(io.Discard, serverConn)
		}()

		_, _, err := clientWS.ReadMessage()
		require.Error(t, err)
		assert.Equal(t, ws.ErrMaskedServerFrame, err)
	})

	// Boundary 5: RFC 8441 §4 Extended CONNECT missing :scheme must be rejected with PROTOCOL_ERROR
	t.Run("H2_Missing_PseudoHeaders_Rejected", func(t *testing.T) {
		tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer tcpLn.Close()

		go func() {
			conn, err := tcpLn.Accept()
			if err != nil {
				return
			}
			sc := h2server.NewServerConn(conn, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
				return nil
			})
			defer sc.Release()
			_ = sc.Serve()
		}()

		clientConn, err := net.Dial("tcp", tcpLn.Addr().String())
		require.NoError(t, err)
		defer clientConn.Close()

		// Send H2 preface + empty SETTINGS
		_, err = clientConn.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n\x00\x00\x00\x04\x00\x00\x00\x00\x00"))
		require.NoError(t, err)

		enc := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(enc)

		// Extended CONNECT missing :scheme (RFC 8441 §4 requires :scheme, :path, :authority)
		var headerPayload []byte
		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)

		for _, kv := range [][2]string{
			{":method", "CONNECT"},
			{":protocol", "websocket"},
			{":authority", "127.0.0.1"},
			{":path", "/chat"},
			// Intentionally omit :scheme
		} {
			hf.Set(kv[0], kv[1])
			headerPayload = enc.AppendHeader(headerPayload, hf, false)
		}

		fr := coreh2.AcquireFrameHeader()
		defer coreh2.ReleaseFrameHeader(fr)

		hFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
		hFrame.SetEndHeaders(true)
		hFrame.SetEndStream(false)
		hFrame.SetHeaders(headerPayload)

		fr.SetStream(1)
		fr.SetFlags(coreh2.FlagEndHeaders)
		fr.SetBody(hFrame)

		bw := bufio.NewWriter(clientConn)
		_, err = fr.WriteTo(bw)
		require.NoError(t, err)
		_ = bw.Flush()

		// Read frames until RST_STREAM(1, PROTOCOL_ERROR) is observed
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
		assert.True(t, foundRst, "expected RST_STREAM(1, PROTOCOL_ERROR) for Extended CONNECT missing :scheme (RFC 8441 §4)")
	})
}
