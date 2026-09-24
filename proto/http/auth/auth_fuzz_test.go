// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth_test

import (
	"strings"
	"testing"

	"github.com/lemon4ksan/mach/proto/http/auth"
)

func FuzzBasicAuth(f *testing.F) {
	f.Add("admin", "secret123")
	f.Add("user_name", "pass")
	f.Add("", "")
	f.Add("alice", "p@ssword!#$")

	f.Fuzz(func(t *testing.T, user, pass string) {
		hdr := auth.FormatBasic(user, pass)
		u, p, err := auth.ParseBasic(hdr)

		// RFC 7617: username cannot contain colon, neither can contain CRLF or null
		if !strings.Contains(user, ":") && !strings.ContainsAny(user+pass, "\r\n\x00") {
			if err != nil {
				t.Fatalf("failed to parse formatted Basic auth header: %s", hdr)
			}
			if u != user || p != pass {
				t.Fatalf("Basic auth mismatch: got (%q, %q), want (%q, %q)", u, p, user, pass)
			}
		}
	})
}

func FuzzBearerAuth(f *testing.F) {
	f.Add("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")
	f.Add("secret-token-12345")
	f.Add("valid_token.123~abc+")

	f.Fuzz(func(t *testing.T, token string) {
		hdr := auth.FormatBearer(token)
		tok, ok := auth.ParseBearer(hdr)

		if auth.IsValidBearerToken(token) {
			if !ok || tok != token {
				t.Fatalf("Bearer auth mismatch: got (%q, %v), want %q", tok, ok, token)
			}
		}
	})
}
