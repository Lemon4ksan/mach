// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/url"
	"slices"

	"github.com/lemon4ksan/mach/net/proxy"
	"github.com/lemon4ksan/mach/net/quic"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

// Dialer establishes HTTP/3 client sessions over direct QUIC or proxy tunnels.
type Dialer struct {
	TLSConfig   *tls.Config
	Settings    *coreh3.Settings
	ProxyDialer proxy.Dialer
	PacketConn  net.PacketConn
}

// DialerOption configures an HTTP/3 Dialer instance.
type DialerOption func(*Dialer)

// WithProxy configures the HTTP/3 Dialer to route QUIC traffic through a proxy.
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

// WithTLSConfig sets the TLS configuration for the QUIC session.
func WithTLSConfig(conf *tls.Config) DialerOption {
	return func(d *Dialer) {
		d.TLSConfig = conf
	}
}

// WithSettings sets the HTTP/3 protocol settings.
func WithSettings(s *coreh3.Settings) DialerOption {
	return func(d *Dialer) {
		d.Settings = s
	}
}

// NewDialer creates an HTTP/3 Dialer configured with the provided options.
// Accepts DialerOption or proxy.Option.
func NewDialer(opts ...any) *Dialer {
	d := &Dialer{}
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

// Dial establishes an HTTP/3 ClientConn to addr (RFC 9114 §3).
func (d *Dialer) Dial(ctx context.Context, addr string, quicOpts ...quic.Option) (*ClientConn, error) {
	var tlsConf *tls.Config
	if d.TLSConfig != nil {
		tlsConf = d.TLSConfig.Clone()
	} else {
		tlsConf = &tls.Config{
			NextProtos: []string{"h3"},
		}
	}

	if tlsConf.ServerName == "" {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}

		tlsConf.ServerName = host
	}

	if !slices.Contains(tlsConf.NextProtos, "h3") {
		tlsConf.NextProtos = append(tlsConf.NextProtos, "h3")
	}

	var (
		qconn  *quic.Conn
		closer io.Closer
		err    error
	)

	switch {
	case d.ProxyDialer != nil:
		pd, ok := d.ProxyDialer.(interface {
			DialPacketContext(context.Context, string, string) (net.PacketConn, error)
		})
		if !ok {
			return nil, errors.New("h3: proxy dialer does not support packet dialing")
		}

		var (
			pconn   net.PacketConn
			udpAddr *net.UDPAddr
		)

		pconn, err = pd.DialPacketContext(ctx, "udp", addr)
		if err != nil {
			return nil, err
		}

		udpAddr, err = net.ResolveUDPAddr("udp", addr)
		if err != nil {
			_ = pconn.Close()

			return nil, err
		}

		qconn, err = quic.Dial(ctx, pconn, udpAddr, tlsConf, quicOpts...)
		if err != nil {
			_ = pconn.Close()

			return nil, err
		}

		closer = pconn

	case d.PacketConn != nil:
		var udpAddr *net.UDPAddr

		udpAddr, err = net.ResolveUDPAddr("udp", addr)
		if err != nil {
			return nil, err
		}

		qconn, err = quic.Dial(ctx, d.PacketConn, udpAddr, tlsConf, quicOpts...)

	default:
		qconn, err = quic.DialAddr(ctx, addr, tlsConf, quicOpts...)
	}

	if err != nil {
		return nil, err
	}

	cc, err := NewClientConn(qconn, d.Settings)
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}

		return nil, err
	}

	if closer != nil {
		cc.UnderlyingCloser = closer
	}

	return cc, nil
}
