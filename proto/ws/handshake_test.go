// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/lemon4ksan/mach/proto/ws"
)

func TestHandshake_ComputeAccept(t *testing.T) {
	// RFC 6455 §1.3 example
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	expected := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got := ws.ComputeAccept(key); got != expected {
		t.Fatalf("ComputeAccept(%q) = %q; want %q", key, got, expected)
	}

	// AppendAccept
	var buf []byte
	buf = ws.AppendAccept(buf, key)
	if string(buf) != expected {
		t.Fatalf("AppendAccept(%q) = %q; want %q", key, string(buf), expected)
	}
}

func TestHandshake_ValidateKey(t *testing.T) {
	validKeys := []string{
		"dGhlIHNhbXBsZSBub25jZQ==",
		"AQIDBAUGBwgJCgsMDQ4PEA==",
		"/////////////////////w==",
		"AAAAAAAAAAAAAAAAAAAAAA==",
	}
	for _, k := range validKeys {
		if !ws.ValidateKey(k) {
			t.Fatalf("ValidateKey(%q) = false; want true", k)
		}
	}

	invalidKeys := []string{
		"",                          // empty
		"short",                     // too short
		"dGhlIHNhbXBsZSBub25jZQ=",   // 23 chars
		"dGhlIHNhbXBsZSBub25jZQ===", // 25 chars
		"dGhlIHNhbXBsZSBub25jZQAA",  // no '==' padding
		"dGhlIHNhbXBsZSBub25jZQ=!",  // invalid character
		"!@#$%^&*()_+{}[]:;<>?,./=", // total garbage
	}
	for _, k := range invalidKeys {
		if ws.ValidateKey(k) {
			t.Fatalf("ValidateKey(%q) = true; want false", k)
		}
	}
}

func TestHandshake_GenerateKey_And_MaskKey(t *testing.T) {
	seenKeys := make(map[string]bool)
	for i := 0; i < 100; i++ {
		key := ws.GenerateKey()
		if len(key) != 24 {
			t.Fatalf("GenerateKey length = %d; want 24", len(key))
		}
		if !ws.ValidateKey(key) {
			t.Fatalf("GenerateKey generated invalid key: %q", key)
		}
		if seenKeys[key] {
			t.Fatalf("GenerateKey generated duplicate key: %q", key)
		}
		seenKeys[key] = true
	}

	var zeroKey ws.MaskKey
	mkey := ws.GenerateMaskKey()
	if mkey == zeroKey {
		// Extremely improbable to be all zeros
		t.Log("Warning: MaskKey was all zeros")
	}
}

func TestHandshake_HasToken(t *testing.T) {
	tests := []struct {
		header string
		token  string
		want   bool
	}{
		{"Upgrade", "upgrade", true},
		{"keep-alive, Upgrade", "upgrade", true},
		{"Upgrade, keep-alive", "upgrade", true},
		{"keep-alive, upgrade, something-else", "upgrade", true},
		{"websocket", "websocket", true},
		{"WEBSOCKET", "websocket", true},
		{"web socket", "websocket", false},
		{"", "websocket", false},
		{"foo, bar, baz", "websocket", false},
	}

	for _, tc := range tests {
		if got := ws.HasToken(tc.header, tc.token); got != tc.want {
			t.Fatalf("HasToken(%q, %q) = %v; want %v", tc.header, tc.token, got, tc.want)
		}
	}
}

func TestHandshake_ValidateHandshakeRequest(t *testing.T) {
	const validKey = "dGhlIHNhbXBsZSBub25jZQ=="

	// Success case
	err := ws.ValidateHandshakeRequest("GET", "HTTP/1.1", "websocket", "Upgrade", validKey, "13")
	if err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}

	// Mixed case and multi-token
	err = ws.ValidateHandshakeRequest("GET", "HTTP/1.1", "foo, WEBSOCKET", "keep-alive, upgrade", validKey, "13")
	if err != nil {
		t.Fatalf("expected valid request with multi-token headers, got %v", err)
	}

	// Invalid method
	if !errors.Is(
		ws.ValidateHandshakeRequest("POST", "HTTP/1.1", "websocket", "Upgrade", validKey, "13"),
		ws.ErrInvalidMethod,
	) {
		t.Fatal("expected ErrInvalidMethod")
	}

	// Invalid Upgrade
	if !errors.Is(
		ws.ValidateHandshakeRequest("GET", "HTTP/1.1", "h2c", "Upgrade", validKey, "13"),
		ws.ErrInvalidUpgrade,
	) {
		t.Fatal("expected ErrInvalidUpgrade")
	}

	// Invalid Connection
	if !errors.Is(
		ws.ValidateHandshakeRequest("GET", "HTTP/1.1", "websocket", "close", validKey, "13"),
		ws.ErrInvalidConnection,
	) {
		t.Fatal("expected ErrInvalidConnection")
	}

	// Invalid Key
	if !errors.Is(
		ws.ValidateHandshakeRequest("GET", "HTTP/1.1", "websocket", "Upgrade", "badkey", "13"),
		ws.ErrInvalidKey,
	) {
		t.Fatal("expected ErrInvalidKey")
	}

	// Invalid Version
	if !errors.Is(
		ws.ValidateHandshakeRequest("GET", "HTTP/1.1", "websocket", "Upgrade", validKey, "8"),
		ws.ErrInvalidVersion,
	) {
		t.Fatal("expected ErrInvalidVersion")
	}
}

func TestHandshake_FormatHandshakeResponse(t *testing.T) {
	const key = "dGhlIHNhbXBsZSBub25jZQ=="
	var buf []byte
	resp, err := ws.FormatHandshakeResponse(buf, key)
	if err != nil {
		t.Fatalf("FormatHandshakeResponse error: %v", err)
	}

	s := string(resp)
	if !strings.HasPrefix(s, "HTTP/1.1 101 Switching Protocols\r\n") {
		t.Fatalf("response missing status line: %q", s)
	}
	if !strings.Contains(s, "Upgrade: websocket\r\n") {
		t.Fatalf("response missing Upgrade header: %q", s)
	}
	if !strings.Contains(s, "Connection: Upgrade\r\n") {
		t.Fatalf("response missing Connection header: %q", s)
	}
	if !strings.Contains(s, "Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n\r\n") {
		t.Fatalf("response missing Sec-WebSocket-Accept: %q", s)
	}

	// Invalid key
	_, err = ws.FormatHandshakeResponse(buf, "invalid-key")
	if !errors.Is(err, ws.ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}
}
