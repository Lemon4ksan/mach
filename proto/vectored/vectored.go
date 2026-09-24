// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package vectored provides zero-copy buffer queues for scatter-gather network and disk I/O.
package vectored

import (
	"io"
	"iter"
	"net"
	"sync"
)

// DefaultMaxVectoredSlices is the maximum number of distinct buffers to hold
// before flushing, matching hyper's MAX_BUF_LIST_BUFFERS (16).
const DefaultMaxVectoredSlices = 16

// BufferQueue represents a zero-copy list of byte slices ready for vectored I/O.
//
// Ported from hyper's WriteBuf and WriteStrategy::Queue:
// It allows assembling multiple non-contiguous memory buffers (such as precomputed headers,
// payload chunks, and trailers) and writing them directly into the underlying socket
// via a single writev (POSIX) or WSASend (Windows) syscall without flattening/copying memory.
type BufferQueue struct {
	bufs       net.Buffers
	totalBytes int64
	maxSlices  int
}

// NewBufferQueue creates a BufferQueue with the default slice capacity limit (16).
func NewBufferQueue() *BufferQueue {
	return WithMaxSlices(DefaultMaxVectoredSlices)
}

// WithMaxSlices creates a BufferQueue with a custom maximum slice count.
func WithMaxSlices(maxSlices int) *BufferQueue {
	if maxSlices <= 0 {
		maxSlices = DefaultMaxVectoredSlices
	}

	return &BufferQueue{
		bufs:      make(net.Buffers, 0, maxSlices),
		maxSlices: maxSlices,
	}
}

// Push appends a byte slice to the vectored buffer queue.
//
// If the slice is empty, it is ignored with 0 overhead.
//
//go:inline
func (q *BufferQueue) Push(b []byte) {
	if len(b) == 0 {
		return
	}
	q.bufs = append(q.bufs, b)
	q.totalBytes += int64(len(b))
}

// SlicesCount returns the number of non-empty slices currently queued.
//
//go:inline
func (q *BufferQueue) SlicesCount() int {
	return len(q.bufs)
}

// TotalBytes returns the total payload size across all queued slices.
//
//go:inline
func (q *BufferQueue) TotalBytes() int64 {
	return q.totalBytes
}

// IsFull returns true if the queue has reached its maximum slice limit.
//
//go:inline
func (q *BufferQueue) IsFull() bool {
	return len(q.bufs) >= q.maxSlices
}

// WriteTo flushes all queued slices into the provided writer using vectored I/O.
//
// If w is a net.Conn, Go's runtime executes OS-level writev / WSASend.
func (q *BufferQueue) WriteTo(w io.Writer) (int64, error) {
	if len(q.bufs) == 0 {
		return 0, nil
	}

	n, err := q.bufs.WriteTo(w)
	q.Reset()

	return n, err
}

// Reset clears the queue for reuse without releasing underlying slice capacity.
func (q *BufferQueue) Reset() {
	q.bufs = q.bufs[:0]
	q.totalBytes = 0
}

// Buffers yields all queued byte slices in FIFO order without mutating the queue.
func (q *BufferQueue) Buffers() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for _, b := range q.bufs {
			if !yield(b) {
				return
			}
		}
	}
}

// All yields the index and byte slice for each queued buffer without mutating the queue.
func (q *BufferQueue) All() iter.Seq2[int, []byte] {
	return func(yield func(int, []byte) bool) {
		for i, b := range q.bufs {
			if !yield(i, b) {
				return
			}
		}
	}
}

var queuePool = sync.Pool{
	New: func() any {
		return NewBufferQueue()
	},
}

// AcquireBufferQueue retrieves a pooled BufferQueue from sync.Pool.
func AcquireBufferQueue() *BufferQueue {
	return queuePool.Get().(*BufferQueue)
}

// ReleaseBufferQueue returns a BufferQueue to sync.Pool after resetting.
func ReleaseBufferQueue(q *BufferQueue) {
	if q != nil {
		q.Reset()
		queuePool.Put(q)
	}
}
