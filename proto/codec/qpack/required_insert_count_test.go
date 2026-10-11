// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"fmt"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// TEST(RequiredInsertCountTest, EncodeRequiredInsertCount)
func TestQpackRequiredInsertCount_EncodeRequiredInsertCount(t *testing.T) {
	assert.Equal(t, uint64(0), EncodeRequiredInsertCount(0, 0))
	assert.Equal(t, uint64(0), EncodeRequiredInsertCount(0, 8))
	assert.Equal(t, uint64(0), EncodeRequiredInsertCount(0, 1024))

	assert.Equal(t, uint64(2), EncodeRequiredInsertCount(1, 8))
	assert.Equal(t, uint64(5), EncodeRequiredInsertCount(20, 8))
	assert.Equal(t, uint64(7), EncodeRequiredInsertCount(106, 10))
}

// TEST(RequiredInsertCountTest, DecodeRequiredInsertCount)
func TestQpackRequiredInsertCount_DecodeRequiredInsertCount(t *testing.T) {
	testData := []struct {
		requiredInsertCount  uint64
		maxEntries           uint64
		totalNumberOfInserts uint64
	}{
		// Maximum dynamic table capacity is zero.
		{0, 0, 0},
		// No dynamic entries in header.
		{0, 100, 0},
		{0, 100, 500},
		// Required Insert Count has not wrapped around yet, no entries evicted.
		{15, 100, 25},
		{20, 100, 10},
		// Required Insert Count has not wrapped around yet, some entries evicted.
		{90, 100, 110},
		// Required Insert Count has wrapped around.
		{234, 100, 180},
		// Required Insert Count has wrapped around many times.
		{5678, 100, 5701},
		// Lowest and highest possible Required Insert Count values
		// for given MaxEntries and total number of insertions.
		{401, 100, 500},
		{600, 100, 500},
	}

	for i, tc := range testData {
		if tc.requiredInsertCount != 0 {
			// Dynamic entries cannot be referenced if dynamic table capacity is zero.
			require.True(t, tc.maxEntries > 0, fmt.Sprintf("case %d: maxEntries must be > 0", i))
			// Entry totalNumberOfInserts - 1 - maxEntries and earlier entries
			// are evicted. Entry requiredInsertCount - 1 is referenced.
			// No evicted entry can be referenced.
			require.True(t, tc.totalNumberOfInserts < tc.requiredInsertCount+tc.maxEntries,
				fmt.Sprintf("case %d: totalNumberOfInserts < requiredInsertCount + maxEntries", i))
			// Every evicted entry must be acknowledged.
			require.True(t, tc.requiredInsertCount <= tc.totalNumberOfInserts+tc.maxEntries,
				fmt.Sprintf("case %d: requiredInsertCount <= totalNumberOfInserts + maxEntries", i))
		}

		encoded := EncodeRequiredInsertCount(tc.requiredInsertCount, tc.maxEntries)
		decoded, ok := DecodeRequiredInsertCount(encoded, tc.maxEntries, tc.totalNumberOfInserts)
		require.True(t, ok, fmt.Sprintf("case %d: decoding must succeed", i))
		assert.Equal(t, tc.requiredInsertCount, decoded, fmt.Sprintf("case %d: decoded value mismatch", i))
	}
}

// TEST(RequiredInsertCountTest, DecodeRequiredInsertCountError)
func TestRequiredInsertCount_DecodeRequiredInsertCountError(t *testing.T) {
	invalidTestData := []struct {
		encodedRequiredInsertCount uint64
		maxEntries                 uint64
		totalNumberOfInserts       uint64
	}{
		// Maximum dynamic table capacity is zero, yet header block
		// claims to have a reference to a dynamic table entry.
		{1, 0, 0},
		{9, 0, 0},
		// Examples from https://github.com/quicwg/base-drafts/issues/2112#issue-389626872.
		{1, 10, 2},
		{18, 10, 2},
		// Encoded Required Insert Count value too small or too large
		// for given MaxEntries and total number of insertions.
		{400, 100, 500},
		{601, 100, 500},
	}

	for i, tc := range invalidTestData {
		_, ok := DecodeRequiredInsertCount(tc.encodedRequiredInsertCount, tc.maxEntries, tc.totalNumberOfInserts)
		assert.False(t, ok, fmt.Sprintf("case %d: expected decode error", i))
	}
}
