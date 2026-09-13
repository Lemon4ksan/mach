// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2_test

import (
	"testing"

	"github.com/lemon4ksan/mach/core/h2"
)

// BenchmarkAcquireRelease_SyncPool vs ConnectionFramePool for the 4 POD frame types.

func BenchmarkAcquireRelease_SyncPool_Ping(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		fr := h2.AcquireFrame(h2.FramePing)
		h2.ReleaseFrame(fr)
	}
}

func BenchmarkAcquireRelease_ConnPool_Ping(b *testing.B) {
	pool := h2.NewConnectionFramePool(1024)
	defer pool.Release()

	b.ReportAllocs()

	for b.Loop() {
		fr := pool.AcquireFrame(h2.FramePing)
		pool.ReleaseFrame(fr)
	}
}

func BenchmarkAcquireRelease_SyncPool_WindowUpdate(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		fr := h2.AcquireFrame(h2.FrameWindowUpdate)
		h2.ReleaseFrame(fr)
	}
}

func BenchmarkAcquireRelease_ConnPool_WindowUpdate(b *testing.B) {
	pool := h2.NewConnectionFramePool(1024)
	defer pool.Release()

	b.ReportAllocs()

	for b.Loop() {
		fr := pool.AcquireFrame(h2.FrameWindowUpdate)
		pool.ReleaseFrame(fr)
	}
}

func BenchmarkAcquireRelease_SyncPool_RstStream(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		fr := h2.AcquireFrame(h2.FrameResetStream)
		h2.ReleaseFrame(fr)
	}
}

func BenchmarkAcquireRelease_ConnPool_RstStream(b *testing.B) {
	pool := h2.NewConnectionFramePool(1024)
	defer pool.Release()

	b.ReportAllocs()

	for b.Loop() {
		fr := pool.AcquireFrame(h2.FrameResetStream)
		pool.ReleaseFrame(fr)
	}
}

// BenchmarkAcquireRelease_PerGoroutinePool - the correct usage pattern:
// each goroutine owns its own pool (simulates H2 read/write loop).
func BenchmarkAcquireRelease_PerGoroutinePool_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		pool := h2.NewConnectionFramePool(1024)
		defer pool.Release()

		for pb.Next() {
			fr := pool.AcquireFrame(h2.FramePing)
			pool.ReleaseFrame(fr)
		}
	})
}
