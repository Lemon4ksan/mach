// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"math"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// TEST(IndexConversions, EncoderStreamRelativeIndex)
func TestIndexConversions_EncoderStreamRelativeIndex(t *testing.T) {
	testData := []struct {
		relativeIndex         uint64
		insertedEntryCount    uint64
		expectedAbsoluteIndex uint64
	}{
		{0, 1, 0},
		{0, 2, 1},
		{1, 2, 0},
		{0, 10, 9},
		{5, 10, 4},
		{9, 10, 0},
	}

	for _, tc := range testData {
		absIndex, ok := EncoderStreamRelativeIndexToAbsoluteIndex(tc.relativeIndex, tc.insertedEntryCount)
		require.True(t, ok)
		assert.Equal(t, tc.expectedAbsoluteIndex, absIndex)

		relIndex := AbsoluteIndexToEncoderStreamRelativeIndex(absIndex, tc.insertedEntryCount)
		assert.Equal(t, tc.relativeIndex, relIndex)
	}
}

// TEST(IndexConversions, RequestStreamRelativeIndex)
func TestIndexConversions_RequestStreamRelativeIndex(t *testing.T) {
	testData := []struct {
		relativeIndex         uint64
		base                  uint64
		expectedAbsoluteIndex uint64
	}{
		{0, 1, 0},
		{0, 2, 1},
		{1, 2, 0},
		{0, 10, 9},
		{5, 10, 4},
		{9, 10, 0},
	}

	for _, tc := range testData {
		absIndex, ok := RequestStreamRelativeIndexToAbsoluteIndex(tc.relativeIndex, tc.base)
		require.True(t, ok)
		assert.Equal(t, tc.expectedAbsoluteIndex, absIndex)

		relIndex := AbsoluteIndexToRequestStreamRelativeIndex(absIndex, tc.base)
		assert.Equal(t, tc.relativeIndex, relIndex)
	}
}

// TEST(IndexConversions, PostBaseIndex)
func TestIndexConversions_PostBaseIndex(t *testing.T) {
	testData := []struct {
		postBaseIndex         uint64
		base                  uint64
		expectedAbsoluteIndex uint64
	}{
		{0, 1, 1},
		{1, 0, 1},
		{2, 0, 2},
		{1, 1, 2},
		{0, 2, 2},
		{1, 2, 3},
	}

	for _, tc := range testData {
		absIndex, ok := PostBaseIndexToAbsoluteIndex(tc.postBaseIndex, tc.base)
		require.True(t, ok)
		assert.Equal(t, tc.expectedAbsoluteIndex, absIndex)

		postBase := AbsoluteIndexToPostBaseIndex(absIndex, tc.base)
		assert.Equal(t, tc.postBaseIndex, postBase)
	}
}

// TEST(IndexConversions, EncoderStreamRelativeIndexUnderflow)
func TestIndexConversions_EncoderStreamRelativeIndexUnderflow(t *testing.T) {
	_, ok1 := EncoderStreamRelativeIndexToAbsoluteIndex(10, 10)
	assert.False(t, ok1)

	_, ok2 := EncoderStreamRelativeIndexToAbsoluteIndex(12, 10)
	assert.False(t, ok2)
}

// TEST(IndexConversions, RequestStreamRelativeIndexUnderflow)
func TestIndexConversions_RequestStreamRelativeIndexUnderflow(t *testing.T) {
	_, ok1 := RequestStreamRelativeIndexToAbsoluteIndex(10, 10)
	assert.False(t, ok1)

	_, ok2 := RequestStreamRelativeIndexToAbsoluteIndex(12, 10)
	assert.False(t, ok2)
}

// TEST(IndexConversions, PostBaseIndexToAbsoluteIndexOverflow)
func TestIndexConversions_PostBaseIndexOverflow(t *testing.T) {
	_, ok := PostBaseIndexToAbsoluteIndex(20, math.MaxUint64-10)
	assert.False(t, ok)
}
