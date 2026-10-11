// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/testing/assert"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

func TestNewServerConnAllocations(t *testing.T) {
	handler := func(req *ServerRequest, res *ServerResponse) error {
		return nil
	}

	allocs := testing.AllocsPerRun(1000, func() {
		_ = NewServerConn(nil, handler)
	})

	assert.Equal(t, 28.0, allocs, "NewServerConn allocation budget")
}

func TestServerObjectPool_ZeroAllocations(t *testing.T) {
	// 1. ServerRequest acquire/reset/put must be 0 allocs
	reqAllocs := testing.AllocsPerRun(1000, func() {
		req := serverReqStorage.Get()
		req.Reset()
		serverReqStorage.Put(req)
	})
	assert.Equal(t, 0.0, reqAllocs, "ServerRequest pool operations must be 0 allocs")

	// 2. ServerResponse acquire/reset/put must be 0 allocs
	resAllocs := testing.AllocsPerRun(1000, func() {
		res := serverResStorage.Get()
		res.Reset()
		serverResStorage.Put(res)
	})
	assert.Equal(t, 0.0, resAllocs, "ServerResponse pool operations must be 0 allocs")

	// 3. Body buffer pool acquire/reset/put must be 0 allocs
	bufAllocs := testing.AllocsPerRun(1000, func() {
		buf := h3BodyBufferStorage.Get()
		buf.Reset()
		h3BodyBufferStorage.Put(buf)
	})
	assert.Equal(t, 0.0, bufAllocs, "h3BodyBufferStorage pool operations must be 0 allocs")

	// 4. Reader pool acquire/reset/put must be 0 allocs
	readerAllocs := testing.AllocsPerRun(1000, func() {
		br := h3ReaderStorage.Get()
		br.Reset(nil)
		h3ReaderStorage.Put(br)
	})
	assert.Equal(t, 0.0, readerAllocs, "h3ReaderStorage pool operations must be 0 allocs")
}

func TestServerFrameEncoding_ZeroAllocations(t *testing.T) {
	var frameHdr [16]byte

	allocs := testing.AllocsPerRun(10000, func() {
		hdrBytes := varint.Append(frameHdr[:0], coreh3.FrameTypeHeaders)
		hdrBytes = varint.Append(hdrBytes, 512)

		dataHdrBytes := varint.Append(frameHdr[:0], coreh3.FrameTypeData)
		_ = varint.Append(dataHdrBytes, 4096)
		_ = hdrBytes
	})

	assert.Equal(t, 0.0, allocs, "Server frame header packing must be 0 allocs")
}

func TestServerFrameDecoding_ZeroAllocations(t *testing.T) {
	var frameHdr [16]byte
	hdrBytes := varint.Append(frameHdr[:0], coreh3.FrameTypeHeaders)
	hdrBytes = varint.Append(hdrBytes, 256)

	r := bytes.NewReader(hdrBytes)

	allocs := testing.AllocsPerRun(1000, func() {
		r.Reset(hdrBytes)
		_, _, _ = coreh3.ReadFrameHeader(r)
	})

	assert.Equal(t, 0.0, allocs, "Frame header decoding from reader must be 0 allocs")
}

func BenchmarkServerPool_AcquireRelease(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		req := serverReqStorage.Get()
		res := serverResStorage.Get()
		buf := h3BodyBufferStorage.Get()

		req.Reset()
		res.Reset()
		buf.Reset()

		h3BodyBufferStorage.Put(buf)
		serverResStorage.Put(res)
		serverReqStorage.Put(req)
	}
}

func BenchmarkServerResponse_Framing(b *testing.B) {
	var frameHdr [16]byte
	bodyLen := uint64(1024)
	respLen := uint64(128)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		hdrBytes := varint.Append(frameHdr[:0], coreh3.FrameTypeHeaders)
		hdrBytes = varint.Append(hdrBytes, respLen)

		dataBytes := varint.Append(frameHdr[:0], coreh3.FrameTypeData)
		dataBytes = varint.Append(dataBytes, bodyLen)

		_ = hdrBytes
		_ = dataBytes
	}
}

// Ensure unused import http is referenced or removed
var _ = http.StatusOK
