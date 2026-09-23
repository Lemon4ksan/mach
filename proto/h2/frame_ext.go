// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

// Continuation extends a sequence of field block fragments across frame boundaries (RFC 9113 §6.10).
//
// A CONTINUATION frame MUST be preceded by a HEADERS, PUSH_PROMISE, or another CONTINUATION frame
// on the same stream without any intervening frames of any other type (RFC 9113 §6.10).
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: Managed by global [framePools] (sync.Pool). Call [Continuation.Reset] prior to reuse.
type Continuation struct {
	endHeaders bool
	rawHeaders []byte
}

// Type returns the FrameType identifier FrameContinuation (0x9, RFC 9113 §6.10).
func (c *Continuation) Type() FrameType { return FrameContinuation }

// Reset clears the END_HEADERS flag and raw header buffer for pool reuse.
func (c *Continuation) Reset() {
	c.endHeaders = false
	c.rawHeaders = c.rawHeaders[:0]
}

// Headers returns the extended field block fragment octets (RFC 9113 §6.10).
func (c *Continuation) Headers() []byte { return c.rawHeaders }

// SetEndHeaders sets the END_HEADERS flag bit (0x4), signaling the completion of the field block (RFC 9113 §6.10).
func (c *Continuation) SetEndHeaders(v bool) { c.endHeaders = v }

// EndHeaders reports whether the END_HEADERS flag bit (0x4) is enabled (RFC 9113 §6.10).
func (c *Continuation) EndHeaders() bool { return c.endHeaders }

// SetHeader replaces the field block fragment with the provided octet slice.
func (c *Continuation) SetHeader(b []byte) { c.rawHeaders = append(c.rawHeaders[:0], b...) }

// AppendHeader appends field block fragment octets to the internal buffer.
func (c *Continuation) AppendHeader(b []byte) { c.rawHeaders = append(c.rawHeaders, b...) }

// Write appends b to the internal header fragment, satisfying the [io.Writer] interface.
func (c *Continuation) Write(b []byte) (int, error) { c.AppendHeader(b); return len(b), nil }

// Deserialize decodes the CONTINUATION frame payload from fr, verifying non-zero stream binding (RFC 9113 §6.10).
func (c *Continuation) Deserialize(fr *FrameHeader) error {
	if fr.Stream() == 0 {
		return NewGoAwayError(ProtocolError, "CONTINUATION frame must be on a specific stream, not 0")
	}

	c.endHeaders = fr.Flags().Has(FlagEndHeaders)
	c.SetHeader(fr.payload)

	return nil
}

// Serialize encodes the CONTINUATION payload and END_HEADERS flag into the destination FrameHeader (RFC 9113 §6.10).
func (c *Continuation) Serialize(fr *FrameHeader) {
	if c.endHeaders {
		fr.SetFlags(fr.Flags().Add(FlagEndHeaders))
	}

	fr.setPayload(c.rawHeaders)
}

// PushPromise notifies the peer in advance of server-initiated streams (RFC 9113 §6.6 & §8.4).
//
// PUSH_PROMISE frames MUST be sent on a peer-initiated, open or half-closed stream.
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: Managed by global [framePools] (sync.Pool). Call [PushPromise.Reset] prior to reuse.
type PushPromise struct {
	pad    bool
	ended  bool
	stream uint32
	header []byte
}

// Type returns the FrameType identifier FramePushPromise (0x5, RFC 9113 §6.6).
func (pp *PushPromise) Type() FrameType { return FramePushPromise }

// PromisedStream returns the reserved 31-bit stream identifier promised by the server (RFC 9113 §6.6).
func (pp *PushPromise) PromisedStream() uint32 { return pp.stream }

// Headers returns the promised request field block fragment octets (RFC 9113 §6.6).
func (pp *PushPromise) Headers() []byte { return pp.header }

// Reset clears the promised stream ID, padding flag, and header buffer for pool reuse.
func (pp *PushPromise) Reset() {
	pp.pad = false
	pp.ended = false
	pp.stream = 0
	pp.header = pp.header[:0]
}

// SetHeader replaces the promised request header block fragment with the provided octet slice.
func (pp *PushPromise) SetHeader(h []byte) { pp.header = append(pp.header[:0], h...) }

// Write appends b to the promised header block fragment, satisfying the [io.Writer] interface.
func (pp *PushPromise) Write(b []byte) (int, error) {
	pp.header = append(pp.header, b...)
	return len(b), nil
}

// Deserialize decodes the promised stream ID, optional padding, and header fragment from fr (RFC 9113 §6.6).
func (pp *PushPromise) Deserialize(fr *FrameHeader) error {
	if fr.Stream() == 0 {
		return NewGoAwayError(ProtocolError, "PUSH_PROMISE frame must be on a specific stream, not 0")
	}

	payload := fr.payload

	if fr.Flags().Has(FlagPadded) {
		var err error

		payload, err = cutPadding(payload, fr.Len())
		if err != nil {
			return err
		}
	}

	if len(payload) < 4 {
		return NewGoAwayError(FrameSizeError, "invalid PUSH_PROMISE frame size (RFC 9113 §6.6)")
	}

	pp.stream = bytesToUint32(payload) & (1<<31 - 1)
	pp.header = append(pp.header[:0], payload[4:]...)
	pp.ended = fr.Flags().Has(FlagEndHeaders)

	return nil
}

// Serialize encodes the promised stream ID and header block fragment into the destination FrameHeader (RFC 9113 §6.6).
func (pp *PushPromise) Serialize(fr *FrameHeader) {
	fr.payload = appendUint32Bytes(fr.payload[:0], pp.stream)
	fr.payload = append(fr.payload, pp.header...)
}
