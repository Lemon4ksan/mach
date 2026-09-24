// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"iter"
	"math"
)

// IndexSet tracks the minimum and maximum dynamic table indices referenced in a header block (RFC 9204).
type IndexSet struct {
	indices  []uint64
	minIndex uint64
	maxIndex uint64
	hasIndex bool
}

// NewIndexSet constructs an empty IndexSet.
func NewIndexSet() *IndexSet {
	return &IndexSet{}
}

// Empty returns true if no indices have been inserted into the set.
func (s *IndexSet) Empty() bool {
	return !s.hasIndex
}

// Size returns the number of indices inserted into the set.
func (s *IndexSet) Size() int {
	return len(s.indices)
}

// MinIndex returns the lowest dynamic index inserted, or (0, false) if empty.
func (s *IndexSet) MinIndex() (uint64, bool) {
	if !s.hasIndex {
		return 0, false
	}
	return s.minIndex, true
}

// MaxIndex returns the highest dynamic index inserted, or (0, false) if empty.
func (s *IndexSet) MaxIndex() (uint64, bool) {
	if !s.hasIndex {
		return 0, false
	}
	return s.maxIndex, true
}

// RequiredInsertCount returns the required dynamic table insert count (max_index + 1), or 0 if empty.
func (s *IndexSet) RequiredInsertCount() uint64 {
	if !s.hasIndex {
		return 0
	}
	return s.maxIndex + 1
}

// Insert adds a dynamic table index to the set and updates min/max bounds.
func (s *IndexSet) Insert(index uint64) {
	s.indices = append(s.indices, index)
	if !s.hasIndex {
		s.minIndex = index
		s.maxIndex = index
		s.hasIndex = true
	} else {
		if index < s.minIndex {
			s.minIndex = index
		}
		if index > s.maxIndex {
			s.maxIndex = index
		}
	}
}

// Indices returns the slice of all inserted dynamic table indices.
func (s *IndexSet) Indices() []uint64 {
	return s.indices
}

// All returns a push iterator over all dynamic indices in the set in insertion order.
// Supports early termination when yield returns false.
// Executes with zero heap allocations.
func (s *IndexSet) All() iter.Seq[uint64] {
	return func(yield func(uint64) bool) {
		for _, idx := range s.indices {
			if !yield(idx) {
				return
			}
		}
	}
}

// HeaderData represents unacknowledged dynamic table references for a single header block (RFC 9204).
type HeaderData struct {
	indices             []uint64
	requiredInsertCount uint64
	minIndex            uint64
	hasMinIndex         bool
}

// NewHeaderData creates HeaderData from a slice of dynamic table indices (encoder path).
func NewHeaderData(indices []uint64) HeaderData {
	if len(indices) == 0 {
		return HeaderData{
			indices:             nil,
			requiredInsertCount: 0,
			minIndex:            0,
			hasMinIndex:         false,
		}
	}
	minIdx := indices[0]
	maxIdx := indices[0]
	for _, idx := range indices[1:] {
		if idx < minIdx {
			minIdx = idx
		}
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	return HeaderData{
		indices:             indices,
		requiredInsertCount: maxIdx + 1,
		minIndex:            minIdx,
		hasMinIndex:         true,
	}
}

// NewHeaderDataWithRIC creates HeaderData directly from Required Insert Count (decoder path).
func NewHeaderDataWithRIC(requiredInsertCount uint64) HeaderData {
	return HeaderData{
		indices:             nil,
		requiredInsertCount: requiredInsertCount,
		minIndex:            0,
		hasMinIndex:         false,
	}
}

// Indices returns the referenced dynamic table indices.
func (h HeaderData) Indices() []uint64 {
	return h.indices
}

// RequiredInsertCount returns the required dynamic table insert count.
func (h HeaderData) RequiredInsertCount() uint64 {
	return h.requiredInsertCount
}

// MinIndex returns the minimum referenced index, or (0, false) if none.
func (h HeaderData) MinIndex() (uint64, bool) {
	return h.minIndex, h.hasMinIndex
}

// BlockingManager manages blocked streams, in-flight header block dependencies,
// dynamic table eviction barriers, and known received counts per RFC 9204.
type BlockingManager struct {
	// streamMap maps stream ID -> FIFO queue of unacknowledged HeaderData
	streamMap map[uint64][]HeaderData

	// blockedStreams is the set of stream IDs currently blocked
	blockedStreams map[uint64]struct{}

	// minIndexRefCounts tracks reference counts of min_index across all in-flight blocks
	minIndexRefCounts map[uint64]uint64

	// smallestBlockingIndex caches the lowest min_index, or math.MaxUint64 if none
	smallestBlockingIndex uint64

	// knownReceivedCount is the peer's known dynamic table insert count
	knownReceivedCount uint64

	// maxBlockedStreams is the maximum allowed concurrent blocked streams (SETTINGS_QPACK_BLOCKED_STREAMS)
	maxBlockedStreams uint64
}

// Note: In Go, NewBlockingManager is the standard constructor function for BlockingManager.

// NewBlockingManager constructs an initialized BlockingManager.
// An optional argument specifies maxBlockedStreams.
func NewBlockingManager(maxBlockedStreams ...uint64) *BlockingManager {
	maxVal := uint64(0)
	if len(maxBlockedStreams) > 0 {
		maxVal = maxBlockedStreams[0]
	}
	return &BlockingManager{
		streamMap:             make(map[uint64][]HeaderData),
		blockedStreams:        make(map[uint64]struct{}),
		minIndexRefCounts:     make(map[uint64]uint64),
		smallestBlockingIndex: math.MaxUint64,
		knownReceivedCount:    0,
		maxBlockedStreams:     maxVal,
	}
}

// NewBlockingManagerWithMax constructs a manager with an explicit maxBlockedStreams limit.
func NewBlockingManagerWithMax(maxBlockedStreams uint64) *BlockingManager {
	return NewBlockingManager(maxBlockedStreams)
}

// OnHeaderBlockSent is called by the encoder when transmitting a header block on streamId.
func (m *BlockingManager) OnHeaderBlockSent(streamId uint64, indices []uint64) {
	headerData := NewHeaderData(indices)
	m.streamMap[streamId] = append(m.streamMap[streamId], headerData)

	if headerData.hasMinIndex {
		m.addMinIndex(headerData.minIndex)
	}

	if headerData.requiredInsertCount > m.knownReceivedCount {
		m.blockedStreams[streamId] = struct{}{}
	}
}

// OnHeaderBlockReceived is called by the decoder when parsing a header block with requiredInsertCount.
// It returns whether streamId is blocked.
func (m *BlockingManager) OnHeaderBlockReceived(streamId, requiredInsertCount uint64) bool {
	headerData := NewHeaderDataWithRIC(requiredInsertCount)
	m.streamMap[streamId] = append(m.streamMap[streamId], headerData)

	if requiredInsertCount > m.knownReceivedCount {
		m.blockedStreams[streamId] = struct{}{}
	}
	_, blocked := m.blockedStreams[streamId]
	return blocked
}

// OnInsertCountIncrement is called when an Insert Count Increment instruction is received.
// It updates knownReceivedCount and unblocks streams whose dependencies are satisfied.
// Returns false if increment causes uint64 overflow.
func (m *BlockingManager) OnInsertCountIncrement(increment uint64) bool {
	if math.MaxUint64-m.knownReceivedCount < increment {
		return false
	}
	m.knownReceivedCount += increment
	m.updateBlockedStreams()
	return true
}

// OnSectionAck is called when a Section Acknowledgment instruction is received for streamId.
// It pops the oldest in-flight header block, updates knownReceivedCount, and unblocks streams.
// Returns false if streamId has no outstanding header blocks.
func (m *BlockingManager) OnSectionAck(streamId uint64) bool {
	return m.OnHeaderAcknowledgement(streamId)
}

// OnHeaderAcknowledgement is an alias for OnSectionAck (RFC 9204 §4.4.1).
func (m *BlockingManager) OnHeaderAcknowledgement(streamId uint64) bool {
	blocks, ok := m.streamMap[streamId]
	if !ok || len(blocks) == 0 {
		return false
	}

	popped := blocks[0]
	if len(blocks) == 1 {
		delete(m.streamMap, streamId)
	} else {
		m.streamMap[streamId] = blocks[1:]
	}

	if popped.hasMinIndex {
		m.removeMinIndex(popped.minIndex)
	}

	if popped.requiredInsertCount > m.knownReceivedCount {
		m.knownReceivedCount = popped.requiredInsertCount
		m.updateBlockedStreams()
	} else if !m.isStreamBlocked(streamId) {
		delete(m.blockedStreams, streamId)
	}

	return true
}

// OnStreamCancellation is called when a Stream Cancellation instruction is received for streamId.
// It removes all outstanding header blocks for the stream without advancing knownReceivedCount.
func (m *BlockingManager) OnStreamCancellation(streamId uint64) {
	blocks, ok := m.streamMap[streamId]
	if ok {
		for i := range blocks {
			if blocks[i].hasMinIndex {
				m.removeMinIndex(blocks[i].minIndex)
			}
		}
		delete(m.streamMap, streamId)
	}
	delete(m.blockedStreams, streamId)
}

// SmallestBlockingIndex returns the minimum index among all unacknowledged dynamic references,
// or math.MaxUint64 if there are no in-flight dynamic table references.
func (m *BlockingManager) SmallestBlockingIndex() uint64 {
	return m.smallestBlockingIndex
}

// smallest_blocking_index provides exact C++ Chromium method naming.
//
// Deprecated: use SmallestBlockingIndex.
func (m *BlockingManager) smallest_blocking_index() uint64 {
	return m.SmallestBlockingIndex()
}

// KnownReceivedCount returns the current known dynamic table insert count acknowledged by peer.
func (m *BlockingManager) KnownReceivedCount() uint64 {
	return m.knownReceivedCount
}

// known_received_count provides exact C++ Chromium method naming.
//
// Deprecated: use KnownReceivedCount.
func (m *BlockingManager) known_received_count() uint64 {
	return m.KnownReceivedCount()
}

// IsBlocked returns true if streamId has any unacknowledged header blocks requiring RIC > knownReceivedCount.
func (m *BlockingManager) IsBlocked(streamId uint64) bool {
	_, blocked := m.blockedStreams[streamId]
	return blocked
}

// is_blocked provides exact C++ Chromium method naming.
//
// Deprecated: use IsBlocked.
func (m *BlockingManager) is_blocked(streamId uint64) bool {
	return m.IsBlocked(streamId)
}

// NumBlockedStreams returns the number of currently blocked streams.
func (m *BlockingManager) NumBlockedStreams() uint64 {
	return uint64(len(m.blockedStreams))
}

// num_blocked_streams provides exact C++ Chromium method naming.
//
// Deprecated: use NumBlockedStreams.
func (m *BlockingManager) num_blocked_streams() uint64 {
	return m.NumBlockedStreams()
}

// BlockedStreams returns a push iterator over all currently blocked stream IDs.
// Supports early termination when yield returns false.
// Executes with zero heap allocations.
func (m *BlockingManager) BlockedStreams() iter.Seq[uint64] {
	return func(yield func(uint64) bool) {
		for streamID := range m.blockedStreams {
			if !yield(streamID) {
				return
			}
		}
	}
}

// MaxBlockedStreams returns the maximum allowed concurrent blocked streams limit.
func (m *BlockingManager) MaxBlockedStreams() uint64 {
	return m.maxBlockedStreams
}

// SetMaxBlockedStreams updates the maximum allowed concurrent blocked streams limit.
func (m *BlockingManager) SetMaxBlockedStreams(maxBlockedStreams uint64) {
	m.maxBlockedStreams = maxBlockedStreams
}

// BlockingAllowedOnStream returns true if sending blocking references on streamId is permitted.
// If streamId is already blocked, it is always permitted (blocked streams count does not increase).
// Otherwise, it is permitted if NumBlockedStreams() < maxBlockedStreams.
func (m *BlockingManager) BlockingAllowedOnStream(streamId uint64, maxBlockedStreams ...uint64) bool {
	maxBlocked := m.maxBlockedStreams
	if len(maxBlockedStreams) > 0 {
		maxBlocked = maxBlockedStreams[0]
	}
	if m.IsBlocked(streamId) {
		return true
	}
	return uint64(len(m.blockedStreams)) < maxBlocked
}

// blocking_allowed_on_stream provides exact C++ Chromium method naming.
//
// Deprecated: use BlockingAllowedOnStream.
func (m *BlockingManager) blocking_allowed_on_stream(streamId uint64, maxBlockedStreams ...uint64) bool {
	return m.BlockingAllowedOnStream(streamId, maxBlockedStreams...)
}

// isStreamBlocked checks whether streamId currently has any block with RIC > knownReceivedCount.
func (m *BlockingManager) isStreamBlocked(streamId uint64) bool {
	blocks, ok := m.streamMap[streamId]
	if !ok || len(blocks) == 0 {
		return false
	}
	for i := range blocks {
		if blocks[i].requiredInsertCount > m.knownReceivedCount {
			return true
		}
	}
	return false
}

// updateBlockedStreams scans all currently blocked streams and removes any whose blocks are now satisfied.
func (m *BlockingManager) updateBlockedStreams() {
	for streamId := range m.blockedStreams {
		if !m.isStreamBlocked(streamId) {
			delete(m.blockedStreams, streamId)
		}
	}
}

func (m *BlockingManager) addMinIndex(index uint64) {
	m.minIndexRefCounts[index]++
	if index < m.smallestBlockingIndex {
		m.smallestBlockingIndex = index
	}
}

func (m *BlockingManager) removeMinIndex(index uint64) {
	count, ok := m.minIndexRefCounts[index]
	if !ok {
		return
	}
	if count <= 1 {
		delete(m.minIndexRefCounts, index)
		if index == m.smallestBlockingIndex {
			m.recomputeSmallestBlockingIndex()
		}
	} else {
		m.minIndexRefCounts[index] = count - 1
	}
}

func (m *BlockingManager) recomputeSmallestBlockingIndex() {
	if len(m.minIndexRefCounts) == 0 {
		m.smallestBlockingIndex = math.MaxUint64
		return
	}
	minVal := uint64(math.MaxUint64)
	for idx := range m.minIndexRefCounts {
		if idx < minVal {
			minVal = idx
		}
	}
	m.smallestBlockingIndex = minVal
}
