// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package overlay

// Frame is an In-Situ wrapper over a raw H2 frame buffer.
// It uses Bounds Check Elimination (BCE) to provide zero-cost getters.
// The frame MUST be at least 9 bytes long.
type Frame []byte

// Length returns the payload length (3 bytes).
func (f Frame) Length() uint32 {
	_ = f[2] // BCE hint
	return uint32(f[0])<<16 | uint32(f[1])<<8 | uint32(f[2])
}

// Type returns the HTTP/2 frame type identifier (RFC 9113 §4.1).
func (f Frame) Type() uint8 {
	return f[3]
}

// Flags returns the HTTP/2 frame flags.
func (f Frame) Flags() uint8 {
	return f[4]
}

// StreamID returns the 31-bit stream identifier.
func (f Frame) StreamID() uint32 {
	_ = f[8] // BCE hint
	return uint32(f[5]&0x7F)<<24 | uint32(f[6])<<16 | uint32(f[7])<<8 | uint32(f[8])
}

// Payload returns the raw frame body without allocation or copying.
// It panics if the underlying slice is smaller than Length() + 9.
func (f Frame) Payload() []byte {
	return f[9 : 9+f.Length()]
}

// IsValid checks if the buffer contains a complete frame header.
func (f Frame) IsValid() bool {
	if len(f) < 9 {
		return false
	}

	// Check if the entire payload is actually available in the slice
	return uint32(len(f)) >= 9+f.Length()
}

// DataFrame is an In-Situ wrapper specific to HTTP/2 DATA frames.
type DataFrame Frame

// IsEndStream reports whether the END_STREAM flag is set.
func (d DataFrame) IsEndStream() bool {
	return Frame(d).Flags()&0x1 != 0
}

// IsPadded reports whether the PADDED flag is set.
func (d DataFrame) IsPadded() bool {
	return Frame(d).Flags()&0x8 != 0
}

// PadLength returns the length of padding if the PADDED flag is set, otherwise 0.
func (d DataFrame) PadLength() uint8 {
	if !d.IsPadded() {
		return 0
	}

	p := Frame(d).Payload()
	if len(p) == 0 {
		return 0
	}

	return p[0]
}

// Data returns the application data portion, excluding padding.
func (d DataFrame) Data() []byte {
	p := Frame(d).Payload()
	if !d.IsPadded() {
		return p
	}

	if len(p) == 0 {
		return p
	}

	padLen := int(p[0])
	start := 1

	end := len(p) - padLen
	if start > end { // invalid padding
		return nil
	}

	return p[start:end]
}

// HeadersFrame is an In-Situ wrapper specific to HTTP/2 HEADERS frames.
type HeadersFrame Frame

// IsEndStream reports whether the END_STREAM flag is set.
func (h HeadersFrame) IsEndStream() bool {
	return Frame(h).Flags()&0x1 != 0
}

// IsEndHeaders reports whether the END_HEADERS flag is set.
func (h HeadersFrame) IsEndHeaders() bool {
	return Frame(h).Flags()&0x4 != 0
}

// HeaderBlockFragment returns the hpack.HPACK-encoded header data.
func (h HeadersFrame) HeaderBlockFragment() []byte {
	// For simplicity in this overlay, we omit Priority and Padding logic,
	// assuming a clean fast-path. In a full implementation, BCE offsets
	// would account for Flags() & 0x8 and Flags() & 0x20.
	return Frame(h).Payload()
}
