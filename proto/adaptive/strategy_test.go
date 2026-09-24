// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package adaptive_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/adaptive"
	"github.com/lemon4ksan/foundation/testing/assert"
)

func TestAdaptiveStrategy_Increments(t *testing.T) {
	strategy := adaptive.New()
	assert.Equal(t, 8192, strategy.Next())

	// Grows if record == next
	strategy.Record(8192)
	assert.Equal(t, 16384, strategy.Next())

	strategy.Record(16384)
	assert.Equal(t, 32768, strategy.Next())

	strategy.Record(32768)
	assert.Equal(t, 65536, strategy.Next())

	strategy.Record(100000)
	assert.Equal(t, 131072, strategy.Next())

	// Stays clamped at max
	strategy.Record(131072)
	assert.Equal(t, 131072, strategy.Next())
}

func TestAdaptiveStrategy_DecrementsHysteresis(t *testing.T) {
	strategy := adaptive.New()
	strategy.Record(8192)
	assert.Equal(t, 16384, strategy.Next())

	// First small read does not decrement immediately (hysteresis)
	strategy.Record(1)
	assert.Equal(t, 16384, strategy.Next(), "first smaller record doesn't decrement yet")

	// Read in range resets hysteresis
	strategy.Record(8192)
	assert.Equal(t, 16384, strategy.Next(), "record was within range")

	// First small read again
	strategy.Record(1)
	assert.Equal(t, 16384, strategy.Next(), "first smaller record again")

	// Second consecutive small read decrements
	strategy.Record(1)
	assert.Equal(t, 8192, strategy.Next(), "second smaller record decrements")

	// Does not decrement below init size
	strategy.Record(1)
	strategy.Record(1)
	assert.Equal(t, 8192, strategy.Next(), "doesn't decrement under minimum")
}

func TestAdaptiveStrategy_Reset(t *testing.T) {
	strategy := adaptive.New()
	strategy.Record(8192)
	strategy.Record(16384)
	assert.Equal(t, 32768, strategy.Next())

	strategy.Reset()
	assert.Equal(t, 8192, strategy.Next())
}

func BenchmarkAdaptiveStrategy_Record(b *testing.B) {
	strategy := adaptive.New()
	b.ReportAllocs()

	for b.Loop() {
		strategy.Record(8192)
		strategy.Record(1)
	}
}
