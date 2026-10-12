// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/lemon4ksan/mach/proto/headkit"
	"github.com/lemon4ksan/mach/proto/ws"
)

// WebSocketUpgradeOption configures the server-side WebSocket upgrade handshake.
type WebSocketUpgradeOption func(*WebSocketUpgradeOptions)

// WebSocketUpgradeOptions contains optional parameters for WebSocket upgrade negotiation.
type WebSocketUpgradeOptions struct {
	Subprotocols []string
}

// WithSubprotocols configures supported WebSocket subprotocols.
func WithSubprotocols(subprotocols ...string) WebSocketUpgradeOption {
	return func(o *WebSocketUpgradeOptions) {
		o.Subprotocols = append(o.Subprotocols, subprotocols...)
	}
}

type h1HijackedConn struct {
	net.Conn
	r *bufio.Reader
	w *bufio.Writer
}

func (h *h1HijackedConn) Read(p []byte) (int, error) {
	return h.r.Read(p)
}

func (h *h1HijackedConn) Write(p []byte) (int, error) {
	n, err := h.w.Write(p)
	if err != nil {
		return n, err
	}
	return n, h.w.Flush()
}

func (h *h1HijackedConn) Close() error {
	_ = h.w.Flush()
	return h.Conn.Close()
}

// UpgradeWebSocket upgrades the current exchange across HTTP/1.1 (RFC 6455),
// HTTP/2 Extended CONNECT (RFC 8441), or HTTP/3 Extended CONNECT (RFC 9220)
// to a full-duplex WebSocket connection.
func (c *Context) UpgradeWebSocket(opts ...WebSocketUpgradeOption) (*ws.Conn, error) {
	options := &WebSocketUpgradeOptions{}
	for _, opt := range opts {
		opt(options)
	}

	switch c.proto {
	case ProtoHTTP11:
		return c.upgradeWebSocketH1(options)
	case ProtoHTTP2:
		return c.upgradeWebSocketH2(options)
	case ProtoHTTP3:
		return c.upgradeWebSocketH3(options)
	default:
		return nil, fmt.Errorf("server: unsupported protocol for websocket upgrade: %s", c.proto)
	}
}

func (c *Context) upgradeWebSocketH1(options *WebSocketUpgradeOptions) (*ws.Conn, error) {
	if c.h1Req == nil {
		return nil, errors.New("server: missing HTTP/1.1 request")
	}

	upgrade := c.h1Req.Headers.Get("Upgrade")
	connHdr := c.h1Req.Headers.Get("Connection")
	if !ws.HeaderContainsToken(upgrade, "websocket") || !ws.HeaderContainsToken(connHdr, "Upgrade") {
		return nil, ws.ErrNotWebSocketUpgrade
	}

	key := c.h1Req.Headers.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, ws.ErrInvalidChallengeKey
	}

	version := c.h1Req.Headers.Get("Sec-WebSocket-Version")
	if version != "13" {
		return nil, ws.ErrUnsupportedVersion
	}

	var selectedProto string
	if len(options.Subprotocols) > 0 {
		clientProtos := c.h1Req.Headers.Get("Sec-WebSocket-Protocol")
		for _, sp := range options.Subprotocols {
			if ws.HeaderContainsToken(clientProtos, sp) {
				selectedProto = sp
				break
			}
		}
	}

	netConn, rw, err := c.h1Req.Hijack()
	if err != nil {
		return nil, err
	}

	acceptKey := ws.ComputeAcceptKey(key)

	var resp strings.Builder
	resp.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	resp.WriteString("Upgrade: websocket\r\n")
	resp.WriteString("Connection: Upgrade\r\n")
	resp.WriteString("Sec-WebSocket-Accept: ")
	resp.WriteString(acceptKey)
	resp.WriteString("\r\n")

	if selectedProto != "" {
		resp.WriteString("Sec-WebSocket-Protocol: ")
		resp.WriteString(selectedProto)
		resp.WriteString("\r\n")
	}
	resp.WriteString("\r\n")

	if _, err := rw.WriteString(resp.String()); err != nil {
		_ = netConn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = netConn.Close()
		return nil, err
	}

	hijacked := &h1HijackedConn{
		Conn: netConn,
		r:    rw.Reader,
		w:    rw.Writer,
	}

	wsConn := ws.NewConnWithReader(hijacked, rw.Reader, true)
	if selectedProto != "" {
		wsConn.SetSubprotocol(selectedProto)
	}

	return wsConn, nil
}

func (c *Context) upgradeWebSocketH2(options *WebSocketUpgradeOptions) (*ws.Conn, error) {
	if c.h2Req == nil {
		return nil, errors.New("server: missing HTTP/2 request")
	}

	if c.h2Req.Method != "CONNECT" || c.h2Req.Protocol != "websocket" {
		return nil, errors.New("server: HTTP/2 request is not an Extended CONNECT websocket request (RFC 8441)")
	}

	if c.h2Req.Stream == nil {
		return nil, errors.New("server: HTTP/2 stream is nil")
	}

	ver := c.h2Req.Headers.Get("Sec-WebSocket-Version")
	if ver == "" {
		ver = c.h2Req.Headers.Get("sec-websocket-version")
	}
	if ver != "" && ver != "13" {
		return nil, ws.ErrUnsupportedVersion
	}

	var selectedProto string
	if len(options.Subprotocols) > 0 {
		clientProtos := c.h2Req.Headers.Get("Sec-WebSocket-Protocol")
		if clientProtos == "" {
			clientProtos = c.h2Req.Headers.Get("sec-websocket-protocol")
		}
		for _, sp := range options.Subprotocols {
			if ws.HeaderContainsToken(clientProtos, sp) {
				selectedProto = sp
				break
			}
		}
	}

	resHdrs := make(http.Header)
	if selectedProto != "" {
		resHdrs.Set("Sec-WebSocket-Protocol", selectedProto)
	}

	if c.h2Req.UpgradeFn != nil {
		if err := c.h2Req.UpgradeFn(http.StatusOK, resHdrs); err != nil {
			return nil, err
		}
	}

	wsConn := ws.NewConn(c.h2Req.Stream, true)
	if selectedProto != "" {
		wsConn.SetSubprotocol(selectedProto)
	}

	return wsConn, nil
}

func (c *Context) upgradeWebSocketH3(options *WebSocketUpgradeOptions) (*ws.Conn, error) {
	if c.h3Req == nil {
		return nil, errors.New("server: missing HTTP/3 request")
	}

	if c.h3Req.Method != "CONNECT" || c.h3Req.Protocol != "websocket" {
		return nil, errors.New("server: HTTP/3 request is not an Extended CONNECT websocket request (RFC 9220)")
	}

	if c.h3Req.Stream == nil {
		return nil, errors.New("server: HTTP/3 stream is nil")
	}

	if ver := c.h3Req.Headers.Get("sec-websocket-version"); ver != "" && ver != "13" {
		return nil, ws.ErrUnsupportedVersion
	}

	var selectedProto string
	if len(options.Subprotocols) > 0 {
		clientProtos := c.h3Req.Headers.Get("sec-websocket-protocol")
		for _, sp := range options.Subprotocols {
			if ws.HeaderContainsToken(clientProtos, sp) {
				selectedProto = sp
				break
			}
		}
	}

	resHdrs := headkit.NewWithCapacity(4)
	if selectedProto != "" {
		resHdrs.Set("sec-websocket-protocol", selectedProto)
	}

	if c.h3Req.UpgradeFn != nil {
		if err := c.h3Req.UpgradeFn(http.StatusOK, resHdrs); err != nil {
			return nil, err
		}
	}

	wsConn := ws.NewConn(c.h3Req.Stream, true)
	if selectedProto != "" {
		wsConn.SetSubprotocol(selectedProto)
	}

	return wsConn, nil
}
