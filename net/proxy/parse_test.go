// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy_test

import (
	"testing"

	"github.com/lemon4ksan/mach/net/proxy"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

func TestParseProxy(t *testing.T) {
	t.Parallel()

	// Scheme provided
	u, err := proxy.Parse("socks5://127.0.0.1:1080")
	require.NoError(t, err)
	assert.Equal(t, "socks5", u.Scheme)
	assert.Equal(t, "127.0.0.1:1080", u.Host)

	// Schemeless with standard SOCKS port
	u, err = proxy.Parse("127.0.0.1:1080")
	require.NoError(t, err)
	assert.Equal(t, "socks5h", u.Scheme)
	assert.Equal(t, "127.0.0.1:1080", u.Host)

	// Schemeless with standard HTTP port
	u, err = proxy.Parse("user:pass@proxy.example.com:8080")
	require.NoError(t, err)
	assert.Equal(t, "http", u.Scheme)
	assert.Equal(t, "proxy.example.com:8080", u.Host)
	assert.Equal(t, "user", u.User.Username())
}
