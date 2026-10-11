// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stackless

// NewFunc returns a direct zero-alloc wrapper for the function f.
//
// In modern Go 1.27 with foundation concurrency and optimized stack management,
// direct execution eliminates worker goroutine sprawl, channel allocations,
// and context-switching overhead.
func NewFunc(f func(ctx any)) func(ctx any) bool {
	if f == nil {
		panic("BUG: f cannot be nil")
	}

	return func(ctx any) bool {
		f(ctx)
		return true
	}
}
