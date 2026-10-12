// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package varint_test

import (
	"bytes"
	"math"
	"sync"
	"testing"

	"github.com/lemon4ksan/mach/net/quic/varint"
)

func TestBoundaryTransitions(t *testing.T) {
	testCases := []struct {
		val         uint64
		expectedLen int
	}{
		{0, 1},
		{varint.Max1Byte, 1},     // 63
		{varint.Max1Byte + 1, 2}, // 64
		{varint.Max2Byte, 2},     // 16383
		{varint.Max2Byte + 1, 4}, // 16384
		{varint.Max4Byte, 4},     // 1073741823
		{varint.Max4Byte + 1, 8}, // 1073741824
		{varint.Max, 8},          // 4611686018427387903
	}

	for _, tc := range testCases {
		enc := varint.Encode(tc.val)
		if len(enc) != tc.expectedLen {
			t.Fatalf("Encode(%d) len = %d, want %d", tc.val, len(enc), tc.expectedLen)
		}

		var buf [8]byte
		n := varint.EncodeSlice(tc.val, buf[:])
		if n != tc.expectedLen {
			t.Fatalf("EncodeSlice(%d) len = %d, want %d", tc.val, n, tc.expectedLen)
		}
		if !bytes.Equal(enc, buf[:n]) {
			t.Fatalf("mismatch between Encode and EncodeSlice for %d", tc.val)
		}

		dec, readLen, err := varint.Decode(enc)
		if err != nil {
			t.Fatalf("Decode(%d) failed: %v", tc.val, err)
		}
		if dec != tc.val {
			t.Fatalf("Decode(%d) = %d", tc.val, dec)
		}
		if readLen != tc.expectedLen {
			t.Fatalf("Decode(%d) readLen = %d, want %d", tc.val, readLen, tc.expectedLen)
		}
	}
}

func TestOverflowPanics(t *testing.T) {
	overflowValues := []uint64{
		1 << 62,
		(1 << 62) + 1,
		^uint64(0),
	}

	for _, v := range overflowValues {
		assertPanic(t, "Encode", func() {
			_ = varint.Encode(v)
		})

		var buf [8]byte
		assertPanic(t, "EncodeSlice", func() {
			_ = varint.EncodeSlice(v, buf[:])
		})
	}
}

func TestBufferBoundsPanics(t *testing.T) {
	// 1-byte value into 0-byte slice
	assertPanic(t, "1B into 0B", func() {
		var buf [0]byte
		_ = varint.EncodeSlice(10, buf[:])
	})

	// 2-byte value into 1-byte slice
	assertPanic(t, "2B into 1B", func() {
		var buf [1]byte
		_ = varint.EncodeSlice(100, buf[:])
	})

	// 4-byte value into 3-byte slice
	assertPanic(t, "4B into 3B", func() {
		var buf [3]byte
		_ = varint.EncodeSlice(100000, buf[:])
	})

	// 8-byte value into 7-byte slice
	assertPanic(t, "8B into 7B", func() {
		var buf [7]byte
		_ = varint.EncodeSlice(2000000000, buf[:])
	})
}

func TestDecodeVarintTruncated(t *testing.T) {
	// Empty payload
	if _, _, err := varint.Decode(nil); err != varint.ErrTruncated {
		t.Fatalf("Decode(nil) = %v, want ErrTruncated", err)
	}
	if _, _, err := varint.Decode([]byte{}); err != varint.ErrTruncated {
		t.Fatalf("Decode([]byte{}) = %v, want ErrTruncated", err)
	}

	// 2-byte tag (0b01) with 1 byte input
	if _, _, err := varint.Decode([]byte{0x40}); err != varint.ErrTruncated {
		t.Fatalf("2-byte tag with 1 byte = %v, want ErrTruncated", err)
	}

	// 4-byte tag (0b10) with 1, 2, 3 bytes input
	for l := 1; l <= 3; l++ {
		payload := make([]byte, l)
		payload[0] = 0x80
		if _, _, err := varint.Decode(payload); err != varint.ErrTruncated {
			t.Fatalf("4-byte tag with %d bytes = %v, want ErrTruncated", l, err)
		}
	}

	// 8-byte tag (0b11) with 1..7 bytes input
	for l := 1; l <= 7; l++ {
		payload := make([]byte, l)
		payload[0] = 0xc0
		if _, _, err := varint.Decode(payload); err != varint.ErrTruncated {
			t.Fatalf("8-byte tag with %d bytes = %v, want ErrTruncated", l, err)
		}
	}
}

func TestNonMinimalEncodings(t *testing.T) {
	// RFC 9000 Section 16: decoders MUST accept all valid encodings, even if non-minimal.
	tests := []struct {
		name     string
		encoded  []byte
		expected uint64
		readLen  int
	}{
		// 0 encoded in 2, 4, 8 bytes
		{"0_in_2B", []byte{0x40, 0x00}, 0, 2},
		{"0_in_4B", []byte{0x80, 0x00, 0x00, 0x00}, 0, 4},
		{"0_in_8B", []byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 0, 8},

		// 25 encoded in 2, 4, 8 bytes
		{"25_in_2B", []byte{0x40, 0x19}, 25, 2},
		{"25_in_4B", []byte{0x80, 0x00, 0x00, 0x19}, 25, 4},
		{"25_in_8B", []byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x19}, 25, 8},

		// 16383 encoded in 4, 8 bytes
		{"16383_in_4B", []byte{0x80, 0x00, 0x3f, 0xff}, 16383, 4},
		{"16383_in_8B", []byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x3f, 0xff}, 16383, 8},

		// 1073741823 encoded in 8 bytes
		{"1073741823_in_8B", []byte{0xc0, 0x00, 0x00, 0x00, 0x3f, 0xff, 0xff, 0xff}, 1073741823, 8},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			val, n, err := varint.Decode(tc.encoded)
			if err != nil {
				t.Fatalf("Decode(%s) failed: %v", tc.name, err)
			}
			if val != tc.expected {
				t.Fatalf("Decode(%s) = %d, want %d", tc.name, val, tc.expected)
			}
			if n != tc.readLen {
				t.Fatalf("Decode(%s) n = %d, want %d", tc.name, n, tc.readLen)
			}
		})
	}
}

func TestIterators(t *testing.T) {
	values := []uint64{0, 25, 63, 64, 15293, 16383, 16384, 494878333, 1073741823, 1073741824, varint.Max}

	var stream []byte
	for _, v := range values {
		stream = append(stream, varint.Encode(v)...)
	}

	// Test DecodeSeq full stream
	var decodedVals []uint64
	var totalRead int
	for v, n := range varint.DecodeSeq(stream) {
		decodedVals = append(decodedVals, v)
		totalRead += n
	}
	if len(decodedVals) != len(values) {
		t.Fatalf("DecodeSeq yielded %d elements, want %d", len(decodedVals), len(values))
	}
	for i := range values {
		if decodedVals[i] != values[i] {
			t.Fatalf("DecodeSeq[%d] = %d, want %d", i, decodedVals[i], values[i])
		}
	}
	if totalRead != len(stream) {
		t.Fatalf("totalRead = %d, want %d", totalRead, len(stream))
	}

	// Test DecodeSeq early break
	count := 0
	for range varint.DecodeSeq(stream) {
		count++
		if count == 3 {
			break
		}
	}
	if count != 3 {
		t.Fatalf("DecodeSeq early break count = %d, want 3", count)
	}

	// Test DecodeSeq with truncated tail
	truncatedStream := append([]byte(nil), stream...)
	truncatedStream = append(truncatedStream, 0x80, 0x01) // 4-byte varint with only 2 bytes
	countTrunc := 0
	for range varint.DecodeSeq(truncatedStream) {
		countTrunc++
	}
	if countTrunc != len(values) {
		t.Fatalf("DecodeSeq with truncated tail yielded %d, want %d", countTrunc, len(values))
	}

	// Test Values full stream
	var valuesSeq []uint64
	for v := range varint.Values(stream) {
		valuesSeq = append(valuesSeq, v)
	}
	if len(valuesSeq) != len(values) {
		t.Fatalf("Values yielded %d elements, want %d", len(valuesSeq), len(values))
	}
	for i := range values {
		if valuesSeq[i] != values[i] {
			t.Fatalf("Values[%d] = %d, want %d", i, valuesSeq[i], values[i])
		}
	}

	// Test Values early break
	countVal := 0
	for range varint.Values(stream) {
		countVal++
		if countVal == 4 {
			break
		}
	}
	if countVal != 4 {
		t.Fatalf("Values early break count = %d, want 4", countVal)
	}

	// Test Values with truncated tail
	countValTrunc := 0
	for range varint.Values(truncatedStream) {
		countValTrunc++
	}
	if countValTrunc != len(values) {
		t.Fatalf("Values with truncated tail yielded %d, want %d", countValTrunc, len(values))
	}
}

func BenchmarkEncodeSlice_1B(b *testing.B) {
	var buf [8]byte
	b.ReportAllocs()
	for b.Loop() {
		_ = varint.EncodeSlice(37, buf[:])
	}
}

func BenchmarkEncodeSlice_2B(b *testing.B) {
	var buf [8]byte
	b.ReportAllocs()
	for b.Loop() {
		_ = varint.EncodeSlice(15293, buf[:])
	}
}

func BenchmarkEncodeSlice_4B(b *testing.B) {
	var buf [8]byte
	b.ReportAllocs()
	for b.Loop() {
		_ = varint.EncodeSlice(494878333, buf[:])
	}
}

func BenchmarkEncodeSlice_8B(b *testing.B) {
	var buf [8]byte
	b.ReportAllocs()
	for b.Loop() {
		_ = varint.EncodeSlice(varint.Max, buf[:])
	}
}

func BenchmarkDecode_1B(b *testing.B) {
	payload := varint.Encode(37)
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = varint.Decode(payload)
	}
}

func BenchmarkDecode_2B(b *testing.B) {
	payload := varint.Encode(15293)
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = varint.Decode(payload)
	}
}

func BenchmarkDecode_4B(b *testing.B) {
	payload := varint.Encode(494878333)
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = varint.Decode(payload)
	}
}

func BenchmarkDecode_8B(b *testing.B) {
	payload := varint.Encode(varint.Max)
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = varint.Decode(payload)
	}
}

func BenchmarkDecodeSeq(b *testing.B) {
	values := []uint64{37, 15293, 494878333, varint.Max}
	var stream []byte
	for _, v := range values {
		stream = append(stream, varint.Encode(v)...)
	}

	b.ReportAllocs()
	for b.Loop() {
		for v, n := range varint.DecodeSeq(stream) {
			_ = v
			_ = n
		}
	}
}

func assertPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("%s did not panic", name)
		}
	}()
	fn()
}

func TestStressVarint_AllBoundariesRoundTrip(t *testing.T) {
	boundaries := []uint64{
		0, 1, 2,
		varint.Max1Byte - 1, varint.Max1Byte, varint.Max1Byte + 1, varint.Max1Byte + 2,
		varint.Max2Byte - 1, varint.Max2Byte, varint.Max2Byte + 1, varint.Max2Byte + 2,
		varint.Max4Byte - 1, varint.Max4Byte, varint.Max4Byte + 1, varint.Max4Byte + 2,
		varint.Max - 2, varint.Max - 1, varint.Max,
	}

	for _, v := range boundaries {
		enc := varint.Encode(v)
		var buf [8]byte
		n := varint.EncodeSlice(v, buf[:])
		if !bytes.Equal(enc, buf[:n]) {
			t.Fatalf("v=%d: Encode and EncodeSlice diverge", v)
		}

		dec, readLen, err := varint.Decode(enc)
		if err != nil {
			t.Fatalf("v=%d: Decode failed: %v", v, err)
		}
		if dec != v {
			t.Fatalf("v=%d: Decode returned %d", v, dec)
		}
		if readLen != len(enc) {
			t.Fatalf("v=%d: readLen=%d != len(enc)=%d", v, readLen, len(enc))
		}
	}
}

func TestStressVarint_OverflowExhaustive(t *testing.T) {
	overflows := []uint64{
		1 << 62,
		(1 << 62) + 1,
		(1 << 62) + 0x123456,
		1 << 63,
		(1 << 63) | (1 << 62),
		math.MaxUint64 - 1,
		math.MaxUint64,
	}

	for _, v := range overflows {
		assertPanic(t, "Encode", func() {
			_ = varint.Encode(v)
		})
		var buf [8]byte
		assertPanic(t, "EncodeSlice", func() {
			_ = varint.EncodeSlice(v, buf[:])
		})
	}
}

func TestStressVarint_IteratorsEarlyBreakAndTruncation(t *testing.T) {
	values := []uint64{
		0, 63, 64, 16383, 16384, 1073741823, 1073741824, varint.Max,
	}

	var stream []byte
	for _, v := range values {
		stream = append(stream, varint.Encode(v)...)
	}

	// 1. Break at every possible step for DecodeSeq
	for stopAt := range len(values) + 1 {
		count := 0
		for val, n := range varint.DecodeSeq(stream) {
			if count == stopAt {
				break
			}
			if val != values[count] {
				t.Fatalf("stopAt %d: step %d val=%d, want %d", stopAt, count, val, values[count])
			}
			_ = n
			count++
		}
		if count != stopAt {
			t.Fatalf("stopAt %d: iterated %d times", stopAt, count)
		}
	}

	// 2. Break at every possible step for Values
	for stopAt := range len(values) + 1 {
		count := 0
		for val := range varint.Values(stream) {
			if count == stopAt {
				break
			}
			if val != values[count] {
				t.Fatalf("stopAt %d: step %d val=%d, want %d", stopAt, count, val, values[count])
			}
			count++
		}
		if count != stopAt {
			t.Fatalf("stopAt %d: iterated %d times", stopAt, count)
		}
	}

	// 3. Truncated stream in the middle of an 8-byte varint
	truncated := append([]byte(nil), stream...)
	truncated = append(truncated, 0xc0, 0x01, 0x02) // 8-byte tag with only 3 bytes
	var decodedTrunc []uint64
	for v := range varint.Values(truncated) {
		decodedTrunc = append(decodedTrunc, v)
	}
	if len(decodedTrunc) != len(values) {
		t.Fatalf("Values with truncated tail yielded %d, want %d", len(decodedTrunc), len(values))
	}
}

func TestStressVarint_Parallel(t *testing.T) {
	const goroutines = 16
	const iters = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			var buf [8]byte
			for i := range iters {
				val := uint64((id*1000 + i) % 100000)
				n := varint.EncodeSlice(val, buf[:])
				dec, rLen, err := varint.Decode(buf[:n])
				if err != nil || dec != val || rLen != n {
					t.Errorf("parallel failed for id %d i %d", id, i)
					return
				}
			}
		}(g)
	}

	wg.Wait()
}

func TestAppendAndLen(t *testing.T) {
	testValues := []uint64{0, 25, 63, 64, 1000, 16383, 16384, 1000000, 1073741823, 1073741824, varint.Max}
	for _, v := range testValues {
		expectedLen := varint.Len(v)
		b := varint.Append(nil, v)
		if len(b) != expectedLen {
			t.Fatalf("Len(%d) = %d, but Append produced %d bytes", v, expectedLen, len(b))
		}
		dec, readLen, err := varint.Decode(b)
		if err != nil {
			t.Fatalf("Decode failed for Append(%d): %v", v, err)
		}
		if dec != v || readLen != expectedLen {
			t.Fatalf("Decode(Append(%d)) = %d, len=%d; want %d, len=%d", v, dec, readLen, v, expectedLen)
		}
	}
}

