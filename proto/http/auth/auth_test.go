// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth_test

import (
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/http/auth"
)

func TestBasicAuth_RFC7617(t *testing.T) {
	t.Parallel()

	formatted := auth.FormatBasic("aladdin", "opensesame")
	assert.Equal(t, "Basic YWxhZGRpbjpvcGVuc2VzYW1l", formatted)

	u, p, err := auth.ParseBasic(formatted)
	assert.NoError(t, err)
	assert.Equal(t, "aladdin", u)
	assert.Equal(t, "opensesame", p)

	ch, ok := auth.ParseBasicChallenge(`Basic realm="WallyWorld", charset="UTF-8"`)
	assert.True(t, ok)
	assert.Equal(t, "WallyWorld", ch.Realm)
	assert.Equal(t, "UTF-8", ch.Charset)
	assert.Equal(t, `Basic realm="WallyWorld", charset="UTF-8"`, ch.String())

	assert.True(t, auth.InScope("https://example.com/api/v1/users", "https://example.com/api"))
	assert.False(t, auth.InScope("https://other.com/api/v1", "https://example.com/api"))
}

func TestBearerAuth_RFC6750(t *testing.T) {
	t.Parallel()

	token := "mF_9.B5f-4.1JqM"
	assert.True(t, auth.IsValidBearerToken(token))

	formatted := auth.FormatBearer(token)
	assert.Equal(t, "Bearer mF_9.B5f-4.1JqM", formatted)

	parsed, ok := auth.ParseBearer(formatted)
	assert.True(t, ok)
	assert.Equal(t, token, parsed)

	ch, ok := auth.ParseBearerChallenge(
		`Bearer realm="example", error="invalid_token", error_description="The access token expired"`,
	)
	require.True(t, ok)
	assert.Equal(t, "example", ch.Realm)
	assert.Equal(t, "invalid_token", ch.Error)
	assert.Equal(t, "The access token expired", ch.ErrorDescription)
}
