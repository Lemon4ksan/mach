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
	for i := range 10 {
		fields[i] = HeaderField{
			Name:  "header-" + strconv.Itoa(i),
			Value: "value-" + strconv.Itoa(i),
		}
	}

	b.ReportAllocs()

	var totalLen int
	for b.Loop() {
		for name, value := range fields.All() {
			totalLen += len(name) + len(value)
		}
	}
	sinkCount = totalLen
}

func BenchmarkHeaderFields_Values(b *testing.B) {
	fields := make(HeaderFields, 10)
	for i := range 10 {
		fields[i] = HeaderField{
			Name:  "header-" + strconv.Itoa(i),
			Value: "value-" + strconv.Itoa(i),
		}
	}

	b.ReportAllocs()

	var totalLen int
	for b.Loop() {
		for hf := range fields.Values() {
			totalLen += len(hf.Name) + len(hf.Value)
		}
	}
	sinkCount = totalLen
}

func BenchmarkIndexSet_All(b *testing.B) {
	set := NewIndexSet()
	for i := range uint64(10) {
		set.Insert(i)
	}

	b.ReportAllocs()

	var sum uint64
	for b.Loop() {
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
	for i := range 10 {
		table.InsertEntry("header-"+strconv.Itoa(i), "value-"+strconv.Itoa(i))
	}

	b.ReportAllocs()

	var sum uint64
	for b.Loop() {
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
	for i := range 10 {
		table.InsertEntry("header-"+strconv.Itoa(i), "value-"+strconv.Itoa(i))
	}

	b.ReportAllocs()

	var sum uint64
	for b.Loop() {
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

	b.ReportAllocs()

	var sum uint64
	for b.Loop() {
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

	b.ReportAllocs()

	var streamID uint64
	for b.Loop() {
		streamID++
		_ = encoder.EncodeHeaderList(streamID, fields, nil)
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

	b.ReportAllocs()

	var streamID uint64
	for b.Loop() {
		streamID++
		for _, err := range decoder.DecodeHeaderBlock(streamID, block) {
			_ = err
		}
	}
}
