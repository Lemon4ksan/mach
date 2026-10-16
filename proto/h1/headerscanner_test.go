// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/mach/proto/h1"
)

var rawHeaderBlock = []byte("Date: Mon, 24 Aug 2026 12:00:00 GMT\r\n" +
	"Content-Type: application/json; charset=utf-8\r\n" +
	"Content-Length: 1024\r\n" +
	"Server: aoni-silicon/1.0\r\n" +
	"Connection: keep-alive\r\n" +
	"X-Frame-Options: SAMEORIGIN\r\n" +
	"X-Content-Type-Options: nosniff\r\n" +
	"X-XSS-Protection: 1; mode=block\r\n" +
	"Strict-Transport-Security: max-age=31536000; includeSubDomains; preload\r\n" +
	"Accept-Ranges: bytes\r\n" +
	"Cache-Control: public, max-age=3600, stale-while-revalidate=60\r\n" +
	"ETag: W/\"65a123-bc90f1\"\r\n" +
	"Vary: Accept-Encoding, User-Agent\r\n" +
	"Access-Control-Allow-Origin: *\r\n" +
	"Access-Control-Allow-Methods: GET, POST, OPTIONS, PUT, DELETE\r\n" +
	"Access-Control-Allow-Headers: Content-Type, Authorization, X-Requested-With\r\n\r\n")

func TestHeaderScanner(t *testing.T) {
	s := h1.NewHeaderScanner(rawHeaderBlock)

	count := 0
	for s.Next() {
		count++
		if len(s.Key()) == 0 || len(s.Value()) == 0 {
			t.Fatalf("empty key or value at header %d", count)
		}
	}

	if s.Err() != nil {
		t.Fatalf("scanner error: %v", s.Err())
	}

	if count != 16 {
		t.Fatalf("expected 16 headers, got %d", count)
	}
}

func TestHeaderScanner_LeadingWhitespaceRejection(t *testing.T) {
	badHeader := []byte(" Leading: bad\r\n\r\n")
	s := h1.NewHeaderScanner(badHeader)
	if s.Next() {
		t.Fatal("expected failure on leading whitespace")
	}
	if s.Err() == nil {
		t.Fatal("expected error on leading whitespace")
	}
}

func TestHeaderScanner_LineContinuation(t *testing.T) {
	folded := []byte("Header: line1\r\n line2\r\n\tline3\r\nNext: val\r\n\r\n")
	s := h1.NewHeaderScanner(folded)

	if !s.Next() {
		t.Fatalf("expected first header, got err: %v", s.Err())
	}
	if !bytes.Equal(s.Key(), []byte("Header")) {
		t.Fatalf("expected key Header, got %s", s.Key())
	}
	if !bytes.Equal(s.Value(), []byte("line1 line2 line3")) {
		t.Fatalf("expected folded value 'line1 line2 line3', got %q", s.Value())
	}

	if !s.Next() {
		t.Fatalf("expected next header, got err: %v", s.Err())
	}
	if !bytes.Equal(s.Key(), []byte("Next")) {
		t.Fatalf("expected key Next, got %s", s.Key())
	}

	if s.Next() {
		t.Fatal("expected no more headers")
	}
}

func TestHeaderScannerZeroAllocs(t *testing.T) {
	var s h1.HeaderScanner
	allocs := testing.AllocsPerRun(100, func() {
		s.Reset(rawHeaderBlock)
		for s.Next() {
			_ = s.Key()
			_ = s.Value()
		}
	})
	if allocs != 0 {
		t.Fatalf("expected 0 allocs, got %v", allocs)
	}
}

func BenchmarkHeaderScanner(b *testing.B) {
	b.SetBytes(int64(len(rawHeaderBlock)))
	b.ReportAllocs()

	var s h1.HeaderScanner
	for b.Loop() {
		s.Reset(rawHeaderBlock)
		for s.Next() {
			if len(s.Key()) == 0 {
				b.Fatal("empty key")
			}
		}
	}
}
