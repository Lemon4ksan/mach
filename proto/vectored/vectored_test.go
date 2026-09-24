// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vectored_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/mach/proto/vectored"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

func TestBufferQueue_Basic(t *testing.T) {
	q := vectored.NewBufferQueue()
	assert.Equal(t, 0, q.SlicesCount())
	assert.Equal(t, int64(0), q.TotalBytes())

	q.Push([]byte("POST /v1/chat HTTP/1.1\r\n"))
	q.Push([]byte("Host: api.example.com\r\n\r\n"))
	q.Push([]byte(`{"message":"hello"}`))
	q.Push([]byte("")) // Should be ignored

	assert.Equal(t, 3, q.SlicesCount())
	assert.Equal(t, int64(68), q.TotalBytes())

	var buf bytes.Buffer
	n, err := q.WriteTo(&buf)
	require.NoError(t, err)
	assert.Equal(t, int64(68), n)
	assert.Equal(t, "POST /v1/chat HTTP/1.1\r\nHost: api.example.com\r\n\r\n{\"message\":\"hello\"}", buf.String())

	// Queue should be reset after WriteTo
	assert.Equal(t, 0, q.SlicesCount())
	assert.Equal(t, int64(0), q.TotalBytes())
}

func TestBufferQueue_Pool(t *testing.T) {
	q := vectored.AcquireBufferQueue()
	q.Push([]byte("test"))
	assert.Equal(t, 1, q.SlicesCount())

	vectored.ReleaseBufferQueue(q)

	q2 := vectored.AcquireBufferQueue()
	assert.Equal(t, 0, q2.SlicesCount())
	vectored.ReleaseBufferQueue(q2)
}

func TestBufferQueue_Iterators(t *testing.T) {
	q := vectored.NewBufferQueue()

	// Empty
	for range q.Buffers() {
		t.Fatal("expected empty Buffers iterator")
	}
	for range q.All() {
		t.Fatal("expected empty All iterator")
	}

	b1 := []byte("first")
	b2 := []byte("second")
	b3 := []byte("third")
	q.Push(b1)
	q.Push(b2)
	q.Push(b3)

	// Buffers iterator with early exit
	var buffers [][]byte
	for b := range q.Buffers() {
		buffers = append(buffers, b)
		if len(buffers) == 2 {
			break
		}
	}
	require.Equal(t, 2, len(buffers))
	require.Equal(t, b1, buffers[0])
	require.Equal(t, b2, buffers[1])

	// All iterator
	buffers = nil
	var indices []int
	for i, b := range q.All() {
		indices = append(indices, i)
		buffers = append(buffers, b)
	}
	require.Equal(t, []int{0, 1, 2}, indices)
	require.Equal(t, [][]byte{b1, b2, b3}, buffers)
}

func BenchmarkBufferQueue_PushAndWrite(b *testing.B) {
	header := []byte("GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")
	body := []byte("12345678901234567890")

	b.ReportAllocs()
	for b.Loop() {
		q := vectored.AcquireBufferQueue()
		q.Push(header)
		q.Push(body)

		var buf bytes.Buffer
		_, _ = q.WriteTo(&buf)

		vectored.ReleaseBufferQueue(q)
	}
}
