// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"bytes"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
)

func TestModern_HeaderFields_Iterators(t *testing.T) {
	fields := HeaderFields{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "application/json"},
		{Name: "content-length", Value: "42"},
		{Name: "custom-header", Value: "custom-value"},
	}

	// 1. All() full iteration
	var collectedAll [][2]string
	for k, v := range fields.All() {
		collectedAll = append(collectedAll, [2]string{k, v})
	}
	if len(collectedAll) != 4 {
		t.Fatalf("expected 4 fields from All(), got %d", len(collectedAll))
	}
	if collectedAll[0] != [2]string{":status", "200"} {
		t.Errorf("unexpected first pair: %v", collectedAll[0])
	}
	if collectedAll[1] != [2]string{"content-type", "application/json"} {
		t.Errorf("unexpected second pair: %v", collectedAll[1])
	}

	// 2. All() early termination
	count := 0
	for range fields.All() {
		count++
		if count == 2 {
			break
		}
	}
	if count != 2 {
		t.Errorf("expected early termination after 2 items, got %d", count)
	}

	// 3. Values() full iteration
	var collectedValues []HeaderField
	for hf := range fields.Values() {
		collectedValues = append(collectedValues, hf)
	}
	if len(collectedValues) != 4 {
		t.Fatalf("expected 4 fields from Values(), got %d", len(collectedValues))
	}
	if collectedValues[2].Name != "content-length" || collectedValues[2].Value != "42" {
		t.Errorf("unexpected field at index 2: %v", collectedValues[2])
	}

	// 4. Values() early termination
	count = 0
	for range fields.Values() {
		count++
		if count == 1 {
			break
		}
	}
	if count != 1 {
		t.Errorf("expected early termination after 1 item, got %d", count)
	}

	// 5. Get() case-insensitivity and existence
	val, ok := fields.Get("Content-Type")
	if !ok || val != "application/json" {
		t.Errorf("expected application/json for Content-Type, got (%q, %v)", val, ok)
	}
	val, ok = fields.Get("CONTENT-LENGTH")
	if !ok || val != "42" {
		t.Errorf("expected 42 for CONTENT-LENGTH, got (%q, %v)", val, ok)
	}
	val, ok = fields.Get(":status")
	if !ok || val != "200" {
		t.Errorf("expected 200 for :status, got (%q, %v)", val, ok)
	}
	val, ok = fields.Get("non-existent")
	if ok || val != "" {
		t.Errorf("expected false for non-existent, got (%q, %v)", val, ok)
	}

	// 6. HeaderField.IsPseudo
	if !fields[0].IsPseudo() {
		t.Errorf("expected :status to be pseudo-header")
	}
	if fields[1].IsPseudo() {
		t.Errorf("expected content-type not to be pseudo-header")
	}
}

func TestModern_Encoder_IteratorAndWriter(t *testing.T) {
	encoder := NewEncoderWithDefaults(nil)

	fields := []HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":scheme", Value: "https"},
		{Name: ":path", Value: "/index.html"},
		{Name: "user-agent", Value: "Foundation-Test/1.0"},
	}

	var sentBytesList uint64
	listEncoded := encoder.EncodeHeaderList(1, fields, &sentBytesList)

	// Reset or create fresh encoders to compare identical states
	encoderSeq := NewEncoderWithDefaults(nil)
	var sentBytesSeq uint64
	seqEncoded := encoderSeq.EncodeHeaderSeq(1, HeaderFields(fields).Values(), &sentBytesSeq)

	if !bytes.Equal(listEncoded, seqEncoded) {
		t.Fatalf("EncodeHeaderSeq output mismatch:\ngot  %x\nwant %x", seqEncoded, listEncoded)
	}
	if sentBytesList != sentBytesSeq {
		t.Errorf("sent bytes mismatch: list=%d, seq=%d", sentBytesList, sentBytesSeq)
	}

	encoderSeq2 := NewEncoderWithDefaults(nil)
	var sentBytesSeq2 uint64
	seq2Encoded := encoderSeq2.EncodeHeaderSeq2(1, HeaderFields(fields).All(), &sentBytesSeq2)

	if !bytes.Equal(listEncoded, seq2Encoded) {
		t.Fatalf("EncodeHeaderSeq2 output mismatch:\ngot  %x\nwant %x", seq2Encoded, listEncoded)
	}

	// Test EncodeHeaderListTo
	encoderWriter := NewEncoderWithDefaults(nil)
	var buf bytes.Buffer
	n, err := encoderWriter.EncodeHeaderListTo(&buf, 1, fields, nil)
	if err != nil {
		t.Fatalf("EncodeHeaderListTo failed: %v", err)
	}
	if n != buf.Len() || !bytes.Equal(buf.Bytes(), listEncoded) {
		t.Fatalf("EncodeHeaderListTo output mismatch: written %d bytes, buffer has %x", n, buf.Bytes())
	}
}

func TestModern_Decoder_DecodeHeaderBlock(t *testing.T) {
	encoder := NewEncoderWithDefaults(nil)
	decoder := NewDecoder(0, 0, nil)

	fields := []HeaderField{
		{Name: ":method", Value: "POST"},
		{Name: ":path", Value: "/submit"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "example.com"},
		{Name: "content-type", Value: "text/plain"},
	}

	block := encoder.EncodeHeaderList(1, fields, nil)

	// Single-call DecodeHeaderBlock
	decoded, err := decoder.DecodeHeaderBlock(1, block)
	if err != nil {
		t.Fatalf("DecodeHeaderBlock failed: %v", err)
	}
	if len(decoded) != len(fields) {
		t.Fatalf("expected %d headers, got %d", len(fields), len(decoded))
	}
	for i := range fields {
		if decoded[i].Name != fields[i].Name || decoded[i].Value != fields[i].Value {
			t.Errorf("field %d mismatch: got %v, want %v", i, decoded[i], fields[i])
		}
	}

	// Single-call DecodeHeaderBlockSeq
	seq, err := decoder.DecodeHeaderBlockSeq(1, block)
	if err != nil {
		t.Fatalf("DecodeHeaderBlockSeq failed: %v", err)
	}
	idx := 0
	for name, value := range seq {
		if name != fields[idx].Name || value != fields[idx].Value {
			t.Errorf(
				"seq field %d mismatch: got (%s, %s), want (%s, %s)",
				idx,
				name,
				value,
				fields[idx].Name,
				fields[idx].Value,
			)
		}
		idx++
	}
	if idx != len(fields) {
		t.Errorf("expected %d fields from seq, got %d", len(fields), idx)
	}
}

func TestModern_ProgressiveDecoder_IOWriterAndCloser(t *testing.T) {
	encoder := NewEncoderWithDefaults(nil)
	decoder := NewDecoder(0, 0, nil)

	fields := []HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "server", Value: "test-server"},
	}
	block := encoder.EncodeHeaderList(2, fields, nil)

	handler := &collectedHeadersHandler{}
	progDec := decoder.CreateProgressiveDecoder(2, handler)

	// Stream through io.Copy
	reader := bytes.NewReader(block)
	n, err := io.Copy(progDec, reader)
	if err != nil {
		t.Fatalf("io.Copy into ProgressiveDecoder failed: %v", err)
	}
	if n != int64(len(block)) {
		t.Fatalf("io.Copy wrote %d bytes, expected %d", n, len(block))
	}

	// Close to finalize
	if err := progDec.Close(); err != nil {
		t.Fatalf("progDec.Close() failed: %v", err)
	}
	if progDec.Err() != nil {
		t.Fatalf("unexpected progDec.Err(): %v", progDec.Err())
	}
	if len(handler.headers) != len(fields) {
		t.Fatalf("expected %d headers decoded, got %d", len(fields), len(handler.headers))
	}
}

func TestModern_StreamSender_FlushTo(t *testing.T) {
	// EncoderStreamSender FlushTo
	sender := NewEncoderStreamSender(nil)
	// Empty flush should return nil
	var buf bytes.Buffer
	if err := sender.FlushTo(&buf); err != nil {
		t.Fatalf("empty FlushTo returned error: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected 0 bytes written, got %d", buf.Len())
	}

	sender.SendDuplicate(3)
	if err := sender.FlushTo(&buf); err != nil {
		t.Fatalf("FlushTo failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatalf("expected non-zero bytes flushed from EncoderStreamSender")
	}

	// DecoderStreamSender FlushTo
	decSender := NewDecoderStreamSender(nil)
	var decBuf bytes.Buffer
	if err := decSender.FlushTo(&decBuf); err != nil {
		t.Fatalf("empty decSender.FlushTo returned error: %v", err)
	}
	decSender.SendSectionAcknowledgement(42)
	if err := decSender.FlushTo(&decBuf); err != nil {
		t.Fatalf("decSender.FlushTo failed: %v", err)
	}
	if decBuf.Len() == 0 {
		t.Fatalf("expected non-zero bytes flushed from DecoderStreamSender")
	}
}

func TestModern_StreamWriter(t *testing.T) {
	var buf bytes.Buffer
	delegate := StreamWriter(&buf)
	if delegate.NumBytesBuffered() != 0 {
		t.Errorf("expected 0 buffered bytes, got %d", delegate.NumBytesBuffered())
	}
	delegate.WriteStreamData([]byte("hello qpack"))
	if buf.String() != "hello qpack" {
		t.Errorf("expected 'hello qpack', got %q", buf.String())
	}
}

func TestModern_StreamReceiver_ReadFrom(t *testing.T) {
	// EncoderStreamSender writes into a buffer, EncoderStreamReceiver reads from it via ReadFrom
	var buf bytes.Buffer
	sender := NewEncoderStreamSender(StreamWriter(&buf))
	sender.SendDuplicate(5)
	sender.Flush()

	type dummyEncoderDelegate struct {
		duplicateCalled bool
		dupIndex        uint64
	}
	del := &dummyEncoderDelegate{}
	rec := NewEncoderStreamReceiver(&struct {
		EncoderStreamReceiverDelegate
	}{
		EncoderStreamReceiverDelegate: &testEncoderStreamDelegate{
			onDuplicate: func(idx uint64) {
				del.duplicateCalled = true
				del.dupIndex = idx
			},
		},
	})

	n, err := rec.ReadFrom(&buf)
	if err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	if n == 0 {
		t.Fatalf("expected bytes read > 0, got %d", n)
	}
	if !del.duplicateCalled || del.dupIndex != 5 {
		t.Errorf("delegate not called as expected: duplicate=%v, index=%d", del.duplicateCalled, del.dupIndex)
	}

	// DecoderStreamSender writes into a buffer, DecoderStreamReceiver reads via ReadFrom
	var decBuf bytes.Buffer
	decSender := NewDecoderStreamSender(StreamWriter(&decBuf))
	decSender.SendInsertCountIncrement(10)
	decSender.Flush()

	var incCalled bool
	var incVal uint64
	decRec := NewDecoderStreamReceiver(&testDecoderStreamDelegate{
		onInsertCountIncrement: func(inc uint64) {
			incCalled = true
			incVal = inc
		},
	})

	dn, err := decRec.ReadFrom(&decBuf)
	if err != nil {
		t.Fatalf("DecoderStreamReceiver.ReadFrom failed: %v", err)
	}
	if dn == 0 {
		t.Fatalf("expected bytes read > 0, got %d", dn)
	}
	if !incCalled || incVal != 10 {
		t.Errorf("delegate not called as expected: incCalled=%v, incVal=%d", incCalled, incVal)
	}
}

type testEncoderStreamDelegate struct {
	onDuplicate func(idx uint64)
}

func (d *testEncoderStreamDelegate) InsertWithNameReference(isStatic bool, nameIndex uint64, value string) {
}
func (d *testEncoderStreamDelegate) InsertWithoutNameReference(name, value string) {}
func (d *testEncoderStreamDelegate) Duplicate(index uint64) {
	if d.onDuplicate != nil {
		d.onDuplicate(index)
	}
}
func (d *testEncoderStreamDelegate) SetDynamicTableCapacity(capacity uint64)      {}
func (d *testEncoderStreamDelegate) Error(qpackError uint64, errorMessage string) {}

type testDecoderStreamDelegate struct {
	onInsertCountIncrement func(inc uint64)
}

func (d *testDecoderStreamDelegate) SectionAck(streamID uint64)         {}
func (d *testDecoderStreamDelegate) StreamCancellation(streamID uint64) {}
func (d *testDecoderStreamDelegate) InsertCountIncrement(increment uint64) {
	if d.onInsertCountIncrement != nil {
		d.onInsertCountIncrement(increment)
	}
}
func (d *testDecoderStreamDelegate) Error(qpackError uint64, errorMessage string) {}

func TestModern_HeaderTable_EntriesIterators(t *testing.T) {
	// EncoderHeaderTable Entries()
	encTable := NewEncoderHeaderTable()
	encTable.SetMaximumDynamicTableCapacity(1024)
	encTable.SetDynamicTableCapacity(1024)
	encTable.InsertEntry("custom-key-1", "val-1")
	encTable.InsertEntry("custom-key-2", "val-2")

	var encEntries []struct {
		idx   uint64
		entry *Entry
	}
	for idx, entry := range encTable.Entries() {
		encEntries = append(encEntries, struct {
			idx   uint64
			entry *Entry
		}{idx, entry})
	}
	if len(encEntries) != 2 {
		t.Fatalf("expected 2 dynamic entries, got %d", len(encEntries))
	}
	if encEntries[0].idx != 0 || encEntries[0].entry.Name != "custom-key-1" {
		t.Errorf("unexpected entry 0: idx=%d, entry=%v", encEntries[0].idx, encEntries[0].entry)
	}
	if encEntries[1].idx != 1 || encEntries[1].entry.Name != "custom-key-2" {
		t.Errorf("unexpected entry 1: idx=%d, entry=%v", encEntries[1].idx, encEntries[1].entry)
	}

	// Early termination
	count := 0
	for range encTable.Entries() {
		count++
		break
	}
	if count != 1 {
		t.Errorf("expected early termination after 1, got %d", count)
	}

	// DecoderHeaderTable Entries()
	decTable := NewDecoderHeaderTable()
	decTable.SetMaximumDynamicTableCapacity(1024)
	decTable.SetDynamicTableCapacity(1024)
	decTable.InsertEntry("dec-key-1", "dec-val-1")
	decTable.InsertEntry("dec-key-2", "dec-val-2")

	var decEntries []struct {
		idx   uint64
		entry *Entry
	}
	for idx, entry := range decTable.Entries() {
		decEntries = append(decEntries, struct {
			idx   uint64
			entry *Entry
		}{idx, entry})
	}
	if len(decEntries) != 2 {
		t.Fatalf("expected 2 dynamic entries in decoder table, got %d", len(decEntries))
	}
	if decEntries[0].entry.Name != "dec-key-1" {
		t.Errorf("unexpected decoder entry 0: %v", decEntries[0].entry)
	}

	// Modernized qpackRingBuffer methods
	rb := &qpackRingBuffer{}
	if !rb.IsEmpty() || rb.Len() != 0 {
		t.Errorf("expected empty ring buffer")
	}
	rb.PushBack(NewEntry("a", "1"))
	if rb.IsEmpty() || rb.Len() != 1 {
		t.Errorf("expected 1 element")
	}
	if rb.Front() == nil || rb.Front().Name != "a" {
		t.Errorf("unexpected Front: %v", rb.Front())
	}
	if rb.At(0) == nil || rb.At(0).Value != "1" {
		t.Errorf("unexpected At(0): %v", rb.At(0))
	}
	popped := rb.PopFront()
	if popped == nil || popped.Name != "a" || !rb.IsEmpty() {
		t.Errorf("unexpected PopFront: %v", popped)
	}
}

func TestModern_BlockingManager_IteratorsAndDeprecations(t *testing.T) {
	bm := NewBlockingManager()
	bm.SetMaxBlockedStreams(10)

	// IndexSet All()
	set := NewIndexSet()
	set.Insert(10)
	set.Insert(20)
	set.Insert(5)

	var indices []uint64
	for idx := range set.All() {
		indices = append(indices, idx)
	}
	if len(indices) != 3 || indices[0] != 10 || indices[1] != 20 || indices[2] != 5 {
		t.Errorf("unexpected indices from All(): %v", indices)
	}

	// Early termination on IndexSet.All()
	count := 0
	for range set.All() {
		count++
		break
	}
	if count != 1 {
		t.Errorf("expected early termination count 1, got %d", count)
	}

	// Block a stream
	bm.OnHeaderBlockSent(1, []uint64{0})
	if !bm.IsBlocked(1) {
		t.Errorf("expected stream 1 to be blocked")
	}

	var blockedList []uint64
	for streamID := range bm.BlockedStreams() {
		blockedList = append(blockedList, streamID)
	}
	if len(blockedList) != 1 || blockedList[0] != 1 {
		t.Errorf("unexpected blocked streams: %v", blockedList)
	}

	// Test deprecated method parity
	if bm.smallest_blocking_index() != bm.SmallestBlockingIndex() {
		t.Errorf("smallest_blocking_index parity error")
	}
	if bm.known_received_count() != bm.KnownReceivedCount() {
		t.Errorf("known_received_count parity error")
	}
	if bm.is_blocked(1) != bm.IsBlocked(1) {
		t.Errorf("is_blocked parity error")
	}
	if bm.num_blocked_streams() != bm.NumBlockedStreams() {
		t.Errorf("num_blocked_streams parity error")
	}
	if bm.blocking_allowed_on_stream(1) != bm.BlockingAllowedOnStream(1) {
		t.Errorf("blocking_allowed_on_stream parity error")
	}
}

func TestModern_Errors_IsAndUnwrap(t *testing.T) {
	baseErr := errors.New("underlying socket closed")
	qerr := WrapError(ErrCodeEncoderStreamError, "stream read failed", baseErr)

	// Test Error() formatting
	str := qerr.Error()
	if !strings.Contains(str, "stream read failed") || !strings.Contains(str, "underlying socket closed") {
		t.Errorf("unexpected Error() string: %s", str)
	}

	// Test Unwrap()
	if !errors.Is(qerr, baseErr) {
		t.Errorf("errors.Is(qerr, baseErr) returned false")
	}

	// Test errors.Is with sentinel
	if !errors.Is(qerr, ErrEncoderStream) {
		t.Errorf("errors.Is(qerr, ErrEncoderStream) returned false")
	}
	if errors.Is(qerr, ErrDecoderStream) {
		t.Errorf("errors.Is(qerr, ErrDecoderStream) returned true for encoder error")
	}

	// Decompression failed
	decompErr := NewError(ErrCodeDecompressionFailed, "invalid huffman")
	if !errors.Is(decompErr, ErrDecompressionFailed) {
		t.Errorf("errors.Is(decompErr, ErrDecompressionFailed) returned false")
	}

	// Decoder stream error
	decErr := NewError(ErrCodeDecoderIntegerTooLarge, "integer overflow in increment")
	if !errors.Is(decErr, ErrDecoderStream) {
		t.Errorf("errors.Is(decErr, ErrDecoderStream) returned false")
	}
	if !errors.Is(decErr, ErrIntegerOverflow) {
		t.Errorf("errors.Is(decErr, ErrIntegerOverflow) returned false")
	}

	// Dynamic table capacity exceeded
	capErr := NewError(ErrCodeEncoderSetDynamicTableCapacity, "capacity error")
	if !errors.Is(capErr, ErrCapacityExceeded) {
		t.Errorf("errors.Is(capErr, ErrCapacityExceeded) returned false")
	}

	// Entry not found
	entryErr := NewError(ErrCodeEncoderInsertionDynamicEntryNotFound, "entry missing")
	if !errors.Is(entryErr, ErrEntryNotFound) {
		t.Errorf("errors.Is(entryErr, ErrEntryNotFound) returned false")
	}

	// Nil safety
	var nilErr *Error
	if nilErr.Error() != "<nil>" {
		t.Errorf("expected <nil> from nilErr.Error(), got %s", nilErr.Error())
	}
	if nilErr.Unwrap() != nil {
		t.Errorf("expected nil from nilErr.Unwrap()")
	}
	if nilErr.Is(ErrDecompressionFailed) {
		t.Errorf("expected false from nilErr.Is()")
	}
}

func TestModern_UntestedFunctions_CoverageElevation(t *testing.T) {
	// 1. varint.go appendInt coverage
	dst := []byte{0x00}
	res := appendInt(dst, 5, 10)
	if len(res) != 1 || res[0] != 0x0a {
		t.Errorf("appendInt small val failed: got %x", res)
	}

	// appendInt with value >= maxPrefix
	dst2 := []byte{0x00}
	res2 := appendInt(dst2, 5, 31+128)
	if len(res2) < 2 {
		t.Errorf("appendInt large val failed: got %x", res2)
	}

	// appendInt panic on prefixLen 0 or > 8
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic on prefixLen 0")
			}
		}()
		appendInt([]byte{0}, 0, 1)
	}()

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic on prefixLen 9")
			}
		}()
		appendInt([]byte{0}, 9, 1)
	}()

	// 2. ProgressiveDecoder Cancel() and EndDecoding()
	decoder := NewDecoder(0, 0, nil)
	progDec := decoder.CreateProgressiveDecoder(10, &collectedHeadersHandler{})
	progDec.Cancel()
	if !progDec.cancelled {
		t.Errorf("expected progDec.cancelled to be true")
	}
	progDec.EndDecoding()

	// 3. Decoder methods
	var errReported uint64
	decoderErr := NewDecoder(1024, 10, func(code uint64, msg string) {
		errReported = code
	})
	decoderErr.OnErrorDetected(QPACK_ENCODER_STREAM_ERROR, "test error")
	if errReported != QPACK_ENCODER_STREAM_ERROR {
		t.Errorf("OnErrorDetected failed: got %d", errReported)
	}
	if decoderErr.LastError() == nil {
		t.Errorf("expected LastError() to be non-nil")
	}

	var senderBuf bytes.Buffer
	decoderErr.SetStreamSenderDelegate(StreamWriter(&senderBuf))
	if decoderErr.DecoderStreamSender() == nil {
		t.Errorf("expected DecoderStreamSender() to be non-nil")
	}
	if decoderErr.DynamicTableCapacity() != 0 {
		t.Errorf("expected initial DynamicTableCapacity to be 0")
	}
	decoderErr.SendStreamCancellation(99)
	decoderErr.FlushDecoderStream()
	if senderBuf.Len() == 0 {
		t.Errorf("expected data written for SendStreamCancellation")
	}

	// 4. Encoder methods
	var encErrReported uint64
	enc := NewEncoder(func(code uint64, msg string) {
		encErrReported = code
	}, HuffmanEncodingEnabled, CookieCrumblingEnabled)
	enc.OnErrorDetected(QPACK_DECODER_STREAM_ERROR, "Encoded integer too large.")
	if encErrReported != QUIC_QPACK_DECODER_STREAM_INTEGER_TOO_LARGE {
		t.Errorf("expected integer too large error code, got %d", encErrReported)
	}
	if enc.LastError() == nil {
		t.Errorf("expected enc.LastError() to be non-nil")
	}
}

func TestModern_IndexConversions_Thorough(t *testing.T) {
	// EncoderRelativeIndexToAbsoluteIndex
	abs, ok := EncoderRelativeIndexToAbsoluteIndex(0, 5)
	if !ok || abs != 4 {
		t.Errorf("EncoderRelativeIndexToAbsoluteIndex failed: got (%d, %v)", abs, ok)
	}
	_, ok = EncoderRelativeIndexToAbsoluteIndex(5, 5)
	if ok {
		t.Errorf("expected false for out of bounds relativeIndex")
	}

	// AbsoluteIndexToEncoderRelativeIndex
	rel := AbsoluteIndexToEncoderRelativeIndex(3, 5)
	if rel != 1 {
		t.Errorf("AbsoluteIndexToEncoderRelativeIndex failed: got %d", rel)
	}

	// AbsoluteIndexToRequestRelativeIndex
	reqRel := AbsoluteIndexToRequestRelativeIndex(2, 5)
	if reqRel != 2 {
		t.Errorf("AbsoluteIndexToRequestRelativeIndex failed: got %d", reqRel)
	}

	// RequestRelativeIndexToAbsoluteIndex
	reqAbs, ok := RequestRelativeIndexToAbsoluteIndex(2, 5)
	if !ok || reqAbs != 2 {
		t.Errorf("RequestRelativeIndexToAbsoluteIndex failed: got (%d, %v)", reqAbs, ok)
	}
	_, ok = RequestRelativeIndexToAbsoluteIndex(5, 5)
	if ok {
		t.Errorf("expected false for out of bounds req relativeIndex")
	}

	// AbsoluteIndexToPostBaseIndex and PostBaseIndexToAbsoluteIndex
	postAbs, ok := PostBaseIndexToAbsoluteIndex(3, 10)
	if !ok || postAbs != 13 {
		t.Errorf("PostBaseIndexToAbsoluteIndex failed: got (%d, %v)", postAbs, ok)
	}
	pbIdx := AbsoluteIndexToPostBaseIndex(13, 10)
	if pbIdx != 3 {
		t.Errorf("AbsoluteIndexToPostBaseIndex failed: got %d", pbIdx)
	}

	// Precondition panic checks
	func() {
		defer func() { _ = recover() }()
		AbsoluteIndexToPostBaseIndex(5, 10)
		t.Errorf("expected panic")
	}()
	func() {
		defer func() { _ = recover() }()
		AbsoluteIndexToRequestStreamRelativeIndex(10, 5)
		t.Errorf("expected panic")
	}()
	func() {
		defer func() { _ = recover() }()
		AbsoluteIndexToEncoderStreamRelativeIndex(10, 5)
		t.Errorf("expected panic")
	}()
}

func TestModern_BlockingManager_Comprehensive(t *testing.T) {
	bm := NewBlockingManagerWithMax(20)
	if bm.MaxBlockedStreams() != 20 {
		t.Errorf("expected MaxBlockedStreams 20, got %d", bm.MaxBlockedStreams())
	}

	hd := NewHeaderData([]uint64{3, 1, 4})
	if len(hd.Indices()) != 3 || hd.Indices()[0] != 3 {
		t.Errorf("unexpected Indices: %v", hd.Indices())
	}
	if hd.RequiredInsertCount() != 5 {
		t.Errorf("expected RIC 5, got %d", hd.RequiredInsertCount())
	}
	minIdx, hasMin := hd.MinIndex()
	if !hasMin || minIdx != 1 {
		t.Errorf("expected minIndex 1, got (%d, %v)", minIdx, hasMin)
	}

	emptyHd := NewHeaderData(nil)
	if emptyHd.Indices() != nil || emptyHd.RequiredInsertCount() != 0 {
		t.Errorf("unexpected empty HeaderData: %v", emptyHd)
	}

	// Multiple minIndex ref counts and recomputing smallestBlockingIndex
	bm.OnHeaderBlockSent(1, []uint64{5})
	bm.OnHeaderBlockSent(2, []uint64{5})
	bm.OnHeaderBlockSent(3, []uint64{2})
	if bm.SmallestBlockingIndex() != 2 {
		t.Errorf("expected smallest blocking index 2, got %d", bm.SmallestBlockingIndex())
	}
	// Acknowledge stream 3
	bm.OnSectionAck(3)
	if bm.SmallestBlockingIndex() != 5 {
		t.Errorf("expected smallest blocking index 5 after popping 2, got %d", bm.SmallestBlockingIndex())
	}
	// Acknowledge stream 1 (ref count for 5 becomes 1)
	bm.OnSectionAck(1)
	if bm.SmallestBlockingIndex() != 5 {
		t.Errorf("expected smallest blocking index 5 with 1 ref remaining, got %d", bm.SmallestBlockingIndex())
	}
	// Acknowledge stream 2 (ref count for 5 becomes 0)
	bm.OnSectionAck(2)

	// BlockedStreams early exit
	bm.OnHeaderBlockSent(10, []uint64{10})
	bm.OnHeaderBlockSent(11, []uint64{11})
	count := 0
	for range bm.BlockedStreams() {
		count++
		break
	}
	if count != 1 {
		t.Errorf("expected early break count 1, got %d", count)
	}
}

func TestModern_Decoder_Comprehensive(t *testing.T) {
	d := NewDecoder(1024, 10, nil)

	// collectedHeadersHandler OnDecodingCompleted coverage
	handler := &collectedHeadersHandler{}
	handler.OnDecodingCompleted()

	// DynamicTableEntryReferenced and MaximumDynamicTableCapacity
	if d.DynamicTableEntryReferenced() {
		t.Errorf("expected false for DynamicTableEntryReferenced initially")
	}
	if d.MaximumDynamicTableCapacity() != 1024 {
		t.Errorf("expected 1024 max capacity, got %d", d.MaximumDynamicTableCapacity())
	}

	// Send helper methods
	var buf bytes.Buffer
	d.SetStreamSenderDelegate(StreamWriter(&buf))
	d.SendInsertCountIncrement(3)
	d.SendSectionAcknowledgement(4)
	d.SendHeaderAcknowledgement(5)
	d.FlushDecoderStream()
	if buf.Len() == 0 {
		t.Errorf("expected instructions flushed to stream sender")
	}

	// Error path: DecodeHeaderBlock on invalid / corrupted block
	_, err := d.DecodeHeaderBlock(99, []byte{0xff, 0xff, 0xff})
	if err == nil {
		t.Errorf("expected error decoding corrupted block")
	}
	_, err = d.DecodeHeaderBlockSeq(99, []byte{0xff, 0xff, 0xff})
	if err == nil {
		t.Errorf("expected error from DecodeHeaderBlockSeq on corrupted block")
	}

	// InsertWithNameReference errors
	var lastErrCode uint64
	dErr := NewDecoder(100, 10, func(code uint64, msg string) {
		lastErrCode = code
	})
	// Invalid static table entry
	dErr.InsertWithNameReference(true, 9999, "val")
	if lastErrCode != QUIC_QPACK_ENCODER_STREAM_INVALID_STATIC_ENTRY {
		t.Errorf("expected invalid static entry error code, got %d", lastErrCode)
	}
	// Static entry does not fit dynamic table capacity
	dErr.headerTable.SetDynamicTableCapacity(10)
	dErr.InsertWithNameReference(true, 0, strings.Repeat("x", 50))
	if lastErrCode != QUIC_QPACK_ENCODER_STREAM_ERROR_INSERTING_STATIC {
		t.Errorf("expected error inserting static, got %d", lastErrCode)
	}
	// Dynamic relative index invalid
	dErr.InsertWithNameReference(false, 99, "val")
	if lastErrCode != QUIC_QPACK_ENCODER_STREAM_INSERTION_INVALID_RELATIVE_INDEX {
		t.Errorf("expected invalid relative index, got %d", lastErrCode)
	}

	// Duplicate errors
	dErr.Duplicate(99)
	if lastErrCode != QUIC_QPACK_ENCODER_STREAM_DUPLICATE_INVALID_RELATIVE_INDEX {
		t.Errorf("expected duplicate invalid relative index, got %d", lastErrCode)
	}

	// Error dispatching messages
	dErr.Error(QPACK_ENCODER_STREAM_ERROR, "String literal too long.")
	if lastErrCode != QUIC_QPACK_ENCODER_STREAM_STRING_LITERAL_TOO_LONG {
		t.Errorf("expected string literal too long, got %d", lastErrCode)
	}
	dErr.Error(QPACK_ENCODER_STREAM_ERROR, "Error in Huffman-encoded string.")
	if lastErrCode != QUIC_QPACK_ENCODER_STREAM_HUFFMAN_ENCODING_ERROR {
		t.Errorf("expected huffman error code, got %d", lastErrCode)
	}
	dErr.Error(QPACK_ENCODER_STREAM_ERROR, "generic error")
	if lastErrCode != QPACK_ENCODER_STREAM_ERROR {
		t.Errorf("expected generic error code, got %d", lastErrCode)
	}
	dErr.Error(777, "custom")
	if lastErrCode != 777 {
		t.Errorf("expected 777, got %d", lastErrCode)
	}
}

func TestModern_Encoder_Comprehensive(t *testing.T) {
	enc := NewEncoderWithDefaults(nil)
	if enc.DynamicTableEntryReferenced() {
		t.Errorf("expected false initially")
	}
	enc.SetMaximumDynamicTableCapacity(2048)
	if enc.MaximumDynamicTableCapacity() != 2048 {
		t.Errorf("expected 2048, got %d", enc.MaximumDynamicTableCapacity())
	}

	// SetMaximumBlockedStreams decreasing should return false
	enc.SetMaximumBlockedStreams(10)
	if enc.SetMaximumBlockedStreams(5) {
		t.Errorf("expected SetMaximumBlockedStreams(5) to return false when current is 10")
	}

	// SetDynamicTableCapacity panic when exceeding maximum
	func() {
		defer func() { _ = recover() }()
		enc.SetDynamicTableCapacity(4096)
		t.Errorf("expected panic on exceeding capacity")
	}()

	// EncoderPeer
	if EncoderPeerMaximumBlockedStreams(enc) != 10 {
		t.Errorf("expected 10 from EncoderPeerMaximumBlockedStreams")
	}
	if EncoderPeerSmallestBlockingIndex(enc) != enc.BlockingManager().SmallestBlockingIndex() {
		t.Errorf("EncoderPeerSmallestBlockingIndex mismatch")
	}
	if EncoderPeerHeaderTable(enc) != enc.HeaderTable() {
		t.Errorf("EncoderPeerHeaderTable mismatch")
	}
}

func TestModern_Errors_Comprehensive(t *testing.T) {
	simpleErr := NewError(ErrCodeNoError, "success")
	if simpleErr.Error() != "qpack: error 0: success" {
		t.Errorf("unexpected Error() string: %s", simpleErr.Error())
	}
	if simpleErr.Is(nil) {
		t.Errorf("expected false for Is(nil)")
	}
	otherErr := NewError(ErrCodeNoError, "different")
	if !simpleErr.Is(otherErr) {
		t.Errorf("expected true for same error code")
	}
	diffErr := NewError(ErrCodeInternalError, "internal")
	if simpleErr.Is(diffErr) {
		t.Errorf("expected false for different error code")
	}
	if simpleErr.Is(errors.New("unrelated")) {
		t.Errorf("expected false for unrelated error")
	}

	// Sentinel checks
	strErr := NewError(ErrCodeEncoderStringLiteralTooLong, "too long")
	if !errors.Is(strErr, ErrStringLiteralTooLong) {
		t.Errorf("expected ErrStringLiteralTooLong match")
	}
	huffErr := NewError(ErrCodeEncoderHuffmanEncodingError, "huffman")
	if !errors.Is(huffErr, ErrHuffman) {
		t.Errorf("expected ErrHuffman match")
	}
	dupNotFoundErr := NewError(ErrCodeEncoderDuplicateDynamicEntryNotFound, "not found")
	if !errors.Is(dupNotFoundErr, ErrEntryNotFound) {
		t.Errorf("expected ErrEntryNotFound match")
	}
}

func TestModern_HeaderTable_Comprehensive(t *testing.T) {
	rb := &qpackRingBuffer{}
	if rb.front() != nil {
		t.Errorf("expected nil front on empty rb")
	}
	if rb.at(-1) != nil || rb.at(5) != nil {
		t.Errorf("expected nil for out of bounds at")
	}
	if rb.popFront() != nil {
		t.Errorf("expected nil popFront on empty rb")
	}

	tb := &HeaderTableBase{}
	tb.RemoveEntryFromEnd()
	tb.baseRemoveEntryFromEnd()
	if tb.DynamicTableEntryReferenced() {
		t.Errorf("expected false for DynamicTableEntryReferenced")
	}
	tb.SetDynamicTableEntryReferenced()
	if !tb.DynamicTableEntryReferenced() {
		t.Errorf("expected true for DynamicTableEntryReferenced")
	}

	// EncoderHeaderTable SmallestAllowedIndex
	encTable := NewEncoderHeaderTable()
	encTable.SetSmallestAllowedIndex(42)
	if encTable.SmallestAllowedIndex() != 42 {
		t.Errorf("expected 42, got %d", encTable.SmallestAllowedIndex())
	}

	// DrainingIndex fractions
	encTable.SetMaximumDynamicTableCapacity(1000)
	encTable.SetDynamicTableCapacity(1000)
	if encTable.DrainingIndex(-0.5) != 0 {
		t.Errorf("expected 0 for negative fraction")
	}
	if encTable.DrainingIndex(1.5) != 0 {
		t.Errorf("expected 0 for empty table")
	}

	// RegisterObserver panic on 0
	decTable := NewDecoderHeaderTable()
	func() {
		defer func() { _ = recover() }()
		decTable.RegisterObserver(0, nil)
		t.Errorf("expected panic on requiredInsertCount=0")
	}()

	// DecoderHeaderTable Entries early termination
	decTable.SetMaximumDynamicTableCapacity(1000)
	decTable.SetDynamicTableCapacity(1000)
	decTable.InsertEntry("k1", "v1")
	decTable.InsertEntry("k2", "v2")
	count := 0
	for range decTable.Entries() {
		count++
		break
	}
	if count != 1 {
		t.Errorf("expected early termination after 1, got %d", count)
	}
}

func TestModern_ProgressiveDecoder_Comprehensive(t *testing.T) {
	d := NewDecoder(0, 0, nil)
	prog := d.CreateProgressiveDecoder(1, &collectedHeadersHandler{})

	// Write after error
	prog.onError(QPACK_DECOMPRESSION_FAILED, "forced error")
	_, err := prog.Write([]byte{1, 2, 3})
	if err == nil {
		t.Errorf("expected error on Write after errorDetected")
	}
	if err := prog.Close(); err == nil {
		t.Errorf("expected error on Close after errorDetected")
	}
	// Calling onError again when already errorDetected
	prog.onError(QPACK_DECOMPRESSION_FAILED, "second error")

	// Close when blocked
	d2 := NewDecoder(1000, 10, nil)
	prog2 := d2.CreateProgressiveDecoder(2, &collectedHeadersHandler{})
	prog2.blocked = true
	_ = prog2.Close()
	if prog2.blocked {
		t.Errorf("expected blocked to be false after Close()")
	}
}

func TestModern_StreamReceiver_Comprehensive(t *testing.T) {
	rec := NewEncoderStreamReceiver(&testEncoderStreamDelegate{})
	rec.OnInstructionDecodingError(InstructionDecoderIntegerTooLarge, "error")
	// Second error call when already errorDetected
	rec.OnInstructionDecodingError(InstructionDecoderIntegerTooLarge, "second error")
	// Decode when errorDetected is ignored
	rec.Decode([]byte{0x00})
	rec.EndDecoding()

	// ReadFrom returning non-EOF error
	errReader := &testErrorReader{err: errors.New("read failed")}
	_, err := rec.ReadFrom(errReader)
	if err == nil {
		t.Errorf("expected read error")
	}

	// DecoderStreamReceiver error paths
	decRec := NewDecoderStreamReceiver(&testDecoderStreamDelegate{})
	decRec.OnInstructionDecodingError(InstructionDecoderIntegerTooLarge, "error")
	decRec.OnInstructionDecodingError(InstructionDecoderIntegerTooLarge, "second")
	decRec.Decode([]byte{0x00})
	decRec.EndDecoding()
	_, err = decRec.ReadFrom(errReader)
	if err == nil {
		t.Errorf("expected read error from decRec")
	}
}

type testErrorReader struct {
	err error
}

func (r *testErrorReader) Read(p []byte) (n int, err error) {
	return 0, r.err
}

func TestModern_Varint_Comprehensive(t *testing.T) {
	// readInt prefixLen 0 or > 8
	_, _, err := readInt(0, []byte{0x00})
	if !errors.Is(err, ErrInvalidInteger) {
		t.Errorf("expected ErrInvalidInteger for prefixLen 0, got %v", err)
	}
	_, _, err = readInt(9, []byte{0x00})
	if !errors.Is(err, ErrInvalidInteger) {
		t.Errorf("expected ErrInvalidInteger for prefixLen 9, got %v", err)
	}
	// readInt empty data
	_, _, err = readInt(5, nil)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF for nil data, got %v", err)
	}
	// readInt incomplete multi-byte
	_, _, err = readInt(5, []byte{0x1f}) // 0x1f is max prefix for 5, expects follow up
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF for missing varint bytes, got %v", err)
	}
	// readInt overflow (shift >= 63)
	overflowBytes := []byte{0x1f, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01}
	_, _, err = readInt(5, overflowBytes)
	if !errors.Is(err, ErrIntegerOverflow) {
		t.Errorf("expected ErrIntegerOverflow, got %v", err)
	}
}

func TestModern_InstructionDecoderAndEncoder_Comprehensive(t *testing.T) {
	var vd qpackVarintDecoder
	vd.Reset()
	if vd.Done() {
		t.Errorf("expected Done false")
	}
	if vd.Error() {
		t.Errorf("expected Error false")
	}
	if vd.Value() != 0 {
		t.Errorf("expected Value 0")
	}

	// HuffmanEncoding.String() default
	invalidHuff := HuffmanEncoding(99)
	if invalidHuff.String() != "HuffmanEncoding(99)" {
		t.Errorf("unexpected string: %s", invalidHuff.String())
	}
}

func TestModern_TargetedBranches(t *testing.T) {
	// 1. splitHeaderField branches
	emptyCookie := splitHeaderField("cookie", "", CookieCrumblingEnabled)
	if len(emptyCookie) != 1 || emptyCookie[0].Value != "" {
		t.Errorf("unexpected empty cookie split: %v", emptyCookie)
	}

	nullDelimited := splitHeaderField("x-header", "v1\x00v2", CookieCrumblingDisabled)
	if len(nullDelimited) != 2 || nullDelimited[0].Value != "v1" || nullDelimited[1].Value != "v2" {
		t.Errorf("unexpected null delimited split: %v", nullDelimited)
	}

	// 2. decodeHuffman with nil arena
	decodedStr, err := decodeHuffman([]byte{0xff}, nil)
	if err != nil {
		t.Errorf("decodeHuffman failed: %v", err)
	}
	_ = decodedStr

	// 3. InstructionDecoder lookupOpcode with nil language
	var id InstructionDecoder
	if id.lookupOpcode(0x80) != nil {
		t.Errorf("expected nil from lookupOpcode with nil language")
	}

	// 4. qpackVarintDecoder.Start with nil data
	var vd qpackVarintDecoder
	n, st := vd.Start(5, nil)
	if n != 0 || st != decodeStatusInProgress {
		t.Errorf("expected (0, inProgress) from Start with nil data, got (%d, %v)", n, st)
	}

	// 5. ProgressiveDecoder Write/Close with lastErr == nil and errorDetected == true
	dec := NewDecoder(100, 10, nil)
	p := dec.CreateProgressiveDecoder(1, &collectedHeadersHandler{})
	p.errorDetected = true
	p.lastErr = nil
	_, err = p.Write([]byte{0x00})
	if !errors.Is(err, ErrDecompressionFailed) {
		t.Errorf("expected ErrDecompressionFailed on Write, got %v", err)
	}

	err = p.Close()
	if !errors.Is(err, ErrDecompressionFailed) {
		t.Errorf("expected ErrDecompressionFailed on Close, got %v", err)
	}

	// 6. ProgressiveDecoder Close when blocked and cancelled
	p2 := dec.CreateProgressiveDecoder(2, &collectedHeadersHandler{})
	p2.blocked = true
	p2.cancelled = true
	_ = p2.Close()

	// 7. PostBase instructions error branches
	p3 := dec.CreateProgressiveDecoder(3, &collectedHeadersHandler{})
	p3.base = 10
	p3.requiredInsertCount = 5
	p3.instructionDecoder.varint = math.MaxUint64 // postBaseIndex overflow
	if p3.doIndexedHeaderFieldPostBaseInstruction() {
		t.Errorf("expected false on postBaseIndex overflow")
	}
	if p3.doLiteralHeaderFieldPostBaseInstruction() {
		t.Errorf("expected false on postBaseIndex overflow")
	}

	// absoluteIndex >= requiredInsertCount
	p4 := dec.CreateProgressiveDecoder(4, &collectedHeadersHandler{})
	p4.base = 10
	p4.requiredInsertCount = 5
	p4.instructionDecoder.varint = 0 // absoluteIndex = 10 >= 5
	if p4.doIndexedHeaderFieldPostBaseInstruction() {
		t.Errorf("expected false on absoluteIndex >= requiredInsertCount")
	}
	p4.errorDetected = false
	if p4.doLiteralHeaderFieldPostBaseInstruction() {
		t.Errorf("expected false on absoluteIndex >= requiredInsertCount")
	}

	// entry == nil (evicted from dynamic table)
	p5 := dec.CreateProgressiveDecoder(5, &collectedHeadersHandler{})
	p5.base = 0
	p5.requiredInsertCount = 10
	p5.instructionDecoder.varint = 2 // absoluteIndex = 2 < 10
	if p5.doIndexedHeaderFieldPostBaseInstruction() {
		t.Errorf("expected false when entry is nil")
	}
	p5.errorDetected = false
	p5.instructionDecoder.sBit = false
	if p5.doLiteralHeaderFieldNameReferenceInstruction() {
		t.Errorf("expected false when entry is nil in literal with name ref")
	}

	// 8. decodeHuffman with non-nil arena
	arena := make([]byte, 0, 32)
	strArena, err := decodeHuffman([]byte{0xff}, &arena)
	if err != nil {
		t.Errorf("decodeHuffman with arena failed: %v", err)
	}
	_ = strArena

	// 9. InstructionDecoder doStartInstruction with unknown opcode
	emptyLang := Language{}
	instDec := NewInstructionDecoder(&emptyLang, &testInstructionDecoderDelegate{})
	instDec.doStartInstruction([]byte{0x42})

	// 10. Decoder InsertWithNameReference dynamic entry doesn't fit
	var encErrCode uint64
	decTableTest := NewDecoder(100, 10, func(code uint64, msg string) {
		encErrCode = code
	})
	decTableTest.headerTable.SetDynamicTableCapacity(100)
	decTableTest.InsertWithoutNameReference("k", "v") // entry 0 inserted (size 34)
	decTableTest.headerTable.SetDynamicTableCapacity(40)
	decTableTest.InsertWithNameReference(false, 0, strings.Repeat("z", 50))
	if encErrCode != QUIC_QPACK_ENCODER_STREAM_ERROR_INSERTING_DYNAMIC {
		t.Errorf("expected QUIC_QPACK_ENCODER_STREAM_ERROR_INSERTING_DYNAMIC, got %d", encErrCode)
	}

	// 11. Decoder Duplicate entry evicted
	decTableTest2 := NewDecoder(100, 10, func(code uint64, msg string) {
		encErrCode = code
	})
	decTableTest2.headerTable.SetDynamicTableCapacity(40)
	decTableTest2.InsertWithoutNameReference("k", "1")
	decTableTest2.InsertWithoutNameReference("k", "2") // evicts entry 0
	decTableTest2.Duplicate(1)                         // maps to evicted absoluteIndex 0
	if encErrCode != QUIC_QPACK_ENCODER_STREAM_DUPLICATE_DYNAMIC_ENTRY_NOT_FOUND {
		t.Errorf("expected duplicate dynamic entry not found, got %d", encErrCode)
	}
}

type testInstructionDecoderDelegate struct{}

func (d *testInstructionDecoderDelegate) OnInstructionDecoded(instruction *Instruction) bool {
	return true
}

func (d *testInstructionDecoderDelegate) OnInstructionDecodingError(
	errorCode InstructionDecoderErrorCode,
	errorMessage string,
) {
}
