// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// -----------------------------------------------------------------------------
// Test Oracle & Helper Utilities
// -----------------------------------------------------------------------------

// adversarialSenderDelegate captures encoder stream bytes and allows simulating buffer backpressure.
type adversarialSenderDelegate struct {
	mu               sync.Mutex
	writes           [][]byte
	numBytesBuffered uint64
	onWriteCallback  func(data []byte)
}

func newAdversarialSenderDelegate() *adversarialSenderDelegate {
	return &adversarialSenderDelegate{
		writes: make([][]byte, 0),
	}
}

func (d *adversarialSenderDelegate) WriteStreamData(data []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	copied := make([]byte, len(data))
	copy(copied, data)
	d.writes = append(d.writes, copied)
	if d.onWriteCallback != nil {
		d.onWriteCallback(copied)
	}
}

func (d *adversarialSenderDelegate) NumBytesBuffered() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.numBytesBuffered
}

func (d *adversarialSenderDelegate) SetNumBytesBuffered(n uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.numBytesBuffered = n
}

func (d *adversarialSenderDelegate) AllWrittenData() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	var total []byte
	for _, w := range d.writes {
		total = append(total, w...)
	}
	return total
}

func (d *adversarialSenderDelegate) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writes = nil
}

// oracleDecoderReceiver implements EncoderStreamReceiverDelegate and maintains a mirror DecoderHeaderTable.
type oracleDecoderReceiver struct {
	table        *DecoderHeaderTable
	errorRecords []errorRecord
}

func newOracleDecoderReceiver(maxCapacity uint64) *oracleDecoderReceiver {
	table := NewDecoderHeaderTable()
	table.SetMaximumDynamicTableCapacity(maxCapacity)
	table.SetDynamicTableCapacity(maxCapacity)
	return &oracleDecoderReceiver{
		table:        table,
		errorRecords: make([]errorRecord, 0),
	}
}

func (r *oracleDecoderReceiver) InsertWithNameReference(isStatic bool, nameIndex uint64, value string) {
	var name string
	if isStatic {
		entry := staticEntries[nameIndex]
		name = entry.Name
	} else {
		absIndex, ok := EncoderStreamRelativeIndexToAbsoluteIndex(nameIndex, r.table.InsertedEntryCount())
		if !ok {
			panic(
				fmt.Sprintf(
					"oracle: invalid relative name index %d for inserted count %d",
					nameIndex,
					r.table.InsertedEntryCount(),
				),
			)
		}
		entry := r.table.LookupEntry(false, absIndex)
		if entry == nil {
			panic(fmt.Sprintf("oracle: dynamic entry at absIndex %d is nil or dropped", absIndex))
		}
		name = entry.Name
	}
	r.table.InsertEntry(name, value)
}

func (r *oracleDecoderReceiver) InsertWithoutNameReference(name, value string) {
	r.table.InsertEntry(name, value)
}

func (r *oracleDecoderReceiver) Duplicate(index uint64) {
	absIndex, ok := EncoderStreamRelativeIndexToAbsoluteIndex(index, r.table.InsertedEntryCount())
	if !ok {
		panic(fmt.Sprintf("oracle: invalid duplicate relative index %d", index))
	}
	entry := r.table.LookupEntry(false, absIndex)
	if entry == nil {
		panic(fmt.Sprintf("oracle: duplicate dynamic entry at absIndex %d is nil", absIndex))
	}
	r.table.InsertEntry(entry.Name, entry.Value)
}

func (r *oracleDecoderReceiver) SetDynamicTableCapacity(capacity uint64) {
	r.table.SetDynamicTableCapacity(capacity)
}

func (r *oracleDecoderReceiver) Error(qpackError uint64, errorMessage string) {
	r.errorRecords = append(r.errorRecords, errorRecord{
		qpackError: qpackError,
		message:    errorMessage,
	})
}

// prefixCaptureDelegate captures the decoded prefix instruction.
type prefixCaptureDelegate struct {
	decoded     bool
	instruction *Instruction
	encodedRIC  uint64
	sBit        bool
	deltaBase   uint64
	decoder     *InstructionDecoder
}

func (p *prefixCaptureDelegate) OnInstructionDecoded(inst *Instruction) bool {
	p.decoded = true
	p.instruction = inst
	p.encodedRIC = p.decoder.Varint()
	p.sBit = p.decoder.SBit()
	p.deltaBase = p.decoder.Varint2()
	return true
}

func (p *prefixCaptureDelegate) OnInstructionDecodingError(errorCode InstructionDecoderErrorCode, errorMessage string) {
	panic(fmt.Sprintf("oracle prefix error: %v %s", errorCode, errorMessage))
}

// reqStreamCaptureDelegate captures request stream instructions and decodes fields.
type reqStreamCaptureDelegate struct {
	decoder *InstructionDecoder
	fields  []HeaderField
	table   *DecoderHeaderTable
	base    uint64
}

func (d *reqStreamCaptureDelegate) OnInstructionDecoded(inst *Instruction) bool {
	switch inst {
	case IndexedHeaderFieldInstruction():
		isStatic := d.decoder.SBit()
		idx := d.decoder.Varint()
		if isStatic {
			if idx >= uint64(len(staticEntries)) {
				panic(fmt.Sprintf("oracle: static index %d out of bounds", idx))
			}
			entry := staticEntries[idx]
			d.fields = append(d.fields, HeaderField{Name: entry.Name, Value: entry.Value})
		} else {
			absIndex, ok := RequestStreamRelativeIndexToAbsoluteIndex(idx, d.base)
			if !ok {
				panic(fmt.Sprintf("oracle: relative index %d >= base %d", idx, d.base))
			}
			entry := d.table.LookupEntry(false, absIndex)
			if entry == nil {
				panic(fmt.Sprintf("oracle: dynamic entry %d not found or evicted", absIndex))
			}
			d.fields = append(d.fields, HeaderField{Name: entry.Name, Value: entry.Value})
		}

	case LiteralHeaderFieldNameReferenceInstruction():
		isStatic := d.decoder.SBit()
		idx := d.decoder.Varint()
		val := d.decoder.Value()
		var name string
		if isStatic {
			if idx >= uint64(len(staticEntries)) {
				panic(fmt.Sprintf("oracle: static name index %d out of bounds", idx))
			}
			name = staticEntries[idx].Name
		} else {
			absIndex, ok := RequestStreamRelativeIndexToAbsoluteIndex(idx, d.base)
			if !ok {
				panic(fmt.Sprintf("oracle: relative name index %d >= base %d", idx, d.base))
			}
			entry := d.table.LookupEntry(false, absIndex)
			if entry == nil {
				panic(fmt.Sprintf("oracle: dynamic entry %d not found", absIndex))
			}
			name = entry.Name
		}
		d.fields = append(d.fields, HeaderField{Name: name, Value: val})

	case LiteralHeaderFieldInstruction():
		d.fields = append(d.fields, HeaderField{Name: d.decoder.Name(), Value: d.decoder.Value()})

	default:
		panic(fmt.Sprintf("oracle: unexpected instruction %v", inst))
	}
	return true
}

func (d *reqStreamCaptureDelegate) OnInstructionDecodingError(
	errorCode InstructionDecoderErrorCode,
	errorMessage string,
) {
	panic(fmt.Sprintf("oracle req stream error: %v %s", errorCode, errorMessage))
}

// decodeHeaderBlock decompresses an encoded header block against an oracle dynamic table.
func decodeHeaderBlock(
	t *testing.T,
	encodedBlock []byte,
	table *DecoderHeaderTable,
	maxEntries uint64,
	totalInserts uint64,
) ([]HeaderField, uint64) {
	t.Helper()
	require.True(t, len(encodedBlock) >= 2, "encoded header block must contain at least prefix bytes")

	// 1. Decode Prefix
	prefixDel := &prefixCaptureDelegate{}
	prefixDec := NewInstructionDecoder(PrefixLanguage(), prefixDel)
	prefixDel.decoder = prefixDec

	consumed := 0
	for consumed < len(encodedBlock) {
		b := encodedBlock[consumed]
		consumed++
		ok := prefixDec.Decode([]byte{b})
		require.True(t, ok, "prefix byte decode must succeed")
		if prefixDel.decoded {
			break
		}
	}
	require.True(t, prefixDel.decoded, "must decode prefix instruction")
	require.False(t, prefixDel.sBit, "Chromium encoder always sets S-bit to false (DeltaBase=0)")
	require.Equal(t, uint64(0), prefixDel.deltaBase, "Chromium encoder always sets DeltaBase to 0")

	ric, ok := DecodeRequiredInsertCount(prefixDel.encodedRIC, maxEntries, totalInserts)
	require.True(t, ok, "RIC decoding must succeed")

	base := ric

	// 2. Decode Request Stream Instructions
	remaining := encodedBlock[consumed:]
	if len(remaining) == 0 {
		return nil, ric
	}

	reqDel := &reqStreamCaptureDelegate{
		table: table,
		base:  base,
	}
	reqDec := NewInstructionDecoder(RequestStreamLanguage(), reqDel)
	reqDel.decoder = reqDec

	okReq := reqDec.Decode(remaining)
	require.True(t, okReq, "request stream decoding must succeed")
	reqDec.EndDecoding()
	require.False(t, reqDec.HasError(), "request stream decoder must have no errors")

	return reqDel.fields, ric
}

// flattenHeaders applies cookie crumbling and null-byte splitting to produce the expected decoded fields.
func flattenHeaders(headers []HeaderField, cookieCrumbling CookieCrumbling) []HeaderField {
	var flattened []HeaderField
	for _, hf := range headers {
		flattened = append(flattened, splitHeaderField(hf.Name, hf.Value, cookieCrumbling)...)
	}
	return flattened
}

// -----------------------------------------------------------------------------
// Test 1: Two-Pass Encoding Stress Test with Full Decompression Oracle
// -----------------------------------------------------------------------------

func TestM4EncoderAdversarial_TwoPassEncodingOracleStress(t *testing.T) {
	for _, huffman := range []HuffmanEncoding{HuffmanEncodingEnabled, HuffmanEncodingDisabled} {
		t.Run(huffman.String(), func(t *testing.T) {
			const maxCapacity = 4096
			const maxBlocked = 100

			errorDel := newMockDecoderStreamErrorDelegate()
			senderDel := newAdversarialSenderDelegate()
			encoder := NewEncoder(errorDel.OnDecoderStreamError, huffman, CookieCrumblingEnabled)
			encoder.SetStreamSenderDelegate(senderDel)
			encoder.SetMaximumBlockedStreams(maxBlocked)
			encoder.SetMaximumDynamicTableCapacity(maxCapacity)
			encoder.SetDynamicTableCapacity(maxCapacity)

			// Oracle decoder maintaining mirror dynamic table
			oracle := newOracleDecoderReceiver(maxCapacity)
			encoderStreamReceiver := NewEncoderStreamReceiver(oracle)

			// Wire up sender writes directly to the oracle receiver
			senderDel.onWriteCallback = func(data []byte) {
				encoderStreamReceiver.Decode(data)
			}

			// Deterministic RNG seed
			rng := rand.New(rand.NewSource(1337))

			// Pool of header names and values to trigger static, dynamic, name-only, and new matches
			staticSampleNames := []string{
				":method",
				":path",
				":scheme",
				":authority",
				":status",
				"content-type",
				"accept",
				"user-agent",
			}
			dynamicSampleNames := []string{
				"x-custom-alpha",
				"x-custom-beta",
				"x-session-token",
				"x-request-trace",
				"authorization",
			}
			cookieSamples := []string{
				"a=1",
				"a=1; b=2; c=3",
				"sid=xyz123; pref=dark; lang=en; v=4",
				"foo=bar;baz=qux", // without space after ';'
				"empty=; trail=1;",
			}

			totalStreams := 250
			totalHeadersEncoded := 0

			for streamID := uint64(1); streamID <= uint64(totalStreams); streamID++ {
				numFields := 1 + rng.Intn(15) // 1 to 15 fields per stream
				headerList := make([]HeaderField, 0, numFields)

				for f := 0; f < numFields; f++ {
					mode := rng.Intn(7)
					switch mode {
					case 0:
						// Exact static match
						headerList = append(headerList, HeaderField{Name: ":method", Value: "GET"})
					case 1:
						// Static name match with dynamic value
						headerList = append(headerList, HeaderField{
							Name:  staticSampleNames[rng.Intn(len(staticSampleNames))],
							Value: fmt.Sprintf("/endpoint/%d?query=%x", rng.Intn(50), rng.Int63()),
						})
					case 2:
						// Dynamic candidate
						headerList = append(headerList, HeaderField{
							Name:  dynamicSampleNames[rng.Intn(len(dynamicSampleNames))],
							Value: fmt.Sprintf("val-%d", rng.Intn(20)),
						})
					case 3:
						// Cookie with crumbling
						headerList = append(headerList, HeaderField{
							Name:  "cookie",
							Value: cookieSamples[rng.Intn(len(cookieSamples))],
						})
					case 4:
						// Null-separated header value
						headerList = append(headerList, HeaderField{
							Name: "x-multi-value",
							Value: fmt.Sprintf(
								"part1-%d\x00part2-%d\x00part3-%d",
								rng.Intn(10),
								rng.Intn(10),
								rng.Intn(10),
							),
						})
					case 5:
						// Long literal header
						longVal := strings.Repeat("A", 1+rng.Intn(120))
						headerList = append(headerList, HeaderField{
							Name:  fmt.Sprintf("x-long-%d", rng.Intn(5)),
							Value: longVal,
						})
					case 6:
						// Empty value or 1-byte value
						headerList = append(headerList, HeaderField{
							Name:  fmt.Sprintf("x-tiny-%d", rng.Intn(5)),
							Value: string([]byte{byte('a' + rng.Intn(26))}),
						})
					}
				}

				totalHeadersEncoded += len(headerList)

				var sentByteCount uint64
				encodedBlock := encoder.EncodeHeaderList(streamID, headerList, &sentByteCount)
				require.True(t, len(encodedBlock) >= 2, "encoded block must not be empty")

				// Oracle decompresses the block using mirror dynamic table
				expectedFlattened := flattenHeaders(headerList, CookieCrumblingEnabled)
				decodedFields, ric := decodeHeaderBlock(
					t,
					encodedBlock,
					oracle.table,
					encoder.HeaderTable().MaxEntries(),
					oracle.table.InsertedEntryCount(),
				)

				// Verify lossless roundtrip decompression
				require.Equal(
					t,
					len(expectedFlattened),
					len(decodedFields),
					fmt.Sprintf("stream %d: decoded field count mismatch", streamID),
				)
				for i := range expectedFlattened {
					assert.Equal(
						t,
						expectedFlattened[i].Name,
						decodedFields[i].Name,
						fmt.Sprintf("stream %d field %d name mismatch", streamID, i),
					)
					assert.Equal(
						t,
						expectedFlattened[i].Value,
						decodedFields[i].Value,
						fmt.Sprintf("stream %d field %d value mismatch", streamID, i),
					)
				}

				// Simulate peer decoder acknowledging the stream or incrementing insert count.
				// Per RFC 9204 Section 4.4.1, Section Acknowledgment is ONLY valid for streams
				// that reference the dynamic table (ric > 0).
				if ric > 0 {
					if rng.Intn(2) == 0 {
						encoder.OnHeaderAcknowledgement(streamID)
					} else {
						// Advance insert count increment up to inserted count
						inserted := encoder.HeaderTable().InsertedEntryCount()
						known := encoder.BlockingManager().KnownReceivedCount()
						if inserted > known {
							inc := uint64(1 + rng.Intn(int(inserted-known)))
							encoder.OnInsertCountIncrement(inc)
						}
						encoder.OnHeaderAcknowledgement(streamID)
					}
				}
			}

			// Final sanity assertions
			assert.Greater(t, totalHeadersEncoded, 1000, "must encode at least 1000 headers")
			assert.Equal(t, 0, len(errorDel.calls), "no decoder stream errors should occur")
			assert.Equal(t, 0, len(oracle.errorRecords), "no receiver errors should occur")
		})
	}
}

// -----------------------------------------------------------------------------
// Test 2: Eviction Barrier Invariant Under Cancellations & Acks (crbug.com/1441880)
// -----------------------------------------------------------------------------

func TestM4EncoderAdversarial_EvictionBarrierPreservationUnderCancellations(t *testing.T) {
	t.Run("DeterministicCancellationBeforeAckPreservesUnackedEntry", func(t *testing.T) {
		// Table capacity for exactly 2 entries (two 38-byte entries = 76 bytes).
		// Entry 0: "foo": "bar" (38 bytes)
		// Entry 1: "bar": "baz" (38 bytes)
		const capacity = 76

		errorDel := newMockDecoderStreamErrorDelegate()
		senderDel := newAdversarialSenderDelegate()
		encoder := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingDisabled, CookieCrumblingEnabled)
		encoder.SetStreamSenderDelegate(senderDel)
		encoder.SetMaximumBlockedStreams(10)
		encoder.SetMaximumDynamicTableCapacity(capacity)
		encoder.SetDynamicTableCapacity(capacity)

		headerTable := encoder.HeaderTable()
		require.Equal(t, uint64(0), headerTable.InsertedEntryCount())
		require.Equal(t, uint64(0), headerTable.DroppedEntryCount())

		// Step 1: Stream 1 inserts Entry 0 ("foo", "bar", 38 bytes)
		output1 := encoder.EncodeHeaderList(1, []HeaderField{{Name: "foo", Value: "bar"}}, nil)
		require.Equal(t, uint64(1), headerTable.InsertedEntryCount())
		require.Equal(t, uint64(0), headerTable.DroppedEntryCount())
		// KnownReceivedCount is 0. Entry 0 is unacknowledged.
		require.Equal(t, uint64(0), encoder.BlockingManager().KnownReceivedCount())

		// Step 2: Stream 1 is cancelled before acknowledgment arrives!
		// In Chromium (crbug.com/1441880), cancelling stream 1 removes its reference from the blocking manager.
		// SmallestBlockingIndex() becomes math.MaxUint64!
		encoder.OnStreamCancellation(1)
		require.Equal(t, uint64(math.MaxUint64), encoder.BlockingManager().SmallestBlockingIndex())
		require.Equal(t, uint64(0), encoder.BlockingManager().KnownReceivedCount())

		// Step 3: Stream 2 attempts to insert a 40-byte entry ("alpha", "beta")
		// DynamicTableSize is 38. Capacity is 76. Space left is 38 < 40.
		// To insert "alpha": "beta", the encoder would HAVE to evict Entry 0!
		// BUT Entry 0 is unacknowledged by peer (KnownReceivedCount == 0).
		// Evicting Entry 0 would corrupt peer decoder state.
		// Invariant: Entry 0 MUST NOT be evicted. Stream 2 MUST encode as literal!
		output2 := encoder.EncodeHeaderList(2, []HeaderField{{Name: "alpha", Value: "beta"}}, nil)
		require.Equal(t, uint64(0), headerTable.DroppedEntryCount(), "unacknowledged Entry 0 MUST NOT be evicted!")
		require.Equal(t, uint64(1), headerTable.InsertedEntryCount(), "no new dynamic insertion should have occurred")

		// Output2 must be encoded as literal without dynamic references (RIC=0)
		// Expected literal header: prefix 0000 + literal header field [0010 0101 alpha] [0000 0100 beta]
		assert.Equal(t, byte(0x00), output2[0], "RIC must be 0")
		assert.Equal(t, byte(0x00), output2[1], "DeltaBase must be 0")

		// Step 4: Stream 3 attempts to insert another entry ("gamma", "delta", 40 bytes)
		// Still cannot evict Entry 0.
		output3 := encoder.EncodeHeaderList(3, []HeaderField{{Name: "gamma", Value: "delta"}}, nil)
		require.Equal(t, uint64(0), headerTable.DroppedEntryCount(), "unacknowledged Entry 0 MUST NOT be evicted!")
		require.Equal(t, uint64(1), headerTable.InsertedEntryCount())

		// Step 5: Peer acknowledges Entry 0 via InsertCountIncrement(1)
		encoder.OnInsertCountIncrement(1)
		require.Equal(t, uint64(1), encoder.BlockingManager().KnownReceivedCount())

		// Step 6: Now that Entry 0 is acknowledged and stream 1 is cancelled:
		// Entry 0 CAN legally be evicted to make room!
		output4 := encoder.EncodeHeaderList(4, []HeaderField{{Name: "alpha", Value: "beta"}}, nil)
		// Entry 0 was evicted to make room for "alpha": "beta"!
		require.Equal(t, uint64(1), headerTable.DroppedEntryCount(), "Entry 0 should now be evicted")
		require.Equal(t, uint64(2), headerTable.InsertedEntryCount(), "new Entry 1 inserted")

		// Output4 should reference the newly inserted dynamic Entry 1!
		// Required Insert Count should be 2.
		ric4, ok := DecodeRequiredInsertCount(
			uint64(output4[0]),
			headerTable.MaxEntries(),
			headerTable.InsertedEntryCount(),
		)
		require.True(t, ok)
		assert.Equal(t, uint64(2), ric4)
		_ = output1
		_ = output3
	})

	t.Run("ActiveStreamReferenceBarrierEvenWhenAcknowledged", func(t *testing.T) {
		// Table capacity for 2 entries (76 bytes).
		const capacity = 76

		errorDel := newMockDecoderStreamErrorDelegate()
		senderDel := newAdversarialSenderDelegate()
		encoder := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingDisabled, CookieCrumblingEnabled)
		encoder.SetStreamSenderDelegate(senderDel)
		encoder.SetMaximumBlockedStreams(10)
		encoder.SetMaximumDynamicTableCapacity(capacity)
		encoder.SetDynamicTableCapacity(capacity)

		headerTable := encoder.HeaderTable()

		// Stream 1 inserts Entry 0 ("foo", "bar", 38 bytes)
		encoder.EncodeHeaderList(1, []HeaderField{{Name: "foo", Value: "bar"}}, nil)

		// Peer acknowledges Entry 0 via InsertCountIncrement(1)
		encoder.OnInsertCountIncrement(1)
		require.Equal(t, uint64(1), encoder.BlockingManager().KnownReceivedCount())

		// Stream 2 references Entry 0 (exact match)
		encoder.EncodeHeaderList(2, []HeaderField{{Name: "foo", Value: "bar"}}, nil)

		// Stream 2 is now in-flight and references Entry 0!
		// Even though Entry 0 is acknowledged by peer (knownReceivedCount == 1),
		// Stream 2 still depends on it in flight!
		// Therefore, SmallestBlockingIndex() is 0.
		require.Equal(t, uint64(0), encoder.BlockingManager().SmallestBlockingIndex())

		// Stream 3 tries to insert "alpha": "beta" (40 bytes), requiring eviction of Entry 0.
		encoder.EncodeHeaderList(3, []HeaderField{{Name: "alpha", Value: "beta"}}, nil)

		// Invariant: Entry 0 MUST NOT be evicted while Stream 2 is unacknowledged!
		assert.Equal(
			t,
			uint64(0),
			headerTable.DroppedEntryCount(),
			"Entry 0 cannot be evicted while active stream references it",
		)

		// Stream 1 acknowledged: Stream 2 is STILL active!
		encoder.OnHeaderAcknowledgement(1)
		assert.Equal(t, uint64(0), headerTable.DroppedEntryCount())

		// Finally, Stream 2 acknowledged:
		encoder.OnHeaderAcknowledgement(2)
		require.Equal(t, uint64(math.MaxUint64), encoder.BlockingManager().SmallestBlockingIndex())

		// Now Stream 4 can insert and evict Entry 0!
		encoder.EncodeHeaderList(4, []HeaderField{{Name: "alpha", Value: "beta"}}, nil)
		assert.Equal(t, uint64(1), headerTable.DroppedEntryCount(), "Entry 0 should now be evicted")
	})

	t.Run("RandomizedMultiStreamInterleavedBarrierStress", func(t *testing.T) {
		const capacity = 256
		const numStreams = 100

		errorDel := newMockDecoderStreamErrorDelegate()
		senderDel := newAdversarialSenderDelegate()
		encoder := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingDisabled, CookieCrumblingEnabled)
		encoder.SetStreamSenderDelegate(senderDel)
		encoder.SetMaximumBlockedStreams(50)
		encoder.SetMaximumDynamicTableCapacity(capacity)
		encoder.SetDynamicTableCapacity(capacity)

		rng := rand.New(rand.NewSource(9999))

		// Invariant tracking model:
		// streamRefs tracks dynamic indices referenced by currently active, unacknowledged streams
		type activeStream struct {
			indices []uint64
		}
		activeStreams := make(map[uint64]*activeStream)

		// Function to compute the strict legal eviction barrier:
		// An entry with index i can only be evicted if:
		// 1. i < knownReceivedCount, AND
		// 2. For every active stream, i < min(indices referenced by that stream)
		computeSafeBarrier := func() uint64 {
			barrier := encoder.BlockingManager().KnownReceivedCount()
			for _, st := range activeStreams {
				for _, idx := range st.indices {
					if idx < barrier {
						barrier = idx
					}
				}
			}
			return barrier
		}

		for i := 0; i < 500; i++ {
			action := rng.Intn(4)
			switch action {
			case 0:
				// Action 0: Encode header on a new or existing stream
				streamID := uint64(1 + rng.Intn(numStreams))
				insertedBefore := encoder.HeaderTable().InsertedEntryCount()

				hf := HeaderField{
					Name:  fmt.Sprintf("header-%d", rng.Intn(20)),
					Value: fmt.Sprintf("val-%d", rng.Intn(100)),
				}

				encoder.EncodeHeaderList(streamID, []HeaderField{hf}, nil)
				insertedAfter := encoder.HeaderTable().InsertedEntryCount()

				// If new entry inserted, record it for stream
				if _, ok := activeStreams[streamID]; !ok {
					activeStreams[streamID] = &activeStream{}
				}
				if insertedAfter > insertedBefore {
					for idx := insertedBefore; idx < insertedAfter; idx++ {
						activeStreams[streamID].indices = append(activeStreams[streamID].indices, idx)
					}
				}

			case 1:
				// Action 1: Stream Cancellation
				if len(activeStreams) > 0 {
					var targetID uint64
					for sid := range activeStreams {
						targetID = sid
						break
					}
					encoder.OnStreamCancellation(targetID)
					delete(activeStreams, targetID)
				}

			case 2:
				// Action 2: Section Acknowledgement
				if len(activeStreams) > 0 {
					var targetID uint64
					for sid := range activeStreams {
						targetID = sid
						break
					}
					encoder.OnHeaderAcknowledgement(targetID)
					delete(activeStreams, targetID)
				}

			case 3:
				// Action 3: Insert Count Increment
				inserted := encoder.HeaderTable().InsertedEntryCount()
				known := encoder.BlockingManager().KnownReceivedCount()
				if inserted > known {
					maxInc := inserted - known
					inc := uint64(1 + rng.Intn(int(maxInc)))
					encoder.OnInsertCountIncrement(inc)
				}
			}

			// Invariant verification at every single step!
			safeBarrier := computeSafeBarrier()
			droppedCount := encoder.HeaderTable().DroppedEntryCount()
			require.True(
				t,
				droppedCount <= safeBarrier,
				fmt.Sprintf(
					"EVICTION BARRIER VIOLATION: droppedCount=%d exceeded safeBarrier=%d at step %d! Unacknowledged or active-referenced entry was prematurely evicted!",
					droppedCount,
					safeBarrier,
					i,
				),
			)
		}
	})
}

// -----------------------------------------------------------------------------
// Test 3: Adversarial Decoder Stream Feedback & Wire Error Handling
// -----------------------------------------------------------------------------

func TestM4EncoderAdversarial_AdversarialDecoderStreamFeedback(t *testing.T) {
	t.Run("ZeroIncrementTriggersError", func(t *testing.T) {
		errorDel := newMockDecoderStreamErrorDelegate()
		encoder := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

		encoder.OnInsertCountIncrement(0)
		require.Equal(t, 1, len(errorDel.calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INVALID_ZERO_INCREMENT, errorDel.calls[0].ErrorCode)
	})

	t.Run("IncrementOverflowTriggersError", func(t *testing.T) {
		errorDel := newMockDecoderStreamErrorDelegate()
		encoder := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
		encoder.SetMaximumDynamicTableCapacity(4096)
		encoder.SetDynamicTableCapacity(4096)
		encoder.HeaderTable().InsertEntry("foo", "bar")

		// First advance knownReceivedCount to 1
		encoder.OnInsertCountIncrement(1)
		require.Equal(t, 0, len(errorDel.calls))

		// Now 1 + math.MaxUint64 overflows uint64
		encoder.OnInsertCountIncrement(math.MaxUint64)
		require.Equal(t, 1, len(errorDel.calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INCREMENT_OVERFLOW, errorDel.calls[0].ErrorCode)
	})

	t.Run("ImpossibleIncrementTriggersError", func(t *testing.T) {
		errorDel := newMockDecoderStreamErrorDelegate()
		encoder := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
		encoder.SetMaximumDynamicTableCapacity(4096)
		encoder.SetDynamicTableCapacity(4096)

		// Table has 0 entries, peer claims increment of 5
		encoder.OnInsertCountIncrement(5)
		require.Equal(t, 1, len(errorDel.calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_IMPOSSIBLE_INSERT_COUNT, errorDel.calls[0].ErrorCode)
	})

	t.Run("UnsolicitedSectionAckTriggersError", func(t *testing.T) {
		errorDel := newMockDecoderStreamErrorDelegate()
		encoder := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

		// Ack for stream with no sent header blocks
		encoder.OnHeaderAcknowledgement(999)
		require.Equal(t, 1, len(errorDel.calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INCORRECT_ACKNOWLEDGEMENT, errorDel.calls[0].ErrorCode)
	})

	t.Run("UnknownStreamCancellationIsSafeNoop", func(t *testing.T) {
		errorDel := newMockDecoderStreamErrorDelegate()
		encoder := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)

		// Cancellation for non-existent stream must not panic or trigger error
		encoder.OnStreamCancellation(12345)
		encoder.OnStreamCancellation(12345) // duplicate
		assert.Equal(t, 0, len(errorDel.calls))
	})

	t.Run("DecoderStreamReceiverWireBytesParsing", func(t *testing.T) {
		errorDel := newMockDecoderStreamErrorDelegate()
		senderDel := newAdversarialSenderDelegate()
		encoder := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
		encoder.SetStreamSenderDelegate(senderDel)
		encoder.SetMaximumBlockedStreams(10)
		encoder.SetMaximumDynamicTableCapacity(4096)
		encoder.SetDynamicTableCapacity(4096)

		// Stream 1 sends a header block with 1 insertion
		encoder.EncodeHeaderList(1, []HeaderField{{Name: "foo", Value: "bar"}}, nil)
		require.Equal(t, uint64(1), encoder.HeaderTable().InsertedEntryCount())

		decoderReceiver := encoder.DecoderStreamReceiver()

		// 1. Send valid SectionAck for stream 1 via wire bytes: [1000 0001] -> 0x81 (stream 1)
		decoderReceiver.Decode([]byte{0x81})
		assert.Equal(t, 0, len(errorDel.calls), "valid section ack wire bytes must succeed")
		assert.Equal(t, uint64(1), encoder.BlockingManager().KnownReceivedCount())

		// 2. Send invalid unsolicited SectionAck for stream 1 via wire bytes (stream 1 has no more blocks):
		decoderReceiver.Decode([]byte{0x81})
		require.Equal(t, 1, len(errorDel.calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INCORRECT_ACKNOWLEDGEMENT, errorDel.calls[0].ErrorCode)

		// 3. Send corrupt integer (varint overflow) on decoder stream: 11 consecutive 0xff bytes
		overflowBytes := bytes.Repeat([]byte{0xff}, 11)
		decoderReceiver.Decode(overflowBytes)
		require.GreaterOrEqual(t, len(errorDel.calls), 2)
	})
}

// -----------------------------------------------------------------------------
// Test 4: Dynamic Capacity Transitions Under Active In-Flight Streams
// -----------------------------------------------------------------------------

func TestM4EncoderAdversarial_DynamicCapacityTransitionsUnderFlight(t *testing.T) {
	const maxCapacity = 4096
	errorDel := newMockDecoderStreamErrorDelegate()
	senderDel := newAdversarialSenderDelegate()
	encoder := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingDisabled, CookieCrumblingEnabled)
	encoder.SetStreamSenderDelegate(senderDel)
	encoder.SetMaximumBlockedStreams(20)
	encoder.SetMaximumDynamicTableCapacity(maxCapacity)
	encoder.SetDynamicTableCapacity(maxCapacity)

	headerTable := encoder.HeaderTable()

	// Fill table with 20 entries
	for i := 0; i < 20; i++ {
		encoder.EncodeHeaderList(uint64(i+1), []HeaderField{
			{Name: fmt.Sprintf("x-header-%d", i), Value: fmt.Sprintf("val-%d", i)},
		}, nil)
	}

	require.Equal(t, uint64(20), headerTable.InsertedEntryCount())
	require.Equal(t, uint64(0), headerTable.DroppedEntryCount())
	initialSize := headerTable.DynamicTableSize()
	require.Greater(t, initialSize, uint64(500))

	// Transition 1: Shrink capacity down to 250 bytes
	senderDel.Clear()
	encoder.SetDynamicTableCapacity(250)
	encoder.EncoderStreamSender().Flush()
	assert.LessOrEqual(t, headerTable.DynamicTableSize(), uint64(250))
	assert.Greater(t, headerTable.DroppedEntryCount(), uint64(0))
	assert.Contains(t, string(senderDel.AllWrittenData()), "\x3f") // capacity instruction opcode

	// Transition 2: Shrink capacity down to 0 bytes!
	senderDel.Clear()
	encoder.SetDynamicTableCapacity(0)
	encoder.EncoderStreamSender().Flush()
	assert.Equal(t, uint64(0), headerTable.DynamicTableSize())
	assert.Equal(t, headerTable.InsertedEntryCount(), headerTable.DroppedEntryCount())

	// While capacity is 0, encode headers: all must encode as literals (RIC = 0)
	outZeroCap := encoder.EncodeHeaderList(100, []HeaderField{
		{Name: "x-foo", Value: "bar"},
		{Name: ":method", Value: "GET"},
	}, nil)
	assert.Equal(t, byte(0x00), outZeroCap[0], "with capacity 0, RIC must be 0")
	assert.Equal(t, byte(0x00), outZeroCap[1], "DeltaBase must be 0")

	// Cancel streams 1..20 so blocked stream quota is freed
	for i := 1; i <= 20; i++ {
		encoder.OnStreamCancellation(uint64(i))
	}
	require.Equal(t, uint64(0), encoder.BlockingManager().NumBlockedStreams())

	// Peer acknowledges the 20 inserts via InsertCountIncrement
	encoder.OnInsertCountIncrement(20)
	require.Equal(t, uint64(20), encoder.BlockingManager().KnownReceivedCount())

	// Transition 3: Grow capacity back to 1024 bytes
	senderDel.Clear()
	encoder.SetDynamicTableCapacity(1024)
	assert.Equal(t, uint64(1024), headerTable.DynamicTableCapacity())

	// New dynamic insertions can now resume!
	outGrow := encoder.EncodeHeaderList(101, []HeaderField{
		{Name: "x-new-entry", Value: "new-val"},
	}, nil)
	require.Greater(t, headerTable.InsertedEntryCount(), uint64(20))
	assert.Greater(t, headerTable.DynamicTableSize(), uint64(0))
	_ = outGrow

	// Transition 4: Rapid alternating cycles of capacity shrinks and grows
	capacities := []uint64{2048, 512, 100, 0, 300, 1500, 0, 4096}
	for step, capVal := range capacities {
		encoder.SetDynamicTableCapacity(capVal)
		assert.LessOrEqual(
			t,
			headerTable.DynamicTableSize(),
			capVal,
			fmt.Sprintf("step %d: table size must not exceed capacity", step),
		)

		// Acknowledge any new inserts so far
		inserted := headerTable.InsertedEntryCount()
		known := encoder.BlockingManager().KnownReceivedCount()
		if inserted > known {
			encoder.OnInsertCountIncrement(inserted - known)
		}

		// Encode on stream
		sid := uint64(200 + step)
		encoder.EncodeHeaderList(sid, []HeaderField{
			{Name: fmt.Sprintf("x-cycle-%d", step), Value: "val"},
		}, nil)
		assert.LessOrEqual(t, headerTable.DynamicTableSize(), capVal)
	}

	assert.Equal(t, 0, len(errorDel.calls))
}

// -----------------------------------------------------------------------------
// Test 5: Blocked Streams Flow Control Limit
// -----------------------------------------------------------------------------

func TestM4EncoderAdversarial_BlockedStreamsFlowControl(t *testing.T) {
	t.Run("MaxBlockedStreamsZeroDisallowsBlockingReferences", func(t *testing.T) {
		errorDel := newMockDecoderStreamErrorDelegate()
		senderDel := newAdversarialSenderDelegate()
		encoder := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingDisabled, CookieCrumblingEnabled)
		encoder.SetStreamSenderDelegate(senderDel)
		encoder.SetMaximumDynamicTableCapacity(4096)
		encoder.SetDynamicTableCapacity(4096)
		encoder.SetMaximumBlockedStreams(0) // 0 blocked streams allowed!

		// Table has capacity, but maxBlockedStreams is 0.
		// Any dynamic insertion or reference to an unacknowledged entry would block the stream.
		// Therefore, encoder MUST NOT insert into dynamic table!
		out := encoder.EncodeHeaderList(1, []HeaderField{{Name: "x-custom", Value: "custom-val"}}, nil)
		assert.Equal(t, uint64(0), encoder.HeaderTable().InsertedEntryCount())
		assert.Equal(t, byte(0x00), out[0], "RIC must be 0")
		assert.Equal(t, uint64(0), encoder.BlockingManager().NumBlockedStreams())
	})

	t.Run("MaxBlockedStreamsOneEnforcesSingleBlockedStreamLimit", func(t *testing.T) {
		errorDel := newMockDecoderStreamErrorDelegate()
		senderDel := newAdversarialSenderDelegate()
		encoder := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingDisabled, CookieCrumblingEnabled)
		encoder.SetStreamSenderDelegate(senderDel)
		encoder.SetMaximumDynamicTableCapacity(4096)
		encoder.SetDynamicTableCapacity(4096)
		encoder.SetMaximumBlockedStreams(1) // only 1 blocked stream allowed!

		// Stream 1 inserts Entry 0 -> Stream 1 becomes blocked
		encoder.EncodeHeaderList(1, []HeaderField{{Name: "foo", Value: "bar"}}, nil)
		require.Equal(t, uint64(1), encoder.HeaderTable().InsertedEntryCount())
		require.Equal(t, uint64(1), encoder.BlockingManager().NumBlockedStreams())
		require.True(t, encoder.BlockingManager().IsBlocked(1))

		// Stream 2 attempts to insert / reference unacknowledged Entry 0:
		// Since 1 stream is already blocked and max is 1, Stream 2 CANNOT block!
		// It must encode as literal!
		out2 := encoder.EncodeHeaderList(2, []HeaderField{{Name: "bar", Value: "baz"}}, nil)
		require.Equal(t, uint64(1), encoder.HeaderTable().InsertedEntryCount(), "Stream 2 must not insert new entries")
		require.Equal(t, byte(0x00), out2[0], "Stream 2 RIC must be 0")
		require.False(t, encoder.BlockingManager().IsBlocked(2))

		// Now Stream 1 is acknowledged by peer
		encoder.OnHeaderAcknowledgement(1)
		require.Equal(t, uint64(0), encoder.BlockingManager().NumBlockedStreams())

		// Now Stream 3 CAN insert into dynamic table!
		encoder.EncodeHeaderList(3, []HeaderField{{Name: "baz", Value: "qux"}}, nil)
		require.Equal(t, uint64(2), encoder.HeaderTable().InsertedEntryCount())
		require.Equal(t, uint64(1), encoder.BlockingManager().NumBlockedStreams())
	})
}

// -----------------------------------------------------------------------------
// Test 6: Control Stream Buffer Backpressure Throttling
// -----------------------------------------------------------------------------

func TestM4EncoderAdversarial_BufferBackpressureThrottling(t *testing.T) {
	errorDel := newMockDecoderStreamErrorDelegate()
	senderDel := newAdversarialSenderDelegate()
	encoder := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingDisabled, CookieCrumblingEnabled)
	encoder.SetStreamSenderDelegate(senderDel)
	encoder.SetMaximumDynamicTableCapacity(4096)
	encoder.SetDynamicTableCapacity(4096)
	encoder.SetMaximumBlockedStreams(10)

	// Simulate buffer saturation >= 64KB (CanWrite() == false)
	senderDel.SetNumBytesBuffered(64 * 1024)
	require.False(t, encoder.EncoderStreamSender().CanWrite())

	// Encode headers while buffer is saturated: dynamic table inserts MUST be inhibited!
	out1 := encoder.EncodeHeaderList(1, []HeaderField{{Name: "x-key", Value: "val"}}, nil)
	assert.Equal(t, uint64(0), encoder.HeaderTable().InsertedEntryCount())
	assert.Equal(t, byte(0x00), out1[0])

	// Drain buffer below threshold
	senderDel.SetNumBytesBuffered(0)
	require.True(t, encoder.EncoderStreamSender().CanWrite())

	// Dynamic insertions resume!
	out2 := encoder.EncodeHeaderList(2, []HeaderField{{Name: "x-key", Value: "val"}}, nil)
	assert.Equal(t, uint64(1), encoder.HeaderTable().InsertedEntryCount())
	assert.Greater(t, out2[0], byte(0x00))
}

// -----------------------------------------------------------------------------
// Test 7: Multi-Goroutine Isolation & Concurrent Race Detector Stress
// -----------------------------------------------------------------------------

func TestM4EncoderAdversarial_ConcurrentStressAndRaceDetector(t *testing.T) {
	// Subtest A: 16 parallel goroutines each running independent Encoder instances
	t.Run("IndependentInstancesParallel", func(t *testing.T) {
		const goroutines = 16
		var wg sync.WaitGroup
		wg.Add(goroutines)

		for g := 0; g < goroutines; g++ {
			go func(gid int) {
				defer wg.Done()

				errorDel := newMockDecoderStreamErrorDelegate()
				senderDel := newAdversarialSenderDelegate()
				enc := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingEnabled, CookieCrumblingEnabled)
				enc.SetStreamSenderDelegate(senderDel)
				enc.SetMaximumDynamicTableCapacity(2048)
				enc.SetDynamicTableCapacity(2048)
				enc.SetMaximumBlockedStreams(10)

				rng := rand.New(rand.NewSource(int64(gid * 1000)))

				for sid := uint64(1); sid <= 50; sid++ {
					hf := []HeaderField{
						{
							Name:  fmt.Sprintf("x-g-%d-key-%d", gid, rng.Intn(5)),
							Value: fmt.Sprintf("val-%d", rng.Intn(20)),
						},
						{Name: ":method", Value: "GET"},
					}
					enc.EncodeHeaderList(sid, hf, nil)

					if rng.Intn(2) == 0 {
						enc.OnHeaderAcknowledgement(sid)
					} else {
						enc.OnStreamCancellation(sid)
					}
				}
			}(g)
		}

		wg.Wait()
	})

	// Subtest B: Concurrent pipeline sharing single encoder under mutex protection
	t.Run("SharedInstanceSynchronizedPipeline", func(t *testing.T) {
		errorDel := newMockDecoderStreamErrorDelegate()
		senderDel := newAdversarialSenderDelegate()
		enc := NewEncoder(errorDel.OnDecoderStreamError, HuffmanEncodingDisabled, CookieCrumblingEnabled)
		enc.SetStreamSenderDelegate(senderDel)
		enc.SetMaximumDynamicTableCapacity(4096)
		enc.SetDynamicTableCapacity(4096)
		enc.SetMaximumBlockedStreams(50)

		var mu sync.Mutex
		var wg sync.WaitGroup
		const ops = 200

		// Worker 1: Encodes headers
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(111))
			for i := uint64(1); i <= ops; i++ {
				mu.Lock()
				enc.EncodeHeaderList(i, []HeaderField{
					{Name: fmt.Sprintf("header-%d", rng.Intn(10)), Value: fmt.Sprintf("val-%d", rng.Intn(20))},
				}, nil)
				mu.Unlock()
			}
		}()

		// Worker 2: Sends acknowledgments and cancellations
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(222))
			for i := uint64(1); i <= ops; i++ {
				mu.Lock()
				inserted := enc.HeaderTable().InsertedEntryCount()
				known := enc.BlockingManager().KnownReceivedCount()
				if inserted > known {
					enc.OnInsertCountIncrement(1)
				}
				if rng.Intn(2) == 0 {
					// safe ack or cancellation
					enc.OnStreamCancellation(i)
				}
				mu.Unlock()
			}
		}()

		wg.Wait()
	})
}
