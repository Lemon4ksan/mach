// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"testing"
)

func BenchmarkArgsPeek(b *testing.B) {
	args := AcquireArgs()
	defer ReleaseArgs(args)
	args.Set("key1", "val1")
	args.Set("key2", "val2")
	key := []byte("key1")

	b.ReportAllocs()
	for b.Loop() {
		_ = args.PeekBytes(key)
	}
}

func BenchmarkArgsAdd(b *testing.B) {
	args := AcquireArgs()
	defer ReleaseArgs(args)
	key := []byte("key")
	val := []byte("val")
	args.AddBytesKV(key, val) // warm up capacity

	b.ReportAllocs()
	for b.Loop() {
		args.Reset()
		args.AddBytesKV(key, val)
	}
}

func BenchmarkArgsParse(b *testing.B) {
	query := []byte("foo=bar&baz=12345&action=test&status=active&lang=golang")
	args := AcquireArgs()
	defer ReleaseArgs(args)
	args.ParseBytes(query) // warm up capacity

	b.ReportAllocs()
	for b.Loop() {
		args.Reset()
		args.ParseBytes(query)
	}
}

func BenchmarkCookieParseBytes(b *testing.B) {
	c := AcquireCookie()
	defer ReleaseCookie(c)
	raw := []byte(
		"session_id=abcdef1234567890; Domain=example.com; Path=/api; Max-Age=3600; Secure; HttpOnly; SameSite=Strict",
	)
	_ = c.ParseBytes(raw) // warm up buffers

	b.ReportAllocs()
	for b.Loop() {
		if err := c.ParseBytes(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCookieAppendBytes(b *testing.B) {
	c := AcquireCookie()
	defer ReleaseCookie(c)
	c.SetKey("session_id")
	c.SetValue("secret_token_123456789")
	c.SetDomain("example.com")
	c.SetPath("/api")
	c.SetMaxAge(3600)
	c.SetSecure(true)
	c.SetHTTPOnly(true)
	c.SetSameSite(CookieSameSiteLaxMode)

	dst := make([]byte, 0, 256)
	b.ReportAllocs()
	for b.Loop() {
		_ = c.AppendBytes(dst[:0])
	}
}

func BenchmarkURIParse(b *testing.B) {
	u := AcquireURI()
	defer ReleaseURI(u)
	raw := []byte("https://example.com:8443/api/v1/resource?id=123&action=update#details")
	_ = u.Parse(nil, raw) // warm up buffers

	b.ReportAllocs()
	for b.Loop() {
		if err := u.Parse(nil, raw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNormalizePath(b *testing.B) {
	src := []byte("/foo/./bar/../baz/qux//test")
	dst := make([]byte, 0, len(src)+1)

	b.ReportAllocs()
	for b.Loop() {
		_ = NormalizePath(dst, src)
	}
}

func BenchmarkAppendUint(b *testing.B) {
	dst := make([]byte, 0, 32)

	b.ReportAllocs()
	for b.Loop() {
		_ = AppendUint(dst[:0], 123456789)
	}
}

func BenchmarkCaseInsensitiveCompare(b *testing.B) {
	a := []byte("Content-Type")
	c := []byte("content-type")

	b.ReportAllocs()
	for b.Loop() {
		_ = CaseInsensitiveCompare(a, c)
	}
}

func BenchmarkNormalizeHeaderKey(b *testing.B) {
	header := []byte("content-type")
	b.ReportAllocs()
	for b.Loop() {
		NormalizeHeaderKey(header, false)
	}
}

func BenchmarkFormatHexUint(b *testing.B) {
	var buf [16]byte
	b.ReportAllocs()
	for b.Loop() {
		_ = FormatHexUint(&buf, 0x1A2B3C)
	}
}
