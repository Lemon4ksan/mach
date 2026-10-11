// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"context"
	"net"

	"github.com/lemon4ksan/mach/net/proxy/socks"
)

// SOCKS5Dialer is a high-performance, zero-allocation SOCKS5 proxy dialer (RFC 1928).
type SOCKS5Dialer = socks.Dialer

// NewSOCKS5Dialer returns an initialized SOCKS5 dialer for the target proxy address (RFC 1928, RFC 1929).
func NewSOCKS5Dialer(network, address string, auth *Auth, forward Dialer) *SOCKS5Dialer {
	d := socks.NewDialer(network, address)
	if forward != nil {
		if f, ok := forward.(ContextDialer); ok {
			d.ProxyDial = func(ctx context.Context, network, address string) (net.Conn, error) {
				return f.DialContext(ctx, network, address)
			}
		} else {
			d.ProxyDial = func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialContext(ctx, forward, network, address)
			}
		}
	}
	if auth != nil {
		up := socks.UsernamePassword{
			Username: auth.User,
			Password: auth.Password,
		}
		d.AuthMethods = []socks.AuthMethod{
			socks.AuthMethodNotRequired,
			socks.AuthMethodUsernamePassword,
		}
		d.Authenticate = up.Authenticate
	}
	return d
}

// SOCKS5 returns a Dialer that makes SOCKSv5 connections to the given
// address with an optional username and password.
// See RFC 1928 and RFC 1929.
func SOCKS5(network, address string, auth *Auth, forward Dialer) (Dialer, error) {
	return NewSOCKS5Dialer(network, address, auth, forward), nil
}
