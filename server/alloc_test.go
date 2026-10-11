// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"

	"github.com/lemon4ksan/mach/proto/headkit"
	"github.com/lemon4ksan/mach/server"
	"github.com/lemon4ksan/mach/server/h1"
	"github.com/lemon4ksan/mach/server/h2"
	"github.com/lemon4ksan/mach/server/h3"
)

func TestContextPoolAllocations(t *testing.T) {
	s := server.New()

	h1Req := &h1.Request{
		Method:  "GET",
		Path:    "/api/resource",
		Query:   "key=value",
		Headers: headkit.NewWithCapacity(8),
	}
	h1Res := &h1.Response{
		Headers: headkit.NewWithCapacity(8),
	}

	allocs := testing.AllocsPerRun(1000, func() {
		ctx := s.NewTestContextH1(h1Req, h1Res)
		_ = ctx.Method()
		_ = ctx.Path()
		_ = ctx.Query()
		_ = ctx.Header("Host")
		ctx.SetStatusCode(200)
		ctx.SetHeader("X-Server", "mach")
		ctx.SetBodyString("hello")
		s.ReleaseTestContext(ctx)
	})

	assert.Equal(t, 0.0, allocs, "Context lifecycle must have 0 allocations on hot path")
}

func TestContextH2Allocations(t *testing.T) {
	s := server.New()

	h2Req := &h2.ServerRequest{
		Method:    "POST",
		Path:      "/upload?chunk=1",
		Scheme:    "https",
		Authority: "localhost:8443",
		Headers:   make(http.Header),
		Body:      []byte("chunk-data"),
		Ctx:       context.Background(),
	}
	h2Res := &h2.ServerResponse{
		Headers: make(http.Header),
	}

	allocs := testing.AllocsPerRun(1000, func() {
		ctx := s.NewTestContextH2(h2Req, h2Res)
		_ = ctx.Method()
		_ = ctx.Path()
		_ = ctx.Query()
		_ = ctx.Scheme()
		_ = ctx.Host()
		ctx.SetStatusCode(200)
		ctx.SetHeader("Content-Type", "application/json")
		ctx.SetBody(h2Req.Body)
		s.ReleaseTestContext(ctx)
	})

	assert.Equal(t, 0.0, allocs, "H2 Context lifecycle must have 0 allocations on hot path")
}

func TestContextH3Allocations(t *testing.T) {
	s := server.New()

	h3Req := &h3.ServerRequest{
		Method:    "GET",
		Path:      "/stream/data?id=123",
		Scheme:    "https",
		Authority: "localhost:443",
		Headers:   headkit.NewWithCapacity(8),
		Ctx:       context.Background(),
	}
	h3Res := &h3.ServerResponse{
		Headers: headkit.NewWithCapacity(8),
	}

	allocs := testing.AllocsPerRun(1000, func() {
		ctx := s.NewTestContextH3(h3Req, h3Res)
		_ = ctx.Method()
		_ = ctx.Path()
		_ = ctx.Query()
		ctx.SetStatusCode(200)
		ctx.SetHeader("Alt-Svc", "clear")
		ctx.SetBodyString("h3-response")
		s.ReleaseTestContext(ctx)
	})

	assert.Equal(t, 0.0, allocs, "H3 Context lifecycle must have 0 allocations on hot path")
}

func BenchmarkContextLifecycle_H1(b *testing.B) {
	s := server.New()
	h1Req := &h1.Request{
		Method:  "GET",
		Path:    "/api/resource",
		Query:   "key=value",
		Headers: headkit.NewWithCapacity(8),
	}
	h1Res := &h1.Response{
		Headers: headkit.NewWithCapacity(8),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := s.NewTestContextH1(h1Req, h1Res)
		_ = ctx.Method()
		_ = ctx.Path()
		_ = ctx.Query()
		_ = ctx.Header("Host")
		ctx.SetStatusCode(200)
		ctx.SetHeader("X-Server", "mach")
		ctx.SetBodyString("hello")
		s.ReleaseTestContext(ctx)
	}
}

func BenchmarkContextLifecycle_H2(b *testing.B) {
	s := server.New()
	h2Req := &h2.ServerRequest{
		Method:    "POST",
		Path:      "/upload?chunk=1",
		Scheme:    "https",
		Authority: "localhost:8443",
		Headers:   make(http.Header),
		Body:      []byte("chunk-data"),
		Ctx:       context.Background(),
	}
	h2Res := &h2.ServerResponse{
		Headers: make(http.Header),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := s.NewTestContextH2(h2Req, h2Res)
		_ = ctx.Method()
		_ = ctx.Path()
		_ = ctx.Query()
		_ = ctx.Scheme()
		_ = ctx.Host()
		ctx.SetStatusCode(200)
		ctx.SetHeader("Content-Type", "application/json")
		ctx.SetBody(h2Req.Body)
		s.ReleaseTestContext(ctx)
	}
}

func BenchmarkContextLifecycle_H3(b *testing.B) {
	s := server.New()
	h3Req := &h3.ServerRequest{
		Method:    "GET",
		Path:      "/stream/data?id=123",
		Scheme:    "https",
		Authority: "localhost:443",
		Headers:   headkit.NewWithCapacity(8),
		Ctx:       context.Background(),
	}
	h3Res := &h3.ServerResponse{
		Headers: headkit.NewWithCapacity(8),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := s.NewTestContextH3(h3Req, h3Res)
		_ = ctx.Method()
		_ = ctx.Path()
		_ = ctx.Query()
		ctx.SetStatusCode(200)
		ctx.SetHeader("Alt-Svc", "clear")
		ctx.SetBodyString("h3-response")
		s.ReleaseTestContext(ctx)
	}
}

