// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package digest_test

import (
	"net/http"
	"testing"

	"github.com/lemon4ksan/mach/proto/semantic/digest"
)

func TestAlloc(t *testing.T) {
	tr := &digest.Transport{
		Username:  "Mufasa",
		Password:  "CircleOfLife",
		Transport: http.DefaultTransport,
	}

	allocs := testing.AllocsPerRun(1000, func() {
		_ = tr.Unwrap()
	})
	if allocs != 0 {
		t.Errorf("expected 0 allocs, got %v", allocs)
	}

	challengeStr := `Digest realm="http-auth@example.org", qop="auth, auth-int", algorithm=SHA-256, nonce="7yp转/近代...", opaque="fqh/转..."`
	challengeBytes := []byte(challengeStr)

	t.Run("ParseChallengeInto_String", func(t *testing.T) {
		var dst digest.Challenge
		allocs := testing.AllocsPerRun(1000, func() {
			if err := digest.ParseChallengeInto(&dst, challengeStr); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("ParseChallengeInto_Bytes", func(t *testing.T) {
		var dst digest.Challenge
		allocs := testing.AllocsPerRun(1000, func() {
			if err := digest.ParseChallengeInto(&dst, challengeBytes); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	credStr := `Digest username="Mufasa", realm="http-auth@example.org", uri="/dir/index.html", algorithm=SHA-256, nonce="7yp转/近代...", nc=00000001, cnonce="f2n983nc3", qop=auth, response="753927...", opaque="fqh/转..."`
	credBytes := []byte(credStr)

	t.Run("ParseCredentialsInto_String", func(t *testing.T) {
		var dst digest.Credentials
		allocs := testing.AllocsPerRun(1000, func() {
			if err := digest.ParseCredentialsInto(&dst, credStr); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("ParseCredentialsInto_Bytes", func(t *testing.T) {
		var dst digest.Credentials
		allocs := testing.AllocsPerRun(1000, func() {
			if err := digest.ParseCredentialsInto(&dst, credBytes); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	authInfoStr := `nextnonce="d248451fc358f4c199c52b6d610541e5", qop=auth, rspauth="2058f7d0f34f7d96416db24c7bdf32b8", cnonce="0a4f113b", nc=00000001`
	authInfoBytes := []byte(authInfoStr)

	t.Run("ParseAuthenticationInfoInto_String", func(t *testing.T) {
		var dst digest.AuthenticationInfo
		allocs := testing.AllocsPerRun(1000, func() {
			if err := digest.ParseAuthenticationInfoInto(&dst, authInfoStr); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("ParseAuthenticationInfoInto_Bytes", func(t *testing.T) {
		var dst digest.AuthenticationInfo
		allocs := testing.AllocsPerRun(1000, func() {
			if err := digest.ParseAuthenticationInfoInto(&dst, authInfoBytes); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})
}
