// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy_test

import (
	"bytes"
	"slices"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/proto/http/zerocopy"
)

func TestArgsBasic(t *testing.T) {
	args := zerocopy.AcquireArgs()
	defer zerocopy.ReleaseArgs(args)

	args.Set("foo", "bar")
	args.Add("foo", "baz")
	args.Set("hello", "world")

	if !bytes.Equal(args.Peek("foo"), []byte("bar")) {
		t.Fatalf("args.Peek('foo') = %s, want bar", args.Peek("foo"))
	}

	multi := args.PeekMulti("foo")
	if len(multi) != 2 || !bytes.Equal(multi[0], []byte("bar")) || !bytes.Equal(multi[1], []byte("baz")) {
		t.Fatalf("args.PeekMulti('foo') = %v", multi)
	}

	if args.Len() != 3 {
		t.Fatalf("args.Len() = %d, want 3", args.Len())
	}

	// Test All iterator
	var pairs []string
	for k, v := range args.All() {
		pairs = append(pairs, string(k)+"="+string(v))
	}
	expected := []string{"foo=bar", "foo=baz", "hello=world"}
	if !slices.Equal(pairs, expected) {
		t.Fatalf("args.All() = %v, want %v", pairs, expected)
	}

	// Parse query string
	args.Reset()
	args.Parse("a=1&b=2&c=3%20test")
	if !bytes.Equal(args.Peek("a"), []byte("1")) {
		t.Fatalf("args.Peek('a') = %s, want 1", args.Peek("a"))
	}
	if !bytes.Equal(args.Peek("c"), []byte("3 test")) {
		t.Fatalf("args.Peek('c') = %s, want '3 test'", args.Peek("c"))
	}
}

func TestURIBasic(t *testing.T) {
	uri := zerocopy.AcquireURI()
	defer zerocopy.ReleaseURI(uri)

	raw := []byte("https://example.com:8443/api/v1/resource?id=123#frag")
	if err := uri.Parse(nil, raw); err != nil {
		t.Fatalf("uri.Parse failed: %v", err)
	}

	if !bytes.Equal(uri.Scheme(), []byte("https")) {
		t.Fatalf("Scheme = %s, want https", uri.Scheme())
	}
	if !bytes.Equal(uri.Host(), []byte("example.com:8443")) {
		t.Fatalf("Host = %s, want example.com:8443", uri.Host())
	}
	if !bytes.Equal(uri.Path(), []byte("/api/v1/resource")) {
		t.Fatalf("Path = %s, want /api/v1/resource", uri.Path())
	}
	if !bytes.Equal(uri.QueryString(), []byte("id=123")) {
		t.Fatalf("QueryString = %s, want id=123", uri.QueryString())
	}
	if !bytes.Equal(uri.Hash(), []byte("frag")) {
		t.Fatalf("Hash = %s, want frag", uri.Hash())
	}

	full := uri.FullURI()
	if !bytes.Equal(full, raw) {
		t.Fatalf("FullURI = %s, want %s", full, raw)
	}
}

func TestCookieBasic(t *testing.T) {
	c := zerocopy.AcquireCookie()
	defer zerocopy.ReleaseCookie(c)

	c.SetKey("session")
	c.SetValue("secret123")
	c.SetDomain("example.com")
	c.SetPath("/")
	c.SetExpire(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))
	c.SetSecure(true)
	c.SetHTTPOnly(true)

	if !bytes.Equal(c.Key(), []byte("session")) {
		t.Fatalf("Key = %s, want session", c.Key())
	}
	if !bytes.Equal(c.Value(), []byte("secret123")) {
		t.Fatalf("Value = %s, want secret123", c.Value())
	}

	buf := c.AppendBytes(nil)
	if !bytes.Contains(buf, []byte("session=secret123")) ||
		!bytes.Contains(buf, []byte("domain=example.com")) ||
		!bytes.Contains(buf, []byte("secure")) ||
		!bytes.Contains(buf, []byte("HttpOnly")) {
		t.Fatalf("AppendBytes = %s", buf)
	}
}

func BenchmarkArgsAll(b *testing.B) {
	args := zerocopy.AcquireArgs()
	defer zerocopy.ReleaseArgs(args)

	args.Set("foo", "bar")
	args.Set("hello", "world")
	args.Set("user", "test")
	args.Set("query", "golang")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		count := 0
		for range args.All() {
			count++
		}
		_ = count
	}
}
