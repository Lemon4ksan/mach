// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import "iter"

// qpackRingBuffer is a circular FIFO queue of *Entry with O(1) push, pop,
// and random access indexing.
type qpackRingBuffer struct {
	entries []*Entry
	head    int
	count   int
}

func (rb *qpackRingBuffer) size() int {
	return rb.count
}

func (rb *qpackRingBuffer) Len() int {
	return rb.count
}

func (rb *qpackRingBuffer) empty() bool {
	return rb.count == 0
}

func (rb *qpackRingBuffer) IsEmpty() bool {
	return rb.count == 0
}

func (rb *qpackRingBuffer) front() *Entry {
	if rb.count == 0 {
		return nil
	}
	return rb.entries[rb.head]
}

func (rb *qpackRingBuffer) Front() *Entry {
	return rb.front()
}

func (rb *qpackRingBuffer) at(offset int) *Entry {
	if offset < 0 || offset >= rb.count {
		return nil
	}
	idx := (rb.head + offset) % len(rb.entries)
	return rb.entries[idx]
}

func (rb *qpackRingBuffer) At(offset int) *Entry {
	return rb.at(offset)
}

func (rb *qpackRingBuffer) pushBack(entry *Entry) {
	if len(rb.entries) == 0 {
		rb.entries = make([]*Entry, 8)
		rb.head = 0
		rb.count = 0
	} else if rb.count == len(rb.entries) {
		newCap := len(rb.entries) * 2
		newEntries := make([]*Entry, newCap)
		for i := 0; i < rb.count; i++ {
			newEntries[i] = rb.entries[(rb.head+i)%len(rb.entries)]
		}
		rb.entries = newEntries
		rb.head = 0
	}
	idx := (rb.head + rb.count) % len(rb.entries)
	rb.entries[idx] = entry
	rb.count++
}

func (rb *qpackRingBuffer) PushBack(entry *Entry) {
	rb.pushBack(entry)
}

func (rb *qpackRingBuffer) popFront() *Entry {
	if rb.count == 0 {
		return nil
	}
	entry := rb.entries[rb.head]
	rb.entries[rb.head] = nil // Avoid memory retention
	rb.head = (rb.head + 1) % len(rb.entries)
	rb.count--
	return entry
}

func (rb *qpackRingBuffer) PopFront() *Entry {
	return rb.popFront()
}

// HeaderTableBase is the base struct for encoder and decoder dynamic header tables.
// It manages capacity enforcement, byte tracking, dynamic entry storage, and FIFO eviction.
type HeaderTableBase struct {
	dynamicEntries              qpackRingBuffer
	dynamicTableSize            uint64
	dynamicTableCapacity        uint64
	maximumDynamicTableCapacity uint64
	maxEntries                  uint64
	droppedEntryCount           uint64
	dynamicTableEntryReferenced bool

	// removeEntryFromEndHook is invoked during eviction. It allows EncoderHeaderTable
	// to evict entries from its dynamic inverted hash maps prior to popping from dynamicEntries.
	removeEntryFromEndHook func()
}

func (t *HeaderTableBase) initBase() {
	t.dynamicTableSize = 0
	t.dynamicTableCapacity = 0
	t.maximumDynamicTableCapacity = 0
	t.maxEntries = 0
	t.droppedEntryCount = 0
	t.dynamicTableEntryReferenced = false
	t.removeEntryFromEndHook = t.baseRemoveEntryFromEnd
}

// EntryFitsDynamicTableCapacity returns whether an entry with name and value has a size
// (including 32 bytes overhead) smaller than or equal to the capacity of the dynamic table.
func (t *HeaderTableBase) EntryFitsDynamicTableCapacity(name, value string) bool {
	return EntrySize(name, value) <= t.dynamicTableCapacity
}

// InsertEntry inserts (name, value) into the dynamic table. Entry must not be larger than
// the capacity of the dynamic table. May evict older entries.
// Returns the zero-based absolute index of the inserted dynamic table entry.
func (t *HeaderTableBase) InsertEntry(name, value string) uint64 {
	entrySize := EntrySize(name, value)
	if entrySize > t.dynamicTableCapacity {
		panic("qpack: entry size exceeds dynamic table capacity")
	}

	index := t.droppedEntryCount + uint64(t.dynamicEntries.size())

	// Evict entries until there is sufficient capacity for the new entry.
	t.EvictDownToCapacity(t.dynamicTableCapacity - entrySize)

	newEntry := NewEntry(name, value)
	t.dynamicTableSize += entrySize
	t.dynamicEntries.pushBack(newEntry)

	return index
}

// SetDynamicTableCapacity changes the dynamic table capacity.
// Returns true on success. Returns false if capacity exceeds maximumDynamicTableCapacity.
func (t *HeaderTableBase) SetDynamicTableCapacity(capacity uint64) bool {
	if capacity > t.maximumDynamicTableCapacity {
		return false
	}
	t.dynamicTableCapacity = capacity
	t.EvictDownToCapacity(capacity)
	return true
}

// SetMaximumDynamicTableCapacity sets maximumDynamicTableCapacity and calculates maxEntries.
// Can be set once or remain unchanged. Returns true on success, false if attempting to change
// an already established non-zero maximum.
func (t *HeaderTableBase) SetMaximumDynamicTableCapacity(maximumDynamicTableCapacity uint64) bool {
	if t.maximumDynamicTableCapacity == 0 {
		t.maximumDynamicTableCapacity = maximumDynamicTableCapacity
		t.maxEntries = maximumDynamicTableCapacity / 32
		return true
	}
	return maximumDynamicTableCapacity == t.maximumDynamicTableCapacity
}

// EvictDownToCapacity evicts entries from the dynamic table until table size is <= capacity.
func (t *HeaderTableBase) EvictDownToCapacity(capacity uint64) {
	for t.dynamicTableSize > capacity {
		t.RemoveEntryFromEnd()
	}
}

// RemoveEntryFromEnd removes a single entry from the dynamic table via hook delegation.
func (t *HeaderTableBase) RemoveEntryFromEnd() {
	if t.removeEntryFromEndHook != nil {
		t.removeEntryFromEndHook()
	} else {
		t.baseRemoveEntryFromEnd()
	}
}

// baseRemoveEntryFromEnd performs the base FIFO removal of the oldest dynamic entry.
func (t *HeaderTableBase) baseRemoveEntryFromEnd() {
	entry := t.dynamicEntries.front()
	if entry == nil {
		return
	}
	entrySize := entry.Size()
	t.dynamicTableSize -= entrySize
	t.dynamicEntries.popFront()
	t.droppedEntryCount++
}

func (t *HeaderTableBase) DynamicTableSize() uint64 {
	return t.dynamicTableSize
}

func (t *HeaderTableBase) DynamicTableCapacity() uint64 {
	return t.dynamicTableCapacity
}

func (t *HeaderTableBase) MaximumDynamicTableCapacity() uint64 {
	return t.maximumDynamicTableCapacity
}

func (t *HeaderTableBase) MaxEntries() uint64 {
	return t.maxEntries
}

func (t *HeaderTableBase) InsertedEntryCount() uint64 {
	return t.droppedEntryCount + uint64(t.dynamicEntries.size())
}

func (t *HeaderTableBase) DroppedEntryCount() uint64 {
	return t.droppedEntryCount
}

func (t *HeaderTableBase) SetDynamicTableEntryReferenced() {
	t.dynamicTableEntryReferenced = true
}

func (t *HeaderTableBase) DynamicTableEntryReferenced() bool {
	return t.dynamicTableEntryReferenced
}

// MatchType represents the nature of a header table lookup match.
type MatchType int

const (
	MatchTypeNoMatch MatchType = iota
	MatchTypeName
	MatchTypeNameAndValue
)

// Chromium naming aliases for 1:1 unit test parity
const (
	MatchNoMatch      = MatchTypeNoMatch
	MatchName         = MatchTypeName
	MatchNameAndValue = MatchTypeNameAndValue

	// Deprecated: use MatchTypeNoMatch.
	KNoMatch = MatchTypeNoMatch
	// Deprecated: use MatchTypeName.
	KName = MatchTypeName
	// Deprecated: use MatchTypeNameAndValue.
	KNameAndValue = MatchTypeNameAndValue
)

// MatchResult describes the result of a header lookup.
type MatchResult struct {
	Match    MatchType
	IsStatic bool
	Index    uint64 // Zero-based absolute index
}

// qpackLookupEntry is used as key in the dynamic exact index map.
type qpackLookupEntry struct {
	name  string
	value string
}

// EncoderHeaderTable manages dynamic and static table lookups for the encoder.
type EncoderHeaderTable struct {
	HeaderTableBase

	dynamicIndex         map[qpackLookupEntry]uint64
	dynamicNameIndex     map[string]uint64
	smallestAllowedIndex uint64
}

// NewEncoderHeaderTable constructs an initialized EncoderHeaderTable.
func NewEncoderHeaderTable() *EncoderHeaderTable {
	t := &EncoderHeaderTable{
		dynamicIndex:     make(map[qpackLookupEntry]uint64),
		dynamicNameIndex: make(map[string]uint64),
	}
	t.initBase()
	t.removeEntryFromEndHook = t.removeEntryFromEndOverride
	return t
}

// InsertEntry inserts (name, value) into the dynamic table, updating inverted indices.
func (t *EncoderHeaderTable) InsertEntry(name, value string) uint64 {
	index := t.HeaderTableBase.InsertEntry(name, value)

	key := qpackLookupEntry{name: name, value: value}
	t.dynamicIndex[key] = index
	t.dynamicNameIndex[name] = index

	return index
}

func (t *EncoderHeaderTable) removeEntryFromEndOverride() {
	entry := t.dynamicEntries.front()
	if entry == nil {
		return
	}
	index := t.droppedEntryCount

	key := qpackLookupEntry{name: entry.Name, value: entry.Value}
	if idx, ok := t.dynamicIndex[key]; ok && idx == index {
		delete(t.dynamicIndex, key)
	}

	if idx, ok := t.dynamicNameIndex[entry.Name]; ok && idx == index {
		delete(t.dynamicNameIndex, entry.Name)
	}

	t.baseRemoveEntryFromEnd()
}

// FindHeaderField searches for (name, value) in static and dynamic tables.
func (t *EncoderHeaderTable) FindHeaderField(name, value string) MatchResult {
	// 1. Exact match in static table.
	if exactIdx, _, hasExact, _ := findStatic(name, value); hasExact {
		return MatchResult{Match: MatchTypeNameAndValue, IsStatic: true, Index: uint64(exactIdx)}
	}

	// 2. Exact match in dynamic table.
	key := qpackLookupEntry{name: name, value: value}
	if index, ok := t.dynamicIndex[key]; ok {
		return MatchResult{Match: MatchTypeNameAndValue, IsStatic: false, Index: index}
	}

	// 3. Name-only match fallback.
	return t.FindHeaderName(name)
}

// FindHeaderName searches for name in static and dynamic tables.
func (t *EncoderHeaderTable) FindHeaderName(name string) MatchResult {
	// 1. Name match in static table (findStatic returns lowest static index).
	if _, nameIdx, _, hasName := findStatic(name, ""); hasName {
		return MatchResult{Match: MatchTypeName, IsStatic: true, Index: uint64(nameIdx)}
	}

	// 2. Name match in dynamic table (dynamicNameIndex stores highest dynamic index).
	if index, ok := t.dynamicNameIndex[name]; ok {
		return MatchResult{Match: MatchTypeName, IsStatic: false, Index: index}
	}

	// 3. No match.
	return MatchResult{Match: MatchTypeNoMatch, IsStatic: false, Index: 0}
}

func (rb *qpackRingBuffer) Entries() iter.Seq[*Entry] {
	return func(yield func(*Entry) bool) {
		for i := 0; i < rb.count; i++ {
			if !yield(rb.entries[(rb.head+i)%len(rb.entries)]) {
				return
			}
		}
	}
}

// MaxInsertSizeWithoutEvictingGivenEntry calculates the maximum size in bytes that can be
// inserted into the dynamic table without evicting the entry with the given index.
func (t *EncoderHeaderTable) MaxInsertSizeWithoutEvictingGivenEntry(index uint64) uint64 {
	if index < t.droppedEntryCount {
		return 0
	}
	if index > t.InsertedEntryCount() {
		return t.dynamicTableCapacity
	}

	maxInsertSize := t.dynamicTableCapacity - t.dynamicTableSize
	entryIndex := t.droppedEntryCount
	for entry := range t.dynamicEntries.Entries() {
		if entryIndex >= index {
			break
		}
		entryIndex++
		maxInsertSize += entry.Size()
	}

	return maxInsertSize
}

// SetSmallestAllowedIndex sets the dynamic table eviction barrier index.
func (t *EncoderHeaderTable) SetSmallestAllowedIndex(index uint64) {
	t.smallestAllowedIndex = index
}

// SmallestAllowedIndex returns the dynamic table eviction barrier index.
func (t *EncoderHeaderTable) SmallestAllowedIndex() uint64 {
	return t.smallestAllowedIndex
}

// DrainingIndex calculates the threshold absolute index below which entries are draining.
func (t *EncoderHeaderTable) DrainingIndex(drainingFraction float64) uint64 {
	if drainingFraction < 0.0 {
		drainingFraction = 0.0
	} else if drainingFraction > 1.0 {
		drainingFraction = 1.0
	}

	requiredSpace := uint64(drainingFraction * float64(t.dynamicTableCapacity))
	spaceAboveDrainingIndex := t.dynamicTableCapacity - t.dynamicTableSize

	if t.dynamicEntries.empty() || spaceAboveDrainingIndex >= requiredSpace {
		return t.droppedEntryCount
	}

	entryIndex := t.droppedEntryCount
	for entry := range t.dynamicEntries.Entries() {
		spaceAboveDrainingIndex += entry.Size()
		entryIndex++
		if spaceAboveDrainingIndex >= requiredSpace {
			return entryIndex
		}
	}

	return t.InsertedEntryCount()
}

// Entries returns an iterator over dynamic table entries with their absolute index.
// Yields (absoluteIndex, *Entry) for each active dynamic entry in ascending order.
// Supports early termination when yield returns false.
// Executes with zero heap allocations.
func (t *EncoderHeaderTable) Entries() iter.Seq2[uint64, *Entry] {
	return func(yield func(uint64, *Entry) bool) {
		idx := t.droppedEntryCount
		for entry := range t.dynamicEntries.Entries() {
			if !yield(idx, entry) {
				return
			}
			idx++
		}
	}
}

// DecoderHeaderTableObserver is notified when the dynamic table insert count
// reaches a specified threshold, or when the table is cancelled/destroyed.
type DecoderHeaderTableObserver interface {
	OnInsertCountReachedThreshold()
	Cancel()
}

type observerEntry struct {
	requiredInsertCount uint64
	observer            DecoderHeaderTableObserver
}

// DecoderHeaderTable manages dynamic table lookups and blocking observers for the decoder.
type DecoderHeaderTable struct {
	HeaderTableBase

	observers []observerEntry
}

// NewDecoderHeaderTable constructs an initialized DecoderHeaderTable.
func NewDecoderHeaderTable() *DecoderHeaderTable {
	t := &DecoderHeaderTable{
		observers: make([]observerEntry, 0, 8),
	}
	t.initBase()
	return t
}

// InsertEntry inserts (name, value) into the dynamic table and notifies eligible observers.
func (t *DecoderHeaderTable) InsertEntry(name, value string) uint64 {
	index := t.HeaderTableBase.InsertEntry(name, value)

	insertedCount := t.InsertedEntryCount()
	for len(t.observers) > 0 {
		if t.observers[0].requiredInsertCount > insertedCount {
			break
		}
		observer := t.observers[0].observer
		t.observers = t.observers[1:]
		observer.OnInsertCountReachedThreshold()
	}

	return index
}

// RegisterObserver registers an observer to be notified when insertedEntryCount reaches requiredInsertCount.
func (t *DecoderHeaderTable) RegisterObserver(requiredInsertCount uint64, observer DecoderHeaderTableObserver) {
	if requiredInsertCount == 0 {
		panic("qpack: requiredInsertCount must be > 0")
	}

	entry := observerEntry{requiredInsertCount: requiredInsertCount, observer: observer}
	idx := len(t.observers)
	for i, o := range t.observers {
		if o.requiredInsertCount > requiredInsertCount {
			idx = i
			break
		}
	}
	t.observers = append(t.observers[:idx], append([]observerEntry{entry}, t.observers[idx:]...)...)
}

// UnregisterObserver removes a previously registered observer.
func (t *DecoderHeaderTable) UnregisterObserver(requiredInsertCount uint64, observer DecoderHeaderTableObserver) {
	for i, o := range t.observers {
		if o.requiredInsertCount == requiredInsertCount && o.observer == observer {
			t.observers = append(t.observers[:i], t.observers[i+1:]...)
			return
		}
	}
}

// Cancel cancels and deregisters all active observers.
func (t *DecoderHeaderTable) Cancel() {
	observers := t.observers
	t.observers = nil
	for _, o := range observers {
		o.observer.Cancel()
	}
}

// Close closes the decoder header table and cancels any remaining observers.
func (t *DecoderHeaderTable) Close() {
	t.Cancel()
}

// LookupEntry returns the entry at absolute index from the static or dynamic table according
// to isStatic. index is zero-based for both tables.
// Returns nil if the entry does not exist or has been evicted.
func (t *DecoderHeaderTable) LookupEntry(isStatic bool, index uint64) *Entry {
	if isStatic {
		if index >= uint64(len(staticEntries)) {
			return nil
		}
		return &staticEntries[index]
	}

	if index < t.droppedEntryCount {
		return nil
	}

	offset := index - t.droppedEntryCount
	if offset >= uint64(t.dynamicEntries.size()) {
		return nil
	}

	return t.dynamicEntries.at(int(offset))
}

// Entries returns an iterator over dynamic table entries with their absolute index.
// Yields (absoluteIndex, *Entry) for each active dynamic entry in ascending order.
// Supports early termination when yield returns false.
// Executes with zero heap allocations.
func (t *DecoderHeaderTable) Entries() iter.Seq2[uint64, *Entry] {
	return func(yield func(uint64, *Entry) bool) {
		idx := t.droppedEntryCount
		for entry := range t.dynamicEntries.Entries() {
			if !yield(idx, entry) {
				return
			}
			idx++
		}
	}
}

var staticEntries [99]Entry

func init() {
	for i, hf := range staticTable {
		staticEntries[i] = Entry(hf)
	}
}
