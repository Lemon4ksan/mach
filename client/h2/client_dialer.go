// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"context"
	"crypto/tls"
	"net"
	"net/url"
	"time"

	"github.com/lemon4ksan/foundation/generic"

	"github.com/lemon4ksan/mach/net/proxy"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

// Dialer establishes outbound HTTP/2 TLS connections using custom network dialers or proxy tunnels
// and performs ALPN protocol negotiation ("h2") per RFC 9113 §3.3.
type Dialer struct {
	Addr           string
	TLSConfig      *tls.Config
	PingInterval   time.Duration
	ProxyDialer    proxy.Dialer
	NetDial        func(addr string) (net.Conn, error)
	RawDial        func(addr string) (net.Conn, error)
	RawDialContext func(ctx context.Context, addr string) (net.Conn, error)
}

// DialerOption configures an HTTP/2 Dialer instance.
type DialerOption func(*Dialer)

// WithProxy configures the HTTP/2 Dialer to route connections through a proxy.
// Accepts proxy.Dialer, *url.URL, proxy URL string, or client.ProxyOption.
func WithProxy(p any) DialerOption {
	return func(d *Dialer) {
		switch v := p.(type) {
		case proxy.Dialer:
			d.ProxyDialer = v
		case *url.URL:
			pd, err := proxy.FromURL(v, proxy.Direct)
			if err == nil {
				d.ProxyDialer = pd
			}
		case string:
			u, err := proxy.Parse(v)
			if err == nil {
				pd, err := proxy.FromURL(u, proxy.Direct)
				if err == nil {
					d.ProxyDialer = pd
				}
			}

		case proxy.Option:
			cfg := &proxy.Config{Dialer: proxy.Direct}
			v(cfg)
			d.ProxyDialer = cfg.Dialer
		}
	}
}

// WithTLSConfig configures the TLS settings on the Dialer.
func WithTLSConfig(conf *tls.Config) DialerOption {
	return func(d *Dialer) {
		d.TLSConfig = conf
	}
}

// WithPingInterval configures periodic PING keepalive interval.
func WithPingInterval(interval time.Duration) DialerOption {
	return func(d *Dialer) {
		d.PingInterval = interval
	}
}

// NewDialer creates an HTTP/2 Dialer targeting addr, configured with the provided options.
// Accepts DialerOption or proxy.Option.
func NewDialer(addr string, opts ...any) *Dialer {
	d := &Dialer{Addr: addr}
	for _, opt := range opts {
		switch v := opt.(type) {
		case DialerOption:
			if v != nil {
				v(d)
			}

		case proxy.Option:
			if v != nil {
				cfg := &proxy.Config{Dialer: proxy.Direct}
				v(cfg)
				d.ProxyDialer = cfg.Dialer
			}

		case func(*Dialer):
			if v != nil {
				v(d)
			}
		}
	}

	return d
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

	var tlsConf *tls.Config
	if d.TLSConfig != nil {
		tlsConf = d.TLSConfig.Clone()
	} else {
		tlsConf = &tls.Config{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS13,
		}
	}

	if tlsConf.ServerName == "" {
		host, _, err := net.SplitHostPort(d.Addr)
		tlsConf.ServerName = generic.Ternary(err == nil, host, d.Addr)
	}

	if !generic.Contains(tlsConf.NextProtos, "h2") {
		tlsConf.NextProtos = append(tlsConf.NextProtos, "h2")
	}

	var (
		c   net.Conn
		err error
	)

	switch {
	case d.ProxyDialer != nil:
		if cd, ok := d.ProxyDialer.(proxy.ContextDialer); ok {
			c, err = cd.DialContext(ctx, "tcp", d.Addr)
		} else {
			c, err = d.ProxyDialer.Dial("tcp", d.Addr)
		}

	case d.NetDial != nil:
		c, err = d.NetDial(d.Addr)
	default:
		var dialer net.Dialer

		c, err = dialer.DialContext(ctx, "tcp", d.Addr)
	}

	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(c, tlsConf)
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
