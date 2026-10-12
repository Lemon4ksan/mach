// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/lemon4ksan/foundation/generic"
)

var (
	crlfcrlf    = []byte("\r\n\r\n")
	crlf        = []byte("\r\n")
	http1Prefix = []byte("HTTP/1.")

	connectBufPool = generic.NewPool(func() *[]byte {
		b := make([]byte, 1024)
		return &b
	})
)

// HTTPConnectDialer establishes an HTTP CONNECT tunnel to an arbitrary destination
// through an HTTP or HTTPS forward proxy (RFC 9110 §9.3.6, RFC 9112).
//
// Concurrency: Fully thread-safe.
// Allocations: 0 allocs/op during handshake execution when no trailing bytes exist.
type HTTPConnectDialer struct {
	ProxyNetwork string
	ProxyAddress string
	Auth         *Auth
	Forward      Dialer
	TLSConfig    *tls.Config // Non-nil for HTTPS proxies (TLS to proxy)
}

// NewHTTPConnectDialer creates an HTTP CONNECT tunnel dialer targeting the proxy at address.
func NewHTTPConnectDialer(network, address string, auth *Auth, forward Dialer) *HTTPConnectDialer {
	return &HTTPConnectDialer{
		ProxyNetwork: generic.Coalesce(network, "tcp"),
		ProxyAddress: address,
		Auth:         auth,
		Forward:      forward,
	}
}

// HTTPConnect returns a Dialer that establishes HTTP CONNECT tunnels to target addresses (RFC 9110 §9.3.6).
func HTTPConnect(network, address string, auth *Auth, forward Dialer) (Dialer, error) {
	return NewHTTPConnectDialer(network, address, auth, forward), nil
}

// Dial connects to the target address on the given network through the HTTP proxy.
func (d *HTTPConnectDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

// DialContext connects to the target address on the given network through the HTTP proxy
// with context cancellation support.
func (d *HTTPConnectDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, errors.New("proxy: network not implemented: " + network)
	}

	if ctx == nil {
		ctx = context.Background()
	}

	var (
		c   net.Conn
		err error
	)

	if d.Forward != nil {
		if cd, ok := d.Forward.(ContextDialer); ok {
			c, err = cd.DialContext(ctx, d.ProxyNetwork, d.ProxyAddress)
		} else {
			c, err = d.Forward.Dial(d.ProxyNetwork, d.ProxyAddress)
		}
	} else {
		var dialer net.Dialer

		c, err = dialer.DialContext(ctx, d.ProxyNetwork, d.ProxyAddress)
	}

	if err != nil {
		return nil, err
	}

	if d.TLSConfig != nil {
		tlsConn := tls.Client(c, d.TLSConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = c.Close()
			return nil, err
		}

		c = tlsConn
	}

	c, err = d.Handshake(ctx, c, address)
	if err != nil {
		_ = c.Close()
		return nil, err
	}

	return c, nil
}

// Handshake executes the HTTP CONNECT protocol handshake on an existing connection c (RFC 9110 §9.3.6).
// It returns the active net.Conn (which may wrap c in a bufferedConn if trailing bytes were read from the proxy)
// and any error, ensuring zero data loss during subsequent TLS or L7 exchanges (EXP-1203).
//
// When no trailing bytes exist beyond the CONNECT response header, Handshake returns c directly
// with 0 heap allocations.
func (d *HTTPConnectDialer) Handshake(ctx context.Context, c net.Conn, address string) (net.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var stop func() bool
	if ctx.Done() != nil {
		if dl, ok := ctx.Deadline(); ok {
			_ = c.SetDeadline(dl)
		}

		stop = context.AfterFunc(ctx, func() {
			_ = c.SetDeadline(time.Unix(1, 0))
		})
		defer func() {
			stop()

			_ = c.SetDeadline(time.Time{})
		}()
	}

	bufPtr := connectBufPool.Get()
	defer connectBufPool.Put(bufPtr)

	buf := *bufPtr

	// 1. Build CONNECT request
	n := copy(buf, "CONNECT ")
	n += copy(buf[n:], address)
	n += copy(buf[n:], " HTTP/1.1\r\nHost: ")
	n += copy(buf[n:], address)
	n += copy(buf[n:], "\r\nUser-Agent: mach/1.0\r\nProxy-Connection: Keep-Alive\r\n")

	if d.Auth != nil && (d.Auth.User != "" || d.Auth.Password != "") {
		var rawAuth [256]byte

		n += copy(buf[n:], "Proxy-Authorization: Basic ")
		rawLen := copy(rawAuth[:], d.Auth.User)
		rawLen += copy(rawAuth[rawLen:], ":")
		rawLen += copy(rawAuth[rawLen:], d.Auth.Password)

		encLen := base64.StdEncoding.EncodedLen(rawLen)
		if n+encLen+2 <= len(buf) {
			base64.StdEncoding.Encode(buf[n:], rawAuth[:rawLen])
			n += encLen
			n += copy(buf[n:], "\r\n")
		}
	}

	n += copy(buf[n:], "\r\n")

	if _, err := c.Write(buf[:n]); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return c, ctxErr
		}

		return c, err
	}

	// 2. Read CONNECT response headers until crlfcrlf
	readLen := 0
	for {
		nr, err := c.Read(buf[readLen:])
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return c, ctxErr
			}

			return c, err
		}

		readLen += nr

		if idx := bytes.Index(buf[:readLen], crlfcrlf); idx != -1 {
			// Find status line
			lineEnd := bytes.Index(buf[:readLen], crlf)
			if lineEnd == -1 {
				lineEnd = idx
			}

			statusLine := buf[:lineEnd]

			if !bytes.HasPrefix(statusLine, http1Prefix) {
				return c, errors.New("proxy: invalid HTTP CONNECT response: " + string(statusLine))
			}

			// Robust status-line whitespace scanner (RFC 9112 §3.1, EXP-1204)
			// Skip "HTTP/1.x" prefix
			spIdx := 0
			for spIdx < len(statusLine) && statusLine[spIdx] != ' ' && statusLine[spIdx] != '\t' {
				spIdx++
			}

			// Skip one or more whitespace delimiters (SP / HTAB)
			for spIdx < len(statusLine) && (statusLine[spIdx] == ' ' || statusLine[spIdx] == '\t') {
				spIdx++
			}

			if spIdx+3 > len(statusLine) {
				return c, errors.New("proxy: malformed HTTP CONNECT status line")
			}

			d1, d2, d3 := statusLine[spIdx], statusLine[spIdx+1], statusLine[spIdx+2]
			if d1 < '0' || d1 > '9' || d2 < '0' || d2 > '9' || d3 < '0' || d3 > '9' {
				return c, errors.New("proxy: non-numeric HTTP status code")
			}

			statusCode := int(d1-'0')*100 + int(d2-'0')*10 + int(d3-'0')

			if statusCode < 200 || statusCode >= 300 {
				return c, fmt.Errorf(
					"proxy: HTTP CONNECT tunnel failed with status %d: %s",
					statusCode,
					string(statusLine),
				)
			}

			// If extra bytes exist beyond "\r\n\r\n", wrap connection to preserve data (EXP-1203)
			headerEnd := idx + 4
			if readLen > headerEnd {
				extra := make([]byte, readLen-headerEnd)
				copy(extra, buf[headerEnd:readLen])
				return &bufferedConn{Conn: c, buf: extra}, nil
			}

			return c, nil
		}

		if readLen == len(buf) {
			return c, errors.New("proxy: HTTP CONNECT response header exceeded 1KB limit")
		}
	}
}

type bufferedConn struct {
	net.Conn
	buf []byte
	off int
}

func (b *bufferedConn) Read(p []byte) (int, error) {
	if b.off < len(b.buf) {
		n := copy(p, b.buf[b.off:])
		b.off += n

		return n, nil
	}

	return b.Conn.Read(p)
}
