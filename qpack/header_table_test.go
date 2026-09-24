// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

const (
	maximumDynamicTableCapacityForTesting uint64 = 1024 * 1024
	staticEntry                           bool   = true
	dynamicEntry                          bool   = false

	kMaximumDynamicTableCapacityForTesting = maximumDynamicTableCapacityForTesting
	kStaticEntry                           = staticEntry
	kDynamicEntry                          = dynamicEntry
)

type qpackHeaderTable interface {
	SetMaximumDynamicTableCapacity(capacity uint64) bool
	SetDynamicTableCapacity(capacity uint64) bool
	EntryFitsDynamicTableCapacity(name, value string) bool
	InsertEntry(name, value string) uint64
	MaxEntries() uint64
	InsertedEntryCount() uint64
	DroppedEntryCount() uint64
}

func forEachHeaderTableType(t *testing.T, runTest func(t *testing.T, createTable func() qpackHeaderTable)) {
	t.Run("EncoderHeaderTable", func(t *testing.T) {
		runTest(t, func() qpackHeaderTable {
			return NewEncoderHeaderTable()
		})
	})
	t.Run("DecoderHeaderTable", func(t *testing.T) {
		runTest(t, func() qpackHeaderTable {
			return NewDecoderHeaderTable()
		})
	})
}

// TYPED_TEST(HeaderTableTest, MaxEntries)
func TestHeaderTable_MaxEntries(t *testing.T) {
	forEachHeaderTableType(t, func(t *testing.T, createTable func() qpackHeaderTable) {
		table1 := createTable()
		require.True(t, table1.SetMaximumDynamicTableCapacity(1024))
		assert.Equal(t, uint64(32), table1.MaxEntries())

		table2 := createTable()
		require.True(t, table2.SetMaximumDynamicTableCapacity(500))
		assert.Equal(t, uint64(15), table2.MaxEntries())
	})
}

// TYPED_TEST(HeaderTableTest, SetDynamicTableCapacity)
func TestHeaderTable_SetDynamicTableCapacity(t *testing.T) {
	forEachHeaderTableType(t, func(t *testing.T, createTable func() qpackHeaderTable) {
		table := createTable()
		require.True(t, table.SetMaximumDynamicTableCapacity(kMaximumDynamicTableCapacityForTesting))
		require.True(t, table.SetDynamicTableCapacity(kMaximumDynamicTableCapacityForTesting))

		// Dynamic table capacity does not affect MaxEntries.
		assert.True(t, table.SetDynamicTableCapacity(1024))
		assert.Equal(t, uint64(32*1024), table.MaxEntries())

		assert.True(t, table.SetDynamicTableCapacity(500))
		assert.Equal(t, uint64(32*1024), table.MaxEntries())

		// Dynamic table capacity cannot exceed maximum dynamic table capacity.
		assert.False(t, table.SetDynamicTableCapacity(2*kMaximumDynamicTableCapacityForTesting))
	})
}

// TYPED_TEST(HeaderTableTest, EntryFitsDynamicTableCapacity)
func TestHeaderTable_EntryFitsDynamicTableCapacity(t *testing.T) {
	forEachHeaderTableType(t, func(t *testing.T, createTable func() qpackHeaderTable) {
		table := createTable()
		require.True(t, table.SetMaximumDynamicTableCapacity(kMaximumDynamicTableCapacityForTesting))
		require.True(t, table.SetDynamicTableCapacity(39))

		assert.True(t, table.EntryFitsDynamicTableCapacity("foo", "bar"))
		assert.True(t, table.EntryFitsDynamicTableCapacity("foo", "bar2"))
		assert.False(t, table.EntryFitsDynamicTableCapacity("foo", "bar12"))
	})
}

func setupEncoderHeaderTable(t *testing.T) *EncoderHeaderTable {
	table := NewEncoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(kMaximumDynamicTableCapacityForTesting))
	require.True(t, table.SetDynamicTableCapacity(kMaximumDynamicTableCapacityForTesting))
	return table
}

// TEST_F(EncoderHeaderTableTest, FindStaticHeaderField)
func TestEncoderHeaderTable_FindStaticHeaderField(t *testing.T) {
	table := setupEncoderHeaderTable(t)

	// A header name that has multiple entries with different values.
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kStaticEntry, Index: 17},
		table.FindHeaderField(":method", "GET"))

	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kStaticEntry, Index: 20},
		table.FindHeaderField(":method", "POST"))

	// ":method: TRACE" does not exist in the static table.
	// Both following calls return the lowest index with key ":method".
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kStaticEntry, Index: 15},
		table.FindHeaderField(":method", "TRACE"))

	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kStaticEntry, Index: 15},
		table.FindHeaderName(":method"))

	// A header name that has a single entry with non-empty value.
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kStaticEntry, Index: 31},
		table.FindHeaderField("accept-encoding", "gzip, deflate, br"))

	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kStaticEntry, Index: 31},
		table.FindHeaderField("accept-encoding", "compress"))

	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kStaticEntry, Index: 31},
		table.FindHeaderField("accept-encoding", ""))

	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kStaticEntry, Index: 31},
		table.FindHeaderName("accept-encoding"))

	// A header name that has a single entry with empty value.
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kStaticEntry, Index: 12},
		table.FindHeaderField("location", ""))

	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kStaticEntry, Index: 12},
		table.FindHeaderField("location", "foo"))

	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kStaticEntry, Index: 12},
		table.FindHeaderName("location"))

	// No matching header name.
	assert.Equal(t, MatchTypeNoMatch, table.FindHeaderField("foo", "").Match)
	assert.Equal(t, MatchTypeNoMatch, table.FindHeaderField("foo", "bar").Match)
	assert.Equal(t, MatchTypeNoMatch, table.FindHeaderName("foo").Match)
}

// TEST_F(EncoderHeaderTableTest, FindDynamicHeaderField)
func TestEncoderHeaderTable_FindDynamicHeaderField(t *testing.T) {
	table := setupEncoderHeaderTable(t)

	// Dynamic table is initially empty.
	assert.Equal(t, MatchTypeNoMatch, table.FindHeaderField("foo", "bar").Match)
	assert.Equal(t, MatchTypeNoMatch, table.FindHeaderField("foo", "baz").Match)
	assert.Equal(t, MatchTypeNoMatch, table.FindHeaderName("foo").Match)

	// Insert one entry.
	table.InsertEntry("foo", "bar")

	// Match name and value.
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kDynamicEntry, Index: 0},
		table.FindHeaderField("foo", "bar"))

	// Match name only.
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kDynamicEntry, Index: 0},
		table.FindHeaderField("foo", "baz"))
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kDynamicEntry, Index: 0},
		table.FindHeaderName("foo"))

	// Insert an identical entry. FindHeaderField() should return the index of
	// the most recently inserted matching entry.
	table.InsertEntry("foo", "bar")

	// Match name and value.
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kDynamicEntry, Index: 1},
		table.FindHeaderField("foo", "bar"))

	// Match name only.
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kDynamicEntry, Index: 1},
		table.FindHeaderField("foo", "baz"))
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kDynamicEntry, Index: 1},
		table.FindHeaderName("foo"))
}

// TEST_F(EncoderHeaderTableTest, FindHeaderFieldPrefersStaticTable)
func TestEncoderHeaderTable_FindHeaderFieldPrefersStaticTable(t *testing.T) {
	table := setupEncoderHeaderTable(t)

	// Insert an entry to the dynamic table that exists in the static table.
	table.InsertEntry(":method", "GET")

	// FindHeaderField() prefers static table if both tables have name-and-value match.
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kStaticEntry, Index: 17},
		table.FindHeaderField(":method", "GET"))

	// FindHeaderField() prefers static table if both tables have name match but no value match,
	// and prefers the first entry with matching name.
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kStaticEntry, Index: 15},
		table.FindHeaderField(":method", "TRACE"))

	// FindHeaderName() prefers static table if both tables have a match, and prefers the first entry.
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kStaticEntry, Index: 15},
		table.FindHeaderName(":method"))

	// Add new entry to the dynamic table.
	table.InsertEntry(":method", "TRACE")

	// FindHeaderField prefers name-and-value match in dynamic table over name only match in static table.
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kDynamicEntry, Index: 1},
		table.FindHeaderField(":method", "TRACE"))
}

// TEST_F(EncoderHeaderTableTest, EvictByInsertion)
func TestEncoderHeaderTable_EvictByInsertion(t *testing.T) {
	table := setupEncoderHeaderTable(t)
	require.True(t, table.SetDynamicTableCapacity(40))

	// Entry size is 3 + 3 + 32 = 38.
	table.InsertEntry("foo", "bar")
	assert.Equal(t, uint64(1), table.InsertedEntryCount())
	assert.Equal(t, uint64(0), table.DroppedEntryCount())

	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kDynamicEntry, Index: 0},
		table.FindHeaderField("foo", "bar"))

	// Inserting second entry evicts the first one.
	table.InsertEntry("baz", "qux")
	assert.Equal(t, uint64(2), table.InsertedEntryCount())
	assert.Equal(t, uint64(1), table.DroppedEntryCount())

	assert.Equal(t, MatchTypeNoMatch, table.FindHeaderField("foo", "bar").Match)
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kDynamicEntry, Index: 1},
		table.FindHeaderField("baz", "qux"))
}

// TEST_F(EncoderHeaderTableTest, EvictByUpdateTableSize)
func TestEncoderHeaderTable_EvictByUpdateTableSize(t *testing.T) {
	table := setupEncoderHeaderTable(t)

	// Entry size is 3 + 3 + 32 = 38.
	table.InsertEntry("foo", "bar")
	table.InsertEntry("baz", "qux")
	assert.Equal(t, uint64(2), table.InsertedEntryCount())
	assert.Equal(t, uint64(0), table.DroppedEntryCount())

	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kDynamicEntry, Index: 0},
		table.FindHeaderField("foo", "bar"))
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kDynamicEntry, Index: 1},
		table.FindHeaderField("baz", "qux"))

	require.True(t, table.SetDynamicTableCapacity(40))
	assert.Equal(t, uint64(2), table.InsertedEntryCount())
	assert.Equal(t, uint64(1), table.DroppedEntryCount())

	assert.Equal(t, MatchTypeNoMatch, table.FindHeaderField("foo", "bar").Match)
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kDynamicEntry, Index: 1},
		table.FindHeaderField("baz", "qux"))

	require.True(t, table.SetDynamicTableCapacity(20))
	assert.Equal(t, uint64(2), table.InsertedEntryCount())
	assert.Equal(t, uint64(2), table.DroppedEntryCount())

	assert.Equal(t, MatchTypeNoMatch, table.FindHeaderField("foo", "bar").Match)
	assert.Equal(t, MatchTypeNoMatch, table.FindHeaderField("baz", "qux").Match)
}

// TEST_F(EncoderHeaderTableTest, EvictOldestOfIdentical)
func TestEncoderHeaderTable_EvictOldestOfIdentical(t *testing.T) {
	table := setupEncoderHeaderTable(t)
	require.True(t, table.SetDynamicTableCapacity(80))

	// Entry size is 3 + 3 + 32 = 38.
	// Insert same entry twice.
	table.InsertEntry("foo", "bar")
	table.InsertEntry("foo", "bar")
	assert.Equal(t, uint64(2), table.InsertedEntryCount())
	assert.Equal(t, uint64(0), table.DroppedEntryCount())

	// Find most recently inserted entry.
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kDynamicEntry, Index: 1},
		table.FindHeaderField("foo", "bar"))

	// Inserting third entry evicts the first one, not the second.
	table.InsertEntry("baz", "qux")
	assert.Equal(t, uint64(3), table.InsertedEntryCount())
	assert.Equal(t, uint64(1), table.DroppedEntryCount())

	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kDynamicEntry, Index: 1},
		table.FindHeaderField("foo", "bar"))
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kDynamicEntry, Index: 2},
		table.FindHeaderField("baz", "qux"))
}

// TEST_F(EncoderHeaderTableTest, EvictOldestOfSameName)
func TestEncoderHeaderTable_EvictOldestOfSameName(t *testing.T) {
	table := setupEncoderHeaderTable(t)
	require.True(t, table.SetDynamicTableCapacity(80))

	// Entry size is 3 + 3 + 32 = 38.
	// Insert two entries with same name but different values.
	table.InsertEntry("foo", "bar")
	table.InsertEntry("foo", "baz")
	assert.Equal(t, uint64(2), table.InsertedEntryCount())
	assert.Equal(t, uint64(0), table.DroppedEntryCount())

	// Find most recently inserted entry with matching name.
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kDynamicEntry, Index: 1},
		table.FindHeaderField("foo", "foo"))

	// Inserting third entry evicts the first one, not the second.
	table.InsertEntry("baz", "qux")
	assert.Equal(t, uint64(3), table.InsertedEntryCount())
	assert.Equal(t, uint64(1), table.DroppedEntryCount())

	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: kDynamicEntry, Index: 1},
		table.FindHeaderField("foo", "foo"))
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: kDynamicEntry, Index: 2},
		table.FindHeaderField("baz", "qux"))
}

// TEST_F(EncoderHeaderTableTest, MaxInsertSizeWithoutEvictingGivenEntry)
func TestEncoderHeaderTable_MaxInsertSizeWithoutEvictingGivenEntry(t *testing.T) {
	table := setupEncoderHeaderTable(t)
	const dynamicTableCapacity uint64 = 100
	require.True(t, table.SetDynamicTableCapacity(dynamicTableCapacity))

	// Empty table can take an entry up to its capacity.
	assert.Equal(t, dynamicTableCapacity, table.MaxInsertSizeWithoutEvictingGivenEntry(0))

	entrySize1 := EntrySize("foo", "bar")
	table.InsertEntry("foo", "bar")
	assert.Equal(t, dynamicTableCapacity-entrySize1, table.MaxInsertSizeWithoutEvictingGivenEntry(0))
	// Table can take an entry up to its capacity if all entries are allowed to be evicted.
	assert.Equal(t, dynamicTableCapacity, table.MaxInsertSizeWithoutEvictingGivenEntry(1))

	entrySize2 := EntrySize("baz", "foobar")
	table.InsertEntry("baz", "foobar")
	// Table can take an entry up to its capacity if all entries are allowed to be evicted.
	assert.Equal(t, dynamicTableCapacity, table.MaxInsertSizeWithoutEvictingGivenEntry(2))
	// Second entry must stay.
	assert.Equal(t, dynamicTableCapacity-entrySize2, table.MaxInsertSizeWithoutEvictingGivenEntry(1))
	// First and second entry must stay.
	assert.Equal(t, dynamicTableCapacity-entrySize2-entrySize1, table.MaxInsertSizeWithoutEvictingGivenEntry(0))

	// Third entry evicts first one.
	entrySize3 := EntrySize("last", "entry")
	table.InsertEntry("last", "entry")
	assert.Equal(t, uint64(1), table.DroppedEntryCount())
	// Table can take an entry up to its capacity if all entries are allowed to be evicted.
	assert.Equal(t, dynamicTableCapacity, table.MaxInsertSizeWithoutEvictingGivenEntry(3))
	// Third entry must stay.
	assert.Equal(t, dynamicTableCapacity-entrySize3, table.MaxInsertSizeWithoutEvictingGivenEntry(2))
	// Second and third entry must stay.
	assert.Equal(t, dynamicTableCapacity-entrySize3-entrySize2, table.MaxInsertSizeWithoutEvictingGivenEntry(1))
}

// TEST_F(EncoderHeaderTableTest, DrainingIndex)
func TestEncoderHeaderTable_DrainingIndex(t *testing.T) {
	table := setupEncoderHeaderTable(t)
	require.True(t, table.SetDynamicTableCapacity(4*EntrySize("foo", "bar")))

	// Empty table: no draining entry.
	assert.Equal(t, uint64(0), table.DrainingIndex(0.0))
	assert.Equal(t, uint64(0), table.DrainingIndex(1.0))

	// Table with one entry.
	table.InsertEntry("foo", "bar")
	// Any entry can be referenced if none of the table is draining.
	assert.Equal(t, uint64(0), table.DrainingIndex(0.0))
	// No entry can be referenced if all of the table is draining.
	assert.Equal(t, uint64(1), table.DrainingIndex(1.0))

	// Table with two entries is at half capacity.
	table.InsertEntry("foo", "bar")
	// Any entry can be referenced if at most half of the table is draining,
	// because current entries only take up half of total capacity.
	assert.Equal(t, uint64(0), table.DrainingIndex(0.0))
	assert.Equal(t, uint64(0), table.DrainingIndex(0.5))
	// No entry can be referenced if all of the table is draining.
	assert.Equal(t, uint64(2), table.DrainingIndex(1.0))

	// Table with four entries is full.
	table.InsertEntry("foo", "bar")
	table.InsertEntry("foo", "bar")
	// Any entry can be referenced if none of the table is draining.
	assert.Equal(t, uint64(0), table.DrainingIndex(0.0))
	// In a full table with identically sized entries, draining_fraction of all entries are draining.
	assert.Equal(t, uint64(2), table.DrainingIndex(0.5))
	// No entry can be referenced if all of the table is draining.
	assert.Equal(t, uint64(4), table.DrainingIndex(1.0))
}

func setupDecoderHeaderTable(t *testing.T) *DecoderHeaderTable {
	table := NewDecoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(kMaximumDynamicTableCapacityForTesting))
	require.True(t, table.SetDynamicTableCapacity(kMaximumDynamicTableCapacityForTesting))
	return table
}

func expectEntryAtIndex(
	t *testing.T,
	table *DecoderHeaderTable,
	isStatic bool,
	index uint64,
	expectedName, expectedValue string,
) {
	entry := table.LookupEntry(isStatic, index)
	require.NotNil(t, entry)
	assert.Equal(t, expectedName, entry.Name)
	assert.Equal(t, expectedValue, entry.Value)
}

func expectNoEntryAtIndex(t *testing.T, table *DecoderHeaderTable, isStatic bool, index uint64) {
	entry := table.LookupEntry(isStatic, index)
	assert.Nil(t, entry)
}

// TEST_F(DecoderHeaderTableTest, LookupStaticEntry)
func TestDecoderHeaderTable_LookupStaticEntry(t *testing.T) {
	table := setupDecoderHeaderTable(t)

	expectEntryAtIndex(t, table, kStaticEntry, 0, ":authority", "")
	expectEntryAtIndex(t, table, kStaticEntry, 1, ":path", "/")

	// 98 is the last entry.
	expectEntryAtIndex(t, table, kStaticEntry, 98, "x-frame-options", "sameorigin")

	expectNoEntryAtIndex(t, table, kStaticEntry, 99)
}

// TEST_F(DecoderHeaderTableTest, InsertAndLookupDynamicEntry)
func TestDecoderHeaderTable_InsertAndLookupDynamicEntry(t *testing.T) {
	table := setupDecoderHeaderTable(t)

	// Dynamic table is initially empty.
	expectNoEntryAtIndex(t, table, kDynamicEntry, 0)
	expectNoEntryAtIndex(t, table, kDynamicEntry, 1)
	expectNoEntryAtIndex(t, table, kDynamicEntry, 2)
	expectNoEntryAtIndex(t, table, kDynamicEntry, 3)

	// Insert one entry.
	table.InsertEntry("foo", "bar")
	expectEntryAtIndex(t, table, kDynamicEntry, 0, "foo", "bar")
	expectNoEntryAtIndex(t, table, kDynamicEntry, 1)
	expectNoEntryAtIndex(t, table, kDynamicEntry, 2)
	expectNoEntryAtIndex(t, table, kDynamicEntry, 3)

	// Insert a different entry.
	table.InsertEntry("baz", "bing")
	expectEntryAtIndex(t, table, kDynamicEntry, 0, "foo", "bar")
	expectEntryAtIndex(t, table, kDynamicEntry, 1, "baz", "bing")
	expectNoEntryAtIndex(t, table, kDynamicEntry, 2)
	expectNoEntryAtIndex(t, table, kDynamicEntry, 3)

	// Insert an entry identical to the most recently inserted one.
	table.InsertEntry("baz", "bing")
	expectEntryAtIndex(t, table, kDynamicEntry, 0, "foo", "bar")
	expectEntryAtIndex(t, table, kDynamicEntry, 1, "baz", "bing")
	expectEntryAtIndex(t, table, kDynamicEntry, 2, "baz", "bing")
	expectNoEntryAtIndex(t, table, kDynamicEntry, 3)
}

// TEST_F(DecoderHeaderTableTest, EvictByInsertion)
func TestDecoderHeaderTable_EvictByInsertion(t *testing.T) {
	table := setupDecoderHeaderTable(t)
	require.True(t, table.SetDynamicTableCapacity(40))

	// Entry size is 3 + 3 + 32 = 38.
	table.InsertEntry("foo", "bar")
	assert.Equal(t, uint64(1), table.InsertedEntryCount())
	assert.Equal(t, uint64(0), table.DroppedEntryCount())
	expectEntryAtIndex(t, table, kDynamicEntry, 0, "foo", "bar")

	// Inserting second entry evicts the first one.
	table.InsertEntry("baz", "qux")
	assert.Equal(t, uint64(2), table.InsertedEntryCount())
	assert.Equal(t, uint64(1), table.DroppedEntryCount())

	expectNoEntryAtIndex(t, table, kDynamicEntry, 0)
	expectEntryAtIndex(t, table, kDynamicEntry, 1, "baz", "qux")
}

// TEST_F(DecoderHeaderTableTest, EvictByUpdateTableSize)
func TestDecoderHeaderTable_EvictByUpdateTableSize(t *testing.T) {
	table := setupDecoderHeaderTable(t)

	expectNoEntryAtIndex(t, table, kDynamicEntry, 0)
	expectNoEntryAtIndex(t, table, kDynamicEntry, 1)

	// Entry size is 3 + 3 + 32 = 38.
	table.InsertEntry("foo", "bar")
	table.InsertEntry("baz", "qux")
	assert.Equal(t, uint64(2), table.InsertedEntryCount())
	assert.Equal(t, uint64(0), table.DroppedEntryCount())

	expectEntryAtIndex(t, table, kDynamicEntry, 0, "foo", "bar")
	expectEntryAtIndex(t, table, kDynamicEntry, 1, "baz", "qux")

	require.True(t, table.SetDynamicTableCapacity(40))
	assert.Equal(t, uint64(2), table.InsertedEntryCount())
	assert.Equal(t, uint64(1), table.DroppedEntryCount())

	expectNoEntryAtIndex(t, table, kDynamicEntry, 0)
	expectEntryAtIndex(t, table, kDynamicEntry, 1, "baz", "qux")

	require.True(t, table.SetDynamicTableCapacity(20))
	assert.Equal(t, uint64(2), table.InsertedEntryCount())
	assert.Equal(t, uint64(2), table.DroppedEntryCount())

	expectNoEntryAtIndex(t, table, kDynamicEntry, 0)
	expectNoEntryAtIndex(t, table, kDynamicEntry, 1)
}

// TEST_F(DecoderHeaderTableTest, EvictOldestOfIdentical)
func TestDecoderHeaderTable_EvictOldestOfIdentical(t *testing.T) {
	table := setupDecoderHeaderTable(t)
	require.True(t, table.SetDynamicTableCapacity(80))

	// Entry size is 3 + 3 + 32 = 38.
	// Insert same entry twice.
	table.InsertEntry("foo", "bar")
	table.InsertEntry("foo", "bar")
	assert.Equal(t, uint64(2), table.InsertedEntryCount())
	assert.Equal(t, uint64(0), table.DroppedEntryCount())

	expectEntryAtIndex(t, table, kDynamicEntry, 0, "foo", "bar")
	expectEntryAtIndex(t, table, kDynamicEntry, 1, "foo", "bar")
	expectNoEntryAtIndex(t, table, kDynamicEntry, 2)

	// Inserting third entry evicts the first one, not the second.
	table.InsertEntry("baz", "qux")
	assert.Equal(t, uint64(3), table.InsertedEntryCount())
	assert.Equal(t, uint64(1), table.DroppedEntryCount())

	expectNoEntryAtIndex(t, table, kDynamicEntry, 0)
	expectEntryAtIndex(t, table, kDynamicEntry, 1, "foo", "bar")
	expectEntryAtIndex(t, table, kDynamicEntry, 2, "baz", "qux")
}

// TEST_F(DecoderHeaderTableTest, EvictOldestOfSameName)
func TestDecoderHeaderTable_EvictOldestOfSameName(t *testing.T) {
	table := setupDecoderHeaderTable(t)
	require.True(t, table.SetDynamicTableCapacity(80))

	// Entry size is 3 + 3 + 32 = 38.
	// Insert two entries with same name but different values.
	table.InsertEntry("foo", "bar")
	table.InsertEntry("foo", "baz")
	assert.Equal(t, uint64(2), table.InsertedEntryCount())
	assert.Equal(t, uint64(0), table.DroppedEntryCount())

	expectEntryAtIndex(t, table, kDynamicEntry, 0, "foo", "bar")
	expectEntryAtIndex(t, table, kDynamicEntry, 1, "foo", "baz")
	expectNoEntryAtIndex(t, table, kDynamicEntry, 2)

	// Inserting third entry evicts the first one, not the second.
	table.InsertEntry("baz", "qux")
	assert.Equal(t, uint64(3), table.InsertedEntryCount())
	assert.Equal(t, uint64(1), table.DroppedEntryCount())

	expectNoEntryAtIndex(t, table, kDynamicEntry, 0)
	expectEntryAtIndex(t, table, kDynamicEntry, 1, "foo", "baz")
	expectEntryAtIndex(t, table, kDynamicEntry, 2, "baz", "qux")
}

type mockDecoderObserver struct {
	thresholdCallCount int
	cancelCallCount    int
}

func (m *mockDecoderObserver) OnInsertCountReachedThreshold() {
	m.thresholdCallCount++
}

func (m *mockDecoderObserver) Cancel() {
	m.cancelCallCount++
}

// TEST_F(DecoderHeaderTableTest, RegisterObserver)
func TestDecoderHeaderTable_RegisterObserver(t *testing.T) {
	table := setupDecoderHeaderTable(t)

	obs1 := &mockDecoderObserver{}
	table.RegisterObserver(1, obs1)
	table.InsertEntry("foo", "bar")
	assert.Equal(t, uint64(1), table.InsertedEntryCount())
	assert.Equal(t, 1, obs1.thresholdCallCount)

	// Registration order does not matter.
	obs2 := &mockDecoderObserver{}
	obs3 := &mockDecoderObserver{}
	table.RegisterObserver(3, obs3)
	table.RegisterObserver(2, obs2)

	table.InsertEntry("foo", "bar")
	assert.Equal(t, uint64(2), table.InsertedEntryCount())
	assert.Equal(t, 1, obs2.thresholdCallCount)
	assert.Equal(t, 0, obs3.thresholdCallCount)

	table.InsertEntry("foo", "bar")
	assert.Equal(t, uint64(3), table.InsertedEntryCount())
	assert.Equal(t, 1, obs3.thresholdCallCount)

	// Multiple observers with identical required_insert_count should all be notified.
	obs4 := &mockDecoderObserver{}
	obs5 := &mockDecoderObserver{}
	table.RegisterObserver(4, obs4)
	table.RegisterObserver(4, obs5)

	table.InsertEntry("foo", "bar")
	assert.Equal(t, uint64(4), table.InsertedEntryCount())
	assert.Equal(t, 1, obs4.thresholdCallCount)
	assert.Equal(t, 1, obs5.thresholdCallCount)
}

// TEST_F(DecoderHeaderTableTest, UnregisterObserver)
func TestDecoderHeaderTable_UnregisterObserver(t *testing.T) {
	table := setupDecoderHeaderTable(t)

	obs1 := &mockDecoderObserver{}
	obs2 := &mockDecoderObserver{}
	obs3 := &mockDecoderObserver{}
	obs4 := &mockDecoderObserver{}

	table.RegisterObserver(1, obs1)
	table.RegisterObserver(2, obs2)
	table.RegisterObserver(2, obs3)
	table.RegisterObserver(3, obs4)

	table.UnregisterObserver(2, obs3)

	table.InsertEntry("foo", "bar")
	table.InsertEntry("foo", "bar")
	table.InsertEntry("foo", "bar")
	assert.Equal(t, uint64(3), table.InsertedEntryCount())

	assert.Equal(t, 1, obs1.thresholdCallCount)
	assert.Equal(t, 1, obs2.thresholdCallCount)
	assert.Equal(t, 0, obs3.thresholdCallCount) // Unregistered observer not notified
	assert.Equal(t, 1, obs4.thresholdCallCount)
}

// TEST_F(DecoderHeaderTableTest, Cancel)
func TestDecoderHeaderTable_Cancel(t *testing.T) {
	table := setupDecoderHeaderTable(t)
	obs := &mockDecoderObserver{}
	table.RegisterObserver(1, obs)

	table.Close()
	assert.Equal(t, 1, obs.cancelCallCount)
}
