// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/foundation/generic"

	h1client "github.com/lemon4ksan/mach/client/h1"
	h2client "github.com/lemon4ksan/mach/client/h2"
	h3client "github.com/lemon4ksan/mach/client/h3"
	"github.com/lemon4ksan/mach/net/quic"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	"github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/proto/ws"
)

var (
	defaultEngineMu sync.Mutex
	defaultEngine   *Engine
)

func getDefaultEngine() *Engine {
	defaultEngineMu.Lock()
	defer defaultEngineMu.Unlock()

	if defaultEngine == nil {
		defaultEngine = NewEngine()
	}

	return defaultEngine
}

// WebSocketOption configures client WebSocket dial parameters.
type WebSocketOption func(*WebSocketDialConfig)

// WebSocketDialConfig contains parameters for dialing a WebSocket connection.
type WebSocketDialConfig struct {
	Headers      map[string]string
	Subprotocols []string
	ForceProto   string // "http/1.1", "h2", "h3"
	TLSConfig    *tls.Config
}

// WithWebSocketHeader sets a custom header to include in the handshake request.
func WithWebSocketHeader(key, val string) WebSocketOption {
	return func(c *WebSocketDialConfig) {
		if c.Headers == nil {
			c.Headers = make(map[string]string)
		}

		c.Headers[key] = val
	}
}

// WithWebSocketSubprotocols configures the requested WebSocket subprotocols.
func WithWebSocketSubprotocols(subprotocols ...string) WebSocketOption {
	return func(c *WebSocketDialConfig) {
		c.Subprotocols = append(c.Subprotocols, subprotocols...)
	}
}

// WithForceProto forces using a specific protocol ("http/1.1", "h2", or "h3").
func WithForceProto(proto string) WebSocketOption {
	return func(c *WebSocketDialConfig) {
		c.ForceProto = strings.ToLower(proto)
	}
}

// WithWebSocketTLSConfig configures the TLS configuration for the connection.
func WithWebSocketTLSConfig(conf *tls.Config) WebSocketOption {
	return func(c *WebSocketDialConfig) {
		c.TLSConfig = conf
	}
}

type h1HijackedConn struct {
	net.Conn
	r *bufio.Reader
	w *bufio.Writer
}

func (h *h1HijackedConn) Read(p []byte) (int, error) {
	if h.r != nil {
		return h.r.Read(p)
	}

	return h.Conn.Read(p)
}

func (h *h1HijackedConn) Write(p []byte) (int, error) {
	if h.w != nil {
		n, err := h.w.Write(p)
		if err != nil {
			return n, err
		}

		return n, h.w.Flush()
	}

	return h.Conn.Write(p)
}

func (h *h1HijackedConn) Close() error {
	if h.w != nil {
		_ = h.w.Flush()
	}

	return h.Conn.Close()
}

// DialWebSocket establishes an intelligent, multi-protocol WebSocket connection.
// It uses default client options and supports HTTP/1.1 Upgrade, HTTP/2 Extended CONNECT (RFC 8441),
// and HTTP/3 Extended CONNECT (RFC 9220).
func DialWebSocket(ctx context.Context, urlStr string, opts ...WebSocketOption) (*ws.Conn, *http.Response, error) {
	return getDefaultEngine().DialWebSocket(ctx, urlStr, opts...)
}

// DialWebSocket establishes a WebSocket connection over HTTP/1.1 (RFC 6455),
// HTTP/2 Extended CONNECT (RFC 8441), or HTTP/3 Extended CONNECT (RFC 9220).
func (e *Engine) DialWebSocket(
	ctx context.Context,
	urlStr string,
	opts ...WebSocketOption,
) (*ws.Conn, *http.Response, error) {
	cfg := &WebSocketDialConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, nil, fmt.Errorf("client: invalid websocket url: %w", err)
	}

	var (
		isTLS       bool
		defaultPort string
	)

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "ws":
		isTLS = false
		defaultPort = "80"
	case "wss":
		isTLS = true
		defaultPort = "443"
	case "http":
		isTLS = false
		defaultPort = "80"
	case "https":
		isTLS = true
		defaultPort = "443"
	default:
		return nil, nil, fmt.Errorf("client: unsupported scheme for websocket dial: %s", u.Scheme)
	}

	host := u.Host
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, defaultPort)
	}

	authority := u.Host

	path := generic.Coalesce(u.RequestURI(), "/")

	baseTLS := generic.CoalesceNil(cfg.TLSConfig, e.TLSConfig)

	var tlsConf *tls.Config
	if baseTLS != nil {
		tlsConf = baseTLS.Clone()
	} else if isTLS {
		tlsConf = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	if isTLS && tlsConf.ServerName == "" {
		h, _, err := net.SplitHostPort(u.Host)
		tlsConf.ServerName = generic.Ternary(err == nil, h, u.Host)
	}

	proto := cfg.ForceProto
	if proto == "h3" {
		return e.dialH3WebSocket(ctx, host, authority, path, cfg, tlsConf)
	}

	if proto == "h2" {
		if !isTLS {
			return nil, nil, errors.New("client: HTTP/2 Extended CONNECT requires TLS")
		}

		return e.dialH2WebSocket(ctx, host, authority, path, cfg, tlsConf)
	}

	if proto == "http/1.1" || proto == "h1" {
		return e.dialH1WebSocket(ctx, host, authority, path, isTLS, cfg, tlsConf)
	}

	// Automatic negotiation
	if !isTLS {
		return e.dialH1WebSocket(ctx, host, authority, path, false, cfg, tlsConf)
	}

	// Check Alt-Svc cache for HTTP/3
	origin := "https://" + host
	if !e.DisableAltSvc && e.AltSvcCache != nil {
		if h3Addr, ok := e.AltSvcCache.GetH3(origin); ok {
			wsConn, resp, err := e.dialH3WebSocket(ctx, h3Addr, authority, path, cfg, tlsConf)
			if err == nil {
				return wsConn, resp, nil
			}

			e.AltSvcCache.MarkFailed(origin, 5*time.Minute)
		}
	}

	// Origin ALPN: advertise h2 and http/1.1
	alpnTLS := tlsConf.Clone()
	alpnTLS.NextProtos = []string{"h2", "http/1.1"}

	tcpConn, err := e.Dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, nil, err
	}

	tlsConn := tls.Client(tcpConn, alpnTLS)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = tcpConn.Close()
		return nil, nil, err
	}

	negotiated := tlsConn.ConnectionState().NegotiatedProtocol
	if negotiated == "h2" {
		h2c := h2client.NewConn(tlsConn, e.H2Opts)
		if err := h2c.Handshake(); err != nil {
			_ = tlsConn.Close()
			return nil, nil, err
		}

		if !h2c.EnableConnect() {
			_ = h2c.Close()
			return nil, nil, errors.New("client: server did not advertise SETTINGS_ENABLE_CONNECT_PROTOCOL")
		}

		return e.startH2WebSocketSession(ctx, h2c, authority, path, cfg)
	}

	// Fallback to HTTP/1.1 over the negotiated TLS connection
	h1c := h1client.NewClientConn(tlsConn)

	return e.startH1WebSocketSession(ctx, h1c, authority, path, cfg)
}

func (e *Engine) dialH1WebSocket(
	ctx context.Context,
	addr string,
	authority string,
	path string,
	isTLS bool,
	cfg *WebSocketDialConfig,
	tlsConf *tls.Config,
) (*ws.Conn, *http.Response, error) {
	var rawConn net.Conn
	if isTLS {
		h1TLS := tlsConf.Clone()
		h1TLS.NextProtos = []string{"http/1.1"}

		tcpConn, err := e.Dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, nil, err
		}

		tc := tls.Client(tcpConn, h1TLS)
		if err := tc.HandshakeContext(ctx); err != nil {
			_ = tcpConn.Close()
			return nil, nil, err
		}

		rawConn = tc
	} else {
		tcpConn, err := e.Dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, nil, err
		}

		rawConn = tcpConn
	}

	h1c := h1client.NewClientConn(rawConn)

	return e.startH1WebSocketSession(ctx, h1c, authority, path, cfg)
}

func (e *Engine) startH1WebSocketSession(
	ctx context.Context,
	h1c *h1client.ClientConn,
	authority string,
	path string,
	cfg *WebSocketDialConfig,
) (*ws.Conn, *http.Response, error) {
	challengeKey := ws.GenerateChallengeKey()

	req := http.AcquireRequest()
	defer http.ReleaseRequest(req)

	req.Header.SetMethod("GET")
	req.SetRequestURI(path)
	req.SetHost(authority)
	req.Header.SetHost(authority)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", challengeKey)
	req.Header.Set("Sec-WebSocket-Version", "13")

	if len(cfg.Subprotocols) > 0 {
		req.Header.Set("Sec-WebSocket-Protocol", strings.Join(cfg.Subprotocols, ", "))
	}

	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	resp := http.AcquireResponse()
	if err := h1c.Do(ctx, req, resp); err != nil {
		http.ReleaseResponse(resp)

		_ = h1c.Close()

		return nil, nil, err
	}

	if resp.StatusCode() != 101 {
		_ = h1c.Close()
		return nil, resp, fmt.Errorf("client: websocket upgrade failed: status code %d", resp.StatusCode())
	}

	acceptKey := string(resp.Header.Peek("Sec-WebSocket-Accept"))
	if acceptKey != ws.ComputeAcceptKey(challengeKey) {
		_ = h1c.Close()
		return nil, resp, ws.ErrInvalidAcceptKey
	}

	hijackedConn, br, bw, err := h1c.Hijack()
	if err != nil {
		_ = h1c.Close()
		return nil, resp, err
	}

	if bw != nil {
		_ = bw.Flush()
	}

	buffered := &h1HijackedConn{
		Conn: hijackedConn,
		r:    br,
		w:    bw,
	}

	wsConn := ws.NewConnWithReader(buffered, br, false)

	selectedSubproto := string(resp.Header.Peek("Sec-WebSocket-Protocol"))
	if selectedSubproto != "" {
		wsConn.SetSubprotocol(selectedSubproto)
	}

	return wsConn, resp, nil
}

func (e *Engine) dialH2WebSocket(
	ctx context.Context,
	addr string,
	authority string,
	path string,
	cfg *WebSocketDialConfig,
	tlsConf *tls.Config,
) (*ws.Conn, *http.Response, error) {
	h2TLS := tlsConf.Clone()
	h2TLS.NextProtos = []string{"h2"}

	tcpConn, err := e.Dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, err
	}

	tlsConn := tls.Client(tcpConn, h2TLS)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = tcpConn.Close()
		return nil, nil, err
	}

	h2c := h2client.NewConn(tlsConn, e.H2Opts)
	if err := h2c.Handshake(); err != nil {
		_ = tlsConn.Close()
		return nil, nil, err
	}

	if !h2c.EnableConnect() {
		_ = h2c.Close()
		return nil, nil, errors.New("client: server did not advertise SETTINGS_ENABLE_CONNECT_PROTOCOL")
	}

	return e.startH2WebSocketSession(ctx, h2c, authority, path, cfg)
}

func (e *Engine) startH2WebSocketSession(
	ctx context.Context,
	h2c *h2client.Conn,
	authority string,
	path string,
	cfg *WebSocketDialConfig,
) (*ws.Conn, *http.Response, error) {
	req := http.AcquireRequest()
	defer http.ReleaseRequest(req)

	req.Header.SetMethod("CONNECT")
	req.Header.Set(":protocol", "websocket")
	req.SetRequestURI(path)
	req.SetHost(authority)
	req.Header.SetHost(authority)
	req.URI().SetScheme("https")
	req.Header.Set("sec-websocket-version", "13")

	if len(cfg.Subprotocols) > 0 {
		req.Header.Set("sec-websocket-protocol", strings.Join(cfg.Subprotocols, ", "))
	}

	for k, v := range cfg.Headers {
		req.Header.Set(strings.ToLower(k), v)
	}

	stream, resp, err := h2c.StartExtendedConnect(ctx, req)
	if err != nil {
		_ = h2c.Close()
		return nil, resp, err
	}

	wsConn := ws.NewConn(&h2WebSocketStreamCloser{ExtendedConnectStream: stream, h2c: h2c}, false)

	selectedSubproto := generic.Coalesce(
		string(resp.Header.Peek("sec-websocket-protocol")),
		string(resp.Header.Peek("Sec-WebSocket-Protocol")),
	)

	if selectedSubproto != "" {
		wsConn.SetSubprotocol(selectedSubproto)
	}

	return wsConn, resp, nil
}

func (e *Engine) dialH3WebSocket(
	ctx context.Context,
	addr string,
	authority string,
	path string,
	cfg *WebSocketDialConfig,
	tlsConf *tls.Config,
) (*ws.Conn, *http.Response, error) {
	h3TLS := tlsConf.Clone()

	h3TLS.NextProtos = []string{"h3"}
	if h3TLS.MinVersion < tls.VersionTLS13 {
		h3TLS.MinVersion = tls.VersionTLS13
	}

	quicConn, err := quic.DialAddr(ctx, addr, h3TLS, quic.WithDatagrams(true))
	if err != nil {
		return nil, nil, err
	}

	h3Settings := e.H3Settings
	if h3Settings == nil {
		h3Settings = &coreh3.Settings{
			MaxFieldSectionSize: 262144,
			EnableDatagrams:     true,
			EnableConnect:       true,
		}
	}

	h3cc, err := h3client.NewClientConn(quicConn, h3Settings)
	if err != nil {
		_ = quicConn.CloseWithError(0, "")
		return nil, nil, err
	}

	// Wait for peer SETTINGS frame per RFC 9220 §3
	waitCtx, waitCancel := context.WithTimeout(ctx, 3*time.Second)
	defer waitCancel()

	if err := h3cc.WaitSettings(waitCtx); err != nil {
		_ = h3cc.Close()
		return nil, nil, fmt.Errorf("client: failed waiting for H3 settings: %w", err)
	}

	if !h3cc.EnableConnect() {
		_ = h3cc.Close()
		return nil, nil, errors.New("client: server did not advertise SETTINGS_ENABLE_CONNECT_PROTOCOL")
	}

	req := http.AcquireRequest()
	defer http.ReleaseRequest(req)

	req.Header.SetMethod("CONNECT")
	req.Header.Set(":protocol", "websocket")
	req.SetRequestURI(path)
	req.SetHost(authority)
	req.Header.SetHost(authority)
	req.URI().SetScheme("https")
	req.Header.Set("sec-websocket-version", "13")

	if len(cfg.Subprotocols) > 0 {
		req.Header.Set("sec-websocket-protocol", strings.Join(cfg.Subprotocols, ", "))
	}

	for k, v := range cfg.Headers {
		req.Header.Set(strings.ToLower(k), v)
	}

	stream, resp, err := h3cc.StartExtendedConnect(ctx, req)
	if err != nil {
		_ = h3cc.Close()
		return nil, resp, err
	}

	wsConn := ws.NewConn(&h3WebSocketStreamCloser{ExtendedConnectStream: stream, h3cc: h3cc}, false)

	selectedSubproto := generic.Coalesce(
		string(resp.Header.Peek("sec-websocket-protocol")),
		string(resp.Header.Peek("Sec-WebSocket-Protocol")),
	)

	if selectedSubproto != "" {
		wsConn.SetSubprotocol(selectedSubproto)
	}

	return wsConn, resp, nil
}

type h2WebSocketStreamCloser struct {
	*h2client.ExtendedConnectStream
	h2c *h2client.Conn
}

func (s *h2WebSocketStreamCloser) Close() error {
	_ = s.ExtendedConnectStream.Close()
	return s.h2c.Close()
}

type h3WebSocketStreamCloser struct {
	*h3client.ExtendedConnectStream
	h3cc *h3client.ClientConn
}

func (s *h3WebSocketStreamCloser) Close() error {
	_ = s.ExtendedConnectStream.Close()
	return s.h3cc.Close()
}
