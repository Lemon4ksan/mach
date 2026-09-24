// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"bytes"
	"testing"
)

func TestCaseInsensitiveCompare(t *testing.T) {
	tests := []struct {
		a, b     string
		expected bool
	}{
		{"", "", true},
		{"a", "a", true},
		{"a", "A", true},
		{"Content-Type", "content-type", true},
		{"Content-Type", "CONTENT-TYPE", true},
		{"User-Agent", "user-agent", true},
		{"Sec-WebSocket-Key", "sec-websocket-key", true},
		// Length mismatch
		{"abc", "abcd", false},
		{"abcd", "abc", false},
		// Mismatch within first 8 bytes
		{"Content-Type", "Contxnt-Type", false},
		// Mismatch after 8 bytes (trailing bytes)
		{"Content-Encoding", "Content-EncodinX", false},
		{"Content-Encoding", "Content-EncodinG", true},
		// Length exactly 8 bytes
		{"12345678", "12345678", true},
		{"12345678", "12345679", false},
		{"abcdefgh", "ABCDEFGH", true},
		// Length exactly 16 bytes
		{"1234567812345678", "1234567812345678", true},
		{"12345678abcdefgh", "12345678ABCDEFGH", true},
		{"1234567812345678", "1234567812345679", false},
	}

	for _, tc := range tests {
		got := CaseInsensitiveCompare([]byte(tc.a), []byte(tc.b))
		if got != tc.expected {
			t.Fatalf("CaseInsensitiveCompare(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.expected)
		}
	}
}

func TestValidHeaderFieldByte(t *testing.T) {
	// Valid RFC 7230 token characters
	validChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&'*+-.^_`|~"
	for _, c := range []byte(validChars) {
		if !ValidHeaderFieldByte(c) {
			t.Fatalf("expected ValidHeaderFieldByte=true for %c", c)
		}
	}

	// Invalid characters (separators, control chars, non-ASCII)
	invalidChars := " (),/:;<=>?@[\\]{}\t\r\n\x00\x1f\x7f"
	for _, c := range []byte(invalidChars) {
		if ValidHeaderFieldByte(c) {
			t.Fatalf("expected ValidHeaderFieldByte=false for %q", c)
		}
	}
	if ValidHeaderFieldByte(128) || ValidHeaderFieldByte(255) {
		t.Fatal("expected false for >= 128")
	}
}

func TestHeaderKeyHelpersAndNormalization(t *testing.T) {
	// InitHeaderKV
	k, v := InitHeaderKV(nil, nil, "content-type", "application/json\r\n", false)
	if string(k) != "Content-Type" || string(v) != "application/json  " {
		t.Fatalf("InitHeaderKV failed: %q, %q", k, v)
	}

	// InitHeaderKV with disableNormalizing = true
	k2, v2 := InitHeaderKV(nil, nil, "content-type", "text/plain", true)
	if string(k2) != "content-type" || string(v2) != "text/plain" {
		t.Fatalf("InitHeaderKV disabled normalizing failed: %q, %q", k2, v2)
	}

	// GetHeaderKeyBytes
	k3 := GetHeaderKeyBytes(nil, "user-agent", false)
	if string(k3) != "User-Agent" {
		t.Fatalf("GetHeaderKeyBytes failed: %q", k3)
	}

	// NormalizeHeaderKey
	// Disabled
	bufDisabled := []byte("authorization")
	NormalizeHeaderKey(bufDisabled, true)
	if string(bufDisabled) != "authorization" {
		t.Fatalf("NormalizeHeaderKey with disabled normalizing failed: %s", bufDisabled)
	}

	// Empty
	NormalizeHeaderKey(nil, false)
	NormalizeHeaderKeyValidated(nil, false)

	// Valid header key normalization
	headers := []struct {
		input, expected string
	}{
		{"content-type", "Content-Type"},
		{"ACCEPT-ENCODING", "Accept-Encoding"},
		{"x-custom-header-value", "X-Custom-Header-Value"},
		{"host", "Host"},
	}
	for _, h := range headers {
		b := []byte(h.input)
		NormalizeHeaderKey(b, false)
		if string(b) != h.expected {
			t.Fatalf("NormalizeHeaderKey(%q) = %q, want %q", h.input, string(b), h.expected)
		}
	}

	// Key with invalid header byte (e.g. colon or space) left untouched
	invalidKey := []byte("content:type")
	NormalizeHeaderKey(invalidKey, false)
	if string(invalidKey) != "content:type" {
		t.Fatalf("expected invalid key untouched, got %s", invalidKey)
	}

	// NormalizeHeaderKeyValidated disabled
	normVal := []byte("header-key")
	NormalizeHeaderKeyValidated(normVal, true)
	if string(normVal) != "header-key" {
		t.Fatalf("expected untouched when disabled, got %s", normVal)
	}
}

func TestRemoveNewLines(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"clean string", "clean string"},
		{"line1\rline2", "line1 line2"},
		{"line1\nline2", "line1 line2"},
		{"line1\r\nline2", "line1  line2"},
		{"line1\n\rline2", "line1  line2"},
		{"\r\nstart", "  start"},
		{"end\r\n", "end  "},
	}

	for _, tc := range tests {
		got := string(RemoveNewLines([]byte(tc.input)))
		if got != tc.expected {
			t.Fatalf("RemoveNewLines(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}

	// Empty slice
	if len(RemoveNewLines(nil)) != 0 {
		t.Fatal("expected empty result for nil input")
	}

	// In-place modification check
	orig := []byte("a\rb\nc")
	res := RemoveNewLines(orig)
	if !bytes.Equal(res, orig) || string(orig) != "a b c" {
		t.Fatalf("RemoveNewLines did not modify in-place: %q", orig)
	}
}
