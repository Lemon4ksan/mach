// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"errors"
	"fmt"
	"io"
	"sync"
)

// ErrDuplicateData is returned when incoming stream data is entirely redundant.
var ErrDuplicateData = errors.New("duplicate stream data")

// reassemblyChunk represents a buffered contiguous range of stream bytes.
type reassemblyChunk struct {
	offset uint64
	data   []byte
}

func (c *reassemblyChunk) end() uint64 {
	return c.offset + uint64(len(c.data))
}

// ReassemblyBuffer manages out-of-order QUIC stream frames, defragmenting sparse chunks,
// resolving overlaps, discarding duplicates, and assembling a contiguous byte stream (RFC 9000 §2.2).
//
// Thread-safety: ReassemblyBuffer uses internal mutex locking and is safe for concurrent access.
type ReassemblyBuffer struct {
	mu sync.Mutex

	readOffset    uint64 // next byte offset to be consumed by the application
	highestOffset uint64 // highest byte offset seen in any received frame
	finalSize     uint64 // final stream size when FIN is received
	hasFinalSize  bool   // true once FIN or RESET_STREAM final size is recorded

	chunks []reassemblyChunk // sorted by chunk.offset with disjoint non-adjacent intervals
}

// NewReassemblyBuffer initializes an empty reassembly buffer starting at byte offset 0.
func NewReassemblyBuffer() *ReassemblyBuffer {
	return &ReassemblyBuffer{
		chunks: make([]reassemblyChunk, 0, 8),
	}
}

// Push inserts stream data at the given offset. It handles out-of-order chunks,
// trims overlaps, drops duplicates, and validates final size consistency (RFC 9000 §4.5).
//
// If the frame violates final size rules, Push returns a TransportError with FinalSizeError.
func (b *ReassemblyBuffer) Push(offset uint64, data []byte, fin bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	end := offset + uint64(len(data))

	// RFC 9000 §4.5: Final Size Error validation
	if b.hasFinalSize {
		if fin && b.finalSize != end {
			return &TransportError{
				ErrorCode: FinalSizeError,
				ErrorMessage: fmt.Sprintf(
					"inconsistent final size: previously %d, received %d",
					b.finalSize, end,
				),
			}
		}
		if end > b.finalSize {
			return &TransportError{
				ErrorCode: FinalSizeError,
				ErrorMessage: fmt.Sprintf(
					"stream frame exceeds final size: offset %d + len %d > %d",
					offset, len(data), b.finalSize,
				),
			}
		}
	}

	if fin {
		if !b.hasFinalSize {
			if end < b.highestOffset {
				return &TransportError{
					ErrorCode: FinalSizeError,
					ErrorMessage: fmt.Sprintf(
						"final size %d is less than highest received offset %d",
						end, b.highestOffset,
					),
				}
			}
			b.finalSize = end
			b.hasFinalSize = true
		}
	}

	if end > b.highestOffset {
		b.highestOffset = end
	}

	// If all incoming data has already been read by the application, drop it.
	if end <= b.readOffset {
		return nil
	}

	// Trim previously read prefix.
	if offset < b.readOffset {
		trim := b.readOffset - offset
		data = data[trim:]
		offset = b.readOffset
	}

	if len(data) == 0 {
		return nil
	}

	b.insertChunk(offset, data)
	return nil
}

// insertChunk incorporates [offset, offset+len(data)] into the sorted disjoint chunks list.
// Overlapping or touching chunks are merged into a single contiguous chunk.
func (b *ReassemblyBuffer) insertChunk(offset uint64, data []byte) {
	end := offset + uint64(len(data))
	n := len(b.chunks)

	// Fast path: append to empty or after last chunk without overlap.
	if n == 0 {
		b.chunks = append(b.chunks, reassemblyChunk{
			offset: offset,
			data:   append([]byte(nil), data...),
		})
		return
	}

	last := &b.chunks[n-1]
	if offset == last.end() {
		// Exactly contiguous with last chunk: extend it.
		last.data = append(last.data, data...)
		return
	}
	if offset > last.end() {
		// Disjoint gap after last chunk.
		b.chunks = append(b.chunks, reassemblyChunk{
			offset: offset,
			data:   append([]byte(nil), data...),
		})
		return
	}

	// Find the range of chunks that touch or overlap with [offset, end].
	// Chunk touches or overlaps if chunk.offset <= end and chunk.end() >= offset.
	firstOverlap := -1
	lastOverlap := -1
	insertIdx := 0

	for i := 0; i < n; i++ {
		c := &b.chunks[i]
		if c.offset <= end && c.end() >= offset {
			if firstOverlap == -1 {
				firstOverlap = i
			}
			lastOverlap = i
		} else if c.offset > end && firstOverlap == -1 {
			insertIdx = i
			break
		}
		if i == n-1 && firstOverlap == -1 {
			insertIdx = n
		}
	}

	if firstOverlap == -1 {
		// No overlap: insert new chunk at insertIdx.
		b.chunks = append(b.chunks, reassemblyChunk{})
		copy(b.chunks[insertIdx+1:], b.chunks[insertIdx:])
		b.chunks[insertIdx] = reassemblyChunk{
			offset: offset,
			data:   append([]byte(nil), data...),
		}
		return
	}

	// Check if new data is completely contained within an existing single chunk.
	if firstOverlap == lastOverlap {
		c := &b.chunks[firstOverlap]
		if c.offset <= offset && c.end() >= end {
			// Redundant duplicate: already fully contained.
			return
		}
	}

	// Merge with all overlapping chunks from firstOverlap to lastOverlap.
	mergedOffset := min(offset, b.chunks[firstOverlap].offset)
	mergedEnd := max(end, b.chunks[lastOverlap].end())
	mergedData := make([]byte, mergedEnd-mergedOffset)

	// Copy existing overlapping chunks into merged buffer.
	for i := firstOverlap; i <= lastOverlap; i++ {
		c := &b.chunks[i]
		startInMerged := c.offset - mergedOffset
		copy(mergedData[startInMerged:], c.data)
	}

	// Copy new data into merged buffer (overwriting duplicate bytes with identical data).
	copy(mergedData[offset-mergedOffset:], data)

	// Replace the overlapping range [firstOverlap..lastOverlap] with the single merged chunk.
	b.chunks[firstOverlap] = reassemblyChunk{
		offset: mergedOffset,
		data:   mergedData,
	}

	numRemoved := lastOverlap - firstOverlap
	if numRemoved > 0 {
		b.chunks = append(b.chunks[:firstOverlap+1], b.chunks[lastOverlap+1:]...)
	}
}

// Read copies contiguous buffered data into p, advancing the read offset.
// If no contiguous data is currently available:
// - Returns io.EOF if the stream has received FIN and read up to the final size.
// - Returns 0, nil if more data is expected.
func (b *ReassemblyBuffer) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(p) == 0 {
		return 0, nil
	}

	if len(b.chunks) == 0 || b.chunks[0].offset > b.readOffset {
		if b.hasFinalSize && b.readOffset >= b.finalSize {
			return 0, io.EOF
		}
		return 0, nil
	}

	c := &b.chunks[0]
	// c.offset must equal b.readOffset due to prefix trimming in insertChunk/push.
	available := len(c.data)
	toCopy := min(len(p), available)
	copy(p, c.data[:toCopy])

	b.readOffset += uint64(toCopy)

	if toCopy == available {
		// Consumed entire first chunk.
		b.chunks = b.chunks[1:]
	} else {
		// Partially consumed first chunk.
		c.offset += uint64(toCopy)
		c.data = c.data[toCopy:]
	}

	if b.hasFinalSize && b.readOffset >= b.finalSize {
		return toCopy, io.EOF
	}

	return toCopy, nil
}

// Peek inspects contiguous buffered data starting at readOffset into p without advancing readOffset.
// Returns the number of bytes available up to len(p).
func (b *ReassemblyBuffer) Peek(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(p) == 0 {
		return 0, nil
	}

	if len(b.chunks) == 0 || b.chunks[0].offset > b.readOffset {
		if b.hasFinalSize && b.readOffset >= b.finalSize {
			return 0, io.EOF
		}
		return 0, nil
	}

	c := &b.chunks[0]
	toCopy := min(len(p), len(c.data))
	copy(p, c.data[:toCopy])

	if b.hasFinalSize && (b.readOffset+uint64(toCopy)) >= b.finalSize {
		return toCopy, io.EOF
	}

	return toCopy, nil
}

// AvailableContiguous returns the number of contiguous bytes ready to read starting at readOffset.
func (b *ReassemblyBuffer) AvailableContiguous() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.chunks) == 0 || b.chunks[0].offset > b.readOffset {
		return 0
	}
	return len(b.chunks[0].data)
}

// ReadOffset returns the current byte offset read by the application.
func (b *ReassemblyBuffer) ReadOffset() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.readOffset
}

// HighestOffset returns the highest byte offset seen in any received frame.
func (b *ReassemblyBuffer) HighestOffset() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.highestOffset
}

// FinalSize returns the final stream size and whether it is known.
func (b *ReassemblyBuffer) FinalSize() (uint64, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.finalSize, b.hasFinalSize
}

// IsAllDataRead reports whether all stream data up to final size has been read.
func (b *ReassemblyBuffer) IsAllDataRead() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hasFinalSize && b.readOffset >= b.finalSize
}
