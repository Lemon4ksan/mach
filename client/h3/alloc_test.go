// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/codec/qpack"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	machhttp "github.com/lemon4ksan/mach/proto/http"
)

type noopDiscardWriter struct{}

func (noopDiscardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func TestSendRequest_AllocationProfile(t *testing.T) {
	cc := &ClientConn{
		qpack: coreh3.NewQPACKCodec(),
	}

	req := machhttp.AcquireRequest()
	defer machhttp.ReleaseRequest(req)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://api.example.com/v1/resource")
	req.Header.Set("accept", "application/json")
	req.Header.Set("user-agent", "mach-client/1.0")

	var dw noopDiscardWriter

	// Warm-up pool and codec
	err := cc.sendRequestTo(dw, req, nil, 0)
	require.NoError(t, err)

	allocs := testing.AllocsPerRun(1000, func() {
		_ = cc.sendRequestTo(dw, req, nil, 0)
	})

	t.Logf("sendRequestTo allocs per run: %v", allocs)
	// QPACK dynamic header encoding and Request URI parsing requires ~35 allocations
	assert.LessOrEqual(t, allocs, 40.0)
}

func TestReadResponse_AllocationProfile(t *testing.T) {
	cc := &ClientConn{
		qpack: coreh3.NewQPACKCodec(),
	}

	// Prepare an encoded response: HEADERS + DATA
	headers := []qpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "application/json"},
		{Name: "server", Value: "mach"},
	}
	hBlock := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, headers, nil)

	var wire bytes.Buffer
	wire.Write(coreh3.AppendHeadersHeader(nil, uint64(len(hBlock))))
	wire.Write(hBlock)

	body := []byte(`{"status":"ok"}`)
	wire.Write(coreh3.AppendDataHeader(nil, uint64(len(body))))
	wire.Write(body)

	wireBytes := wire.Bytes()

	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseResponse(resp)

	// Warm-up
	reader := bytes.NewReader(wireBytes)
	_, err := cc.readResponseFrom(reader, resp, 0)
	require.NoError(t, err)

	allocs := testing.AllocsPerRun(1000, func() {
		reader.Reset(wireBytes)
		resp.Reset()
		_, _ = cc.readResponseFrom(reader, resp, 0)
	})

	t.Logf("readResponseFrom allocs per run: %v", allocs)
	// Response parsing uses stack buffers for headers <= 4KB and pooled data buffers
	assert.LessOrEqual(t, allocs, 25.0)
}

func TestH3_ZeroAllocationFramePacking(t *testing.T) {
	var stackBuf [16]byte

	allocs := testing.AllocsPerRun(10000, func() {
		b := varint.Append(stackBuf[:0], coreh3.FrameTypeHeaders)
		_ = varint.Append(b, 2048)
	})

	assert.Equal(t, 0.0, allocs, "Frame header packing must have 0 allocations")
}

func BenchmarkSendRequest_HotPath(b *testing.B) {
	cc := &ClientConn{
		qpack: coreh3.NewQPACKCodec(),
	}

	req := machhttp.AcquireRequest()
	defer machhttp.ReleaseRequest(req)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://api.example.com/v1/resource")
	req.Header.Set("accept", "application/json")
	req.Header.Set("user-agent", "mach-client/1.0")

	var dw noopDiscardWriter

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = cc.sendRequestTo(dw, req, nil, 0)
	}
}

func BenchmarkReadResponse_HotPath(b *testing.B) {
	cc := &ClientConn{
		qpack: coreh3.NewQPACKCodec(),
	}

	headers := []qpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "application/json"},
		{Name: "server", Value: "mach"},
	}
	hBlock := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, headers, nil)

	var wire bytes.Buffer
	wire.Write(coreh3.AppendHeadersHeader(nil, uint64(len(hBlock))))
	wire.Write(hBlock)

	body := []byte(`{"status":"ok"}`)
	wire.Write(coreh3.AppendDataHeader(nil, uint64(len(body))))
	wire.Write(body)

	wireBytes := wire.Bytes()

	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseResponse(resp)

	reader := bytes.NewReader(wireBytes)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		reader.Reset(wireBytes)
		resp.Reset()
		_, _ = cc.readResponseFrom(reader, resp, 0)
	}
}
