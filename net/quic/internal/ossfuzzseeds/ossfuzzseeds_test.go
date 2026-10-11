// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ossfuzzseeds

import (
	"errors"
	"os"
	"testing"
)

func TestCorpusEntryFixedArgs(t *testing.T) {
	tests := []struct {
		name string
		args []any
	}{
		{"bool-true", []any{true}},
		{"bool-false", []any{false}},
		{"int", []any{int(42)}},
		{"int8", []any{int8(-5)}},
		{"int16", []any{int16(1000)}},
		{"int32", []any{int32(100000)}},
		{"int64", []any{int64(10000000000)}},
		{"uint", []any{uint(42)}},
		{"uint8", []any{uint8(255)}},
		{"uint16", []any{uint16(65535)}},
		{"uint32", []any{uint32(4000000)}},
		{"uint64", []any{uint64(90000000000)}},
		{"float32", []any{float32(3.14)}},
		{"float64", []any{float64(2.718281828)}},
		{"mixed-fixed", []any{true, uint16(1234), int64(-5678)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := CorpusEntry(tt.args...)
			if err != nil {
				t.Fatalf("CorpusEntry failed: %v", err)
			}
			if len(b) == 0 {
				t.Fatal("expected non-empty corpus entry bytes")
			}
		})
	}
}

func TestCorpusEntryDynamicArgs(t *testing.T) {
	b1, err := CorpusEntry([]byte("hello world"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(b1) == 0 {
		t.Fatal("expected non-empty byte slice")
	}

	b2, err := CorpusEntry("quic-packet-seed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(b2) == 0 {
		t.Fatal("expected non-empty string entry")
	}

	b3, err := CorpusEntry([]byte("first"), "second", []byte("third"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(b3) == 0 {
		t.Fatal("expected non-empty multi-dynamic entry")
	}
}

func TestCorpusEntryUnsupportedType(t *testing.T) {
	type custom struct{}
	_, err := CorpusEntry(custom{})
	if err == nil {
		t.Fatal("expected error for unsupported struct type, got nil")
	}
}

func TestCorpusEntryUnencodableDynamicArgs(t *testing.T) {
	d1 := make([]byte, 1)
	d2 := make([]byte, 1)
	d3 := make([]byte, 100000)
	_, err := CorpusEntry(d1, d2, d3)
	if !errors.Is(err, ErrUnencodableDynamicCorpusArgs) {
		t.Fatalf("expected ErrUnencodableDynamicCorpusArgs, got %v", err)
	}
}

func TestSha256Name(t *testing.T) {
	data := []byte("test seed data")
	name1 := sha256Name(data)
	name2 := sha256Name(data)

	if name1 != name2 {
		t.Fatalf("sha256Name is not deterministic: %s != %s", name1, name2)
	}
	if len(name1) != 16 {
		t.Fatalf("expected sha256Name to be 16 hex chars, got len=%d (%s)", len(name1), name1)
	}
}

func TestHelperDisabled(t *testing.T) {
	oldDir := os.Getenv("FUZZ_CORPUS_DIR")
	defer func() {
		if oldDir != "" {
			_ = os.Setenv("FUZZ_CORPUS_DIR", oldDir)
		} else {
			_ = os.Unsetenv("FUZZ_CORPUS_DIR")
		}
	}()
	_ = os.Unsetenv("FUZZ_CORPUS_DIR")

	h := New(nil)
	if h.enabled {
		t.Fatal("expected helper to be disabled when FUZZ_CORPUS_DIR is unset")
	}
}
