// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"errors"
)

// Maximum possible frame header size in RFC 6455:
// 2 bytes base + 8 bytes extended payload length + 4 bytes mask key = 14 bytes.
const maxHeaderSize = 14

// FrameChunk represents a parsed chunk of an RFC 6455 WebSocket frame.
//
// In streaming environments, large frames or fragmented network packets may be
// split across multiple FrameChunks.
// Header is non-nil ONLY on the first chunk of a frame.
type FrameChunk struct {
	// Header contains the frame header metadata for the first chunk of a frame.
	// For subsequent chunks belonging to the same frame, Header is nil.
	Header *Header

	// Payload is the payload slice referencing the input buffer.
	// Note: unmasking should be applied using Header.MaskKey with the appropriate
	// frame offset if Header.Masked is true.
	Payload []byte

	// FinalChunk is true if this chunk contains the final payload bytes for the current frame.
	FinalChunk bool
}

// Parser is a zero-allocation streaming RFC 6455 frame parser.
//
// It handles partial frame headers, multi-byte extended lengths, fragmented TCP chunks,
// and streams containing multiple consecutive frames.
type Parser struct {
	headerBuf    [maxHeaderSize]byte
	headerBufLen int

	currentHeader Header
	hasHeader     bool
	remainingLen  int64

	err error

	// Preallocated header ring to ensure 0 heap allocations when decoding frames
	hdrRing    [64]Header
	hdrRingIdx int
}

// Reset resets the parser state for reuse on a new connection.
func (p *Parser) Reset() {
	p.headerBufLen = 0
	p.currentHeader = Header{}
	p.hasHeader = false
	p.remainingLen = 0
	p.err = nil
	p.hdrRingIdx = 0
}

// Err returns the first sticky protocol or parse error encountered, if any.
func (p *Parser) Err() error {
	return p.err
}

// Decode parses frames from data and appends parsed FrameChunks to chunks.
//
// Returns nil on success, or an error if a protocol violation occurs.
// Once an error is encountered, the parser enters an error state and subsequent
// calls will return the same error.
func (p *Parser) Decode(data []byte, chunks *[]FrameChunk) error {
	if p.err != nil {
		return p.err
	}

	if len(data) == 0 && !p.hasHeader && p.headerBufLen == 0 {
		return nil
	}

	firstChunk := false

	// Handle incomplete header buffer carried over from previous Decode calls.
	if p.headerBufLen > 0 {
		origSize := p.headerBufLen

		needed := maxHeaderSize - origSize
		if needed > len(data) {
			needed = len(data)
		}

		copy(p.headerBuf[origSize:], data[:needed])
		totalBuffered := origSize + needed

		h, hlen, err := ParseHeader(p.headerBuf[:totalBuffered])
		if errors.Is(err, ErrIncomplete) {
			p.headerBufLen = totalBuffered
			return nil
		}

		if err != nil {
			p.err = err
			return err
		}

		p.currentHeader = h
		p.hasHeader = true
		p.remainingLen = h.PayloadLen
		p.headerBufLen = 0

		consumedFromData := hlen - origSize
		data = data[consumedFromData:]
		firstChunk = true
	}

	for len(data) > 0 || firstChunk {
		if !p.hasHeader {
			h, hlen, err := ParseHeader(data)
			if errors.Is(err, ErrIncomplete) {
				copy(p.headerBuf[:len(data)], data)
				p.headerBufLen = len(data)
				return nil
			}

			if err != nil {
				p.err = err
				return err
			}

			p.currentHeader = h
			p.hasHeader = true
			p.remainingLen = h.PayloadLen
			data = data[hlen:]
			firstChunk = true
		}

		// Decode payload chunk.
		take := int64(len(data))
		if take > p.remainingLen {
			take = p.remainingLen
		}

		var payloadSlice []byte
		if take > 0 {
			payloadSlice = data[:take]
			data = data[take:]
		}

		p.remainingLen -= take

		finalChunk := (p.remainingLen == 0)
		if finalChunk {
			p.hasHeader = false
		}

		var hdrPtr *Header
		if firstChunk {
			slot := p.hdrRingIdx & 63
			p.hdrRing[slot] = p.currentHeader
			hdrPtr = &p.hdrRing[slot]
			p.hdrRingIdx++
		}

		*chunks = append(*chunks, FrameChunk{
			Header:     hdrPtr,
			Payload:    payloadSlice,
			FinalChunk: finalChunk,
		})

		firstChunk = false
	}

	return nil
}
