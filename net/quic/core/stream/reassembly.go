// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/lemon4ksan/mach/net/quic/varint"
)

// ErrDuplicateData is returned when incoming stream data is entirely redundant.
var ErrDuplicateData = errors.New("duplicate stream data")

const (
	smallChunkSize = 2048
	largeChunkSize = 16384
)

var (
	smallChunkPool = sync.Pool{
		New: func() any {
			b := make([]byte, smallChunkSize)
			return &b
		},
	}
	largeChunkPool = sync.Pool{
		New: func() any {
			b := make([]byte, largeChunkSize)
			return &b
		},
	}
)

func allocChunkData(size int) ([]byte, *[]byte, bool) {
	if size <= smallChunkSize {
		ptr := smallChunkPool.Get().(*[]byte)
		return (*ptr)[:size], ptr, false
	}
	if size <= largeChunkSize {
		ptr := largeChunkPool.Get().(*[]byte)
		return (*ptr)[:size], ptr, true
	}
	return make([]byte, size), nil, false
}

func freeChunkData(ptr *[]byte, isLarge bool) {
	if ptr == nil {
		return
	}
	if isLarge {
		largeChunkPool.Put(ptr)
	} else {
		smallChunkPool.Put(ptr)
	}
}

// reassemblyChunk represents a buffered contiguous range of stream bytes.
type reassemblyChunk struct {
	offset  uint64
	data    []byte
	pooled  *[]byte
	isLarge bool
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

	initialChunks [8]reassemblyChunk
	chunks        []reassemblyChunk // sorted by chunk.offset with disjoint non-adjacent intervals
}

// NewReassemblyBuffer initializes an empty reassembly buffer starting at byte offset 0.
func NewReassemblyBuffer() *ReassemblyBuffer {
	b := &ReassemblyBuffer{}
	b.chunks = b.initialChunks[:0]
	return b
}

// Push inserts stream data at the given offset. It handles out-of-order chunks,
// trims overlaps, drops duplicates, and validates final size consistency (RFC 9000 §4.5).
//
// If the frame violates final size rules, Push returns a TransportError with FinalSizeError.
func (b *ReassemblyBuffer) Push(offset uint64, data []byte, fin bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// RFC 9000 §4.5: Validate 62-bit varint limit and integer overflow
	if offset > varint.Max || uint64(len(data)) > varint.Max {
		return &TransportError{
			ErrorCode: FrameEncodingError,
			ErrorMessage: fmt.Sprintf(
				"stream offset %d or length %d exceeds max varint",
				offset, len(data),
			),
		}
	}

	end := offset + uint64(len(data))
	if end < offset || end > varint.Max {
		return &TransportError{
			ErrorCode: FrameEncodingError,
			ErrorMessage: fmt.Sprintf(
				"stream frame [offset %d, len %d] exceeds max representable offset (2^62 - 1)",
				offset, len(data),
			),
		}
	}

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

	return b.insertChunk(offset, data)
}

// insertChunk incorporates [offset, offset+len(data)] into the sorted disjoint chunks list.
// Overlapping or touching chunks are merged into a single contiguous chunk.
// RFC 9000 §2.2: If divergent data is detected in overlapping segments, ProtocolViolation is returned.
func (b *ReassemblyBuffer) insertChunk(offset uint64, data []byte) error {
	end := offset + uint64(len(data))
	n := len(b.chunks)

	// Fast path: append to empty or after last chunk without overlap.
	if n == 0 {
		buf, pooled, isLarge := allocChunkData(len(data))
		copy(buf, data)
		b.chunks = append(b.chunks, reassemblyChunk{
			offset:  offset,
			data:    buf,
			pooled:  pooled,
			isLarge: isLarge,
		})

		return nil
	}

	last := &b.chunks[n-1]
	if offset == last.end() {
		// Exactly contiguous with last chunk: extend in-place if pooled buffer has space
		if last.pooled != nil && cap(*last.pooled)-len(last.data) >= len(data) {
			last.data = append(last.data, data...)
			return nil
		}
		buf, pooled, isLarge := allocChunkData(len(data))
		copy(buf, data)
		b.chunks = append(b.chunks, reassemblyChunk{
			offset:  offset,
			data:    buf,
			pooled:  pooled,
			isLarge: isLarge,
		})
		return nil
	}

	if offset > last.end() {
		// Disjoint gap after last chunk.
		buf, pooled, isLarge := allocChunkData(len(data))
		copy(buf, data)
		b.chunks = append(b.chunks, reassemblyChunk{
			offset:  offset,
			data:    buf,
			pooled:  pooled,
			isLarge: isLarge,
		})

		return nil
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
		buf, pooled, isLarge := allocChunkData(len(data))
		copy(buf, data)
		b.chunks = append(b.chunks, reassemblyChunk{})
		copy(b.chunks[insertIdx+1:], b.chunks[insertIdx:])
		b.chunks[insertIdx] = reassemblyChunk{
			offset:  offset,
			data:    buf,
			pooled:  pooled,
			isLarge: isLarge,
		}

		return nil
	}

	// RFC 9000 §2.2: If an endpoint detects that a peer has sent different data at the
	// same offset on the same stream, it MUST close the connection with PROTOCOL_VIOLATION.
	for i := firstOverlap; i <= lastOverlap; i++ {
		c := &b.chunks[i]
		overlapStart := max(offset, c.offset)

		overlapEnd := min(end, c.end())
		if overlapEnd > overlapStart {
			existingPart := c.data[overlapStart-c.offset : overlapEnd-c.offset]

			newPart := data[overlapStart-offset : overlapEnd-offset]
			if !bytes.Equal(existingPart, newPart) {
				return &TransportError{
					ErrorCode: ProtocolViolation,
					ErrorMessage: fmt.Sprintf(
						"stream frame divergent data in overlapping range [%d, %d)",
						overlapStart, overlapEnd,
					),
				}
			}
		}
	}

	// Check if new data is completely contained within an existing single chunk.
	if firstOverlap == lastOverlap {
		c := &b.chunks[firstOverlap]
		if c.offset <= offset && c.end() >= end {
			// Redundant duplicate: already fully contained and verified identical.
			return nil
		}
	}

	// Merge with all overlapping chunks from firstOverlap to lastOverlap.
	mergedOffset := min(offset, b.chunks[firstOverlap].offset)
	mergedEnd := max(end, b.chunks[lastOverlap].end())
	mergedLen := int(mergedEnd - mergedOffset)
	mergedData, mergedPooled, mergedIsLarge := allocChunkData(mergedLen)

	// Copy existing overlapping chunks into merged buffer.
	for i := firstOverlap; i <= lastOverlap; i++ {
		c := &b.chunks[i]
		startInMerged := c.offset - mergedOffset
		copy(mergedData[startInMerged:], c.data)
		freeChunkData(c.pooled, c.isLarge)
	}

	// Copy new data into merged buffer (overwriting duplicate bytes with identical data).
	copy(mergedData[offset-mergedOffset:], data)

	// Replace the overlapping range [firstOverlap..lastOverlap] with the single merged chunk.
	b.chunks[firstOverlap] = reassemblyChunk{
		offset:  mergedOffset,
		data:    mergedData,
		pooled:  mergedPooled,
		isLarge: mergedIsLarge,
	}

	numRemoved := lastOverlap - firstOverlap
	if numRemoved > 0 {
		newLen := len(b.chunks) - numRemoved
		copy(b.chunks[firstOverlap+1:], b.chunks[lastOverlap+1:])

		for k := newLen; k < len(b.chunks); k++ {
			b.chunks[k] = reassemblyChunk{}
		}

		b.chunks = b.chunks[:newLen]
	}

	return nil
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
		// Consumed entire first chunk; release pooled buffer
		freeChunkData(c.pooled, c.isLarge)
		b.chunks[0] = reassemblyChunk{}

		b.chunks = b.chunks[1:]
		if len(b.chunks) == 0 {
			b.chunks = b.initialChunks[:0]
		}
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

// Reset releases all buffered chunks back to the pool and resets read offsets.
func (b *ReassemblyBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i := range b.chunks {
		freeChunkData(b.chunks[i].pooled, b.chunks[i].isLarge)
		b.chunks[i] = reassemblyChunk{}
	}
	b.chunks = b.initialChunks[:0]
	b.readOffset = 0
	b.highestOffset = 0
	b.finalSize = 0
	b.hasFinalSize = false
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

// SetFinalSize explicitly records the final stream size (e.g. from RESET_STREAM).
func (b *ReassemblyBuffer) SetFinalSize(finalSize uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.finalSize = finalSize

	b.hasFinalSize = true
	if finalSize > b.highestOffset {
		b.highestOffset = finalSize
	}
}

// IsAllDataRead reports whether all stream data up to final size has been read.
func (b *ReassemblyBuffer) IsAllDataRead() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hasFinalSize && b.readOffset >= b.finalSize
}
