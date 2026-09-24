// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"testing"
)

// -----------------------------------------------------------------------------
// 1. Push Iterators Adversarial Stress & Edge Cases
// -----------------------------------------------------------------------------

func TestChallenger_PushIterators_HeaderFields_EdgeCases(t *testing.T) {
	// A. Nil HeaderFields
	var nilHFS HeaderFields
	nilAllCount := 0
	for range nilHFS.All() {
		nilAllCount++
	}
	if nilAllCount != 0 {
		t.Errorf("expected 0 from nilHFS.All(), got %d", nilAllCount)
	}

	nilValCount := 0
	for range nilHFS.Values() {
		nilValCount++
	}
	if nilValCount != 0 {
		t.Errorf("expected 0 from nilHFS.Values(), got %d", nilValCount)
	}

	if _, ok := nilHFS.Get("any"); ok {
		t.Errorf("expected Get on nilHFS to return false")
	}

	// B. Empty HeaderFields
	emptyHFS := HeaderFields{}
	for range emptyHFS.All() {
		t.Errorf("unexpected iteration on emptyHFS.All()")
	}
	for range emptyHFS.Values() {
		t.Errorf("unexpected iteration on emptyHFS.Values()")
	}
	if _, ok := emptyHFS.Get("any"); ok {
		t.Errorf("expected Get on emptyHFS to return false")
	}

	// C. Massive HeaderFields (20,000 entries)
	const largeCount = 20000
	largeHFS := make(HeaderFields, largeCount)
	for i := 0; i < largeCount; i++ {
		largeHFS[i] = HeaderField{
			Name:  "custom-header-" + strconv.Itoa(i),
			Value: "value-" + strconv.Itoa(i),
		}
	}

	// Full All() iteration
	allVisited := 0
	for k, v := range largeHFS.All() {
		expectedKey := "custom-header-" + strconv.Itoa(allVisited)
		expectedVal := "value-" + strconv.Itoa(allVisited)
		if k != expectedKey || v != expectedVal {
			t.Fatalf("mismatch at %d: got (%s, %s), want (%s, %s)", allVisited, k, v, expectedKey, expectedVal)
		}
		allVisited++
	}
	if allVisited != largeCount {
		t.Fatalf("expected %d items visited, got %d", largeCount, allVisited)
	}

	// Full Values() iteration
	valVisited := 0
	for hf := range largeHFS.Values() {
		expectedKey := "custom-header-" + strconv.Itoa(valVisited)
		if hf.Name != expectedKey {
			t.Fatalf("Values() mismatch at %d: got %s, want %s", valVisited, hf.Name, expectedKey)
		}
		valVisited++
	}
	if valVisited != largeCount {
		t.Fatalf("expected %d values visited, got %d", largeCount, valVisited)
	}

	// Early break variations
	breakPoints := []int{1, 5, 500, largeCount / 2, largeCount - 1}
	for _, bp := range breakPoints {
		cnt := 0
		for range largeHFS.All() {
			cnt++
			if cnt == bp {
				break
			}
		}
		if cnt != bp {
			t.Errorf("expected early break at %d, got %d", bp, cnt)
		}

		cnt = 0
		for range largeHFS.Values() {
			cnt++
			if cnt == bp {
				break
			}
		}
		if cnt != bp {
			t.Errorf("expected early break at %d in Values(), got %d", bp, cnt)
		}
	}

	// D. HeaderFields.Get edge cases (case-insensitivity, pseudo headers)
	mixedHFS := HeaderFields{
		{Name: ":status", Value: "200"},
		{Name: ":path", Value: "/index.html"},
		{Name: "Content-Type", Value: "application/json"},
		{Name: "content-type", Value: "text/plain"}, // duplicate name
		{Name: "X-Custom-TOKEN", Value: "Secret123"},
	}

	// Case sensitivity & folding
	if val, ok := mixedHFS.Get("content-type"); !ok || val != "application/json" {
		t.Errorf("expected application/json (first match), got (%s, %v)", val, ok)
	}
	if val, ok := mixedHFS.Get("CONTENT-TYPE"); !ok || val != "application/json" {
		t.Errorf("expected application/json for uppercase, got (%s, %v)", val, ok)
	}
	if val, ok := mixedHFS.Get("x-custom-token"); !ok || val != "Secret123" {
		t.Errorf("expected Secret123 for lowercase lookup, got (%s, %v)", val, ok)
	}
	if val, ok := mixedHFS.Get(":STATUS"); !ok || val != "200" {
		t.Errorf("expected 200 for :STATUS lookup, got (%s, %v)", val, ok)
	}
	if val, ok := mixedHFS.Get("nonexistent"); ok || val != "" {
		t.Errorf("expected false for nonexistent header")
	}

	// E. HeaderField.IsPseudo
	if !(HeaderField{Name: ":path"}).IsPseudo() {
		t.Errorf("expected :path to be pseudo")
	}
	if !(HeaderField{Name: ":"}).IsPseudo() {
		t.Errorf("expected : to be pseudo")
	}
	if (HeaderField{Name: "path"}).IsPseudo() {
		t.Errorf("expected path not to be pseudo")
	}
	if (HeaderField{Name: ""}).IsPseudo() {
		t.Errorf("expected empty name not to be pseudo")
	}
}

func TestChallenger_PushIterators_HeaderTables_ChurnAndEviction(t *testing.T) {
	// A. EncoderHeaderTable churn and iteration
	encTable := NewEncoderHeaderTable()
	encTable.SetMaximumDynamicTableCapacity(200)
	encTable.SetDynamicTableCapacity(200)

	// Empty iteration
	for range encTable.Entries() {
		t.Errorf("unexpected entry from empty encTable.Entries()")
	}

	// Insert 20 entries of size 40 each (name "kXX" (3), value "vXX" (3) + 32 overhead = 38 bytes)
	// Capacity 200 allows 5 entries simultaneously (5 * 38 = 190 bytes).
	// Inserting 20 entries will evict 15 entries!
	for i := 0; i < 20; i++ {
		encTable.InsertEntry(fmt.Sprintf("k%02d", i), fmt.Sprintf("v%02d", i))
	}

	if encTable.DroppedEntryCount() != 15 {
		t.Fatalf("expected 15 dropped entries, got %d", encTable.DroppedEntryCount())
	}
	if encTable.InsertedEntryCount() != 20 {
		t.Fatalf("expected 20 inserted entries, got %d", encTable.InsertedEntryCount())
	}

	// Iterate over active entries
	var encCollected []uint64
	for absIdx, entry := range encTable.Entries() {
		encCollected = append(encCollected, absIdx)
		expectedName := fmt.Sprintf("k%02d", absIdx)
		if entry.Name != expectedName {
			t.Errorf("mismatch at dynamic entry %d: got name %s, want %s", absIdx, entry.Name, expectedName)
		}
	}
	if len(encCollected) != 5 {
		t.Fatalf("expected 5 active dynamic entries, got %d", len(encCollected))
	}
	if encCollected[0] != 15 || encCollected[4] != 19 {
		t.Errorf("unexpected index range: %v", encCollected)
	}

	// Early break in EncoderHeaderTable.Entries()
	breakCount := 0
	for range encTable.Entries() {
		breakCount++
		if breakCount == 2 {
			break
		}
	}
	if breakCount != 2 {
		t.Errorf("expected early break at 2, got %d", breakCount)
	}

	// B. DecoderHeaderTable churn and iteration
	decTable := NewDecoderHeaderTable()
	decTable.SetMaximumDynamicTableCapacity(200)
	decTable.SetDynamicTableCapacity(200)

	for range decTable.Entries() {
		t.Errorf("unexpected entry from empty decTable.Entries()")
	}

	for i := 0; i < 25; i++ {
		decTable.InsertEntry(fmt.Sprintf("d%02d", i), fmt.Sprintf("val%02d", i))
	}

	var decCollected []uint64
	for absIdx, entry := range decTable.Entries() {
		decCollected = append(decCollected, absIdx)
		expectedName := fmt.Sprintf("d%02d", absIdx)
		if entry.Name != expectedName {
			t.Errorf("mismatch in decoder dynamic entry %d: got %s, want %s", absIdx, entry.Name, expectedName)
		}
	}
	if len(decCollected) != int(decTable.dynamicEntries.Len()) {
		t.Fatalf("decoder entries count mismatch: got %d, want %d", len(decCollected), decTable.dynamicEntries.Len())
	}

	// Early break
	breakCount = 0
	for range decTable.Entries() {
		breakCount++
		break
	}
	if breakCount != 1 {
		t.Errorf("expected early break at 1, got %d", breakCount)
	}
}

func TestChallenger_PushIterators_IndexSetAndBlockingManager(t *testing.T) {
	// A. IndexSet edge cases
	set := NewIndexSet()
	if !set.Empty() || set.Size() != 0 {
		t.Errorf("expected empty IndexSet")
	}
	if _, ok := set.MinIndex(); ok {
		t.Errorf("expected false for MinIndex on empty set")
	}
	if _, ok := set.MaxIndex(); ok {
		t.Errorf("expected false for MaxIndex on empty set")
	}
	if set.RequiredInsertCount() != 0 {
		t.Errorf("expected 0 RIC on empty set")
	}
	for range set.All() {
		t.Errorf("unexpected item from empty IndexSet.All()")
	}

	// Large insertion into IndexSet
	const setSize = 5000
	for i := uint64(0); i < setSize; i++ {
		// insert in reverse order
		set.Insert(setSize - 1 - i)
	}
	if set.Size() != setSize {
		t.Fatalf("expected size %d, got %d", setSize, set.Size())
	}
	minIdx, hasMin := set.MinIndex()
	if !hasMin || minIdx != 0 {
		t.Errorf("expected minIndex 0, got (%d, %v)", minIdx, hasMin)
	}
	maxIdx, hasMax := set.MaxIndex()
	if !hasMax || maxIdx != setSize-1 {
		t.Errorf("expected maxIndex %d, got (%d, %v)", setSize-1, maxIdx, hasMax)
	}
	if set.RequiredInsertCount() != setSize {
		t.Errorf("expected RIC %d, got %d", setSize, set.RequiredInsertCount())
	}

	// Verify All() preserves insertion order
	idxCount := 0
	for idx := range set.All() {
		expected := setSize - 1 - uint64(idxCount)
		if idx != expected {
			t.Fatalf("IndexSet.All() order mismatch at %d: got %d, want %d", idxCount, idx, expected)
		}
		idxCount++
	}
	if idxCount != setSize {
		t.Fatalf("expected %d elements, got %d", setSize, idxCount)
	}

	// Early break in IndexSet.All()
	count := 0
	for range set.All() {
		count++
		if count == 7 {
			break
		}
	}
	if count != 7 {
		t.Errorf("expected early break at 7, got %d", count)
	}

	// B. BlockingManager.BlockedStreams edge cases
	bm := NewBlockingManager(1000)
	for range bm.BlockedStreams() {
		t.Errorf("unexpected stream from empty BlockedStreams()")
	}

	// Block 100 streams (stream sID refers to dynamic index sID-1, so RIC = sID)
	for sID := uint64(1); sID <= 100; sID++ {
		bm.OnHeaderBlockSent(sID, []uint64{sID - 1})
	}
	if bm.NumBlockedStreams() != 100 {
		t.Fatalf("expected 100 blocked streams, got %d", bm.NumBlockedStreams())
	}

	blockedMap := make(map[uint64]bool)
	for sID := range bm.BlockedStreams() {
		blockedMap[sID] = true
	}
	if len(blockedMap) != 100 {
		t.Fatalf("expected 100 unique streams from BlockedStreams(), got %d", len(blockedMap))
	}

	// Early break on BlockedStreams()
	count = 0
	for range bm.BlockedStreams() {
		count++
		if count == 15 {
			break
		}
	}
	if count != 15 {
		t.Errorf("expected early break at 15, got %d", count)
	}

	// Unblock 50 streams via InsertCountIncrement (satisfies RIC <= 50)
	bm.OnInsertCountIncrement(50)
	if bm.NumBlockedStreams() != 50 {
		t.Fatalf("expected 50 remaining blocked streams, got %d", bm.NumBlockedStreams())
	}

	// Stream cancellation
	for sID := uint64(51); sID <= 100; sID++ {
		bm.OnStreamCancellation(sID)
	}
	if bm.NumBlockedStreams() != 0 {
		t.Fatalf("expected 0 blocked streams after cancellation, got %d", bm.NumBlockedStreams())
	}
}

// -----------------------------------------------------------------------------
// 2. Interface Compliance & Streaming Robustness
// -----------------------------------------------------------------------------

func TestChallenger_Streaming_ProgressiveDecoder_ChunkedAndByteByByte(t *testing.T) {
	encoder := NewEncoderWithDefaults(nil)
	decoder := NewDecoder(0, 0, nil)

	fields := []HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "application/json"},
		{Name: "cache-control", Value: "no-cache, no-store, must-revalidate"},
		{Name: "x-request-id", Value: "7b4e2d31-9f1a-4c28-bb73-8a0f9e3d1c52"},
		{Name: "date", Value: "Tue, 22 Sep 2026 14:00:00 GMT"},
	}

	block := encoder.EncodeHeaderList(1, fields, nil)

	// A. 1 Byte per Write
	handler1 := &collectedHeadersHandler{}
	progDec1 := decoder.CreateProgressiveDecoder(1, handler1)

	for i := 0; i < len(block); i++ {
		n, err := progDec1.Write(block[i : i+1])
		if err != nil {
			t.Fatalf("Write at byte %d failed: %v", i, err)
		}
		if n != 1 {
			t.Fatalf("Write wrote %d bytes, want 1", n)
		}
	}
	if err := progDec1.Close(); err != nil {
		t.Fatalf("progDec1.Close() failed: %v", err)
	}
	if len(handler1.headers) != len(fields) {
		t.Fatalf("byte-by-byte: decoded %d headers, want %d", len(handler1.headers), len(fields))
	}
	for i := range fields {
		if handler1.headers[i].Name != fields[i].Name || handler1.headers[i].Value != fields[i].Value {
			t.Errorf("byte-by-byte header %d mismatch: got %v, want %v", i, handler1.headers[i], fields[i])
		}
	}

	// B. Irregular Chunk Sizes (3, 7, 2, 13 bytes)
	handler2 := &collectedHeadersHandler{}
	progDec2 := decoder.CreateProgressiveDecoder(2, handler2)
	chunkSizes := []int{3, 7, 2, 13, 1, 5}
	chunkIdx := 0
	offset := 0
	for offset < len(block) {
		sz := chunkSizes[chunkIdx%len(chunkSizes)]
		chunkIdx++
		if offset+sz > len(block) {
			sz = len(block) - offset
		}
		n, err := progDec2.Write(block[offset : offset+sz])
		if err != nil {
			t.Fatalf("chunked write failed at offset %d: %v", offset, err)
		}
		if n != sz {
			t.Fatalf("chunked write wrote %d bytes, want %d", n, sz)
		}
		offset += sz
	}
	if err := progDec2.Close(); err != nil {
		t.Fatalf("progDec2.Close() failed: %v", err)
	}
	if len(handler2.headers) != len(fields) {
		t.Fatalf("irregular chunking: decoded %d headers, want %d", len(handler2.headers), len(fields))
	}
}

func TestChallenger_Streaming_ProgressiveDecoder_CorruptAndAdversarial(t *testing.T) {
	decoder := NewDecoder(0, 0, nil)

	corruptPayloads := [][]byte{
		{0xff},                               // incomplete prefix
		{0x00, 0xff},                         // incomplete instruction
		{0x00, 0x80 | 99},                    // invalid static table index
		{0x1f, 0xff, 0xff, 0xff, 0xff, 0xff}, // truncated varint
		{0x00, 0x20, 0x81, 0x00},             // literal with invalid huffman
		{0x80, 0x80, 0x80, 0x80, 0x80},       // random garbage
	}

	for idx, corrupt := range corruptPayloads {
		handler := &collectedHeadersHandler{}
		progDec := decoder.CreateProgressiveDecoder(uint64(idx+10), handler)

		n, err := progDec.Write(corrupt)
		closeErr := progDec.Close()

		// Either Write or Close MUST return an error matching ErrDecompressionFailed
		if err == nil && closeErr == nil {
			t.Errorf("corrupt payload %d: expected error from Write or Close, both succeeded (n=%d)", idx, n)
		}

		checkErr := err
		if checkErr == nil {
			checkErr = closeErr
		}
		if !errors.Is(checkErr, ErrDecompressionFailed) {
			t.Errorf("corrupt payload %d: expected ErrDecompressionFailed, got %v", idx, checkErr)
		}

		// Subsequent Write calls after error must fail gracefully, never panic
		_, writeAfterErr := progDec.Write([]byte{0x00})
		if writeAfterErr == nil {
			t.Errorf("corrupt payload %d: expected error writing after failure", idx)
		}
		if !errors.Is(writeAfterErr, ErrDecompressionFailed) {
			t.Errorf("corrupt payload %d: writeAfterErr mismatch: got %v", idx, writeAfterErr)
		}
	}

	// Double Close idempotency on successfully completed decoder
	enc := NewEncoderWithDefaults(nil)
	validBlock := enc.EncodeHeaderList(999, []HeaderField{{Name: ":status", Value: "200"}}, nil)
	handlerValid := &collectedHeadersHandler{}
	pDecValid := decoder.CreateProgressiveDecoder(999, handlerValid)
	if _, err := pDecValid.Write(validBlock); err != nil {
		t.Fatalf("write validBlock failed: %v", err)
	}
	if err := pDecValid.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := pDecValid.Close(); err != nil {
		t.Fatalf("second Close failed on completed decoder: %v", err)
	}

	// Double Close idempotency on unstarted/empty decoder (both return ErrDecompressionFailed)
	handlerEmpty := &collectedHeadersHandler{}
	pDecEmpty := decoder.CreateProgressiveDecoder(1000, handlerEmpty)
	err1 := pDecEmpty.Close()
	if !errors.Is(err1, ErrDecompressionFailed) {
		t.Fatalf("expected ErrDecompressionFailed on empty Close, got %v", err1)
	}
	err2 := pDecEmpty.Close()
	if !errors.Is(err2, ErrDecompressionFailed) {
		t.Fatalf("expected ErrDecompressionFailed on second empty Close, got %v", err2)
	}

	// Buffer limit exceeded on blocked stream
	decBlocked := NewDecoder(100, 1, nil)
	blockedHandler := &collectedHeadersHandler{}
	// Set maxBufferedData = 5
	pDecBlocked := decBlocked.CreateProgressiveDecoderWithMaxBufferedData(1, 5, blockedHandler)
	// Block the stream by simulating RIC = 5 when inserted count = 0
	pDecBlocked.blocked = true
	pDecBlocked.prefixDecoded = true

	// Write 10 bytes -> exceeds maxBufferedData (5)
	_, writeErr := pDecBlocked.Write([]byte("1234567890"))
	if writeErr == nil && pDecBlocked.Close() == nil {
		t.Fatalf("expected error exceeding maxBufferedData")
	}
	if !errors.Is(pDecBlocked.Err(), ErrDecompressionFailed) {
		t.Errorf("expected ErrDecompressionFailed for buffer overflow, got %v", pDecBlocked.Err())
	}
}

func TestChallenger_Streaming_StreamReceiver_ReadFrom(t *testing.T) {
	// A. EncoderStreamReceiver.ReadFrom
	t.Run("EncoderStreamReceiver_ReadFrom_Corrupt", func(t *testing.T) {
		rec := NewEncoderStreamReceiver(&testEncoderStreamDelegate{})
		corruptData := []byte{
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01, // varint overflow
		}
		_, err := rec.ReadFrom(bytes.NewReader(corruptData))
		if err == nil {
			t.Fatalf("expected error reading corrupt encoder stream data")
		}
		if !errors.Is(err, ErrEncoderStream) {
			t.Errorf("expected ErrEncoderStream, got %v", err)
		}
	})

	t.Run("EncoderStreamReceiver_ReadFrom_IOError", func(t *testing.T) {
		rec := NewEncoderStreamReceiver(&testEncoderStreamDelegate{})
		expectedErr := errors.New("network socket closed abruptly")
		failingReader := &adversarialReader{err: expectedErr}
		_, err := rec.ReadFrom(failingReader)
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected %v from ReadFrom, got %v", expectedErr, err)
		}
	})

	t.Run("EncoderStreamReceiver_ReadFrom_SlowByteByByte", func(t *testing.T) {
		var buf bytes.Buffer
		sender := NewEncoderStreamSender(StreamWriter(&buf))
		sender.SendDuplicate(42)
		sender.SendSetDynamicTableCapacity(2048)
		sender.Flush()

		var duplicateSeen uint64
		var capSeen uint64
		rec := NewEncoderStreamReceiver(&dummyFullEncoderDelegate{
			onDuplicate: func(idx uint64) { duplicateSeen = idx },
			onSetCap:    func(cap uint64) { capSeen = cap },
		})

		slowR := &adversarialReader{data: buf.Bytes(), maxChunk: 1}
		n, err := rec.ReadFrom(slowR)
		if err != nil {
			t.Fatalf("slow ReadFrom failed: %v", err)
		}
		if n != int64(buf.Len()) {
			t.Errorf("expected %d bytes read, got %d", buf.Len(), n)
		}
		if duplicateSeen != 42 || capSeen != 2048 {
			t.Errorf("delegate callbacks mismatch: dup=%d, cap=%d", duplicateSeen, capSeen)
		}
	})

	// B. DecoderStreamReceiver.ReadFrom
	t.Run("DecoderStreamReceiver_ReadFrom_Corrupt", func(t *testing.T) {
		rec := NewDecoderStreamReceiver(&testDecoderStreamDelegate{})
		corruptData := []byte{
			0x3f,
			0x80,
			0x80,
			0x80,
			0x80,
			0x80,
			0x80,
			0x80,
			0x80,
			0x80,
			0x02, // 10th extension byte summand=2 > 1 -> uint64 overflow
		}
		_, err := rec.ReadFrom(bytes.NewReader(corruptData))
		if err == nil {
			t.Fatalf("expected error from corrupt decoder stream data")
		}
		if !errors.Is(err, ErrDecoderStream) {
			t.Errorf("expected ErrDecoderStream, got %v", err)
		}
	})

	t.Run("DecoderStreamReceiver_ReadFrom_IOError", func(t *testing.T) {
		rec := NewDecoderStreamReceiver(&testDecoderStreamDelegate{})
		expectedErr := errors.New("read timeout")
		failingReader := &adversarialReader{err: expectedErr}
		_, err := rec.ReadFrom(failingReader)
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected %v from ReadFrom, got %v", expectedErr, err)
		}
	})
}

type adversarialReader struct {
	data     []byte
	err      error
	maxChunk int
}

func (r *adversarialReader) Read(p []byte) (n int, err error) {
	if r.err != nil {
		return 0, r.err
	}
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	limit := len(p)
	if r.maxChunk > 0 && limit > r.maxChunk {
		limit = r.maxChunk
	}
	if limit > len(r.data) {
		limit = len(r.data)
	}
	copy(p, r.data[:limit])
	r.data = r.data[limit:]
	return limit, nil
}

type dummyFullEncoderDelegate struct {
	testEncoderStreamDelegate
	onSetCap func(cap uint64)
}

func (d *dummyFullEncoderDelegate) SetDynamicTableCapacity(capacity uint64) {
	if d.onSetCap != nil {
		d.onSetCap(capacity)
	}
}

func TestChallenger_Streaming_StreamSender_FlushTo_And_StreamWriter(t *testing.T) {
	// A. StreamWriter failing writer
	failingW := &adversarialWriter{err: errors.New("broken pipe")}
	delegate := StreamWriter(failingW)
	// WriteStreamData should not panic even if underlying writer fails
	delegate.WriteStreamData([]byte("test data"))
	if delegate.NumBytesBuffered() != 0 {
		t.Errorf("expected 0 buffered bytes")
	}

	// B. EncoderStreamSender FlushTo
	encSender := NewEncoderStreamSender(nil)
	// FlushTo on empty sender
	if err := encSender.FlushTo(failingW); err != nil {
		t.Errorf("expected nil on empty FlushTo, got %v", err)
	}

	encSender.SendDuplicate(1)
	if encSender.BufferedByteCount() == 0 {
		t.Fatalf("expected buffered bytes after SendDuplicate")
	}
	flushErr := encSender.FlushTo(failingW)
	if !errors.Is(flushErr, failingW.err) {
		t.Errorf("expected %v from FlushTo, got %v", failingW.err, flushErr)
	}
	// Buffer should be cleared after FlushTo
	if encSender.BufferedByteCount() != 0 {
		t.Errorf("expected buffer to be cleared after FlushTo even on error, got %d", encSender.BufferedByteCount())
	}

	// C. DecoderStreamSender FlushTo
	decSender := NewDecoderStreamSender(nil)
	if err := decSender.FlushTo(failingW); err != nil {
		t.Errorf("expected nil on empty decSender.FlushTo, got %v", err)
	}

	decSender.SendSectionAcknowledgement(10)
	flushDecErr := decSender.FlushTo(failingW)
	if !errors.Is(flushDecErr, failingW.err) {
		t.Errorf("expected %v from decSender.FlushTo, got %v", failingW.err, flushDecErr)
	}
	if len(decSender.buffer) != 0 {
		t.Errorf("expected decoder buffer to be cleared after FlushTo")
	}
}

type adversarialWriter struct {
	err error
}

func (w *adversarialWriter) Write(p []byte) (n int, err error) {
	if w.err != nil {
		return 0, w.err
	}
	return len(p), nil
}

// -----------------------------------------------------------------------------
// 3. Sentinel Errors and Unwrapping
// -----------------------------------------------------------------------------

func TestChallenger_Errors_SentinelAndUnwrapping_Exhaustive(t *testing.T) {
	// Matrix of all encoder stream error codes matching ErrEncoderStream
	encoderCodes := []ErrorCode{
		ErrCodeEncoderStreamError,
		ErrCodeEncoderIntegerTooLarge,
		ErrCodeEncoderStringLiteralTooLong,
		ErrCodeEncoderHuffmanEncodingError,
		ErrCodeEncoderInvalidStaticEntry,
		ErrCodeEncoderErrorInsertingStatic,
		ErrCodeEncoderInsertionInvalidRelativeIndex,
		ErrCodeEncoderInsertionDynamicEntryNotFound,
		ErrCodeEncoderErrorInsertingDynamic,
		ErrCodeEncoderErrorInsertingLiteral,
		ErrCodeEncoderDuplicateInvalidRelativeIndex,
		ErrCodeEncoderDuplicateDynamicEntryNotFound,
		ErrCodeEncoderSetDynamicTableCapacity,
	}

	for _, code := range encoderCodes {
		err := NewError(code, "test msg")
		if !errors.Is(err, ErrEncoderStream) {
			t.Errorf("code %d failed to match ErrEncoderStream via errors.Is", code)
		}
		if errors.Is(err, ErrDecoderStream) {
			t.Errorf("code %d erroneously matched ErrDecoderStream", code)
		}
	}

	// Matrix of all decoder stream error codes matching ErrDecoderStream
	decoderCodes := []ErrorCode{
		ErrCodeDecoderStreamError,
		ErrCodeDecoderIntegerTooLarge,
		ErrCodeDecoderInvalidZeroIncrement,
		ErrCodeDecoderIncrementOverflow,
		ErrCodeDecoderImpossibleInsertCount,
		ErrCodeDecoderIncorrectAcknowledgement,
	}

	for _, code := range decoderCodes {
		err := NewError(code, "test msg")
		if !errors.Is(err, ErrDecoderStream) {
			t.Errorf("code %d failed to match ErrDecoderStream via errors.Is", code)
		}
		if errors.Is(err, ErrEncoderStream) {
			t.Errorf("code %d erroneously matched ErrEncoderStream", code)
		}
	}

	// Specialized sentinel error matches
	specializedTests := []struct {
		code     ErrorCode
		sentinel error
	}{
		{ErrCodeDecompressionFailed, ErrDecompressionFailed},
		{ErrCodeEncoderIntegerTooLarge, ErrIntegerOverflow},
		{ErrCodeDecoderIntegerTooLarge, ErrIntegerOverflow},
		{ErrCodeEncoderStringLiteralTooLong, ErrStringLiteralTooLong},
		{ErrCodeEncoderHuffmanEncodingError, ErrHuffman},
		{ErrCodeEncoderSetDynamicTableCapacity, ErrCapacityExceeded},
		{ErrCodeEncoderInsertionDynamicEntryNotFound, ErrEntryNotFound},
		{ErrCodeEncoderDuplicateDynamicEntryNotFound, ErrEntryNotFound},
	}

	for _, tc := range specializedTests {
		err := NewError(tc.code, "specialized")
		if !errors.Is(err, tc.sentinel) {
			t.Errorf("code %d failed to match specialized sentinel %v", tc.code, tc.sentinel)
		}
	}

	// Deep wrapping with fmt.Errorf %w
	baseErr := errors.New("socket EOF")
	qErr := WrapError(ErrCodeEncoderHuffmanEncodingError, "bad huffman", baseErr)
	level1 := fmt.Errorf("level1 wrap: %w", qErr)
	level2 := fmt.Errorf("level2 wrap: %w", level1)

	if !errors.Is(level2, baseErr) {
		t.Errorf("failed to unwrap down to baseErr through multiple layers")
	}
	if !errors.Is(level2, ErrHuffman) {
		t.Errorf("failed to match ErrHuffman through multiple layers")
	}
	if !errors.Is(level2, ErrEncoderStream) {
		t.Errorf("failed to match ErrEncoderStream through multiple layers")
	}

	// errors.As extraction
	var extracted *Error
	if !errors.As(level2, &extracted) {
		t.Fatalf("errors.As failed on multi-level wrapped error")
	}
	if extracted.Code != ErrCodeEncoderHuffmanEncodingError {
		t.Errorf("extracted Code mismatch: got %d, want %d", extracted.Code, ErrCodeEncoderHuffmanEncodingError)
	}
	if extracted.Message != "bad huffman" {
		t.Errorf("extracted Message mismatch: got %q", extracted.Message)
	}

	// DecodeHeaderBlock error return unwraps to ErrDecompressionFailed
	dec := NewDecoder(0, 0, nil)
	_, decErr := dec.DecodeHeaderBlock(1, []byte{0xff, 0xff, 0xff})
	if decErr == nil {
		t.Fatalf("expected error from DecodeHeaderBlock on garbage")
	}
	if !errors.Is(decErr, ErrDecompressionFailed) {
		t.Errorf("expected DecodeHeaderBlock error to match ErrDecompressionFailed, got %v", decErr)
	}
	var decQErr *Error
	if !errors.As(decErr, &decQErr) {
		t.Errorf("expected decErr to be extractable as *Error via errors.As")
	}
}

// -----------------------------------------------------------------------------
// 4. Zero-Allocation Validation via testing.AllocsPerRun
// -----------------------------------------------------------------------------

func TestChallenger_ZeroAllocation_EmpiricalVerifications(t *testing.T) {
	// A. HeaderFields.All()
	fields := HeaderFields{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "application/json"},
		{Name: "date", Value: "today"},
	}
	allocsAll := testing.AllocsPerRun(100, func() {
		sum := 0
		for k, v := range fields.All() {
			sum += len(k) + len(v)
		}
		_ = sum
	})
	if allocsAll != 0 {
		t.Errorf("HeaderFields.All() allocated %v allocs/op, want 0", allocsAll)
	}

	// B. HeaderFields.Values()
	allocsValues := testing.AllocsPerRun(100, func() {
		sum := 0
		for hf := range fields.Values() {
			sum += len(hf.Name) + len(hf.Value)
		}
		_ = sum
	})
	if allocsValues != 0 {
		t.Errorf("HeaderFields.Values() allocated %v allocs/op, want 0", allocsValues)
	}

	// C. IndexSet.All()
	set := NewIndexSet()
	for i := uint64(0); i < 10; i++ {
		set.Insert(i)
	}
	allocsSet := testing.AllocsPerRun(100, func() {
		sum := uint64(0)
		for idx := range set.All() {
			sum += idx
		}
		_ = sum
	})
	if allocsSet != 0 {
		t.Errorf("IndexSet.All() allocated %v allocs/op, want 0", allocsSet)
	}

	// D. EncoderHeaderTable.Entries()
	encTable := NewEncoderHeaderTable()
	encTable.SetMaximumDynamicTableCapacity(1024)
	encTable.SetDynamicTableCapacity(1024)
	encTable.InsertEntry("key1", "val1")
	encTable.InsertEntry("key2", "val2")
	allocsEncTable := testing.AllocsPerRun(100, func() {
		sum := uint64(0)
		for idx, entry := range encTable.Entries() {
			sum += idx + uint64(len(entry.Name))
		}
		_ = sum
	})
	if allocsEncTable != 0 {
		t.Errorf("EncoderHeaderTable.Entries() allocated %v allocs/op, want 0", allocsEncTable)
	}

	// E. DecoderHeaderTable.Entries()
	decTable := NewDecoderHeaderTable()
	decTable.SetMaximumDynamicTableCapacity(1024)
	decTable.SetDynamicTableCapacity(1024)
	decTable.InsertEntry("dkey1", "dval1")
	decTable.InsertEntry("dkey2", "dval2")
	allocsDecTable := testing.AllocsPerRun(100, func() {
		sum := uint64(0)
		for idx, entry := range decTable.Entries() {
			sum += idx + uint64(len(entry.Name))
		}
		_ = sum
	})
	if allocsDecTable != 0 {
		t.Errorf("DecoderHeaderTable.Entries() allocated %v allocs/op, want 0", allocsDecTable)
	}

	// F. BlockingManager.BlockedStreams()
	bm := NewBlockingManager(100)
	for i := uint64(1); i <= 5; i++ {
		bm.OnHeaderBlockSent(i, []uint64{0})
	}
	allocsBM := testing.AllocsPerRun(100, func() {
		sum := uint64(0)
		for sID := range bm.BlockedStreams() {
			sum += sID
		}
		_ = sum
	})
	if allocsBM != 0 {
		t.Errorf("BlockingManager.BlockedStreams() allocated %v allocs/op, want 0", allocsBM)
	}
}

// -----------------------------------------------------------------------------
// 5. Concurrency Safety and Heavy Race Stress
// -----------------------------------------------------------------------------

func TestChallenger_Concurrency_RaceStress(t *testing.T) {
	// A. Concurrent Push Iterator Reads (immutable shared HeaderFields & IndexSet)
	hfs := HeaderFields{
		{Name: ":status", Value: "200"},
		{Name: "server", Value: "Foundation-QPACK"},
		{Name: "content-type", Value: "application/json"},
		{Name: "content-length", Value: "1024"},
	}

	set := NewIndexSet()
	for i := uint64(0); i < 50; i++ {
		set.Insert(i)
	}

	const numWorkers = 30
	const iterations = 500

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for iter := 0; iter < iterations; iter++ {
				// Iterate All()
				countAll := 0
				for range hfs.All() {
					countAll++
				}
				if countAll != 4 {
					t.Errorf("worker %d: unexpected All() count %d", workerID, countAll)
				}

				// Iterate Values()
				countVal := 0
				for range hfs.Values() {
					countVal++
				}
				if countVal != 4 {
					t.Errorf("worker %d: unexpected Values() count %d", workerID, countVal)
				}

				// Iterate IndexSet
				sum := uint64(0)
				for idx := range set.All() {
					sum += idx
				}
				if sum != 1225 { // sum(0..49) = 49*50/2 = 1225
					t.Errorf("worker %d: unexpected IndexSet sum %d", workerID, sum)
				}

				// Get()
				if val, ok := hfs.Get("Content-Type"); !ok || val != "application/json" {
					t.Errorf("worker %d: unexpected Get result", workerID)
				}
			}
		}(w)
	}

	wg.Wait()

	// B. Concurrent Encoders and Decoders Under Heavy Churn
	var encDecWg sync.WaitGroup
	encDecWg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer encDecWg.Done()
			enc := NewEncoderWithDefaults(nil)
			dec := NewDecoder(0, 0, nil)

			headers := []HeaderField{
				{Name: ":method", Value: "POST"},
				{Name: ":path", Value: "/api/v1/stream/" + strconv.Itoa(workerID)},
				{Name: ":scheme", Value: "https"},
				{Name: ":authority", Value: "foundation.test"},
				{Name: "user-agent", Value: "Agent-" + strconv.Itoa(workerID)},
			}

			for i := 0; i < 50; i++ {
				block := enc.EncodeHeaderList(uint64(i+1), headers, nil)
				decoded, err := dec.DecodeHeaderBlock(uint64(i+1), block)
				if err != nil {
					t.Errorf("worker %d iter %d: DecodeHeaderBlock failed: %v", workerID, i, err)
					return
				}
				if len(decoded) != len(headers) {
					t.Errorf("worker %d iter %d: decoded count mismatch: %d", workerID, i, len(decoded))
					return
				}
			}
		}(w)
	}

	encDecWg.Wait()
}
