// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/mach/proto/h1"
)

func TestVectorTrimSpace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"   ", ""},
		{"\t\t\t", ""},
		{"  \t \t  ", ""},
		{"hello", "hello"},
		{"   hello", "hello"},
		{"hello   ", "hello"},
		{"   hello   ", "hello"},
		{"\t\tContent-Type: text/plain\t ", "Content-Type: text/plain"},
		{"                                a                                ", "a"},
	}

	for _, tc := range tests {
		got := string(h1.TrimSpace([]byte(tc.input)))
		if got != tc.want {
			t.Errorf("TrimSpace(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestVectorIndexColon(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", -1},
		{"Host: localhost", 4},
		{":authority", 0},
		{"Content-Length: 100", 14},
		{"No colon here", -1},
	}

	for _, tc := range tests {
		got := h1.IndexColon([]byte(tc.input))
		if got != tc.want {
			t.Errorf("IndexColon(%q) = %d; want %d", tc.input, got, tc.want)
		}
	}
}

func TestVectorHexUint(t *testing.T) {
	var buf [16]byte
	n := h1.FormatHexUint(&buf, 0x1A2B)
	if string(buf[:n]) != "1a2b" {
		t.Fatalf("FormatHexUint want 1a2b, got %s", string(buf[:n]))
	}

	val, consumed, err := h1.ParseHexUint([]byte("1a2b"))
	if err != nil || val != 0x1a2b || consumed != 4 {
		t.Fatalf("ParseHexUint want (0x1a2b, 4, nil), got (%x, %d, %v)", val, consumed, err)
	}
}

func TestVectorDecodeArgAppend(t *testing.T) {
	src := []byte("a+b%20c%21%2B")
	dst := h1.DecodeArgAppend(nil, src)
	want := "a b c!+"
	if string(dst) != want {
		t.Fatalf("DecodeArgAppend = %q; want %q", string(dst), want)
	}

	dstNoPlus := h1.DecodeArgAppendNoPlus(nil, src)
	wantNoPlus := "a+b c!+"
	if string(dstNoPlus) != wantNoPlus {
		t.Fatalf("DecodeArgAppendNoPlus = %q; want %q", string(dstNoPlus), wantNoPlus)
	}

	invalidHex := []byte("%2G%G2%2")
	dstInv := h1.DecodeArgAppend(nil, invalidHex)
	if !bytes.Equal(dstInv, invalidHex) {
		t.Fatalf("DecodeArgAppend invalid hex = %q; want %q", string(dstInv), string(invalidHex))
	}
}

func TestVectorZeroAllocs(t *testing.T) {
	var buf [16]byte
	allocs := testing.AllocsPerRun(100, func() {
		_ = h1.FormatHexUint(&buf, 0x1A2B3C)
		_, _, _ = h1.ParseHexUint([]byte("1a2b3c"))
		_ = h1.TrimSpace([]byte("   Content-Type: text/plain   "))
		_ = h1.IndexColon([]byte("Content-Type: text/plain"))
	})
	if allocs != 0 {
		t.Fatalf("expected 0 allocs, got %v", allocs)
	}
}

func BenchmarkVectorFormatHexUint(b *testing.B) {
	var buf [16]byte
	b.ReportAllocs()
	for b.Loop() {
		_ = h1.FormatHexUint(&buf, 0x1A2B3C)
	}
}

func BenchmarkVectorParseHexUint(b *testing.B) {
	src := []byte("1a2b3c")
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = h1.ParseHexUint(src)
	}
}

func BenchmarkVectorTrimSpace(b *testing.B) {
	src := []byte("   Content-Type: application/json; charset=utf-8   ")
	b.ReportAllocs()
	for b.Loop() {
		_ = h1.TrimSpace(src)
	}
}

func BenchmarkVectorDecodeArgAppend(b *testing.B) {
	src := []byte("query=hello+world%20test%21&action=search%26destroy&filter=active")
	dst := make([]byte, 0, len(src))
	b.ReportAllocs()
	for b.Loop() {
		_ = h1.DecodeArgAppend(dst[:0], src)
	}
}
