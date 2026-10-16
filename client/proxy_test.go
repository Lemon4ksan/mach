// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client_test

import (
	"net/url"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/client"
	"github.com/lemon4ksan/mach/net/proxy"
)

func TestWithProxy_Resolution(t *testing.T) {
	t.Parallel()

	// 1. Direct default
	dDefault := client.ResolveProxy()
	assert.Equal(t, proxy.Direct, dDefault)

	// 2. Resolve from SOCKS5 string
	d1 := client.ResolveProxy(client.WithProxy("socks5://127.0.0.1:1080"))
	require.NotNil(t, d1)
	_, ok := d1.(*proxy.SOCKS5Dialer)
	assert.True(t, ok)

	// 3. Resolve from HTTP CONNECT string
	d2 := client.ResolveProxy(client.WithProxy("http://127.0.0.1:8080"))
	require.NotNil(t, d2)
	_, ok = d2.(*proxy.HTTPConnectDialer)
	assert.True(t, ok)

	// 4. Resolve from *url.URL
	u, err := url.Parse("https://proxy.example.com:8443")
	require.NoError(t, err)
	d3 := client.ResolveProxy(client.WithProxy(u))
	require.NotNil(t, d3)
	hd, ok := d3.(*proxy.HTTPConnectDialer)
	require.True(t, ok)
	require.NotNil(t, hd.TLSConfig)
	assert.Equal(t, "proxy.example.com", hd.TLSConfig.ServerName)

	// 5. Resolve from explicit proxy.Dialer
	pDialer, err := proxy.SOCKS5("tcp", "127.0.0.1:9050", nil, nil)
	require.NoError(t, err)
	d4 := client.ResolveProxy(client.WithProxy(pDialer))
	assert.Equal(t, pDialer, d4)
}
