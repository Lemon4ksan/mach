// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3_test

import (
	"context"
	"crypto/tls"
	"net/url"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/client/h3"
	"github.com/lemon4ksan/mach/net/proxy"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

func TestH3_Dialer_WithProxy_Options(t *testing.T) {
	t.Parallel()

	// 1. With string socks5 URL using h3.WithProxy
	d1 := h3.NewDialer(h3.WithProxy("socks5://127.0.0.1:1080"))
	require.NotNil(t, d1.ProxyDialer)

	// 2. With string http URL using h3.WithProxy
	d2 := h3.NewDialer(h3.WithProxy("http://127.0.0.1:8080"))
	require.NotNil(t, d2.ProxyDialer)
	_, ok := d2.ProxyDialer.(*proxy.HTTPConnectDialer)
	assert.True(t, ok)

	// 3. With explicit proxy.Dialer
	pDialer, err := proxy.SOCKS5("tcp", "127.0.0.1:1080", nil, nil)
	require.NoError(t, err)
	d3 := h3.NewDialer(h3.WithProxy(pDialer))
	assert.Equal(t, pDialer, d3.ProxyDialer)

	// 4. With *url.URL
	u, err := url.Parse("http://proxy.corp:3128")
	require.NoError(t, err)
	d4 := h3.NewDialer(h3.WithProxy(u))
	require.NotNil(t, d4.ProxyDialer)

	// 5. With settings and TLS
	tlsConf := &tls.Config{InsecureSkipVerify: true}
	settings := &coreh3.Settings{MaxFieldSectionSize: 65536}
	d5 := h3.NewDialer(
		h3.WithProxy("socks5://127.0.0.1:1080"),
		h3.WithTLSConfig(tlsConf),
		h3.WithSettings(settings),
	)
	assert.Equal(t, tlsConf, d5.TLSConfig)
	assert.Equal(t, settings, d5.Settings)
	assert.NotNil(t, d5.ProxyDialer)
}

func TestH3_EXP1212_SharedTLSConfigNotMutated(t *testing.T) {
	t.Parallel()

	sharedTLS := &tls.Config{
		InsecureSkipVerify: true,
	}

	d := h3.NewDialer(h3.WithTLSConfig(sharedTLS))

	// Dial with a cancelled context so it aborts early after setup
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, _ = d.Dial(ctx, "alpha.example.com:443")
	_, _ = d.Dial(ctx, "beta.example.com:443")

	// Verify sharedTLS ServerName and NextProtos were not mutated in place
	assert.Equal(t, "", sharedTLS.ServerName, "shared TLSConfig.ServerName must remain untouched")
	assert.Equal(t, 0, len(sharedTLS.NextProtos), "shared TLSConfig.NextProtos must remain untouched")
}

func TestH3_EXP1213_ProxyDialerMustSupportPacketDialing(t *testing.T) {
	t.Parallel()

	// Direct proxy.Direct only implements Dial / DialContext (stream), not DialPacketContext
	d := h3.NewDialer(h3.WithProxy(proxy.Direct))

	ctx := t.Context()
	_, err := d.Dial(ctx, "127.0.0.1:443")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "proxy dialer does not support packet dialing")
}
