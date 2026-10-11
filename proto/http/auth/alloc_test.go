// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http/auth"
)

func TestAlloc(t *testing.T) {
	header := "Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ=="

	t.Run("ParseBasicBytes", func(t *testing.T) {
		buf := make([]byte, len(header))
		allocs := testing.AllocsPerRun(1000, func() {
			copy(buf, header)
			user, pass, err := auth.ParseBasicBytes(buf)
			if err != nil || string(user) != "Aladdin" || string(pass) != "open sesame" {
				t.Fatalf("unexpected parse: user=%s pass=%s err=%v", user, pass, err)
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for ParseBasicBytes, got %v", allocs)
		}
	})

	t.Run("ParseBearer", func(t *testing.T) {
		bearerHeader := "Bearer mF_9.B5f-4.1JqM"
		allocs := testing.AllocsPerRun(1000, func() {
			token, ok := auth.ParseBearer(bearerHeader)
			if !ok || token != "mF_9.B5f-4.1JqM" {
				t.Fatalf("unexpected parse: token=%s ok=%v", token, ok)
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for ParseBearer, got %v", allocs)
		}
	})

	t.Run("ParseBearerBytes", func(t *testing.T) {
		bearerHeaderBytes := []byte("Bearer mF_9.B5f-4.1JqM")
		allocs := testing.AllocsPerRun(1000, func() {
			token, ok := auth.ParseBearerBytes(bearerHeaderBytes)
			if !ok || string(token) != "mF_9.B5f-4.1JqM" {
				t.Fatalf("unexpected parse: token=%s ok=%v", token, ok)
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for ParseBearerBytes, got %v", allocs)
		}
	})

	t.Run("IsValidBearerToken", func(t *testing.T) {
		token := "mF_9.B5f-4.1JqM"
		allocs := testing.AllocsPerRun(1000, func() {
			if !auth.IsValidBearerToken(token) {
				t.Fatal("expected valid")
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for IsValidBearerToken, got %v", allocs)
		}
	})

	t.Run("BasicChallengeAppendFormat", func(t *testing.T) {
		ch := auth.BasicChallenge{Realm: "WallyWorld", Charset: "UTF-8"}
		var buf [128]byte
		allocs := testing.AllocsPerRun(1000, func() {
			_ = ch.AppendFormat(buf[:0])
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for BasicChallenge.AppendFormat, got %v", allocs)
		}
	})

	t.Run("BearerChallengeAppendFormat", func(t *testing.T) {
		ch := auth.BearerChallenge{Realm: "example", Error: "invalid_token"}
		var buf [128]byte
		allocs := testing.AllocsPerRun(1000, func() {
			_ = ch.AppendFormat(buf[:0])
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for BearerChallenge.AppendFormat, got %v", allocs)
		}
	})
}
