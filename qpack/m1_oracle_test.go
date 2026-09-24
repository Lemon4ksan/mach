// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// trackingObserver records call timestamps and actions for order verification.
type trackingObserver struct {
	id        string
	threshold uint64
	firedAt   int
	cancelled bool
	tracker   *[]string
	onFired   func()
}

func (o *trackingObserver) OnInsertCountReachedThreshold() {
	if o.tracker != nil {
		*o.tracker = append(*o.tracker, fmt.Sprintf("FIRED:%s:%d", o.id, o.threshold))
	}
	if o.onFired != nil {
		o.onFired()
	}
}

func (o *trackingObserver) Cancel() {
	o.cancelled = true
	if o.tracker != nil {
		*o.tracker = append(*o.tracker, fmt.Sprintf("CANCELLED:%s:%d", o.id, o.threshold))
	}
}

// ============================================================================
// Oracle Test 1: Dynamic Table Eviction with Identical Names & Distinct Values
// ============================================================================

func TestOracle_Eviction_IdenticalNamesDistinctValues(t *testing.T) {
	// Each entry: len("common-key") + len("val_X") + 32 = 10 + 5 + 32 = 47 bytes.
	// Capacity 47 * 3 = 141 bytes => can hold exactly 3 entries at a time.
	const entrySize = 47
	const capacity = entrySize * 3

	table := NewEncoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(capacity))
	require.True(t, table.SetDynamicTableCapacity(capacity))

	// Insert 3 entries with the same name "common-key" and distinct values.
	idx0 := table.InsertEntry("common-key", "val_0") // abs 0
	idx1 := table.InsertEntry("common-key", "val_1") // abs 1
	idx2 := table.InsertEntry("common-key", "val_2") // abs 2

	assert.Equal(t, uint64(0), idx0)
	assert.Equal(t, uint64(1), idx1)
	assert.Equal(t, uint64(2), idx2)
	assert.Equal(t, uint64(3), table.InsertedEntryCount())
	assert.Equal(t, uint64(0), table.DroppedEntryCount())

	// Exact matches should all resolve to their specific absolute indices.
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 0},
		table.FindHeaderField("common-key", "val_0"))
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 1},
		table.FindHeaderField("common-key", "val_1"))
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 2},
		table.FindHeaderField("common-key", "val_2"))

	// Name lookup must resolve to the MOST RECENT entry (index 2).
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 2},
		table.FindHeaderName("common-key"))
	// Non-matching value with matching name must fall back to most recent entry (index 2).
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 2},
		table.FindHeaderField("common-key", "val_unknown"))

	// Insert 4th entry: should evict entry 0 ("val_0").
	idx3 := table.InsertEntry("common-key", "val_3") // abs 3
	assert.Equal(t, uint64(3), idx3)
	assert.Equal(t, uint64(4), table.InsertedEntryCount())
	assert.Equal(t, uint64(1), table.DroppedEntryCount())

	// Entry 0 is evicted: exact match for "val_0" must fail and fall back to Name match with highest index (3).
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 3},
		table.FindHeaderField("common-key", "val_0"))

	// Remaining entries (1, 2, 3) must still match exactly.
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 1},
		table.FindHeaderField("common-key", "val_1"))
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 2},
		table.FindHeaderField("common-key", "val_2"))
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 3},
		table.FindHeaderField("common-key", "val_3"))

	// Most recent name index is now 3.
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 3},
		table.FindHeaderName("common-key"))

	// Insert entry with a DIFFERENT name "other-key" (same size 47 bytes).
	// This evicts entry 1 ("common-key", "val_1").
	idx4 := table.InsertEntry("other-key", "val_4") // abs 4
	assert.Equal(t, uint64(4), idx4)
	assert.Equal(t, uint64(2), table.DroppedEntryCount())

	// "common-key", "val_1" is evicted: fallback to Name match at index 3 ("common-key", "val_3").
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 3},
		table.FindHeaderField("common-key", "val_1"))
	// "common-key", "val_2" still in table (exact match at 2).
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 2},
		table.FindHeaderField("common-key", "val_2"))
	// "common-key", "val_3" still in table (exact match at 3).
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 3},
		table.FindHeaderField("common-key", "val_3"))
	// "other-key" exact match at 4.
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 4},
		table.FindHeaderField("other-key", "val_4"))
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 4},
		table.FindHeaderName("other-key"))

	// Evict entries 2 and 3 by inserting two more "other-key" entries.
	table.InsertEntry("other-key", "val_5") // abs 5, evicts 2 ("common-key", "val_2")
	table.InsertEntry("other-key", "val_6") // abs 6, evicts 3 ("common-key", "val_3")

	assert.Equal(t, uint64(4), table.DroppedEntryCount())

	// Now ALL "common-key" entries have been evicted.
	// Both FindHeaderField and FindHeaderName for "common-key" must return MatchTypeNoMatch!
	assert.Equal(t, MatchResult{Match: MatchTypeNoMatch, IsStatic: false, Index: 0},
		table.FindHeaderField("common-key", "val_0"))
	assert.Equal(t, MatchResult{Match: MatchTypeNoMatch, IsStatic: false, Index: 0},
		table.FindHeaderField("common-key", "val_2"))
	assert.Equal(t, MatchResult{Match: MatchTypeNoMatch, IsStatic: false, Index: 0},
		table.FindHeaderField("common-key", "val_3"))
	assert.Equal(t, MatchResult{Match: MatchTypeNoMatch, IsStatic: false, Index: 0},
		table.FindHeaderName("common-key"))

	// "other-key" has entries 4, 5, 6.
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 6},
		table.FindHeaderName("other-key"))
}

func TestOracle_Eviction_InterleavedIdenticalAndDifferent(t *testing.T) {
	// Test interleaved insertion of identical names, identical values, and distinct entries.
	const cap = 200
	table := NewEncoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(cap))
	require.True(t, table.SetDynamicTableCapacity(cap))

	// Insert sequence
	// 0: A:1 (size: 1+1+32=34)
	// 1: A:2 (size: 34)
	// 2: A:1 (size: 34) -> duplicate of 0!
	// 3: B:1 (size: 34)
	// 4: A:3 (size: 34)
	// Total size for 5 entries = 170 <= 200.
	table.InsertEntry("A", "1") // 0
	table.InsertEntry("A", "2") // 1
	table.InsertEntry("A", "1") // 2
	table.InsertEntry("B", "1") // 3
	table.InsertEntry("A", "3") // 4

	// Exact match for A:1 should return index 2 (most recent identical entry).
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 2},
		table.FindHeaderField("A", "1"))
	// Exact match for A:2 should return 1.
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 1},
		table.FindHeaderField("A", "2"))
	// Name match for A should return 4.
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 4},
		table.FindHeaderName("A"))

	// Insert 6th entry: C:1 (size 34). 170 + 34 = 204 > 200 -> evicts entry 0 (A:1).
	table.InsertEntry("C", "1") // 5
	assert.Equal(t, uint64(1), table.DroppedEntryCount())

	// Even though entry 0 (A:1) was evicted, A:1 still exists in table at index 2!
	// So FindHeaderField("A", "1") MUST still return exact match at index 2!
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 2},
		table.FindHeaderField("A", "1"))

	// Evict entries 1 and 2 by inserting two more entries.
	table.InsertEntry("C", "2") // 6, evicts 1 (A:2)
	table.InsertEntry("C", "3") // 7, evicts 2 (A:1)
	assert.Equal(t, uint64(3), table.DroppedEntryCount())

	// Now A:1 is evicted. A:2 is evicted. A:3 is still present at index 4!
	// A:1 query should fall back to Name match at index 4.
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 4},
		table.FindHeaderField("A", "1"))
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 4},
		table.FindHeaderField("A", "2"))
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 4},
		table.FindHeaderField("A", "3"))

	// Evict entry 3 (B:1) and entry 4 (A:3).
	table.InsertEntry("C", "4") // 8, evicts 3 (B:1)
	table.InsertEntry("C", "5") // 9, evicts 4 (A:3)
	assert.Equal(t, uint64(5), table.DroppedEntryCount())

	// Now A and B have NO entries in the table.
	assert.Equal(t, MatchResult{Match: MatchTypeNoMatch, IsStatic: false, Index: 0},
		table.FindHeaderName("A"))
	assert.Equal(t, MatchResult{Match: MatchTypeNoMatch, IsStatic: false, Index: 0},
		table.FindHeaderName("B"))
}

// ============================================================================
// Oracle Test 2: Match Results Comparison (FindHeaderField vs FindHeaderName)
// ============================================================================

func TestOracle_MatchResults_ComprehensiveComparison(t *testing.T) {
	table := NewEncoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(10000))
	require.True(t, table.SetDynamicTableCapacity(10000))

	// Case 1: Exact static match exists.
	// Static table has ":method: GET" at index 17. First ":method" is at index 15.
	fieldRes := table.FindHeaderField(":method", "GET")
	nameRes := table.FindHeaderName(":method")
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: true, Index: 17}, fieldRes)
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: true, Index: 15}, nameRes)
	// Field result gives exact index 17; Name result gives lowest static index 15.
	assert.True(t, fieldRes.Match == MatchTypeNameAndValue)
	assert.True(t, nameRes.Match == MatchTypeName)
	assert.NotEqual(t, fieldRes.Index, nameRes.Index)

	// Case 2: Static name match exists, but value does NOT match static, AND dynamic table has exact match.
	// Static has ":method", but not ":method: OPTIONS-CUSTOM".
	table.InsertEntry(":method", "OPTIONS-CUSTOM") // dynamic index 0
	fieldRes = table.FindHeaderField(":method", "OPTIONS-CUSTOM")
	nameRes = table.FindHeaderName(":method")
	// FindHeaderField prefers dynamic exact match (MatchTypeNameAndValue, isStatic: false, index: 0).
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 0}, fieldRes)
	// FindHeaderName prefers static name match (MatchTypeName, isStatic: true, index: 15).
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: true, Index: 15}, nameRes)

	// Case 3: Both static and dynamic have exact match.
	// Insert ":method: GET" into dynamic table.
	table.InsertEntry(":method", "GET") // dynamic index 1
	fieldRes = table.FindHeaderField(":method", "GET")
	nameRes = table.FindHeaderName(":method")
	// Static table exact match takes precedence over dynamic exact match.
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: true, Index: 17}, fieldRes)
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: true, Index: 15}, nameRes)

	// Case 4: Name in static table, value not in static and not in dynamic.
	fieldRes = table.FindHeaderField(":method", "NONEXISTENT-METHOD")
	nameRes = table.FindHeaderName(":method")
	// FindHeaderField falls back to FindHeaderName(":method") -> static name match at 15.
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: true, Index: 15}, fieldRes)
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: true, Index: 15}, nameRes)
	assert.Equal(t, fieldRes, nameRes)

	// Case 5: Name NOT in static table, multiple entries in dynamic table.
	table.InsertEntry("x-custom-metric", "alpha") // dynamic index 2
	table.InsertEntry("x-custom-metric", "beta")  // dynamic index 3
	table.InsertEntry("x-custom-metric", "gamma") // dynamic index 4

	// Exact matches for each:
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 2},
		table.FindHeaderField("x-custom-metric", "alpha"))
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 3},
		table.FindHeaderField("x-custom-metric", "beta"))
	assert.Equal(t, MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: 4},
		table.FindHeaderField("x-custom-metric", "gamma"))

	// Non-matching value for x-custom-metric:
	fieldRes = table.FindHeaderField("x-custom-metric", "delta")
	nameRes = table.FindHeaderName("x-custom-metric")
	// Both must return the MOST RECENT dynamic index (4).
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 4}, fieldRes)
	assert.Equal(t, MatchResult{Match: MatchTypeName, IsStatic: false, Index: 4}, nameRes)
	assert.Equal(t, fieldRes, nameRes)

	// Case 6: Name not in static and not in dynamic.
	fieldRes = table.FindHeaderField("completely-unknown", "whatever")
	nameRes = table.FindHeaderName("completely-unknown")
	assert.Equal(t, MatchResult{Match: MatchTypeNoMatch, IsStatic: false, Index: 0}, fieldRes)
	assert.Equal(t, MatchResult{Match: MatchTypeNoMatch, IsStatic: false, Index: 0}, nameRes)
	assert.Equal(t, fieldRes, nameRes)
}

// ============================================================================
// Oracle Test 3: Observer Threshold Notification Firing Order & Unregistration
// ============================================================================

func TestOracle_Observer_ThresholdFiringOrderAndStability(t *testing.T) {
	table := NewDecoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(10000))
	require.True(t, table.SetDynamicTableCapacity(10000))

	var tracker []string

	// Register observers with out-of-order thresholds and duplicates:
	// Threshold 5 (obs_5)
	// Threshold 2 (obs_2a)
	// Threshold 8 (obs_8)
	// Threshold 2 (obs_2b) - registered after 2a
	// Threshold 1 (obs_1)
	// Threshold 2 (obs_2c) - registered after 2b
	// Threshold 5 (obs_5b) - registered after 5
	obs5 := &trackingObserver{id: "obs_5", threshold: 5, tracker: &tracker}
	obs2a := &trackingObserver{id: "obs_2a", threshold: 2, tracker: &tracker}
	obs8 := &trackingObserver{id: "obs_8", threshold: 8, tracker: &tracker}
	obs2b := &trackingObserver{id: "obs_2b", threshold: 2, tracker: &tracker}
	obs1 := &trackingObserver{id: "obs_1", threshold: 1, tracker: &tracker}
	obs2c := &trackingObserver{id: "obs_2c", threshold: 2, tracker: &tracker}
	obs5b := &trackingObserver{id: "obs_5b", threshold: 5, tracker: &tracker}

	table.RegisterObserver(5, obs5)
	table.RegisterObserver(2, obs2a)
	table.RegisterObserver(8, obs8)
	table.RegisterObserver(2, obs2b)
	table.RegisterObserver(1, obs1)
	table.RegisterObserver(2, obs2c)
	table.RegisterObserver(5, obs5b)

	// Unregister obs2b before any insertion.
	table.UnregisterObserver(2, obs2b)

	// Insert 1: obs_1 should fire.
	table.InsertEntry("h", "1")
	require.Equal(t, []string{"FIRED:obs_1:1"}, tracker)

	// Insert 2: obs_2a and obs_2c should fire IN FIFO ORDER (obs2a then obs2c).
	// obs_2b must NOT fire because it was unregistered.
	table.InsertEntry("h", "2")
	require.Equal(t, []string{
		"FIRED:obs_1:1",
		"FIRED:obs_2a:2",
		"FIRED:obs_2c:2",
	}, tracker)

	// Insert 3 and 4: no observers should fire.
	table.InsertEntry("h", "3")
	table.InsertEntry("h", "4")
	assert.Equal(t, 3, len(tracker))

	// Insert 5: obs_5 and obs_5b should fire in registration order.
	table.InsertEntry("h", "5")
	require.Equal(t, []string{
		"FIRED:obs_1:1",
		"FIRED:obs_2a:2",
		"FIRED:obs_2c:2",
		"FIRED:obs_5:5",
		"FIRED:obs_5b:5",
	}, tracker)

	// Close table while obs_8 is still pending: obs_8 should receive Cancel().
	table.Close()
	require.Equal(t, []string{
		"FIRED:obs_1:1",
		"FIRED:obs_2a:2",
		"FIRED:obs_2c:2",
		"FIRED:obs_5:5",
		"FIRED:obs_5b:5",
		"CANCELLED:obs_8:8",
	}, tracker)
}

func TestOracle_Observer_DynamicReentrancy(t *testing.T) {
	// Verify that an observer registering or unregistering another observer
	// during OnInsertCountReachedThreshold callback operates cleanly without panics.
	table := NewDecoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(10000))
	require.True(t, table.SetDynamicTableCapacity(10000))

	var tracker []string

	obsVictim := &trackingObserver{id: "victim", threshold: 2, tracker: &tracker}
	obsSpawner := &trackingObserver{id: "spawner", threshold: 1, tracker: &tracker}
	obsFuture := &trackingObserver{id: "future", threshold: 3, tracker: &tracker}

	// When spawner fires at 1, it unregisters victim (threshold 2) and registers future (threshold 3).
	obsSpawner.onFired = func() {
		table.UnregisterObserver(2, obsVictim)
		table.RegisterObserver(3, obsFuture)
	}

	table.RegisterObserver(1, obsSpawner)
	table.RegisterObserver(2, obsVictim)

	// Insert 1: spawner fires, mutates observer list.
	table.InsertEntry("h", "1")
	require.Equal(t, []string{"FIRED:spawner:1"}, tracker)

	// Insert 2: victim was unregistered, so nothing fires.
	table.InsertEntry("h", "2")
	require.Equal(t, []string{"FIRED:spawner:1"}, tracker)

	// Insert 3: future fires!
	table.InsertEntry("h", "3")
	require.Equal(t, []string{"FIRED:spawner:1", "FIRED:future:3"}, tracker)
}

// ============================================================================
// Oracle Test 4: Property-Based Math Invariants (RIC and Index Conversions)
// ============================================================================

func TestOracle_Property_RequiredInsertCountRoundtrip(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	testCases := []struct {
		maxEntries uint64
	}{
		{maxEntries: 1},
		{maxEntries: 10},
		{maxEntries: 100},
		{maxEntries: 512},
		{maxEntries: 1024},
		{maxEntries: 4096},
	}

	for _, tc := range testCases {
		maxEntries := tc.maxEntries
		fullRange := 2 * maxEntries

		// 1. Check exact boundary around zero: RIC = 0.
		enc0 := EncodeRequiredInsertCount(0, maxEntries)
		assert.Equal(t, uint64(0), enc0)
		dec0, ok0 := DecodeRequiredInsertCount(enc0, maxEntries, 0)
		require.True(t, ok0)
		assert.Equal(t, uint64(0), dec0)

		// 2. Systematic verification across sliding window of totalNumberOfInserts:
		// For any RIC in [1, 1000], if totalNumberOfInserts is within [RIC - maxEntries, RIC + maxEntries - 1],
		// decoding the encoded RIC must yield the exact original RIC.
		for ric := uint64(1); ric <= 1000; ric++ {
			enc := EncodeRequiredInsertCount(ric, maxEntries)
			assert.True(t, enc >= 1 && enc <= fullRange, "enc out of [1, 2*maxEntries] range")

			// Choose totalNumberOfInserts valid under RFC 9204 §4.5.1.1:
			// totalNumberOfInserts can be anywhere within maxEntries distance from RIC.
			minTotal := uint64(0)
			if ric >= maxEntries {
				minTotal = ric - maxEntries
			}
			maxTotal := ric + maxEntries - 1

			// Test min, midpoint, and max totalNumberOfInserts
			totals := []uint64{minTotal, ric, maxTotal}
			for _, total := range totals {
				dec, ok := DecodeRequiredInsertCount(enc, maxEntries, total)
				require.True(
					t,
					ok,
					fmt.Sprintf("failed decoding ric=%d, maxEntries=%d, total=%d", ric, maxEntries, total),
				)
				require.Equal(
					t,
					ric,
					dec,
					fmt.Sprintf("mismatch decoding ric=%d, maxEntries=%d, total=%d", ric, maxEntries, total),
				)
			}
		}

		// 3. Fuzz random RIC and total combinations within the valid unwrap window:
		for i := 0; i < 2000; i++ {
			ric := uint64(rng.Int63n(1000000)) + 1
			delta := uint64(rng.Int63n(int64(maxEntries))) // delta in [0, maxEntries - 1]
			var total uint64
			if rng.Intn(2) == 0 {
				total = ric + delta
			} else {
				if ric > delta {
					total = ric - delta
				} else {
					total = 0
				}
			}

			enc := EncodeRequiredInsertCount(ric, maxEntries)
			dec, ok := DecodeRequiredInsertCount(enc, maxEntries, total)
			require.True(t, ok)
			require.Equal(t, ric, dec)
		}
	}
}

func TestOracle_Property_CoordinateIndexConversions(t *testing.T) {
	rng := rand.New(rand.NewSource(1337))

	// Encoder stream conversions:
	// absIndex in [0, insertedCount - 1]
	for i := 0; i < 5000; i++ {
		insertedCount := uint64(rng.Int63n(1000000)) + 1
		absIndex := uint64(rng.Int63n(int64(insertedCount)))

		relIndex := AbsoluteIndexToEncoderStreamRelativeIndex(absIndex, insertedCount)
		recoveredAbs, ok := EncoderStreamRelativeIndexToAbsoluteIndex(relIndex, insertedCount)
		require.True(t, ok)
		require.Equal(t, absIndex, recoveredAbs)
	}

	// Request stream relative conversions:
	// absIndex in [0, base - 1]
	for i := 0; i < 5000; i++ {
		base := uint64(rng.Int63n(1000000)) + 1
		absIndex := uint64(rng.Int63n(int64(base)))

		relIndex := AbsoluteIndexToRequestStreamRelativeIndex(absIndex, base)
		recoveredAbs, ok := RequestStreamRelativeIndexToAbsoluteIndex(relIndex, base)
		require.True(t, ok)
		require.Equal(t, absIndex, recoveredAbs)
	}

	// Post-base conversions:
	// absIndex >= base
	for i := 0; i < 5000; i++ {
		base := uint64(rng.Int63n(500000))
		delta := uint64(rng.Int63n(500000))
		absIndex := base + delta

		postBaseIndex := AbsoluteIndexToPostBaseIndex(absIndex, base)
		recoveredAbs, ok := PostBaseIndexToAbsoluteIndex(postBaseIndex, base)
		require.True(t, ok)
		require.Equal(t, absIndex, recoveredAbs)
	}
}

// ============================================================================
// Oracle Test 5: Invariants Under High Volume Dynamic Resizing & Churn
// ============================================================================

func TestOracle_Invariants_HighVolumeChurn(t *testing.T) {
	const maxCap = 2048
	table := NewEncoderHeaderTable()
	require.True(t, table.SetMaximumDynamicTableCapacity(maxCap))
	require.True(t, table.SetDynamicTableCapacity(maxCap))

	rng := rand.New(rand.NewSource(999))

	keys := []string{"content-type", "user-agent", "x-req-id", "cookie", "accept-encoding", "custom-header"}
	values := []string{"val-a", "val-b", "val-c", "val-d", "long-payload-string-for-sizing-variation"}

	for iter := 0; iter < 1000; iter++ {
		action := rng.Intn(10)
		if action < 8 {
			// Insert entry
			k := keys[rng.Intn(len(keys))]
			v := values[rng.Intn(len(values))]
			size := EntrySize(k, v)
			if size <= table.DynamicTableCapacity() {
				table.InsertEntry(k, v)
			}
		} else {
			// Resize capacity
			newCap := uint64(rng.Int63n(maxCap + 1))
			ok := table.SetDynamicTableCapacity(newCap)
			require.True(t, ok)
		}

		// Invariant checks on base table:
		assert.True(t, table.DynamicTableSize() <= table.DynamicTableCapacity(),
			fmt.Sprintf("size %d > cap %d", table.DynamicTableSize(), table.DynamicTableCapacity()))
		assert.Equal(t, table.DroppedEntryCount()+uint64(table.dynamicEntries.size()),
			table.InsertedEntryCount(), "count invariant violated")

		// DrainingIndex bounds check:
		dr0 := table.DrainingIndex(0.0)
		dr1 := table.DrainingIndex(1.0)
		drMid := table.DrainingIndex(0.5)
		assert.True(t, dr0 <= drMid && drMid <= dr1, "draining index monotonic property violated")
		assert.True(t, dr1 <= table.InsertedEntryCount(), "draining index exceeded inserted count")
		assert.True(t, dr0 >= table.DroppedEntryCount(), "draining index below dropped count")
	}
}

// ============================================================================
// Benchmarks: Performance & Throughput Measurement
// ============================================================================

func BenchmarkOracle_InsertAndEvict(b *testing.B) {
	const cap = 4096
	table := NewEncoderHeaderTable()
	table.SetMaximumDynamicTableCapacity(cap)
	table.SetDynamicTableCapacity(cap)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		k := "custom-header"
		v := "benchmark-value"
		table.InsertEntry(k, v)
	}
}

func BenchmarkOracle_FindHeaderField_StaticExact(b *testing.B) {
	table := NewEncoderHeaderTable()
	table.SetMaximumDynamicTableCapacity(4096)
	table.SetDynamicTableCapacity(4096)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = table.FindHeaderField(":method", "GET")
	}
}

func BenchmarkOracle_FindHeaderField_DynamicExact(b *testing.B) {
	table := NewEncoderHeaderTable()
	table.SetMaximumDynamicTableCapacity(4096)
	table.SetDynamicTableCapacity(4096)
	table.InsertEntry("x-custom-metric", "val-123")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = table.FindHeaderField("x-custom-metric", "val-123")
	}
}

func BenchmarkOracle_FindHeaderField_FallbackName(b *testing.B) {
	table := NewEncoderHeaderTable()
	table.SetMaximumDynamicTableCapacity(4096)
	table.SetDynamicTableCapacity(4096)
	table.InsertEntry("x-custom-metric", "val-123")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = table.FindHeaderField("x-custom-metric", "val-different")
	}
}

func BenchmarkOracle_RequiredInsertCount_Encode(b *testing.B) {
	const maxEntries = 512
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = EncodeRequiredInsertCount(uint64(i+1), maxEntries)
	}
}

func BenchmarkOracle_RequiredInsertCount_Decode(b *testing.B) {
	const maxEntries = 512
	enc := EncodeRequiredInsertCount(100, maxEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = DecodeRequiredInsertCount(enc, maxEntries, 105)
	}
}

func BenchmarkOracle_CoordinateConversions(b *testing.B) {
	const insertedCount uint64 = 1000
	const base uint64 = 500

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rel := AbsoluteIndexToRequestStreamRelativeIndex(450, base)
		_, _ = RequestStreamRelativeIndexToAbsoluteIndex(rel, base)
	}
}
