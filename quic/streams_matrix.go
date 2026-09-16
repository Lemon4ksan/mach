package quic

import (
	"sync/atomic"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
)

const streamChunkSize = 1024

type streamChunk[T incomingStream] struct {
	entries [streamChunkSize]atomic.Pointer[incomingStreamEntry[T]]
}

// streamsMatrix is a lock-free 2D array (matrix) for monotonic QUIC stream IDs.
// It replaces map[protocol.StreamID] to eliminate RWMutex contention and hashing overhead.
type streamsMatrix[T incomingStream] struct {
	chunks atomic.Pointer[[]*streamChunk[T]]
	count  int
}

func newStreamsMatrix[T incomingStream]() *streamsMatrix[T] {
	m := &streamsMatrix[T]{}
	initial := make([]*streamChunk[T], 0)
	m.chunks.Store(&initial)
	return m
}

func (m *streamsMatrix[T]) get(id protocol.StreamID) (incomingStreamEntry[T], bool) {
	idx := id.StreamNum() - 1
	chunkIdx := idx / streamChunkSize

	chunksPtr := m.chunks.Load()
	if chunksPtr == nil || chunkIdx >= protocol.StreamNum(len(*chunksPtr)) {
		return incomingStreamEntry[T]{}, false
	}

	chunk := (*chunksPtr)[chunkIdx]
	if chunk == nil {
		return incomingStreamEntry[T]{}, false
	}

	entryPtr := chunk.entries[idx%streamChunkSize].Load()
	if entryPtr == nil {
		return incomingStreamEntry[T]{}, false
	}

	return *entryPtr, true
}

func (m *streamsMatrix[T]) set(id protocol.StreamID, entry incomingStreamEntry[T]) {
	idx := id.StreamNum() - 1
	chunkIdx := idx / streamChunkSize

	chunksPtr := m.chunks.Load()
	var chunks []*streamChunk[T]
	if chunksPtr != nil {
		chunks = *chunksPtr
	}

	// Grow chunks if necessary (Copy-On-Write)
	if chunkIdx >= protocol.StreamNum(len(chunks)) {
		newLen := chunkIdx + 1
		newChunks := make([]*streamChunk[T], newLen)
		copy(newChunks, chunks)
		for i := protocol.StreamNum(len(chunks)); i < newLen; i++ {
			newChunks[i] = &streamChunk[T]{}
		}
		chunks = newChunks
		m.chunks.Store(&chunks)
	}

	chunk := chunks[chunkIdx]
	
	// If it was nil before, increment count
	if chunk.entries[idx%streamChunkSize].Load() == nil {
		m.count++
	}
	
	// We copy the entry to heap so we can point to it
	e := entry
	chunk.entries[idx%streamChunkSize].Store(&e)
}

func (m *streamsMatrix[T]) del(id protocol.StreamID) {
	idx := id.StreamNum() - 1
	chunkIdx := idx / streamChunkSize

	chunksPtr := m.chunks.Load()
	if chunksPtr == nil || chunkIdx >= protocol.StreamNum(len(*chunksPtr)) {
		return
	}

	chunk := (*chunksPtr)[chunkIdx]
	if chunk == nil {
		return
	}

	if chunk.entries[idx%streamChunkSize].Load() != nil {
		m.count--
		chunk.entries[idx%streamChunkSize].Store(nil)
	}
}

func (m *streamsMatrix[T]) len() int {
	return m.count
}

// iterate is used during shutdown
func (m *streamsMatrix[T]) iterate(f func(incomingStreamEntry[T])) {
	chunksPtr := m.chunks.Load()
	if chunksPtr == nil {
		return
	}
	for _, chunk := range *chunksPtr {
		if chunk == nil {
			continue
		}
		for i := 0; i < streamChunkSize; i++ {
			entry := chunk.entries[i].Load()
			if entry != nil {
				f(*entry)
			}
		}
	}
}
