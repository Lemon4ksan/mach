// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// =============================================================================
// Custom Adversarial Mock Delegates for Milestone 4
// =============================================================================

type advM4ErrorCall struct {
	ErrorCode    uint64
	ErrorMessage string
}

type advM4ErrorDelegate struct {
	mu    sync.Mutex
	calls []advM4ErrorCall
}

func newAdvM4ErrorDelegate() *advM4ErrorDelegate {
	return &advM4ErrorDelegate{
		calls: make([]advM4ErrorCall, 0),
	}
}

func (d *advM4ErrorDelegate) OnDecoderStreamError(errorCode uint64, errorMessage string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, advM4ErrorCall{ErrorCode: errorCode, ErrorMessage: errorMessage})
}

func (d *advM4ErrorDelegate) Calls() []advM4ErrorCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	cp := make([]advM4ErrorCall, len(d.calls))
	copy(cp, d.calls)
	return cp
}

func (d *advM4ErrorDelegate) Count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.calls)
}

func (d *advM4ErrorDelegate) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = nil
}

type advM4SenderDelegate struct {
	mu               sync.Mutex
	numBytesBuffered uint64
	writes           [][]byte
}

func newAdvM4SenderDelegate() *advM4SenderDelegate {
	return &advM4SenderDelegate{
		writes: make([][]byte, 0),
	}
}

func (d *advM4SenderDelegate) WriteStreamData(data []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	d.writes = append(d.writes, cp)
}

func (d *advM4SenderDelegate) NumBytesBuffered() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.numBytesBuffered
}

func (d *advM4SenderDelegate) SetNumBytesBuffered(n uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.numBytesBuffered = n
}

func (d *advM4SenderDelegate) ClearWrites() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writes = nil
}

func (d *advM4SenderDelegate) TotalWritten() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	var total []byte
	for _, w := range d.writes {
		total = append(total, w...)
	}
	return total
}

func (d *advM4SenderDelegate) WritesCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.writes)
}

// =============================================================================
// Independent Bit-Level RFC 9204 Decoder Oracle
// =============================================================================

// decodeOracleVarint decodes an RFC 9204 / RFC 7541 variable length integer with N prefix bits.
func decodeOracleVarint(data []byte, offset *int, prefixBits uint8) (uint64, error) {
	if *offset >= len(data) {
		return 0, fmt.Errorf("oracle: unexpected EOF reading varint prefix at offset %d", *offset)
	}
	maxPrefix := byte((1 << prefixBits) - 1)
	prefixVal := uint64(data[*offset] & maxPrefix)
	*offset++

	if prefixVal < uint64(maxPrefix) {
		return prefixVal, nil
	}

	val := prefixVal
	shift := uint(0)
	for {
		if *offset >= len(data) {
			return 0, fmt.Errorf("oracle: unexpected EOF reading varint continuation at offset %d", *offset)
		}
		b := data[*offset]
		*offset++

		if shift >= 64 {
			return 0, fmt.Errorf("oracle: varint integer overflow (>64 bits)")
		}
		val += uint64(b&0x7f) << shift
		shift += 7

		if b&0x80 == 0 {
			break
		}
	}
	return val, nil
}

// decodeOracleString decodes a string literal (Huffman or raw) with N prefix bits for length.
func decodeOracleString(data []byte, offset *int, prefixBits uint8, huffmanBitMask byte) (string, error) {
	if *offset >= len(data) {
		return "", fmt.Errorf("oracle: unexpected EOF reading string literal at offset %d", *offset)
	}
	isHuffman := (data[*offset] & huffmanBitMask) != 0
	strLen, err := decodeOracleVarint(data, offset, prefixBits)
	if err != nil {
		return "", err
	}
	if uint64(*offset)+strLen > uint64(len(data)) {
		return "", fmt.Errorf("oracle: string length %d exceeds remaining buffer %d", strLen, len(data)-*offset)
	}
	raw := data[*offset : *offset+int(strLen)]
	*offset += int(strLen)

	if isHuffman {
		decoded, err := decodeHuffman(raw, nil)
		if err != nil {
			return "", fmt.Errorf("oracle: huffman decode error: %w", err)
		}
		return decoded, nil
	}
	return string(raw), nil
}

// OracleDynamicTable independently models RFC 9204 dynamic table mutations.
type OracleDynamicTable struct {
	entries      []Entry
	capacity     uint64
	maxCapacity  uint64
	currentSize  uint64
	totalInserts uint64
	droppedCount uint64
}

func newOracleDynamicTable(maxCapacity uint64) *OracleDynamicTable {
	return &OracleDynamicTable{
		entries:     make([]Entry, 0),
		capacity:    0,
		maxCapacity: maxCapacity,
	}
}

func (t *OracleDynamicTable) SetCapacity(capacity uint64) error {
	if capacity > t.maxCapacity {
		return fmt.Errorf("oracle: capacity %d exceeds max capacity %d", capacity, t.maxCapacity)
	}
	t.capacity = capacity
	for t.currentSize > t.capacity && len(t.entries) > 0 {
		oldest := t.entries[0]
		sz := EntrySize(oldest.Name, oldest.Value)
		t.currentSize -= sz
		t.entries = t.entries[1:]
		t.droppedCount++
	}
	return nil
}

func (t *OracleDynamicTable) Insert(name, value string) {
	entrySize := EntrySize(name, value)
	if entrySize > t.capacity {
		// All entries evicted, new entry cannot fit
		t.droppedCount += uint64(len(t.entries))
		t.entries = nil
		t.currentSize = 0
		return
	}
	for t.currentSize+entrySize > t.capacity && len(t.entries) > 0 {
		oldest := t.entries[0]
		sz := EntrySize(oldest.Name, oldest.Value)
		t.currentSize -= sz
		t.entries = t.entries[1:]
		t.droppedCount++
	}
	t.entries = append(t.entries, Entry{Name: name, Value: value})
	t.currentSize += entrySize
	t.totalInserts++
}

// FeedEncoderStream parses instructions emitted on the encoder stream to keep table synchronized.
func (t *OracleDynamicTable) FeedEncoderStream(data []byte) error {
	offset := 0
	for offset < len(data) {
		b := data[offset]
		switch {
		case b&0x80 != 0:
			// Insert With Name Reference (RFC 9204 §4.3.1): [1 T NNNNNN] [H VVVVVVV]
			isStatic := (b & 0x40) != 0
			nameIndex, err := decodeOracleVarint(data, &offset, 6)
			if err != nil {
				return err
			}
			val, err := decodeOracleString(data, &offset, 7, 0x80)
			if err != nil {
				return err
			}
			var name string
			if isStatic {
				if nameIndex >= uint64(len(staticTable)) {
					return fmt.Errorf("oracle: invalid static name index %d", nameIndex)
				}
				name = staticTable[nameIndex].Name
			} else {
				if nameIndex >= t.totalInserts {
					return fmt.Errorf(
						"oracle: invalid dynamic name relative index %d with total %d",
						nameIndex,
						t.totalInserts,
					)
				}
				absIndex := t.totalInserts - nameIndex - 1
				if absIndex < t.droppedCount {
					return fmt.Errorf(
						"oracle: dynamic name entry %d has been evicted (dropped %d)",
						absIndex,
						t.droppedCount,
					)
				}
				name = t.entries[absIndex-t.droppedCount].Name
			}
			t.Insert(name, val)

		case (b & 0xc0) == 0x40:
			// Insert Without Name Reference (RFC 9204 §4.3.2): [01 H NNNNN] [H VVVVVVV]
			name, err := decodeOracleString(data, &offset, 5, 0x20)
			if err != nil {
				return err
			}
			val, err := decodeOracleString(data, &offset, 7, 0x80)
			if err != nil {
				return err
			}
			t.Insert(name, val)

		case (b & 0xe0) == 0x20:
			// Set Dynamic Table Capacity (RFC 9204 §4.3.4): [001 CCCCC]
			cap, err := decodeOracleVarint(data, &offset, 5)
			if err != nil {
				return err
			}
			if err := t.SetCapacity(cap); err != nil {
				return err
			}

		case (b & 0xe0) == 0x00:
			// Duplicate (RFC 9204 §4.3.3): [000 NNNNN]
			relIndex, err := decodeOracleVarint(data, &offset, 5)
			if err != nil {
				return err
			}
			if relIndex >= t.totalInserts {
				return fmt.Errorf("oracle: invalid duplicate relative index %d with total %d", relIndex, t.totalInserts)
			}
			absIndex := t.totalInserts - relIndex - 1
			if absIndex < t.droppedCount {
				return fmt.Errorf("oracle: duplicate entry %d has been evicted", absIndex)
			}
			entry := t.entries[absIndex-t.droppedCount]
			t.Insert(entry.Name, entry.Value)

		default:
			return fmt.Errorf("oracle: unknown opcode 0x%02x on encoder stream at offset %d", b, offset)
		}
	}
	return nil
}

// DecodeHeaderBlock parses an RFC 9204 request stream header block using the current dynamic table.
func (t *OracleDynamicTable) DecodeHeaderBlock(data []byte) ([]HeaderField, uint64, uint64, error) {
	if len(data) < 2 {
		return nil, 0, 0, fmt.Errorf("oracle: header block too short (%d bytes)", len(data))
	}
	offset := 0

	// 1. Header Block Prefix (RFC 9204 §4.5.1)
	encodedRIC, err := decodeOracleVarint(data, &offset, 8)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("oracle: failed to decode encoded RIC: %w", err)
	}

	if offset >= len(data) {
		return nil, 0, 0, fmt.Errorf("oracle: unexpected EOF reading S bit and Delta Base")
	}
	sBit := (data[offset] & 0x80) != 0
	deltaBase, err := decodeOracleVarint(data, &offset, 7)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("oracle: failed to decode Delta Base: %w", err)
	}

	var ric uint64
	if encodedRIC != 0 {
		maxEntries := t.capacity / 32
		var ok bool
		ric, ok = DecodeRequiredInsertCount(encodedRIC, maxEntries, t.totalInserts)
		if !ok {
			return nil, 0, 0, fmt.Errorf(
				"oracle: failed to decode Required Insert Count (encoded %d, maxEntries %d, total %d)",
				encodedRIC,
				maxEntries,
				t.totalInserts,
			)
		}
	}

	var base uint64
	if !sBit {
		base = ric + deltaBase
	} else {
		if deltaBase >= ric {
			return nil, 0, 0, fmt.Errorf("oracle: underflow computing base (ric %d, deltaBase %d)", ric, deltaBase)
		}
		base = ric - deltaBase - 1
	}

	// 2. Field Lines
	var result []HeaderField
	for offset < len(data) {
		b := data[offset]
		switch {
		case b&0x80 != 0:
			// Indexed Header Field (RFC 9204 §4.5.2): [1 T NNNNNN]
			isStatic := (b & 0x40) != 0
			idx, err := decodeOracleVarint(data, &offset, 6)
			if err != nil {
				return nil, 0, 0, err
			}
			if isStatic {
				if idx >= uint64(len(staticTable)) {
					return nil, 0, 0, fmt.Errorf("oracle: invalid static index %d", idx)
				}
				result = append(result, staticTable[idx])
			} else {
				relIndex := idx
				if relIndex >= base {
					return nil, 0, 0, fmt.Errorf("oracle: dynamic relative index %d >= base %d", relIndex, base)
				}
				absIndex := base - relIndex - 1
				if absIndex < t.droppedCount || absIndex >= t.totalInserts {
					return nil, 0, 0, fmt.Errorf("oracle: dynamic index %d out of bounds [%d, %d)",
						absIndex, t.droppedCount, t.totalInserts)
				}
				entry := t.entries[absIndex-t.droppedCount]
				result = append(result, HeaderField{Name: entry.Name, Value: entry.Value})
			}

		case (b & 0xc0) == 0x40:
			// Literal Header Field With Name Reference (RFC 9204 §4.5.4): [01 N T NNNN] [H VVVVVVV]
			isStatic := (b & 0x10) != 0
			nameIdx, err := decodeOracleVarint(data, &offset, 4)
			if err != nil {
				return nil, 0, 0, err
			}
			val, err := decodeOracleString(data, &offset, 7, 0x80)
			if err != nil {
				return nil, 0, 0, err
			}
			var name string
			if isStatic {
				if nameIdx >= uint64(len(staticTable)) {
					return nil, 0, 0, fmt.Errorf("oracle: invalid static name index %d", nameIdx)
				}
				name = staticTable[nameIdx].Name
			} else {
				relIndex := nameIdx
				if relIndex >= base {
					return nil, 0, 0, fmt.Errorf("oracle: dynamic relative name index %d >= base %d", relIndex, base)
				}
				absIndex := base - relIndex - 1
				if absIndex < t.droppedCount || absIndex >= t.totalInserts {
					return nil, 0, 0, fmt.Errorf("oracle: dynamic name index %d out of bounds [%d, %d)",
						absIndex, t.droppedCount, t.totalInserts)
				}
				name = t.entries[absIndex-t.droppedCount].Name
			}
			result = append(result, HeaderField{Name: name, Value: val})

		case (b & 0xe0) == 0x20:
			// Literal Header Field Without Name Reference (RFC 9204 §4.5.6): [001 N H NNN] [H VVVVVVV]
			name, err := decodeOracleString(data, &offset, 3, 0x08)
			if err != nil {
				return nil, 0, 0, err
			}
			val, err := decodeOracleString(data, &offset, 7, 0x80)
			if err != nil {
				return nil, 0, 0, err
			}
			result = append(result, HeaderField{Name: name, Value: val})

		case (b & 0xf0) == 0x10:
			// Indexed Header Field With Post-Base Index (RFC 9204 §4.5.3): [0001 NNNN]
			postBaseIndex, err := decodeOracleVarint(data, &offset, 4)
			if err != nil {
				return nil, 0, 0, err
			}
			absIndex := base + postBaseIndex
			if absIndex < t.droppedCount || absIndex >= t.totalInserts {
				return nil, 0, 0, fmt.Errorf("oracle: post-base dynamic index %d out of bounds", absIndex)
			}
			entry := t.entries[absIndex-t.droppedCount]
			result = append(result, HeaderField{Name: entry.Name, Value: entry.Value})

		case (b & 0xf0) == 0x00:
			// Literal Header Field With Post-Base Name Reference (RFC 9204 §4.5.5): [0000 N NNN] [H VVVVVVV]
			postBaseIndex, err := decodeOracleVarint(data, &offset, 3)
			if err != nil {
				return nil, 0, 0, err
			}
			val, err := decodeOracleString(data, &offset, 7, 0x80)
			if err != nil {
				return nil, 0, 0, err
			}
			absIndex := base + postBaseIndex
			if absIndex < t.droppedCount || absIndex >= t.totalInserts {
				return nil, 0, 0, fmt.Errorf("oracle: post-base dynamic name index %d out of bounds", absIndex)
			}
			name := t.entries[absIndex-t.droppedCount].Name
			result = append(result, HeaderField{Name: name, Value: val})

		default:
			return nil, 0, 0, fmt.Errorf("oracle: unknown opcode 0x%02x on request stream at offset %d", b, offset)
		}
	}

	return result, ric, base, nil
}

// =============================================================================
// Adversarial Tests: Decoder Feedback Anomaly Rejection (Error Codes 186 - 190)
// =============================================================================

// TestM4StreamFeedback_Error186_IntegerTooLarge tests decoder stream integer overflow rejection.
func TestM4StreamFeedback_Error186_IntegerTooLarge(t *testing.T) {
	testCases := []struct {
		name       string
		opcodeByte byte
		prefixMask byte
	}{
		{"HeaderAcknowledgement", 0x80, 0x7f},
		{"StreamCancellation", 0x40, 0x3f},
		{"InsertCountIncrement", 0x00, 0x3f},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errDelegate := newAdvM4ErrorDelegate()
			encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

			// 11 continuation bytes with MSB set causes varint integer overflow (>64 bits)
			var malformed []byte
			malformed = append(malformed, tc.opcodeByte|tc.prefixMask) // fill prefix completely
			for i := 0; i < 11; i++ {
				malformed = append(malformed, 0x80)
			}

			encoder.DecoderStreamReceiver().Decode(malformed)

			calls := errDelegate.Calls()
			require.Equal(t, 1, len(calls), "expected exactly 1 error call")
			assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INTEGER_TOO_LARGE, calls[0].ErrorCode)
			assert.Equal(t, "Encoded integer too large.", calls[0].ErrorMessage)

			// Error latching: sending further data after error must be silently ignored
			encoder.DecoderStreamReceiver().Decode([]byte{0x01})
			assert.Equal(t, 1, errDelegate.Count(), "further decoder stream data must be ignored after error")
		})

		t.Run(tc.name+"_ChunkedByteByByte", func(t *testing.T) {
			errDelegate := newAdvM4ErrorDelegate()
			encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

			var malformed []byte
			malformed = append(malformed, tc.opcodeByte|tc.prefixMask)
			for i := 0; i < 11; i++ {
				malformed = append(malformed, 0x80)
			}

			// Feed byte-by-byte into decoder receiver
			for _, b := range malformed {
				encoder.DecoderStreamReceiver().Decode([]byte{b})
				if errDelegate.Count() > 0 {
					break
				}
			}

			calls := errDelegate.Calls()
			require.Equal(t, 1, len(calls))
			assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INTEGER_TOO_LARGE, calls[0].ErrorCode)
		})
	}
}

// TestM4StreamFeedback_Error187_InvalidZeroIncrement tests rejection of zero increment.
func TestM4StreamFeedback_Error187_InvalidZeroIncrement(t *testing.T) {
	t.Run("DirectAPI", func(t *testing.T) {
		errDelegate := newAdvM4ErrorDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

		encoder.OnInsertCountIncrement(0)

		calls := errDelegate.Calls()
		require.Equal(t, 1, len(calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INVALID_ZERO_INCREMENT, calls[0].ErrorCode)
		assert.Equal(t, "Invalid increment value 0.", calls[0].ErrorMessage)
		assert.Equal(t, uint64(0), encoder.BlockingManager().KnownReceivedCount())
	})

	t.Run("WireFormatOpcode00", func(t *testing.T) {
		errDelegate := newAdvM4ErrorDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

		// 0x00 is Insert Count Increment with 6-bit value = 0
		encoder.DecoderStreamReceiver().Decode([]byte{0x00})

		calls := errDelegate.Calls()
		require.Equal(t, 1, len(calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INVALID_ZERO_INCREMENT, calls[0].ErrorCode)
		assert.Equal(t, "Invalid increment value 0.", calls[0].ErrorMessage)
	})

	t.Run("RepeatedZeroIncrementStress", func(t *testing.T) {
		errDelegate := newAdvM4ErrorDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

		for i := 0; i < 20; i++ {
			encoder.OnInsertCountIncrement(0)
		}
		assert.Equal(t, 20, errDelegate.Count())
		for _, c := range errDelegate.Calls() {
			assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INVALID_ZERO_INCREMENT, c.ErrorCode)
		}
	})
}

// TestM4StreamFeedback_Error188_IncrementOverflow tests detection of uint64 overflow in KRC.
func TestM4StreamFeedback_Error188_IncrementOverflow(t *testing.T) {
	t.Run("DirectAPI_KnownReceivedCountPlusIncrementOverflow", func(t *testing.T) {
		errDelegate := newAdvM4ErrorDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

		headerTable := EncoderPeerHeaderTable(encoder)
		headerTable.SetMaximumDynamicTableCapacity(4096)
		headerTable.SetDynamicTableCapacity(4096)
		headerTable.InsertEntry("test-name", "test-val")

		// First increment of 1 is valid (KRC = 1 <= 1)
		encoder.OnInsertCountIncrement(1)
		require.Equal(t, 0, errDelegate.Count())
		assert.Equal(t, uint64(1), encoder.BlockingManager().KnownReceivedCount())

		// Second increment of MaxUint64 causes KRC overflow (1 + MaxUint64 > MaxUint64)
		encoder.OnInsertCountIncrement(math.MaxUint64)
		calls := errDelegate.Calls()
		require.Equal(t, 1, len(calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INCREMENT_OVERFLOW, calls[0].ErrorCode)
		assert.Equal(t, "Insert Count Increment instruction causes overflow.", calls[0].ErrorMessage)
		assert.Equal(t, uint64(1), encoder.BlockingManager().KnownReceivedCount(), "overflow must not corrupt KRC")
	})

	t.Run("DirectAPI_BoundaryKRCPlusDelta", func(t *testing.T) {
		errDelegate := newAdvM4ErrorDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

		headerTable := EncoderPeerHeaderTable(encoder)
		headerTable.SetMaximumDynamicTableCapacity(4096)
		headerTable.SetDynamicTableCapacity(4096)
		for i := 0; i < 10; i++ {
			headerTable.InsertEntry(fmt.Sprintf("k%d", i), "v")
		}

		// Advance KRC to 5
		encoder.OnInsertCountIncrement(5)
		require.Equal(t, 0, errDelegate.Count())
		assert.Equal(t, uint64(5), encoder.BlockingManager().KnownReceivedCount())

		// Increment of MaxUint64 - 4 causes overflow (5 + MaxUint64 - 4 > MaxUint64)
		encoder.OnInsertCountIncrement(math.MaxUint64 - 4)
		calls := errDelegate.Calls()
		require.Equal(t, 1, len(calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INCREMENT_OVERFLOW, calls[0].ErrorCode)
	})
}

// TestM4StreamFeedback_Error189_ImpossibleInsertCount tests rejection of KRC > InsertedEntryCount.
func TestM4StreamFeedback_Error189_ImpossibleInsertCount(t *testing.T) {
	t.Run("EmptyTableIncrementOne", func(t *testing.T) {
		errDelegate := newAdvM4ErrorDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

		encoder.OnInsertCountIncrement(1)
		calls := errDelegate.Calls()
		require.Equal(t, 1, len(calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_IMPOSSIBLE_INSERT_COUNT, calls[0].ErrorCode)
		assert.Equal(
			t,
			"Increment value 1 raises known received count to 1 exceeding inserted entry count 0",
			calls[0].ErrorMessage,
		)
	})

	t.Run("MultiInsertSequenceThenImpossibleIncrement", func(t *testing.T) {
		errDelegate := newAdvM4ErrorDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

		headerTable := EncoderPeerHeaderTable(encoder)
		headerTable.SetMaximumDynamicTableCapacity(4096)
		headerTable.SetDynamicTableCapacity(4096)
		for i := 0; i < 5; i++ {
			headerTable.InsertEntry(fmt.Sprintf("key-%d", i), "val")
		}

		// Valid increment up to 5
		encoder.OnInsertCountIncrement(3)
		assert.Equal(t, 0, errDelegate.Count())
		encoder.OnInsertCountIncrement(2)
		assert.Equal(t, 0, errDelegate.Count())
		assert.Equal(t, uint64(5), encoder.BlockingManager().KnownReceivedCount())

		// Increment beyond 5 triggers error 189
		encoder.OnInsertCountIncrement(1)
		calls := errDelegate.Calls()
		require.Equal(t, 1, len(calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_IMPOSSIBLE_INSERT_COUNT, calls[0].ErrorCode)
		assert.Contains(t, calls[0].ErrorMessage, "exceeding inserted entry count 5")
	})

	t.Run("CumulativeInsertionsBoundaryAcrossEvictions", func(t *testing.T) {
		errDelegate := newAdvM4ErrorDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

		headerTable := EncoderPeerHeaderTable(encoder)
		// Small capacity: only 40 bytes (room for 1 entry)
		headerTable.SetMaximumDynamicTableCapacity(40)
		headerTable.SetDynamicTableCapacity(40)

		// Insert 10 entries consecutively, causing entries 0..8 to be evicted
		for i := 0; i < 10; i++ {
			headerTable.InsertEntry(fmt.Sprintf("k%d", i), "v")
		}

		assert.Equal(t, uint64(10), headerTable.InsertedEntryCount())
		assert.Equal(t, uint64(9), headerTable.DroppedEntryCount())

		// In RFC 9204 §4.4.3, KRC is compared against total cumulative insertions (10), NOT active entries (1).
		// Incrementing to 10 is completely valid!
		encoder.OnInsertCountIncrement(10)
		require.Equal(t, 0, errDelegate.Count())
		assert.Equal(t, uint64(10), encoder.BlockingManager().KnownReceivedCount())

		// Incrementing to 11 exceeds total insertions -> triggers error 189
		encoder.OnInsertCountIncrement(1)
		calls := errDelegate.Calls()
		require.Equal(t, 1, len(calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_IMPOSSIBLE_INSERT_COUNT, calls[0].ErrorCode)
	})
}

// TestM4StreamFeedback_Error190_IncorrectAcknowledgement tests rejection of invalid Section Acks.
func TestM4StreamFeedback_Error190_IncorrectAcknowledgement(t *testing.T) {
	t.Run("UnknownStreamAck", func(t *testing.T) {
		errDelegate := newAdvM4ErrorDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

		encoder.OnHeaderAcknowledgement(99)
		calls := errDelegate.Calls()
		require.Equal(t, 1, len(calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INCORRECT_ACKNOWLEDGEMENT, calls[0].ErrorCode)
		assert.Equal(
			t,
			"Header Acknowledgement received for stream 99 with no outstanding header blocks.",
			calls[0].ErrorMessage,
		)
	})

	t.Run("StaticOnlyHeaderBlockProducesNoAckExpectation", func(t *testing.T) {
		errDelegate := newAdvM4ErrorDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
		encoder.SetMaximumBlockedStreams(1)

		// Header list with only static references
		encoder.EncodeHeaderList(5, []HeaderField{{Name: ":method", Value: "GET"}}, nil)

		// RFC 9204: Section Ack for stream with NO dynamic references is invalid
		encoder.OnHeaderAcknowledgement(5)
		calls := errDelegate.Calls()
		require.Equal(t, 1, len(calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INCORRECT_ACKNOWLEDGEMENT, calls[0].ErrorCode)
		assert.Contains(t, calls[0].ErrorMessage, "stream 5 with no outstanding header blocks")
	})

	t.Run("DoubleAcknowledgementOnDynamicStream", func(t *testing.T) {
		errDelegate := newAdvM4ErrorDelegate()
		senderDelegate := newAdvM4SenderDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
		encoder.SetStreamSenderDelegate(senderDelegate)
		encoder.SetMaximumBlockedStreams(1)
		encoder.SetMaximumDynamicTableCapacity(4096)
		encoder.SetDynamicTableCapacity(4096)

		// Stream 7 sends dynamic header block
		encoder.EncodeHeaderList(7, []HeaderField{{Name: "dyn-foo", Value: "dyn-bar"}}, nil)

		// First ack is valid
		encoder.OnHeaderAcknowledgement(7)
		assert.Equal(t, 0, errDelegate.Count())

		// Second ack triggers error 190
		encoder.OnHeaderAcknowledgement(7)
		calls := errDelegate.Calls()
		require.Equal(t, 1, len(calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INCORRECT_ACKNOWLEDGEMENT, calls[0].ErrorCode)
	})

	t.Run("AcknowledgementAfterStreamCancellation", func(t *testing.T) {
		errDelegate := newAdvM4ErrorDelegate()
		senderDelegate := newAdvM4SenderDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
		encoder.SetStreamSenderDelegate(senderDelegate)
		encoder.SetMaximumBlockedStreams(1)
		encoder.SetMaximumDynamicTableCapacity(4096)
		encoder.SetDynamicTableCapacity(4096)

		// Stream 12 sends dynamic header block
		encoder.EncodeHeaderList(12, []HeaderField{{Name: "dyn-cancel", Value: "val"}}, nil)

		// Stream 12 cancelled
		encoder.OnStreamCancellation(12)

		// Ack arriving after cancellation triggers error 190
		encoder.OnHeaderAcknowledgement(12)
		calls := errDelegate.Calls()
		require.Equal(t, 1, len(calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INCORRECT_ACKNOWLEDGEMENT, calls[0].ErrorCode)
	})

	t.Run("InterleavedMultiStreamAcks", func(t *testing.T) {
		errDelegate := newAdvM4ErrorDelegate()
		senderDelegate := newAdvM4SenderDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
		encoder.SetStreamSenderDelegate(senderDelegate)
		encoder.SetMaximumBlockedStreams(4)
		encoder.SetMaximumDynamicTableCapacity(4096)
		encoder.SetDynamicTableCapacity(4096)

		// Stream 1 sends 2 dynamic blocks
		encoder.EncodeHeaderList(1, []HeaderField{{Name: "s1-a", Value: "v"}}, nil)
		encoder.EncodeHeaderList(1, []HeaderField{{Name: "s1-b", Value: "v"}}, nil)

		// Stream 2 sends 1 dynamic block
		encoder.EncodeHeaderList(2, []HeaderField{{Name: "s2-a", Value: "v"}}, nil)

		// Ack stream 2 -> 0 left
		encoder.OnHeaderAcknowledgement(2)
		assert.Equal(t, 0, errDelegate.Count())

		// Duplicate ack on stream 2 -> Error 190
		encoder.OnHeaderAcknowledgement(2)
		assert.Equal(t, 1, errDelegate.Count())
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INCORRECT_ACKNOWLEDGEMENT, errDelegate.Calls()[0].ErrorCode)

		// Stream 1 still has 2 blocks outstanding:
		encoder.OnHeaderAcknowledgement(1) // 1 left
		assert.Equal(t, 1, errDelegate.Count())
		encoder.OnHeaderAcknowledgement(1) // 0 left
		assert.Equal(t, 1, errDelegate.Count())

		// Third ack on stream 1 -> Error 190
		encoder.OnHeaderAcknowledgement(1)
		assert.Equal(t, 2, errDelegate.Count())
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INCORRECT_ACKNOWLEDGEMENT, errDelegate.Calls()[1].ErrorCode)
	})
}

// TestM4StreamFeedback_FuzzingDecoderStreamAnomalies fuzzes DecoderStreamReceiver with malformed byte sequences.
func TestM4StreamFeedback_FuzzingDecoderStreamAnomalies(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	for iter := 0; iter < 100; iter++ {
		errDelegate := newAdvM4ErrorDelegate()
		encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

		// Random length from 1 to 128 bytes
		length := rng.Intn(128) + 1
		buf := make([]byte, length)
		rng.Read(buf)

		// Random delivery pattern: 1-byte, multi-byte chunks, or all at once
		chunkMode := rng.Intn(3)
		if chunkMode == 0 {
			// All at once
			encoder.DecoderStreamReceiver().Decode(buf)
		} else if chunkMode == 1 {
			// 1 byte at a time
			for _, b := range buf {
				encoder.DecoderStreamReceiver().Decode([]byte{b})
			}
		} else {
			// Random chunk sizes
			pos := 0
			for pos < len(buf) {
				chunkSize := rng.Intn(8) + 1
				if pos+chunkSize > len(buf) {
					chunkSize = len(buf) - pos
				}
				encoder.DecoderStreamReceiver().Decode(buf[pos : pos+chunkSize])
				pos += chunkSize
			}
		}
		encoder.DecoderStreamReceiver().EndDecoding()

		// Robustness guarantee: must never panic, crash, or enter undefined state
	}
}

// =============================================================================
// Adversarial Tests: 64KB Sender Backpressure Transitions
// =============================================================================

// TestM4StreamFeedback_Backpressure_64KBBoundaryTransitions verifies exact 64KB threshold transitions.
func TestM4StreamFeedback_Backpressure_64KBBoundaryTransitions(t *testing.T) {
	const maxThreshold = 64 * 1024 // 65536

	errDelegate := newAdvM4ErrorDelegate()
	senderDelegate := newAdvM4SenderDelegate()
	encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
	encoder.SetStreamSenderDelegate(senderDelegate)
	encoder.SetMaximumBlockedStreams(100)
	encoder.SetMaximumDynamicTableCapacity(4096)
	encoder.SetDynamicTableCapacity(4096)
	// Flush SetDynamicTableCapacity instruction to delegate and clear writes so buffer starts empty
	encoder.EncoderStreamSender().Flush()
	senderDelegate.ClearWrites()

	// Step 1: Exactly 65535 (64KB - 1) -> CanWrite() must be true
	senderDelegate.SetNumBytesBuffered(maxThreshold - 1)
	assert.True(t, encoder.EncoderStreamSender().CanWrite(), "CanWrite should be true at 65535 bytes")

	var sentBytes uint64
	senderDelegate.ClearWrites()
	out1 := encoder.EncodeHeaderList(1, []HeaderField{{Name: "bp-key-1", Value: "val-1"}}, &sentBytes)
	assert.True(t, sentBytes > 0, "dynamic insertion must occur when CanWrite is true")
	assert.True(t, len(senderDelegate.TotalWritten()) > 0)
	assert.NotEqual(t, []byte{0x00, 0x00}, out1[:2], "must have non-zero RIC in header block prefix")

	// Step 2: Exactly 65536 (64KB) -> CanWrite() must be true (<= threshold)
	senderDelegate.SetNumBytesBuffered(maxThreshold)
	assert.True(t, encoder.EncoderStreamSender().CanWrite(), "CanWrite should be true at exactly 65536 bytes")

	senderDelegate.ClearWrites()
	encoder.EncodeHeaderList(2, []HeaderField{{Name: "bp-key-2", Value: "val-2"}}, &sentBytes)
	assert.True(t, sentBytes > 0, "dynamic insertion must occur at exactly 65536 bytes")

	// Step 3: Exactly 65537 (64KB + 1) -> CanWrite() must flip cleanly to false
	senderDelegate.SetNumBytesBuffered(maxThreshold + 1)
	assert.False(t, encoder.EncoderStreamSender().CanWrite(), "CanWrite should be false at 65537 bytes")

	senderDelegate.ClearWrites()
	out3 := encoder.EncodeHeaderList(3, []HeaderField{{Name: "bp-key-3", Value: "val-3"}}, &sentBytes)
	assert.Equal(t, uint64(0), sentBytes, "dynamic insertion must be suppressed when CanWrite is false")
	assert.Equal(t, 0, len(senderDelegate.TotalWritten()), "no encoder stream instructions emitted")
	assert.Equal(t, []byte{0x00, 0x00}, out3[:2], "prefix must be RIC=0, Base=0 when literal fallback is used")

	// Step 4: Drain buffer to 0 -> CanWrite() flips back to true
	senderDelegate.SetNumBytesBuffered(0)
	assert.True(t, encoder.EncoderStreamSender().CanWrite(), "CanWrite should be true after buffer drains")

	senderDelegate.ClearWrites()
	encoder.EncodeHeaderList(4, []HeaderField{{Name: "bp-key-4", Value: "val-4"}}, &sentBytes)
	assert.True(t, sentBytes > 0, "dynamic insertion must resume after buffer drains")

	// Table integrity verification: entries 1, 2, and 4 must exist at indices 0, 1, 2; entry 3 was omitted
	headerTable := EncoderPeerHeaderTable(encoder)
	assert.Equal(t, uint64(3), headerTable.InsertedEntryCount(), "exactly 3 entries inserted into table")
}

// TestM4StreamFeedback_Backpressure_DrainingAndStaticFallback verifies fallback behavior.
func TestM4StreamFeedback_Backpressure_DrainingAndStaticFallback(t *testing.T) {
	errDelegate := newAdvM4ErrorDelegate()
	senderDelegate := newAdvM4SenderDelegate()
	encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
	encoder.SetStreamSenderDelegate(senderDelegate)
	encoder.SetMaximumBlockedStreams(2)
	encoder.SetMaximumDynamicTableCapacity(4096)
	encoder.SetDynamicTableCapacity(4096)

	// Populate dynamic table with initial entries
	encoder.EncodeHeaderList(1, []HeaderField{
		{Name: "common-name", Value: "val-a"},
		{Name: "common-name", Value: "val-b"},
	}, nil)

	// Under backpressure: match with static name (:method) falls back to literal with static name ref
	senderDelegate.SetNumBytesBuffered(70000)
	senderDelegate.ClearWrites()

	var sentBytes uint64
	out := encoder.EncodeHeaderList(2, []HeaderField{{Name: ":method", Value: "CUSTOM"}}, &sentBytes)
	assert.Equal(t, uint64(0), sentBytes)
	assert.Equal(t, 0, len(senderDelegate.TotalWritten()))

	// First byte after prefix should be Literal Header Field With Static Name Reference
	// Pattern: 01 N T NNNN -> 0x5f (T=1 static, name index 15 for :method)
	require.True(t, len(out) >= 3)
	assert.Equal(t, byte(0x5f), out[2], "must encode literal with static name reference for :method")
}

// TestM4StreamFeedback_Backpressure_OscillationStress stress-tests flapping backpressure.
func TestM4StreamFeedback_Backpressure_OscillationStress(t *testing.T) {
	errDelegate := newAdvM4ErrorDelegate()
	senderDelegate := newAdvM4SenderDelegate()
	encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
	encoder.SetStreamSenderDelegate(senderDelegate)
	encoder.SetMaximumBlockedStreams(10)
	encoder.SetMaximumDynamicTableCapacity(8192)
	encoder.SetDynamicTableCapacity(8192)

	oracleTable := newOracleDynamicTable(8192)
	// Initial capacity set instruction
	err := oracleTable.FeedEncoderStream(senderDelegate.TotalWritten())
	require.NoError(t, err)
	senderDelegate.ClearWrites()

	for i := 0; i < 50; i++ {
		isBackpressured := (i % 2) == 1
		if isBackpressured {
			senderDelegate.SetNumBytesBuffered(70000)
		} else {
			senderDelegate.SetNumBytesBuffered(0)
		}

		headers := []HeaderField{
			{Name: fmt.Sprintf("osc-key-%d", i), Value: fmt.Sprintf("osc-val-%d", i)},
		}

		var sentBytes uint64
		out := encoder.EncodeHeaderList(uint64(i+1), headers, &sentBytes)

		if isBackpressured {
			assert.Equal(t, uint64(0), sentBytes, fmt.Sprintf("iteration %d: sentBytes must be 0", i))
			assert.Equal(t, 0, senderDelegate.WritesCount(), fmt.Sprintf("iteration %d: no stream writes", i))
		} else {
			assert.True(t, sentBytes > 0, fmt.Sprintf("iteration %d: dynamic insertion expected", i))
			// Feed emitted stream bytes to oracle table
			err := oracleTable.FeedEncoderStream(senderDelegate.TotalWritten())
			require.NoError(t, err, fmt.Sprintf("iteration %d: oracle must parse encoder stream", i))
			senderDelegate.ClearWrites()

			// Acknowledge dynamic entry so future streams are not blocked by stream limit
			encoder.OnInsertCountIncrement(1)
			encoder.OnHeaderAcknowledgement(uint64(i + 1))
		}

		// Verify every produced header block decodes correctly via oracle
		decoded, _, _, err := oracleTable.DecodeHeaderBlock(out)
		require.NoError(t, err, fmt.Sprintf("iteration %d: oracle decode must succeed", i))
		require.Equal(t, 1, len(decoded), fmt.Sprintf("iteration %d: field count", i))
		assert.Equal(t, headers[0].Name, decoded[0].Name, fmt.Sprintf("iteration %d: name match", i))
		assert.Equal(t, headers[0].Value, decoded[0].Value, fmt.Sprintf("iteration %d: value match", i))
	}
}

// =============================================================================
// Adversarial Tests: Wire Format Prefix & Indexing Verification Against Oracle
// =============================================================================

// TestM4StreamFeedback_Oracle_PrefixAndRequestRelativeWireEncoding tests wire layout and relative index math.
func TestM4StreamFeedback_Oracle_PrefixAndRequestRelativeWireEncoding(t *testing.T) {
	runModes := []struct {
		name    string
		huffman HuffmanEncoding
	}{
		{"HuffmanEnabled", HuffmanEncodingEnabled},
		{"HuffmanDisabled", HuffmanEncodingDisabled},
	}

	for _, mode := range runModes {
		t.Run(mode.name, func(t *testing.T) {
			errDelegate := newAdvM4ErrorDelegate()
			senderDelegate := newAdvM4SenderDelegate()
			encoder := NewEncoder(errDelegate.OnDecoderStreamError, mode.huffman, CookieCrumblingEnabled)
			encoder.SetStreamSenderDelegate(senderDelegate)
			encoder.SetMaximumBlockedStreams(5)
			encoder.SetMaximumDynamicTableCapacity(4096)
			encoder.SetDynamicTableCapacity(4096)

			oracleTable := newOracleDynamicTable(4096)
			err := oracleTable.FeedEncoderStream(senderDelegate.TotalWritten())
			require.NoError(t, err)
			senderDelegate.ClearWrites()

			// Subtest 1: Empty Header List
			{
				out := encoder.EncodeHeaderList(1, []HeaderField{}, nil)
				require.Equal(t, []byte{0x00, 0x00}, out, "empty headers must emit prefix 0000")
				decoded, ric, base, err := oracleTable.DecodeHeaderBlock(out)
				require.NoError(t, err)
				assert.Equal(t, uint64(0), ric)
				assert.Equal(t, uint64(0), base)
				assert.Equal(t, 0, len(decoded))
			}

			// Subtest 2: Static Headers Only
			{
				headers := []HeaderField{
					{Name: ":method", Value: "GET"},
					{Name: ":scheme", Value: "https"},
					{Name: ":path", Value: "/"},
				}
				out := encoder.EncodeHeaderList(2, headers, nil)
				decoded, ric, base, err := oracleTable.DecodeHeaderBlock(out)
				require.NoError(t, err)
				assert.Equal(t, uint64(0), ric)
				assert.Equal(t, uint64(0), base)
				require.Equal(t, len(headers), len(decoded))
				for i := range headers {
					assert.Equal(t, headers[i].Name, decoded[i].Name)
					assert.Equal(t, headers[i].Value, decoded[i].Value)
				}
			}

			// Subtest 3: Dynamic Headers and Request-Relative Index Verification
			{
				headers := []HeaderField{
					{Name: "custom-alpha", Value: "val-alpha"},
					{Name: "custom-beta", Value: "val-beta"},
					{Name: "custom-gamma", Value: "val-gamma"},
				}
				var sentBytes uint64
				out := encoder.EncodeHeaderList(3, headers, &sentBytes)
				require.True(t, sentBytes > 0)

				// Synchronize oracle dynamic table
				err := oracleTable.FeedEncoderStream(senderDelegate.TotalWritten())
				require.NoError(t, err)
				senderDelegate.ClearWrites()

				// Oracle decode
				decoded, ric, base, err := oracleTable.DecodeHeaderBlock(out)
				require.NoError(t, err)
				assert.Equal(t, uint64(3), ric)
				assert.Equal(t, uint64(3), base)
				require.Equal(t, 3, len(decoded))
				for i := range headers {
					assert.Equal(t, headers[i].Name, decoded[i].Name)
					assert.Equal(t, headers[i].Value, decoded[i].Value)
				}

				// Inspect raw request-relative index bytes in out:
				// Base = 3.
				// Entries inserted: alpha (abs 0), beta (abs 1), gamma (abs 2).
				// Field lines encoded:
				// alpha: relIndex = Base - abs - 1 = 3 - 0 - 1 = 2 -> wire byte 0x82
				// beta:  relIndex = Base - abs - 1 = 3 - 1 - 1 = 1 -> wire byte 0x81
				// gamma: relIndex = Base - abs - 1 = 3 - 2 - 1 = 0 -> wire byte 0x80
				// Prefix is 2 bytes: 04 00 (RIC=3 encoded is (3 % 256) + 1 = 4)
				require.True(t, len(out) >= 5)
				assert.Equal(t, byte(0x04), out[0], "Encoded RIC must be 4")
				assert.Equal(t, byte(0x00), out[1], "Delta Base must be 0")
				assert.Equal(t, byte(0x82), out[2], "relative index for alpha must be 2")
				assert.Equal(t, byte(0x81), out[3], "relative index for beta must be 1")
				assert.Equal(t, byte(0x80), out[4], "relative index for gamma must be 0")
			}
		})
	}
}

// TestM4StreamFeedback_Oracle_MultibyteRICPrefixEncoding verifies multibyte RIC prefix encoding (RIC > 255).
func TestM4StreamFeedback_Oracle_MultibyteRICPrefixEncoding(t *testing.T) {
	errDelegate := newAdvM4ErrorDelegate()
	senderDelegate := newAdvM4SenderDelegate()
	encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
	encoder.SetStreamSenderDelegate(senderDelegate)
	encoder.SetMaximumBlockedStreams(10)

	// Capacity: 32768 -> MaxEntries = 1024, 2*MaxEntries = 2048
	const cap = 32768
	encoder.SetMaximumDynamicTableCapacity(cap)
	encoder.SetDynamicTableCapacity(cap)

	oracleTable := newOracleDynamicTable(cap)
	err := oracleTable.FeedEncoderStream(senderDelegate.TotalWritten())
	require.NoError(t, err)
	senderDelegate.ClearWrites()

	// Insert 260 entries in batches so that RIC exceeds 255
	for batch := 0; batch < 26; batch++ {
		var batchHeaders []HeaderField
		for item := 0; item < 10; item++ {
			idx := batch*10 + item
			batchHeaders = append(batchHeaders, HeaderField{
				Name:  fmt.Sprintf("bulk-k-%03d", idx),
				Value: fmt.Sprintf("bulk-v-%03d", idx),
			})
		}
		encoder.EncodeHeaderList(uint64(batch+1), batchHeaders, nil)
		err := oracleTable.FeedEncoderStream(senderDelegate.TotalWritten())
		require.NoError(t, err)
		senderDelegate.ClearWrites()

		// Acknowledge batch entries so subsequent batches can continue inserting
		encoder.OnInsertCountIncrement(10)
		encoder.OnHeaderAcknowledgement(uint64(batch + 1))
	}

	headerTable := EncoderPeerHeaderTable(encoder)
	assert.Equal(t, uint64(260), headerTable.InsertedEntryCount())

	// Now reference the newest entry (abs index 259). RIC = 260.
	// RFC 9204 §4.5.1: Encoded RIC = (RIC % (2 * MaxEntries)) + 1 = (260 % 2048) + 1 = 261.
	// 261 exceeds 8-bit prefix max (255), so first byte must be 0xff, followed by 261 - 255 = 6 (0x06).
	testHeader := []HeaderField{
		{Name: "bulk-k-259", Value: "bulk-v-259"},
	}
	out := encoder.EncodeHeaderList(100, testHeader, nil)
	require.True(t, len(out) >= 4)

	assert.Equal(t, byte(0xff), out[0], "first byte of multi-byte prefix must be 0xff")
	assert.Equal(t, byte(0x06), out[1], "varint extension must be 6 (261 - 255)")
	assert.Equal(t, byte(0x00), out[2], "Delta Base must be 0")

	// Oracle decode verification
	decoded, ric, base, err := oracleTable.DecodeHeaderBlock(out)
	require.NoError(t, err)
	assert.Equal(t, uint64(260), ric)
	assert.Equal(t, uint64(260), base)
	require.Equal(t, 1, len(decoded))
	assert.Equal(t, testHeader[0].Name, decoded[0].Name)
	assert.Equal(t, testHeader[0].Value, decoded[0].Value)
}

// TestM4StreamFeedback_Oracle_ComplexHeaderBlockRoundtrip tests complex mixed headers with oracle.
func TestM4StreamFeedback_Oracle_ComplexHeaderBlockRoundtrip(t *testing.T) {
	errDelegate := newAdvM4ErrorDelegate()
	senderDelegate := newAdvM4SenderDelegate()
	encoder := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
	encoder.SetStreamSenderDelegate(senderDelegate)
	encoder.SetMaximumBlockedStreams(10)
	encoder.SetMaximumDynamicTableCapacity(4096)
	encoder.SetDynamicTableCapacity(4096)

	oracleTable := newOracleDynamicTable(4096)
	err := oracleTable.FeedEncoderStream(senderDelegate.TotalWritten())
	require.NoError(t, err)
	senderDelegate.ClearWrites()

	requestHeaders := []HeaderField{
		{Name: ":method", Value: "POST"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "example.org:443"},
		{Name: ":path", Value: "/api/v2/resource"},
		{Name: "content-type", Value: "application/json; charset=utf-8"},
		{Name: "user-agent", Value: "Antigravity/1.0 (QPACK Challenger)"},
		{Name: "cookie", Value: "sessionid=xyz123; tracking=abc456"},
		{Name: "x-request-id", Value: "f47ac10b-58cc-4372-a567-0e02b2c3d479"},
		{Name: "accept-encoding", Value: "gzip, deflate, br"},
	}

	out := encoder.EncodeHeaderList(1, requestHeaders, nil)
	err = oracleTable.FeedEncoderStream(senderDelegate.TotalWritten())
	require.NoError(t, err)

	decoded, _, _, err := oracleTable.DecodeHeaderBlock(out)
	require.NoError(t, err)

	// Flatten expected fields with cookie crumbling
	var expected []HeaderField
	for _, hf := range requestHeaders {
		expected = append(expected, splitHeaderField(hf.Name, hf.Value, CookieCrumblingEnabled)...)
	}

	require.Equal(t, len(expected), len(decoded))
	for i := range expected {
		assert.Equal(t, expected[i].Name, decoded[i].Name, fmt.Sprintf("field %d name", i))
		assert.Equal(t, expected[i].Value, decoded[i].Value, fmt.Sprintf("field %d value", i))
	}
}

// =============================================================================
// Adversarial Tests: Concurrency and Race Detector Verification
// =============================================================================

// TestM4StreamFeedback_ConcurrentStreamsRace tests thread-safety under race detector.
func TestM4StreamFeedback_ConcurrentStreamsRace(t *testing.T) {
	var wg sync.WaitGroup
	numWorkers := 8

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			errDelegate := newAdvM4ErrorDelegate()
			senderDelegate := newAdvM4SenderDelegate()
			enc := NewEncoder(errDelegate.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
			enc.SetStreamSenderDelegate(senderDelegate)
			enc.SetMaximumBlockedStreams(5)
			enc.SetMaximumDynamicTableCapacity(2048)
			enc.SetDynamicTableCapacity(2048)

			for step := 0; step < 20; step++ {
				streamID := uint64(step*10 + workerID)
				headers := []HeaderField{
					{Name: fmt.Sprintf("worker-%d-key-%d", workerID, step), Value: "val"},
					{Name: ":method", Value: "GET"},
				}
				out := enc.EncodeHeaderList(streamID, headers, nil)
				assert.True(t, len(out) >= 2)

				// Ingest feedback
				enc.OnInsertCountIncrement(1)
				enc.OnHeaderAcknowledgement(streamID)
			}
		}(w)
	}

	wg.Wait()
}
