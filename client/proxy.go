// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"github.com/lemon4ksan/mach/net/proxy"
)

// ProxyOption configures proxy settings across client transports (H1, H2, H3).
type ProxyOption = proxy.Option

// ProxyConfig holds normalized proxy configuration.
type ProxyConfig = proxy.Config

// WithProxy returns a ProxyOption that routes client traffic through the specified proxy.
//
// The proxy parameter accepts:
//   - proxy.Dialer or proxy.ContextDialer (e.g. *proxy.HTTPConnectDialer, *proxy.SOCKS5Dialer)
//   - *url.URL (e.g. parsed from "socks5://127.0.0.1:1080" or "http://127.0.0.1:8080")
//   - string containing a proxy URL or address
var WithProxy = proxy.WithProxy

// ResolveProxy resolves a proxy.Dialer from one or more ProxyOption functions.
// If no proxy option is provided or configured, it defaults to proxy.Direct.
var ResolveProxy = proxy.ResolveProxy
