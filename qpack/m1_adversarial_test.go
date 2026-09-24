// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// Reference dynamic table model (oracle) to independently check EncoderHeaderTable
type oracleModel struct {
	capacity uint64
	entries  []*Entry
	dropped  uint64
}

func newOracleModel(cap uint64) *oracleModel {
	return &oracleModel{
		capacity: cap,
		entries:  make([]*Entry, 0),
		dropped:  0,
	}
}

func (m *oracleModel) totalSize() uint64 {
	var sz uint64
	for _, e := range m.entries {
		sz += e.Size()
	}
	return sz
}

func (m *oracleModel) evictDownTo(targetCap uint64) {
	for m.totalSize() > targetCap && len(m.entries) > 0 {
		m.entries = m.entries[1:]
		m.dropped++
	}
}

func (m *oracleModel) setCapacity(newCap uint64) {
	m.capacity = newCap
	m.evictDownTo(newCap)
}

func (m *oracleModel) insert(name, value string) uint64 {
	e := NewEntry(name, value)
	sz := e.Size()
	if sz > m.capacity {
		panic("oracle: entry size exceeds capacity")
	}
	m.evictDownTo(m.capacity - sz)
	idx := m.dropped + uint64(len(m.entries))
	m.entries = append(m.entries, e)
	return idx
}

func (m *oracleModel) findExactDynamic(name, value string) (uint64, bool) {
	// Search newest to oldest
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].Name == name && m.entries[i].Value == value {
			return m.dropped + uint64(i), true
		}
	}
	return 0, false
}

func (m *oracleModel) findNameDynamic(name string) (uint64, bool) {
	// Search newest to oldest
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].Name == name {
			return m.dropped + uint64(i), true
		}
	}
	return 0, false
}

// ----------------------------------------------------------------------------
// Test 1: High-Volume Random Churn & Rapid Resize Cycles with Oracle Comparison
// ----------------------------------------------------------------------------
func TestAdversarial_Table_RapidChurnAndOracleParity(t *testing.T) {
	const maxCap = 8192
	rng := rand.New(rand.NewSource(123456789))

	table := NewEncoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(maxCap))
	require.True(t, table.SetDynamicTableCapacity(maxCap))

	oracle := newOracleModel(maxCap)

	names := []string{
		"custom-header",
		"x-token",
		"auth-bearer",
		":path",
		"accept-encoding",
		"user-agent",
		"x-random-header",
	}
	values := []string{"val1", "val2", "val3", "long-value-string-with-padding-1234567890", "", "a", "b"}

	const numOps = 25000
	for op := 0; op < numOps; op++ {
		action := rng.Intn(10)
		switch {
		case action < 6: // Insert random entry
			name := names[rng.Intn(len(names))]
			val := values[rng.Intn(len(values))]
			if rng.Intn(20) == 0 {
				// Occasionally insert random generated string
				name = fmt.Sprintf("rand-name-%d", rng.Intn(500))
				val = fmt.Sprintf("rand-val-%d", rng.Intn(500))
			}

			entrySz := EntrySize(name, val)
			if entrySz <= table.DynamicTableCapacity() {
				idxTable := table.InsertEntry(name, val)
				idxOracle := oracle.insert(name, val)
				require.Equal(t, idxOracle, idxTable)
			}

		case action < 8: // Capacity resize (shrink or expand)
			caps := []uint64{0, 32, 64, 128, 500, 1024, 2048, 4096, 8192}
			newCap := caps[rng.Intn(len(caps))]
			require.True(t, table.SetDynamicTableCapacity(newCap))
			oracle.setCapacity(newCap)

		default: // Random query check
			queryName := names[rng.Intn(len(names))]
			queryVal := values[rng.Intn(len(values))]

			res := table.FindHeaderField(queryName, queryVal)
			if !res.IsStatic {
				if res.Match == MatchTypeNameAndValue {
					expectedIdx, found := oracle.findExactDynamic(queryName, queryVal)
					require.True(t, found, "Table found dynamic exact match that oracle did not find")
					require.Equal(t, expectedIdx, res.Index)
				} else if res.Match == MatchTypeName {
					// Oracle exact must not match
					_, foundExact := oracle.findExactDynamic(queryName, queryVal)
					require.False(t, foundExact)
					// Name must match
					expectedIdx, foundName := oracle.findNameDynamic(queryName)
					require.True(t, foundName)
					require.Equal(t, expectedIdx, res.Index)
				} else {
					// No match
					_, foundExact := oracle.findExactDynamic(queryName, queryVal)
					require.False(t, foundExact)
					_, foundName := oracle.findNameDynamic(queryName)
					require.False(t, foundName)
				}
			}
		}

		// Invariant checks
		require.Equal(t, oracle.totalSize(), table.DynamicTableSize(), fmt.Sprintf("Size mismatch at op %d", op))
		require.Equal(t, oracle.dropped, table.DroppedEntryCount(), fmt.Sprintf("Dropped count mismatch at op %d", op))
		require.Equal(
			t,
			oracle.dropped+uint64(len(oracle.entries)),
			table.InsertedEntryCount(),
			fmt.Sprintf("Inserted count mismatch at op %d", op),
		)
		require.LessOrEqual(
			t,
			table.DynamicTableSize(),
			table.DynamicTableCapacity(),
			"Dynamic table size exceeds capacity",
		)
	}

	// Final shrink to zero: verify complete cleanup of maps and ring buffer
	require.True(t, table.SetDynamicTableCapacity(0))
	oracle.setCapacity(0)
	require.Equal(t, uint64(0), table.DynamicTableSize())
	require.Equal(t, 0, len(table.dynamicIndex))
	require.Equal(t, 0, len(table.dynamicNameIndex))
	require.Equal(t, 0, table.dynamicEntries.size())
}

// ----------------------------------------------------------------------------
// Test 2: Inverted Map Eviction Edge Cases & Duplicate Key Churn
// ----------------------------------------------------------------------------
func TestAdversarial_InvertedMap_DuplicateEvictionOrder(t *testing.T) {
	table := NewEncoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(10000))
	// Capacity for exactly 3 entries: each entry ("k", "v") is 1 + 1 + 32 = 34 bytes.
	// 3 * 34 = 102 bytes.
	require.True(t, table.SetDynamicTableCapacity(102))

	// Step 1: Insert 3 identical entries
	idx0 := table.InsertEntry("k", "0") // idx 0
	idx1 := table.InsertEntry("k", "0") // idx 1
	idx2 := table.InsertEntry("k", "0") // idx 2
	require.Equal(t, uint64(0), idx0)
	require.Equal(t, uint64(1), idx1)
	require.Equal(t, uint64(2), idx2)
	require.Equal(t, uint64(3), table.InsertedEntryCount())
	require.Equal(t, uint64(0), table.DroppedEntryCount())

	// Match must be index 2
	res := table.FindHeaderField("k", "0")
	require.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 2}, res)

	// Step 2: Insert 4th entry ("k", "1"). Evicts entry 0.
	idx3 := table.InsertEntry("k", "1") // idx 3
	require.Equal(t, uint64(3), idx3)
	require.Equal(t, uint64(1), table.DroppedEntryCount())

	// ("k", "0") should still match index 2!
	res = table.FindHeaderField("k", "0")
	require.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 2}, res)
	// ("k", "1") matches index 3
	res = table.FindHeaderField("k", "1")
	require.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 3}, res)
	// Name match for "k" must return highest index 3
	resName := table.FindHeaderName("k")
	require.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 3}, resName)

	// Step 3: Insert ("k", "2"). Evicts entry 1 ("k", "0").
	idx4 := table.InsertEntry("k", "2") // idx 4
	require.Equal(t, uint64(4), idx4)
	require.Equal(t, uint64(2), table.DroppedEntryCount())
	// ("k", "0") should still match index 2!
	res = table.FindHeaderField("k", "0")
	require.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 2}, res)

	// Step 4: Insert ("o", "0"). Evicts entry 2 ("k", "0")!
	// Now NO ("k", "0") exists in dynamic table!
	table.InsertEntry("o", "0") // idx 5, evicts entry 2
	require.Equal(t, uint64(3), table.DroppedEntryCount())

	// ("k", "0") exact match should no longer exist; should fall back to name match for "k" at index 4 ("k", "2")!
	res = table.FindHeaderField("k", "0")
	require.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 4}, res)

	// Step 5: Evict all "k" entries
	table.InsertEntry("o", "1") // evicts idx 3 ("k", "1")
	table.InsertEntry("o", "2") // evicts idx 4 ("k", "2")
	require.Equal(t, uint64(5), table.DroppedEntryCount())

	// Now "k" should not match at all!
	resName = table.FindHeaderName("k")
	require.Equal(t, MatchTypeNoMatch, resName.Match)
	res = table.FindHeaderField("k", "0")
	require.Equal(t, MatchTypeNoMatch, res.Match)
}

// ----------------------------------------------------------------------------
// Test 3: Ring Buffer Stress & Memory Retention / Leak Check
// ----------------------------------------------------------------------------
func TestAdversarial_RingBuffer_NoMemoryLeakOnEviction(t *testing.T) {
	// Allocate a decoder header table
	table := NewDecoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(2000))
	require.True(t, table.SetDynamicTableCapacity(2000))

	// Insert 100,000 entries causing rapid eviction
	payload := string(make([]byte, 100))
	for i := 0; i < 100000; i++ {
		table.InsertEntry("header-name", payload)
	}

	require.Equal(t, uint64(100000), table.InsertedEntryCount())
	require.Greater(t, table.DroppedEntryCount(), uint64(99900))
	require.LessOrEqual(t, uint64(table.dynamicEntries.size()), uint64(20))

	// Verify that internal ring buffer array slots for evicted elements are nil
	nilCount := 0
	nonNilCount := 0
	for _, ptr := range table.dynamicEntries.entries {
		if ptr == nil {
			nilCount++
		} else {
			nonNilCount++
		}
	}
	require.Equal(t, table.dynamicEntries.size(), nonNilCount, "Non-nil entries in slice must match active size")
	require.Greater(t, nilCount, 0, "Evicted ring buffer slots must be nil to allow GC")

	// Trigger GC to confirm no heap bloat or panic
	runtime.GC()
}

// ----------------------------------------------------------------------------
// Test 4: Coordinate Conversions Near math.MaxUint64 and Boundary Limits
// ----------------------------------------------------------------------------
func TestAdversarial_CoordinateConversions_NearMaxUint64(t *testing.T) {
	// A. Encoder Stream Relative Index
	{
		// 1. Extreme boundary: insertedEntryCount = math.MaxUint64
		// abs = math.MaxUint64 - 1
		rel := AbsoluteIndexToEncoderStreamRelativeIndex(math.MaxUint64-1, math.MaxUint64)
		require.Equal(t, uint64(0), rel)

		abs, ok := EncoderStreamRelativeIndexToAbsoluteIndex(0, math.MaxUint64)
		require.True(t, ok)
		require.Equal(t, uint64(math.MaxUint64-1), abs)

		// abs = 0, total = math.MaxUint64
		rel = AbsoluteIndexToEncoderStreamRelativeIndex(0, math.MaxUint64)
		require.Equal(t, uint64(math.MaxUint64-1), rel)

		abs, ok = EncoderStreamRelativeIndexToAbsoluteIndex(math.MaxUint64-1, math.MaxUint64)
		require.True(t, ok)
		require.Equal(t, uint64(0), abs)

		// rel = math.MaxUint64, total = math.MaxUint64 -> out of bounds
		_, ok = EncoderStreamRelativeIndexToAbsoluteIndex(math.MaxUint64, math.MaxUint64)
		require.False(t, ok)

		// Underflow check: rel >= insertedEntryCount
		_, ok = EncoderStreamRelativeIndexToAbsoluteIndex(100, 50)
		require.False(t, ok)

		// Panic check on abs >= insertedEntryCount
		assert.Panics(t, func() {
			AbsoluteIndexToEncoderStreamRelativeIndex(math.MaxUint64, math.MaxUint64)
		})
	}

	// B. Request Stream Relative Index
	{
		// base = math.MaxUint64, abs = math.MaxUint64 - 1
		rel := AbsoluteIndexToRequestStreamRelativeIndex(math.MaxUint64-1, math.MaxUint64)
		require.Equal(t, uint64(0), rel)

		abs, ok := RequestStreamRelativeIndexToAbsoluteIndex(0, math.MaxUint64)
		require.True(t, ok)
		require.Equal(t, uint64(math.MaxUint64-1), abs)

		// base = math.MaxUint64, abs = 0
		rel = AbsoluteIndexToRequestStreamRelativeIndex(0, math.MaxUint64)
		require.Equal(t, uint64(math.MaxUint64-1), rel)

		abs, ok = RequestStreamRelativeIndexToAbsoluteIndex(math.MaxUint64-1, math.MaxUint64)
		require.True(t, ok)
		require.Equal(t, uint64(0), abs)

		// rel >= base: out of bounds
		_, ok = RequestStreamRelativeIndexToAbsoluteIndex(math.MaxUint64, math.MaxUint64)
		require.False(t, ok)

		_, ok = RequestStreamRelativeIndexToAbsoluteIndex(50, 50)
		require.False(t, ok)

		assert.Panics(t, func() {
			AbsoluteIndexToRequestStreamRelativeIndex(50, 50)
		})
	}

	// C. Post-Base Index
	{
		// base = 0, postBaseIndex = 0
		abs, ok := PostBaseIndexToAbsoluteIndex(0, 0)
		require.True(t, ok)
		require.Equal(t, uint64(0), abs)
		require.Equal(t, uint64(0), AbsoluteIndexToPostBaseIndex(abs, 0))

		// base = 0, postBaseIndex = math.MaxUint64 - 1
		abs, ok = PostBaseIndexToAbsoluteIndex(math.MaxUint64-1, 0)
		require.True(t, ok)
		require.Equal(t, uint64(math.MaxUint64-1), abs)
		require.Equal(t, uint64(math.MaxUint64-1), AbsoluteIndexToPostBaseIndex(abs, 0))

		// base = math.MaxUint64 - 1, postBaseIndex = 0
		abs, ok = PostBaseIndexToAbsoluteIndex(0, math.MaxUint64-1)
		require.True(t, ok)
		require.Equal(t, uint64(math.MaxUint64-1), abs)
		require.Equal(t, uint64(0), AbsoluteIndexToPostBaseIndex(abs, math.MaxUint64-1))

		// Overflow conditions:
		// postBaseIndex >= math.MaxUint64 - base
		// 1. base = math.MaxUint64, postBaseIndex = 0 -> overflow/error
		_, ok = PostBaseIndexToAbsoluteIndex(0, math.MaxUint64)
		require.False(t, ok)

		// 2. base = math.MaxUint64 - 5, postBaseIndex = 5 -> overflow/error
		_, ok = PostBaseIndexToAbsoluteIndex(5, math.MaxUint64-5)
		require.False(t, ok)

		// 3. base = math.MaxUint64 - 5, postBaseIndex = 6 -> overflow/error
		_, ok = PostBaseIndexToAbsoluteIndex(6, math.MaxUint64-5)
		require.False(t, ok)

		// 4. base = math.MaxUint64 - 5, postBaseIndex = 4 -> ok (result is MaxUint64 - 1)
		abs, ok = PostBaseIndexToAbsoluteIndex(4, math.MaxUint64-5)
		require.True(t, ok)
		require.Equal(t, uint64(math.MaxUint64-1), abs)

		// Panic on abs < base for AbsoluteIndexToPostBaseIndex
		assert.Panics(t, func() {
			AbsoluteIndexToPostBaseIndex(10, 11)
		})
	}
}

// ----------------------------------------------------------------------------
// Test 5: Required Insert Count Modulo Wrapping for Extreme Counts (10^6+)
// ----------------------------------------------------------------------------
func TestAdversarial_RIC_ModuloWrapping_ExtremeCounts(t *testing.T) {
	testMaxEntries := []uint64{1, 2, 5, 16, 64, 128, 512, 1024, 65536}

	for _, maxEntries := range testMaxEntries {
		fullRange := 2 * maxEntries

		// Test multiple wrap cycles from 0 up to 2,000,000
		wrapMultiples := []uint64{
			0, 1, 2, 3, 5, 10, 50, 100, 500, 1000, 5000, 10000, 50000, 100000, 500000, 1000000,
		}

		for _, mult := range wrapMultiples {
			total := mult * fullRange
			// Test around total: boundary offsets
			offsets := []int64{
				-int64(maxEntries) + 1,
				-int64(maxEntries) / 2,
				0,
				int64(maxEntries) / 2,
				int64(maxEntries),
			}

			for _, off := range offsets {
				if off < -int64(maxEntries)+1 || off > int64(maxEntries) {
					continue
				}
				ricInt := int64(total) + off
				if ricInt <= 0 {
					continue
				}
				ric := uint64(ricInt)

				// 1. Encode
				enc := EncodeRequiredInsertCount(ric, maxEntries)
				require.GreaterOrEqual(t, enc, uint64(1))
				require.LessOrEqual(t, enc, fullRange)

				// 2. Decode
				dec, ok := DecodeRequiredInsertCount(enc, maxEntries, total)
				require.True(
					t,
					ok,
					fmt.Sprintf("Failed to decode RIC %d with total %d, maxEntries %d", ric, total, maxEntries),
				)
				require.Equal(
					t,
					ric,
					dec,
					fmt.Sprintf(
						"Decoded RIC did not match original: got %d, want %d (total=%d, maxEntries=%d)",
						dec,
						ric,
						total,
						maxEntries,
					),
				)
			}
		}

		// Test extreme values near uint64 overflow.
		// Chromium's algorithm performs intermediate addition `reqInsertCount += totalNumberOfInserts`
		// before subtracting `currentWrapped`. Therefore, intermediate overflow occurs if
		// total > MaxUint64 - 2*FullRange. We test the maximum possible totals that Chromium supports.
		nearOverflowTotals := []uint64{
			math.MaxUint64 - 4*fullRange - 1000,
			math.MaxUint64 - 2*fullRange - 1,
		}

		for _, total := range nearOverflowTotals {
			offsets := []int64{-int64(maxEntries) + 1, 0, int64(maxEntries)}
			for _, off := range offsets {
				ric := uint64(int64(total) + off)
				enc := EncodeRequiredInsertCount(ric, maxEntries)
				dec, ok := DecodeRequiredInsertCount(enc, maxEntries, total)
				require.True(
					t,
					ok,
					fmt.Sprintf(
						"Failed near uint64 max: total=%d, maxEntries=%d, off=%d, ric=%d",
						total,
						maxEntries,
						off,
						ric,
					),
				)
				require.Equal(t, ric, dec)
			}
		}
	}
}

// ----------------------------------------------------------------------------
// Test 6: Required Insert Count Error Handling & Malformed Wire Rejection
// ----------------------------------------------------------------------------
func TestAdversarial_RIC_ErrorRejections(t *testing.T) {
	// Zero max entries
	{
		// enc == 0 with maxEntries == 0 is valid (both indicate no dynamic entries)
		dec, ok := DecodeRequiredInsertCount(0, 0, 0)
		require.True(t, ok)
		require.Equal(t, uint64(0), dec)

		// enc > 0 with maxEntries == 0 must fail
		_, ok = DecodeRequiredInsertCount(1, 0, 0)
		require.False(t, ok)

		_, ok = DecodeRequiredInsertCount(5, 0, 100)
		require.False(t, ok)
	}

	// Encoded value exceeds 2 * maxEntries
	{
		const maxEntries = 50
		_, ok := DecodeRequiredInsertCount(2*maxEntries+1, maxEntries, 100)
		require.False(t, ok)

		_, ok = DecodeRequiredInsertCount(math.MaxUint64, maxEntries, 100)
		require.False(t, ok)
	}

	// RIC unwrapping that results in zero when enc > 0
	{
		const maxEntries = 10
		// enc = 1, total = 0 -> req = 0, current_wrapped = 0.
		// reqInsertCount (0) == currentWrapped (0) -> currentWrapped >= reqInsertCount -> underflow rejection!
		_, ok := DecodeRequiredInsertCount(1, maxEntries, 0)
		require.False(t, ok, "RIC unwrapping to 0 must be rejected")
	}

	// Overflow guard: reqInsertCount + totalNumberOfInserts > MaxUint64
	{
		const maxEntries uint64 = 100
		var total uint64 = math.MaxUint64 - 10
		enc := 2 * maxEntries // max wire value
		_, ok := DecodeRequiredInsertCount(enc, maxEntries, total)
		_ = ok
		_, ok = DecodeRequiredInsertCount(enc, maxEntries, math.MaxUint64)
		_ = ok
	}
}

// ----------------------------------------------------------------------------
// Test 7: Decoder Observers Stress Test & FIFO Invariant
// ----------------------------------------------------------------------------
func TestAdversarial_DecoderObservers_HeavyRegistrationAndFIFO(t *testing.T) {
	table := NewDecoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(10000))
	require.True(t, table.SetDynamicTableCapacity(10000))

	const numObservers = 5000
	type testObs struct {
		id       int
		thresh   uint64
		fired    bool
		order    int
		canceled bool
	}

	var fireOrder int
	observers := make([]*testObs, numObservers)

	// Callback wrapper
	type obsImpl struct {
		o *testObs
	}

	makeObs := func(o *testObs) DecoderHeaderTableObserver {
		return &mockObsWrapper{
			onThreshold: func() {
				o.fired = true
				fireOrder++
				o.order = fireOrder
			},
			onCancel: func() {
				o.canceled = true
			},
		}
	}

	rng := rand.New(rand.NewSource(987654321))
	// Randomly assign thresholds between 1 and 200
	for i := 0; i < numObservers; i++ {
		thresh := uint64(rng.Intn(200) + 1)
		observers[i] = &testObs{id: i, thresh: thresh}
		table.RegisterObserver(thresh, makeObs(observers[i]))
	}

	// Insert 250 entries
	for i := 0; i < 250; i++ {
		table.InsertEntry("key", "val")
	}

	// All observers with threshold <= 250 must have fired
	for _, o := range observers {
		if o.thresh <= 250 {
			require.True(t, o.fired, fmt.Sprintf("Observer %d with threshold %d did not fire", o.id, o.thresh))
		} else {
			require.False(
				t,
				o.fired,
				fmt.Sprintf("Observer %d with threshold %d should not have fired", o.id, o.thresh),
			)
		}
	}

	// Register some observers with thresholds > 250 (e.g. 500)
	lateObs := &testObs{id: 9999, thresh: 500}
	table.RegisterObserver(500, makeObs(lateObs))

	// Close table -> should cancel all un-fired observers
	table.Close()
	require.True(t, lateObs.canceled, "Unfired observer must be cancelled on Close()")

	// Further insertions should not trigger cancelled observers
	table.InsertEntry("key", "val")
	require.False(t, lateObs.fired, "Cancelled observer must not fire after Close()")
}

type mockObsWrapper struct {
	onThreshold func()
	onCancel    func()
}

func (m *mockObsWrapper) OnInsertCountReachedThreshold() {
	if m.onThreshold != nil {
		m.onThreshold()
	}
}

func (m *mockObsWrapper) Cancel() {
	if m.onCancel != nil {
		m.onCancel()
	}
}

// ----------------------------------------------------------------------------
// Test 8: Zero Capacity and Exact-Capacity Limits
// ----------------------------------------------------------------------------
func TestAdversarial_ZeroCapacityAndExactLimits(t *testing.T) {
	// Table capacity 0
	table := &HeaderTableBase{}
	table.initBase()
	require.True(t, table.SetMaximumDynamicTableCapacity(100))
	require.True(t, table.SetDynamicTableCapacity(0))

	assert.False(t, table.EntryFitsDynamicTableCapacity("a", "b"))
	assert.Panics(t, func() {
		table.InsertEntry("a", "b")
	})

	// Exact capacity 34: entry of size 34 ("a", "b" -> 1 + 1 + 32 = 34)
	require.True(t, table.SetDynamicTableCapacity(34))
	assert.True(t, table.EntryFitsDynamicTableCapacity("a", "b"))
	assert.False(t, table.EntryFitsDynamicTableCapacity("aa", "b"))

	// Insert exactly fits
	idx0 := table.InsertEntry("a", "b")
	require.Equal(t, uint64(0), idx0)
	require.Equal(t, uint64(34), table.DynamicTableSize())
	require.Equal(t, uint64(0), table.DroppedEntryCount())

	// Insert second entry of size 34: evicts first entry
	idx1 := table.InsertEntry("a", "b")
	require.Equal(t, uint64(1), idx1)
	require.Equal(t, uint64(34), table.DynamicTableSize())
	require.Equal(t, uint64(1), table.DroppedEntryCount())

	// Setting capacity to 33: evicts resident entry of size 34
	require.True(t, table.SetDynamicTableCapacity(33))
	require.Equal(t, uint64(0), table.DynamicTableSize())
	require.Equal(t, uint64(2), table.DroppedEntryCount())
}

// ----------------------------------------------------------------------------
// Test 9: MaxInsertSizeWithoutEvicting & DrainingIndex Stress with Oracle
// ----------------------------------------------------------------------------
func TestAdversarial_MaxInsertSizeAndDrainingIndex_Oracle(t *testing.T) {
	const capacity uint64 = 500
	table := NewEncoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(capacity))
	require.True(t, table.SetDynamicTableCapacity(capacity))

	// Insert 10 entries of varying sizes
	for i := 0; i < 10; i++ {
		table.InsertEntry(fmt.Sprintf("name-%d", i), fmt.Sprintf("val-string-%d", i*2))
	}

	// 1. MaxInsertSizeWithoutEvictingGivenEntry
	// For all indices below droppedEntryCount: must return 0
	for idx := uint64(0); idx < table.DroppedEntryCount(); idx++ {
		require.Equal(t, uint64(0), table.MaxInsertSizeWithoutEvictingGivenEntry(idx))
	}

	// For index > InsertedEntryCount(): must return capacity
	require.Equal(t, capacity, table.MaxInsertSizeWithoutEvictingGivenEntry(table.InsertedEntryCount()+10))
	require.Equal(t, capacity, table.MaxInsertSizeWithoutEvictingGivenEntry(table.InsertedEntryCount()))

	// For an entry currently in table: verify that inserting that exact size doesn't evict it
	for idx := table.DroppedEntryCount(); idx < table.InsertedEntryCount(); idx++ {
		maxInsert := table.MaxInsertSizeWithoutEvictingGivenEntry(idx)
		require.LessOrEqual(t, maxInsert, capacity)
	}

	// 2. DrainingIndex boundary checks
	require.Equal(t, table.DroppedEntryCount(), table.DrainingIndex(-0.5))
	require.Equal(t, table.DroppedEntryCount(), table.DrainingIndex(0.0))
	require.Equal(t, table.InsertedEntryCount(), table.DrainingIndex(1.0))
	require.Equal(t, table.InsertedEntryCount(), table.DrainingIndex(1.5))

	// Mid fractions: must be monotonically non-decreasing
	prev := table.DrainingIndex(0.0)
	for frac := 0.1; frac <= 1.0; frac += 0.1 {
		curr := table.DrainingIndex(frac)
		require.GreaterOrEqual(t, curr, prev)
		require.GreaterOrEqual(t, curr, table.DroppedEntryCount())
		require.LessOrEqual(t, curr, table.InsertedEntryCount())
		prev = curr
	}
}

// ----------------------------------------------------------------------------
// Test 10: Static Table Precedence, Static Entry 48 Conformance, and Dynamic Shadowing
// ----------------------------------------------------------------------------
func TestAdversarial_StaticTablePrecedenceAndImageGifFix(t *testing.T) {
	table := NewEncoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(1000))
	require.True(t, table.SetDynamicTableCapacity(1000))

	// 1. Verify Entry 48 conformance: RFC 9204 Appendix A defines index 48 as
	// Name: "content-type", Value: "image/gif".
	res := table.FindHeaderField("content-type", "image/gif")
	require.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: true, Index: 48}, res)

	// 2. Insert dynamic entry identical to static entry 48
	table.InsertEntry("content-type", "image/gif")

	// FindHeaderField must STILL return the static table match (index 48), NOT dynamic!
	res = table.FindHeaderField("content-type", "image/gif")
	require.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: true, Index: 48}, res)

	// 3. Name-only match preference: static over dynamic
	// Static index 0 is ":authority" with empty value
	res = table.FindHeaderField(":authority", "example.com")
	require.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: true, Index: 0}, res)

	// Insert dynamic ":authority" with value "dynamic.com"
	dynIdx := table.InsertEntry(":authority", "dynamic.com")
	// Name lookup for non-matching value should STILL prefer static name match (index 0)
	res = table.FindHeaderField(":authority", "other.com")
	require.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: true, Index: 0}, res)

	// But exact value "dynamic.com" should match the dynamic entry!
	res = table.FindHeaderField(":authority", "dynamic.com")
	require.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: dynIdx}, res)
}

// ----------------------------------------------------------------------------
// Test 11: Coordinate Conversions Exhaustive Random Fuzzing Invariants
// ----------------------------------------------------------------------------
func TestAdversarial_CoordinateConversions_ExhaustiveFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(55555))
	const numIterations = 50000

	for i := 0; i < numIterations; i++ {
		// 1. Encoder Relative
		total := uint64(rng.Int63n(1000000) + 1)
		abs := uint64(rng.Int63n(int64(total)))

		rel := AbsoluteIndexToEncoderStreamRelativeIndex(abs, total)
		recoveredAbs, ok := EncoderStreamRelativeIndexToAbsoluteIndex(rel, total)
		require.True(t, ok)
		require.Equal(t, abs, recoveredAbs)

		// 2. Request Relative
		base := uint64(rng.Int63n(1000000) + 1)
		abs = uint64(rng.Int63n(int64(base)))

		rel = AbsoluteIndexToRequestStreamRelativeIndex(abs, base)
		recoveredAbs, ok = RequestStreamRelativeIndexToAbsoluteIndex(rel, base)
		require.True(t, ok)
		require.Equal(t, abs, recoveredAbs)

		// 3. Post-Base
		base = uint64(rng.Int63n(1000000))
		postBase := uint64(rng.Int63n(1000000))

		abs, ok = PostBaseIndexToAbsoluteIndex(postBase, base)
		require.True(t, ok)
		require.Equal(t, base+postBase, abs)
		recoveredPostBase := AbsoluteIndexToPostBaseIndex(abs, base)
		require.Equal(t, postBase, recoveredPostBase)
	}
}

// ----------------------------------------------------------------------------
// Test 12: Decoder Dynamic Table Lookup Invariants for Evicted & Out-of-Bounds
// ----------------------------------------------------------------------------
func TestAdversarial_DecoderLookup_EvictedAndOutOfBounds(t *testing.T) {
	table := NewDecoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(200))
	require.True(t, table.SetDynamicTableCapacity(200))

	// Insert entries until eviction occurs
	for i := 0; i < 50; i++ {
		table.InsertEntry(fmt.Sprintf("n-%d", i), fmt.Sprintf("v-%d", i))
	}

	require.Greater(t, table.DroppedEntryCount(), uint64(0))

	// Index < droppedEntryCount: must return nil
	for i := uint64(0); i < table.DroppedEntryCount(); i++ {
		entry := table.LookupEntry(false, i)
		require.Nil(t, entry, fmt.Sprintf("Evicted index %d must return nil", i))
	}

	// Index >= InsertedEntryCount(): must return nil
	for i := table.InsertedEntryCount(); i < table.InsertedEntryCount()+20; i++ {
		entry := table.LookupEntry(false, i)
		require.Nil(t, entry, fmt.Sprintf("Future index %d must return nil", i))
	}

	// Resident indices: must return correct entry
	for i := table.DroppedEntryCount(); i < table.InsertedEntryCount(); i++ {
		entry := table.LookupEntry(false, i)
		require.NotNil(t, entry, fmt.Sprintf("Resident index %d must return entry", i))
		require.Equal(t, fmt.Sprintf("n-%d", i), entry.Name)
		require.Equal(t, fmt.Sprintf("v-%d", i), entry.Value)
	}

	// Static table bounds
	require.NotNil(t, table.LookupEntry(true, 0))
	require.NotNil(t, table.LookupEntry(true, 98))
	require.Nil(t, table.LookupEntry(true, 99))
	require.Nil(t, table.LookupEntry(true, 100))
	require.Nil(t, table.LookupEntry(true, math.MaxUint64))
}

// ----------------------------------------------------------------------------
// Test 13: SetMaximumDynamicTableCapacity Immutability and Reconfiguration
// ----------------------------------------------------------------------------
func TestAdversarial_HeaderTable_SetMaximumCapacityImmutability(t *testing.T) {
	table := NewEncoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(1024))
	require.Equal(t, uint64(1024), table.MaximumDynamicTableCapacity())
	require.Equal(t, uint64(32), table.MaxEntries())

	// Setting to the exact same value should succeed
	require.True(t, table.SetMaximumDynamicTableCapacity(1024))

	// Setting to a different value once initialized must return false (immutable)
	require.False(t, table.SetMaximumDynamicTableCapacity(2048))
	require.False(t, table.SetMaximumDynamicTableCapacity(512))
	require.False(t, table.SetMaximumDynamicTableCapacity(0))

	// Table capacity and maxEntries must remain unchanged
	require.Equal(t, uint64(1024), table.MaximumDynamicTableCapacity())
	require.Equal(t, uint64(32), table.MaxEntries())
}
