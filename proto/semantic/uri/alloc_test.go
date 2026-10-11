// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package uri_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/semantic/uri"
)

func TestAlloc(t *testing.T) {
	query := []byte("foo=bar&baz=qux&num=12345")
	url := []byte("https://example.com/api/v1/resource?foo=bar#section")

	allocs := testing.AllocsPerRun(1000, func() {
		args := uri.AcquireArgs()
		uri.Parse(args, query)
		_ = args.Peek("foo")
		uri.ReleaseArgs(args)

		u := uri.AcquireURI()
		u.Parse(nil, url)
		_ = u.Path()
		_ = u.QueryString()
		uri.ReleaseURI(u)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on uri hot path, got %v", allocs)
	}
}

func TestDecodeArgAppend_ZeroAlloc(t *testing.T) {
	dst := make([]byte, 0, 64)
	src := []byte("hello%20world%21%2Bmore%2Fbytes")

	allocs := testing.AllocsPerRun(1000, func() {
		_ = uri.DecodeArgAppend(dst[:0], src)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on DecodeArgAppend, got %v", allocs)
	}
}

func TestParse_ZeroAlloc(t *testing.T) {
	t.Run("GenericString", func(t *testing.T) {
		query := "foo=bar&baz=qux&num=12345"
		allocs := testing.AllocsPerRun(1000, func() {
			args := uri.AcquireArgs()
			uri.Parse(args, query)
			_ = args.Peek("foo")
			uri.ReleaseArgs(args)
		})
		if allocs > 0 {
			t.Fatalf("expected 0 allocations on Parse string, got %v", allocs)
		}
	})

	t.Run("GenericBytes", func(t *testing.T) {
		query := []byte("foo=bar&baz=qux&num=12345")
		allocs := testing.AllocsPerRun(1000, func() {
			args := uri.AcquireArgs()
			uri.Parse(args, query)
			_ = args.Peek("foo")
			uri.ReleaseArgs(args)
		})
		if allocs > 0 {
			t.Fatalf("expected 0 allocations on Parse bytes, got %v", allocs)
		}
	})

	t.Run("ParseArgsLegacy", func(t *testing.T) {
		query := "foo=bar&baz=qux&num=12345"
		allocs := testing.AllocsPerRun(1000, func() {
			args := uri.AcquireArgs()
			uri.ParseArgs(args, query)
			_ = args.Peek("foo")
			uri.ReleaseArgs(args)
		})
		if allocs > 0 {
			t.Fatalf("expected 0 allocations on ParseArgs legacy, got %v", allocs)
		}
	})
}

func TestStandaloneArgs_ZeroAlloc(t *testing.T) {
	kvs := []uri.ArgsKV{
		{Key: []byte("alpha"), Value: []byte("1")},
		{Key: []byte("beta"), Value: []byte("2")},
		{Key: []byte("gamma"), Value: []byte("3")},
	}

	keyStr := "beta"
	keyBytes := []byte("beta")
	valStr := "2_updated"
	valBytes := []byte("2_updated")

	t.Run("PeekArg_String", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = uri.PeekArg(kvs, keyStr)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("PeekArg_Bytes", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = uri.PeekArg(kvs, keyBytes)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("HasArg_String", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = uri.HasArg(kvs, keyStr)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("HasArg_Bytes", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = uri.HasArg(kvs, keyBytes)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("SetArg_String_Existing", func(t *testing.T) {
		slice := make([]uri.ArgsKV, len(kvs))
		copy(slice, kvs)
		allocs := testing.AllocsPerRun(1000, func() {
			_ = uri.SetArg(slice, keyStr, valStr, false)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("SetArg_Bytes_Existing", func(t *testing.T) {
		slice := make([]uri.ArgsKV, len(kvs))
		copy(slice, kvs)
		allocs := testing.AllocsPerRun(1000, func() {
			_ = uri.SetArg(slice, keyBytes, valBytes, false)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("DelAllArgs_String", func(t *testing.T) {
		slice := make([]uri.ArgsKV, len(kvs))
		allocs := testing.AllocsPerRun(1000, func() {
			copy(slice, kvs)
			_ = uri.DelAllArgs(slice, keyStr)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("DelAllArgs_Bytes", func(t *testing.T) {
		slice := make([]uri.ArgsKV, len(kvs))
		allocs := testing.AllocsPerRun(1000, func() {
			copy(slice, kvs)
			_ = uri.DelAllArgs(slice, keyBytes)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("DelAllArgsStable_String", func(t *testing.T) {
		slice := make([]uri.ArgsKV, len(kvs))
		allocs := testing.AllocsPerRun(1000, func() {
			copy(slice, kvs)
			_ = uri.DelAllArgsStable(slice, keyStr)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("DelAllArgsStable_Bytes", func(t *testing.T) {
		slice := make([]uri.ArgsKV, len(kvs))
		allocs := testing.AllocsPerRun(1000, func() {
			copy(slice, kvs)
			_ = uri.DelAllArgsStable(slice, keyBytes)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})
}

