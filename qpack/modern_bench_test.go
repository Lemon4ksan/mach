// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"strconv"
	"testing"
)

var sinkCount int

func BenchmarkHeaderFields_All(b *testing.B) {
	fields := make(HeaderFields, 10)
	for i := 0; i < 10; i++ {
		fields[i] = HeaderField{
			Name:  "header-" + strconv.Itoa(i),
			Value: "value-" + strconv.Itoa(i),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	var totalLen int
	for i := 0; i < b.N; i++ {
		for name, value := range fields.All() {
			totalLen += len(name) + len(value)
		}
	}
	sinkCount = totalLen
}

func BenchmarkHeaderFields_Values(b *testing.B) {
	fields := make(HeaderFields, 10)
	for i := 0; i < 10; i++ {
		fields[i] = HeaderField{
			Name:  "header-" + strconv.Itoa(i),
			Value: "value-" + strconv.Itoa(i),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	var totalLen int
	for i := 0; i < b.N; i++ {
		for hf := range fields.Values() {
			totalLen += len(hf.Name) + len(hf.Value)
		}
	}
	sinkCount = totalLen
}

func BenchmarkIndexSet_All(b *testing.B) {
	set := NewIndexSet()
	for i := uint64(0); i < 10; i++ {
		set.Insert(i)
	}

	b.ResetTimer()
	b.ReportAllocs()

	var sum uint64
	for i := 0; i < b.N; i++ {
		for idx := range set.All() {
			sum += idx
		}
	}
	sinkCount = int(sum)
}

func BenchmarkEncoderHeaderTable_Entries(b *testing.B) {
	table := NewEncoderHeaderTable()
	table.SetMaximumDynamicTableCapacity(4096)
	table.SetDynamicTableCapacity(4096)
	for i := 0; i < 10; i++ {
		table.InsertEntry("header-"+strconv.Itoa(i), "value-"+strconv.Itoa(i))
	}

	b.ResetTimer()
	b.ReportAllocs()

	var sum uint64
	for i := 0; i < b.N; i++ {
		for idx, entry := range table.Entries() {
			sum += idx + uint64(len(entry.Name))
		}
	}
	sinkCount = int(sum)
}

func BenchmarkDecoderHeaderTable_Entries(b *testing.B) {
	table := NewDecoderHeaderTable()
	table.SetMaximumDynamicTableCapacity(4096)
	table.SetDynamicTableCapacity(4096)
	for i := 0; i < 10; i++ {
		table.InsertEntry("header-"+strconv.Itoa(i), "value-"+strconv.Itoa(i))
	}

	b.ResetTimer()
	b.ReportAllocs()

	var sum uint64
	for i := 0; i < b.N; i++ {
		for idx, entry := range table.Entries() {
			sum += idx + uint64(len(entry.Name))
		}
	}
	sinkCount = int(sum)
}

func BenchmarkBlockingManager_BlockedStreams(b *testing.B) {
	bm := NewBlockingManager()
	bm.SetMaxBlockedStreams(100)
	for i := uint64(1); i <= 10; i++ {
		bm.OnHeaderBlockSent(i, []uint64{0})
	}

	b.ResetTimer()
	b.ReportAllocs()

	var sum uint64
	for i := 0; i < b.N; i++ {
		for streamID := range bm.BlockedStreams() {
			sum += streamID
		}
	}
	sinkCount = int(sum)
}

func BenchmarkEncoder_EncodeHeaderList_StaticOnly(b *testing.B) {
	encoder := NewEncoderWithDefaults(nil)
	fields := []HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "example.com"},
		{Name: ":path", Value: "/"},
		{Name: "user-agent", Value: "curl/7.68.0"},
		{Name: "accept", Value: "*/*"},
		{Name: "accept-encoding", Value: "gzip, deflate, br"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = encoder.EncodeHeaderList(uint64(i), fields, nil)
	}
}

func BenchmarkDecoder_DecodeHeaderBlock(b *testing.B) {
	encoder := NewEncoderWithDefaults(nil)
	decoder := NewDecoder(0, 0, nil)
	fields := []HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "example.com"},
		{Name: ":path", Value: "/"},
	}
	block := encoder.EncodeHeaderList(1, fields, nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = decoder.DecodeHeaderBlock(uint64(i+1), block)
	}
}
