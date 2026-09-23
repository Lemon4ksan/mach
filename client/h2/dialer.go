// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

// Dialer establishes outbound HTTP/2 TLS connections using custom network dialers
// and performs ALPN protocol negotiation ("h2") per RFC 9113 §3.3.
type Dialer struct {
	Addr           string
	TLSConfig      *tls.Config
	PingInterval   time.Duration
	NetDial        func(addr string) (net.Conn, error)
	RawDial        func(addr string) (net.Conn, error)
	RawDialContext func(ctx context.Context, addr string) (net.Conn, error)
}

// Dial establishes an HTTP/2 TLS connection to Dialer.Addr and performs the initial handshake.
func (d *Dialer) Dial(opts ConnOpts) (*Conn, error) {
	return d.DialContext(context.Background(), opts)
}

// DialContext establishes an HTTP/2 TLS connection with context cancellation support.
func (d *Dialer) DialContext(ctx context.Context, opts ConnOpts) (*Conn, error) {
	c, err := d.tryDial(ctx)
	if err != nil {
		return nil, err
	}

	nc := NewConn(c, opts)
	err = nc.Handshake()

	return nc, err
}

func (d *Dialer) tryDial(ctx context.Context) (net.Conn, error) {
	if d.RawDialContext != nil {
		return d.RawDialContext(ctx, d.Addr)
	}

	if d.RawDial != nil {
		return d.RawDial(d.Addr)
	}

	if d.TLSConfig == nil {
		d.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS13,
		}
	}

	if d.TLSConfig.ServerName == "" {
		host, _, err := net.SplitHostPort(d.Addr)
		if err != nil {
			host = d.Addr
		}

		d.TLSConfig.ServerName = host
	}

	d.TLSConfig.NextProtos = append(d.TLSConfig.NextProtos, "h2")

	var (
		c   net.Conn
		err error
	)

	if d.NetDial != nil {
		c, err = d.NetDial(d.Addr)
	} else {
		var dialer net.Dialer

		c, err = dialer.DialContext(ctx, "tcp", d.Addr)
	}

	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(c, d.TLSConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}

	if tlsConn.ConnectionState().NegotiatedProtocol != "h2" {
		_ = c.Close()
		return nil, coreh2.ErrServerSupport
	}

	return tlsConn, nil
}
