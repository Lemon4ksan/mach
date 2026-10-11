// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hpack

import (
	"bytes"
	"errors"
	"math"
	"testing"
)

func TestHuffmanEncodingSymmetry(t *testing.T) {
	inputs := []string{
		"aoni",
		":method",
		"GET",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	}

	for _, input := range inputs {
		src := []byte(input)

		encoded := HuffmanEncode(nil, src)
		decoded := HuffmanDecode(nil, encoded)

		if !bytes.Equal(decoded, src) {
			t.Fatalf("Huffman decoding failure for %q: got %q", input, string(decoded))
		}
	}
}

func TestHPACKDynamicTableShrinking(t *testing.T) {
	hp := AcquireHPACK()
	defer ReleaseHPACK(hp)

	hp.SetMaxTableSize(100)

	for range 10 {
		hf := AcquireHeaderField()

		hf.Set("x-large-header-key", "some-very-large-value-that-causes-table-eviction")
		hp.addDynamic(hf)

		ReleaseHeaderField(hf)
	}

	if hp.DynamicSize() > 100 {
		t.Fatalf("dynamic table size (%d) exceeded max size limit (100)", hp.DynamicSize())
	}
}

// TestRFC7541AppendixCExamples validates HPACK encoding/decoding against official RFC 7541 Appendix C test vectors.
func TestRFC7541AppendixCExamples(t *testing.T) {
	// C.1.1: 10 with 5-bit prefix -> 0x0a (binary: 00001010)
	buf := appendInt(nil, 5, 10)
	if len(buf) != 1 || buf[0] != 0x0a {
		t.Fatalf("RFC 7541 C.1.1 failed: got %x, want 0a", buf)
	}

	_, val, err := readInt(5, buf)
	if err != nil || val != 10 {
		t.Fatalf("RFC 7541 C.1.1 readInt failed: got %d, err %v, want 10", val, err)
	}

	// C.1.2: 1337 with 5-bit prefix -> 0x1f, 0x9a, 0x0a
	buf = appendInt(nil, 5, 1337)

	expected1337 := []byte{0x1f, 0x9a, 0x0a}
	if !bytes.Equal(buf, expected1337) {
		t.Fatalf("RFC 7541 C.1.2 failed: got %x, want %x", buf, expected1337)
	}

	_, val, err = readInt(5, buf)
	if err != nil || val != 1337 {
		t.Fatalf("RFC 7541 C.1.2 readInt failed: got %d, err %v, want 1337", val, err)
	}

	// C.1.3: 42 on 8-bit boundary -> 0x2a
	buf = appendInt(nil, 8, 42)
	if len(buf) != 1 || buf[0] != 0x2a {
		t.Fatalf("RFC 7541 C.1.3 failed: got %x, want 2a", buf)
	}

	_, val, err = readInt(8, buf)
	if err != nil || val != 42 {
		t.Fatalf("RFC 7541 C.1.3 readInt failed: got %d, err %v, want 42", val, err)
	}

	// C.2.4: Indexed Header Field (:method: GET) -> index 2 -> 0x82
	hp := AcquireHPACK()
	defer ReleaseHPACK(hp)

	hf := AcquireHeaderField()
	defer ReleaseHeaderField(hf)

	hf.Set(":method", "GET")

	enc := hp.AppendHeader(nil, hf, false)
	if len(enc) != 1 || enc[0] != 0x82 {
		t.Fatalf("RFC 7541 C.2.4 failed: got %x, want 82", enc)
	}
}

func BenchmarkHPACK_Encode(b *testing.B) {
	hp := AcquireHPACK()
	defer ReleaseHPACK(hp)

	hf := AcquireHeaderField()
	defer ReleaseHeaderField(hf)

	hf.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	dst := make([]byte, 0, 256)

	b.ReportAllocs()

	for b.Loop() {
		_ = hp.AppendHeader(dst[:0], hf, true)
	}
}

func BenchmarkHPACK_Decode(b *testing.B) {
	hpEnc := AcquireHPACK()
	defer ReleaseHPACK(hpEnc)

	hpDec := AcquireHPACK()
	defer ReleaseHPACK(hpDec)

	hf := AcquireHeaderField()
	defer ReleaseHeaderField(hf)

	hf.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	encoded := hpEnc.AppendHeader(nil, hf, true)

	target := AcquireHeaderField()
	defer ReleaseHeaderField(target)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = hpDec.Next(target, encoded)
	}
}

func BenchmarkHuffman_Encode(b *testing.B) {
	src := []byte("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	dst := make([]byte, 0, 128)

	b.ReportAllocs()

	for b.Loop() {
		_ = HuffmanEncode(dst[:0], src)
	}
}

func BenchmarkHuffman_Decode(b *testing.B) {
	src := []byte("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	encoded := HuffmanEncode(nil, src)
	dst := make([]byte, 0, 128)

	b.ReportAllocs()

	for b.Loop() {
		_ = HuffmanDecode(dst[:0], encoded)
	}
}

func TestHPACK_IntegerDecoding_RFC7541(t *testing.T) {
	// Truncated integer (prefix full, no continuation bytes)
	_, _, err := readInt(7, []byte{0x7f})
	if !errors.Is(err, ErrIntegerTruncated) {
		t.Fatalf("expected ErrIntegerTruncated for truncated 7-bit prefix, got %v", err)
	}

	_, _, err = readInt(5, []byte{0x1f})
	if !errors.Is(err, ErrIntegerTruncated) {
		t.Fatalf("expected ErrIntegerTruncated for truncated 5-bit prefix, got %v", err)
	}

	// Incomplete continuation (continuation byte has MSB=1 but EOF reached)
	_, _, err = readInt(7, []byte{0x7f, 0x85})
	if !errors.Is(err, ErrIntegerTruncated) {
		t.Fatalf("expected ErrIntegerTruncated for incomplete continuation, got %v", err)
	}

	// Integer overflow (> 64-bit integer encoding)
	overflowBytes := []byte{0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}
	_, _, err = readInt(7, overflowBytes)
	if !errors.Is(err, ErrIntegerOverflow) {
		t.Fatalf("expected ErrIntegerOverflow, got %v", err)
	}

	// Valid multi-byte integer
	rem, val, err := readInt(7, []byte{0x7f, 0x00, 0x42})
	if err != nil {
		t.Fatalf("unexpected error for valid 127: %v", err)
	}
	if val != 127 || len(rem) != 1 || rem[0] != 0x42 {
		t.Fatalf("got val=%d, rem=%x; want 127, [42]", val, rem)
	}
}

func TestHPACK_MalformedInputs_Robustness(t *testing.T) {
	hp := AcquireHPACK()
	defer ReleaseHPACK(hp)

	hf := AcquireHeaderField()
	defer ReleaseHeaderField(hf)

	// 1. Index 0 is forbidden (RFC 7541 §6.1)
	_, err := hp.Next(hf, []byte{0x80})
	if !errors.Is(err, ErrIndexZero) {
		t.Fatalf("expected ErrIndexZero, got %v", err)
	}

	// 2. Truncated indexed representation
	_, err = hp.Next(hf, []byte{0xff})
	if !errors.Is(err, ErrIntegerTruncated) {
		t.Fatalf("expected ErrIntegerTruncated for 0xff, got %v", err)
	}

	// 3. Overflow index in peek should not panic
	hfNil := hp.peek(math.MaxUint64)
	if hfNil != nil {
		t.Fatalf("expected nil for math.MaxUint64, got %v", hfNil)
	}
	hfNil = hp.peek(math.MaxUint64 - 10)
	if hfNil != nil {
		t.Fatalf("expected nil for math.MaxUint64 - 10, got %v", hfNil)
	}

	// 4. Malformed string length exceeding buffer
	_, err = hp.Next(hf, []byte{0x40, 0x01, 'k', 0x50, 'v'})
	if err == nil {
		t.Fatal("expected error for truncated string value, got nil")
	}

	// 5. Dynamic table shrink underflow protection
	hp.SetMaxTableSize(100)
	hp.dynamicSize = 10
	hp.maxTableSize = 5
	// Insert dummy field with size 20 > dynamicSize to test underflow
	hp.dynamic = append(hp.dynamic, &HeaderField{key: []byte("foo"), value: []byte("bar")})
	hp.shrink()
	if hp.dynamicSize != 0 {
		t.Fatalf("expected dynamicSize 0, got %d", hp.dynamicSize)
	}

	// 6. Decode iterator progress guarantee
	for _, e := range hp.Decode([]byte{0xff}) {
		if e == nil {
			t.Fatal("expected error on malformed input in Decode iterator")
		}
	}
}

func TestHuffmanGenericHarmonization(t *testing.T) {
	raw := "www.example.com"
	rawBytes := []byte(raw)

	// 1. HuffmanEncode with string and []byte
	encStr := HuffmanEncode(nil, raw)
	encBytes := HuffmanEncode(nil, rawBytes)
	if !bytes.Equal(encStr, encBytes) {
		t.Fatalf("HuffmanEncode string and bytes mismatch: %x vs %x", encStr, encBytes)
	}

	// 2. HuffmanEncodeLength with string and []byte
	lenStr := HuffmanEncodeLength(raw)
	lenBytes := HuffmanEncodeLength(rawBytes)
	if lenStr != lenBytes || lenStr != len(encStr) {
		t.Fatalf("HuffmanEncodeLength mismatch: str=%d, bytes=%d, actual=%d", lenStr, lenBytes, len(encStr))
	}

	// 3. AppendHuffman with string and []byte
	appStr := AppendHuffman(nil, raw)
	appBytes := AppendHuffman(nil, rawBytes)
	if !bytes.Equal(appStr, encStr) || !bytes.Equal(appBytes, encBytes) {
		t.Fatal("AppendHuffman output mismatch")
	}

	// 4. AppendHuffmanDecode with string and []byte
	decFromBytes, err := AppendHuffmanDecode(nil, encBytes)
	if err != nil || string(decFromBytes) != raw {
		t.Fatalf("AppendHuffmanDecode from bytes failed: %s, %v", decFromBytes, err)
	}

	decFromStr, err := AppendHuffmanDecode(nil, string(encBytes))
	if err != nil || string(decFromStr) != raw {
		t.Fatalf("AppendHuffmanDecode from string failed: %s, %v", decFromStr, err)
	}

	// 5. HuffmanDecodeToString with string and []byte
	decStrFromBytes, err := HuffmanDecodeToString(encBytes)
	if err != nil || decStrFromBytes != raw {
		t.Fatalf("HuffmanDecodeToString from bytes failed: %s, %v", decStrFromBytes, err)
	}

	decStrFromStr, err := HuffmanDecodeToString(string(encBytes))
	if err != nil || decStrFromStr != raw {
		t.Fatalf("HuffmanDecodeToString from string failed: %s, %v", decStrFromStr, err)
	}

	// 6. HuffmanDecode with string and []byte
	decRawBytes := HuffmanDecode(nil, encBytes)
	if string(decRawBytes) != raw {
		t.Fatalf("HuffmanDecode bytes failed: %s", decRawBytes)
	}
	decRawStr := HuffmanDecode(nil, string(encBytes))
	if string(decRawStr) != raw {
		t.Fatalf("HuffmanDecode string failed: %s", decRawStr)
	}

	// 7. Canonical AppendHuffman and HuffmanEncodeLength with string
	appEnc := AppendHuffman(nil, raw)
	if !bytes.Equal(appEnc, encStr) {
		t.Fatal("AppendHuffman mismatch")
	}
	appLen := HuffmanEncodeLength(raw)
	if appLen != lenStr {
		t.Fatalf("HuffmanEncodeLength mismatch: %d vs %d", appLen, lenStr)
	}
}
