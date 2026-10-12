// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"net/url"

	"github.com/lemon4ksan/foundation/generic"
)

// Option configures proxy settings across client transports (H1, H2, H3).
type Option func(*Config)

// Config holds normalized proxy configuration.
type Config struct {
	Dialer Dialer
}

// WithProxy returns an Option that routes client traffic through the specified proxy.
//
// The proxy parameter accepts:
//   - Dialer or ContextDialer (e.g. *HTTPConnectDialer, *SOCKS5Dialer)
//   - *url.URL (e.g. parsed from "socks5://127.0.0.1:1080" or "http://127.0.0.1:8080")
//   - string containing a proxy URL or address
func WithProxy(p any) Option {
	return func(cfg *Config) {
		switch v := p.(type) {
		case Dialer:
			cfg.Dialer = v
		case *url.URL:
			d, err := FromURL(v, Direct)
			if err == nil {
				cfg.Dialer = d
			}
		case string:
			u, err := Parse(v)
			if err == nil {
				d, err := FromURL(u, Direct)
				if err == nil {
					cfg.Dialer = d
				}
			}
		}
	}
}

// ResolveProxy resolves a Dialer from one or more Option functions.
// If no proxy option is provided or configured, it defaults to Direct.
func ResolveProxy(opts ...Option) Dialer {
	var cfg Config
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return generic.CoalesceNil[Dialer](cfg.Dialer, Direct)
}
