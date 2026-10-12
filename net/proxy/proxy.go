// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"os"
	"sync"
)

// A Dialer is a means to establish a connection.
// Custom dialers should also implement ContextDialer.
type Dialer interface {
	// Dial connects to the given address via the proxy.
	Dial(network, addr string) (c net.Conn, err error)
}

// Auth contains authentication parameters that specific Dialers may require.
type Auth struct {
	User, Password string
}

// FromEnvironment returns the dialer specified by the proxy-related
// variables in the environment and makes underlying connections
// directly.
func FromEnvironment() Dialer {
	return FromEnvironmentUsing(Direct)
}

// FromEnvironmentUsing returns the dialer specified by the proxy-related
// variables in the environment and makes underlying connections
// using the provided forwarding Dialer.
func FromEnvironmentUsing(forward Dialer) Dialer {
	allProxy := allProxyEnv.Get()
	if len(allProxy) == 0 {
		return forward
	}

	proxyURL, err := url.Parse(allProxy)
	if err != nil {
		return forward
	}

	proxy, err := FromURL(proxyURL, forward)
	if err != nil {
		return forward
	}

	noProxy := noProxyEnv.Get()
	if len(noProxy) == 0 {
		return proxy
	}

	perHost := NewPerHost(proxy, forward)
	perHost.AddFromString(noProxy)

	return perHost
}

// proxySchemes is a map from URL schemes to a function that creates a Dialer
// from a URL with such a scheme.
var (
	proxyMu      sync.RWMutex
	proxySchemes map[string]func(*url.URL, Dialer) (Dialer, error)
)

// RegisterDialerType takes a URL scheme and a function to generate Dialers from
// a URL with that scheme and a forwarding Dialer. Registered schemes are used
// by FromURL.
func RegisterDialerType(scheme string, f func(*url.URL, Dialer) (Dialer, error)) {
	proxyMu.Lock()
	defer proxyMu.Unlock()

	if proxySchemes == nil {
		proxySchemes = make(map[string]func(*url.URL, Dialer) (Dialer, error))
	}

	proxySchemes[scheme] = f
}

// FromURL returns a Dialer given a URL specification and an underlying
// Dialer for it to make network requests.
func FromURL(u *url.URL, forward Dialer) (Dialer, error) {
	var auth *Auth
	if u.User != nil {
		auth = new(Auth)
		auth.User = u.User.Username()

		if p, ok := u.User.Password(); ok {
			auth.Password = p
		}
	}

	switch u.Scheme {
	case "socks5", "socks5h":
		addr := u.Hostname()
		port := u.Port()

		if port == "" {
			port = "1080"
		}

		return SOCKS5("tcp", net.JoinHostPort(addr, port), auth, forward)

	case "http":
		addr := u.Hostname()
		port := u.Port()

		if port == "" {
			port = "80"
		}

		return HTTPConnect("tcp", net.JoinHostPort(addr, port), auth, forward)

	case "https":
		addr := u.Hostname()
		port := u.Port()

		if port == "" {
			port = "443"
		}

		d := NewHTTPConnectDialer("tcp", net.JoinHostPort(addr, port), auth, forward)
		d.TLSConfig = &tls.Config{ServerName: addr}

		return d, nil
	}

	// If the scheme doesn't match any of the built-in schemes, see if it
	// was registered by another package.
	proxyMu.RLock()
	defer proxyMu.RUnlock()

	if f := proxySchemes[u.Scheme]; f != nil {
		return f(u, forward)
	}

	return nil, errors.New("proxy: unknown scheme: " + u.Scheme)
}

var (
	allProxyEnv = &envOnce{
		names: []string{"ALL_PROXY", "all_proxy", "HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"},
	}
	noProxyEnv = &envOnce{
		names: []string{"NO_PROXY", "no_proxy"},
	}
)

// envOnce looks up an environment variable (optionally by multiple
// names) once.
type envOnce struct {
	names []string
	once  sync.Once
	val   string
}

func (e *envOnce) Get() string {
	e.once.Do(e.init)
	return e.val
}

func (e *envOnce) init() {
	for _, n := range e.names {
		e.val = os.Getenv(n)
		if e.val != "" {
			return
		}
	}
}

// reset is used by tests
func (e *envOnce) reset() {
	e.once = sync.Once{}
	e.val = ""
}
