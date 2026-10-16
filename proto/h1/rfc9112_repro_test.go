// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/lemon4ksan/mach/proto/h1"
)

// TestRepro_ChunkedTrailingWhitespaceWithoutSemicolon demonstrates that chunk-size
// with trailing spaces but without a chunk extension semicolon is illegally accepted (RFC 9112 §7.1).
func TestRepro_ChunkedTrailingWhitespaceWithoutSemicolon(t *testing.T) {
	raw := "5 \r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal(
			"RFC 9112 §7.1 violation: trailing whitespace on chunk-size without extension semicolon should be rejected",
		)
	}
}

// TestRepro_ChunkExtensionSyntaxValidation demonstrates that malformed chunk extensions
// are stripped without syntax validation (RFC 9112 §7.1.1).
func TestRepro_ChunkExtensionSyntaxValidation(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"EmptyExtensionName", "5;\r\nhello\r\n0\r\n\r\n"},
		{"MissingExtensionNameBeforeEquals", "5;=value\r\nhello\r\n0\r\n\r\n"},
		{"SpaceInExtensionName", "5;ext name=val\r\nhello\r\n0\r\n\r\n"},
		{"UnclosedQuotedString", "5;ext=\"unclosed\r\nhello\r\n0\r\n\r\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(tc.raw)))
			_, err := io.ReadAll(cr)
			if err == nil {
				t.Fatalf("RFC 9112 §7.1.1 violation: malformed chunk extension %q must be rejected", tc.raw)
			}
		})
	}
}

// TestRepro_TrailerUnexpectedEOF demonstrates that premature EOF during trailer draining
// incorrectly returns EOF instead of ErrUnexpectedEOF (RFC 9112 §7.1.2).
func TestRepro_TrailerUnexpectedEOF(t *testing.T) {
	// Stream ends abruptly after a trailer line without the terminating CRLF line
	raw := "5\r\nhello\r\n0\r\nX-Trailer: value\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("RFC 9112 §7.1.2 violation: stream truncated during trailers must return io.ErrUnexpectedEOF, got nil")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}

// TestRepro_TrailerBareLF demonstrates that trailer lines with bare LF are accepted (RFC 9112 §2.2 & §7.1.2).
func TestRepro_TrailerBareLF(t *testing.T) {
	raw := "5\r\nhello\r\n0\r\nX-Trailer: value\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("RFC 9112 §2.2 violation: bare LF in trailer line must be rejected")
	}
}

// TestRepro_TrailerForbiddenFields demonstrates that forbidden framing/routing fields in trailers
// are not validated (RFC 9112 §7.1.2).
func TestRepro_TrailerForbiddenFields(t *testing.T) {
	forbidden := []string{
		"5\r\nhello\r\n0\r\nTransfer-Encoding: chunked\r\n\r\n",
		"5\r\nhello\r\n0\r\nContent-Length: 10\r\n\r\n",
		"5\r\nhello\r\n0\r\nHost: evil.com\r\n\r\n",
	}

	for _, raw := range forbidden {
		cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
		_, err := io.ReadAll(cr)
		if err == nil {
			t.Fatalf("RFC 9112 §7.1.2 violation: forbidden trailer in %q must be rejected", raw)
		}
	}
}

// TestRepro_HeaderScanner_NumericKey demonstrates that header keys starting with non-ASCII-letter
// (such as digits or symbols) are mishandled by HeaderScanner.
func TestRepro_HeaderScanner_NumericKey(t *testing.T) {
	raw := []byte("2FA-Code: 123456\r\nHost: example.com\r\n\r\n")
	s := h1.NewHeaderScanner(raw)
	if !s.Next() {
		t.Fatalf("expected first header, got err: %v", s.Err())
	}
	if string(s.Key()) != "2FA-Code" {
		t.Fatalf("expected key 2FA-Code, got %q", string(s.Key()))
	}
	if string(s.Value()) != "123456" {
		t.Fatalf("expected val 123456, got %q", string(s.Value()))
	}
	if !s.Next() {
		t.Fatalf("expected second header, got err: %v", s.Err())
	}
	if string(s.Key()) != "Host" {
		t.Fatalf("expected key Host, got %q", string(s.Key()))
	}
}
