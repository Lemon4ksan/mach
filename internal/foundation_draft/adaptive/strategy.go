// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package adaptive provides an adaptive read buffer sizing strategy with hysteresis decay.
package adaptive

import (
	"math/bits"
)

const (
	// DefaultInitBufferSize is the initial buffer size allocated before trying to read from IO.
	DefaultInitBufferSize = 8192

	// DefaultMaxBufferSize is the default maximum read buffer size (128 KB).
	DefaultMaxBufferSize = 131072
)

// Strategy provides an adaptive buffer sizing strategy ported from hyper's ReadStrategy::Adaptive.
//
// It grows by powers of two when reads fill the current buffer, and uses a two-step
// hysteresis mechanism before decrementing to prevent memory thrashing under bursty traffic.
type Strategy struct {
	next        int
	max         int
	initSize    int
	decreaseNow bool
}

// New creates a new adaptive Strategy with default parameters (8KB init, 128KB max).
func New() Strategy {
	return WithLimits(DefaultInitBufferSize, DefaultMaxBufferSize)
}

// WithLimits creates an adaptive Strategy with custom initial and maximum limits.
func WithLimits(initSize, maxSize int) Strategy {
	if initSize < 512 {
		initSize = 512
	}

	if maxSize < initSize {
		maxSize = initSize
	}

	return Strategy{
		next:        initSize,
		max:         maxSize,
		initSize:    initSize,
		decreaseNow: false,
	}
}

// Next returns the buffer size to allocate for the upcoming read operation.
//
//go:inline
func (s *Strategy) Next() int {
	return s.next
}

// Max returns the upper limit for buffer allocation.
//
//go:inline
func (s *Strategy) Max() int {
	return s.max
}

// Record updates the strategy state with the number of bytes read in the last operation.
func (s *Strategy) Record(bytesRead int) {
	if bytesRead >= s.next {
		// Read filled or exceeded current buffer size -> double the buffer size up to max.
		s.next = min(incrPowerOfTwo(s.next), s.max)
		s.decreaseNow = false
		return
	}

	decrTo := prevPowerOfTwo(s.next)
	if bytesRead < decrTo {
		if s.decreaseNow {
			// Second consecutive smaller read -> step down buffer size.
			s.next = max(decrTo, s.initSize)
			s.decreaseNow = false
		} else {
			// First smaller read -> set decreaseNow flag (two-step hysteresis).
			s.decreaseNow = true
		}
	} else {
		// A read within the current step range cancels any pending decrease.
		s.decreaseNow = false
	}
}

// Reset resets the adaptive strategy back to its initial buffer size.
func (s *Strategy) Reset() {
	s.next = s.initSize
	s.decreaseNow = false
}

// incrPowerOfTwo returns the next power of two (or doubles the size).
//
//go:inline
func incrPowerOfTwo(n int) int {
	if n <= 0 {
		return DefaultInitBufferSize
	}

	next := n << 1
	if next < n { // overflow protection
		return n
	}

	return next
}

// prevPowerOfTwo returns the previous power of two for a given size.
//
//go:inline
func prevPowerOfTwo(n int) int {
	if n <= 4 {
		return 1
	}

	// Use leading zeros to calculate the previous power of two with 0 allocations in 1 CPU instruction.
	lz := bits.LeadingZeros64(uint64(n - 1))

	return 1 << (63 - lz)
}
