// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

// Headers carries a field block fragment and optionally opens or terminates a stream (RFC 9113 §6.2).
//
// HEADERS frames can be sent on a stream in the "idle", "reserved (local)", "open", or "half-closed (remote)"
// state. The HEADERS frame can include priority information (deprecated per RFC 9113 §5.3.2) and padding.
//
// Concurrency: Not thread-safe; instances are pooled and must be owned by a single goroutine.
// Lifecycle: Instances are managed by [framePools] (sync.Pool). Call [Headers.Reset] prior to reuse.
type Headers struct {
	hasPadding bool
	stream     uint32
	weight     uint8
	endStream  bool
	endHeaders bool
	priority   bool
	exclusive  bool
	rawHeaders []byte
}

// Type returns the FrameType identifier FrameHeaders (0x1, RFC 9113 §6.2).
func (h *Headers) Type() FrameType { return FrameHeaders }

// Headers returns the raw HPACK-encoded field block fragment octets (RFC 9113 §6.2).
func (h *Headers) Headers() []byte { return h.rawHeaders }

// SetHeaders replaces the HPACK field block fragment with the provided octet slice.
func (h *Headers) SetHeaders(b []byte) { h.rawHeaders = append(h.rawHeaders[:0], b...) }

// AppendRawHeaders appends raw HPACK-encoded field block octets to the internal buffer.
func (h *Headers) AppendRawHeaders(b []byte) { h.rawHeaders = append(h.rawHeaders, b...) }

// EndStream reports whether the END_STREAM flag bit (0x1) is set on this frame (RFC 9113 §6.2).
func (h *Headers) EndStream() bool { return h.endStream }

// SetEndStream sets the END_STREAM flag bit (0x1), indicating this frame concludes the stream (RFC 9113 §6.2).
func (h *Headers) SetEndStream(v bool) { h.endStream = v }

// EndHeaders reports whether the END_HEADERS flag bit (0x4) is set, indicating a complete field section (RFC 9113 §6.2).
func (h *Headers) EndHeaders() bool { return h.endHeaders }

// SetEndHeaders sets the END_HEADERS flag bit (0x4), signaling no subsequent CONTINUATION frames follow (RFC 9113 §6.2).
func (h *Headers) SetEndHeaders(v bool) { h.endHeaders = v }

// Stream returns the 31-bit stream dependency identifier when priority signaling is enabled (RFC 9113 §6.2 & §5.3.1).
func (h *Headers) Stream() uint32 { return h.stream }

// SetStream sets the 31-bit stream dependency identifier for priority signaling (RFC 9113 §6.2 & §5.3.1).
func (h *Headers) SetStream(stream uint32) { h.stream = stream }

// Weight returns the priority weight octet representing value weight+1 in range [1, 256] (RFC 9113 §6.2 & §5.3.2).
func (h *Headers) Weight() byte { return h.weight }

// SetWeight sets the priority weight octet (RFC 9113 §6.2 & §5.3.2).
func (h *Headers) SetWeight(w byte) { h.weight = w }

// Exclusive reports whether the exclusive dependency bit is enabled in the priority specification (RFC 9113 §6.2 & §5.3.1).
func (h *Headers) Exclusive() bool { return h.exclusive }

// SetExclusive sets or clears the exclusive dependency bit in the priority specification (RFC 9113 §6.2 & §5.3.1).
func (h *Headers) SetExclusive(v bool) { h.exclusive = v }

// Padding reports whether the PADDED flag bit (0x8) is set on this frame (RFC 9113 §6.2).
func (h *Headers) Padding() bool { return h.hasPadding }

// SetPadding configures whether the PADDED flag bit (0x8) is emitted during serialization (RFC 9113 §6.2).
func (h *Headers) SetPadding(v bool) { h.hasPadding = v }

// Reset clears all header state, priority flags, and payload buffers for pool reuse.
func (h *Headers) Reset() {
	h.hasPadding = false
	h.stream = 0
	h.weight = 0
	h.endStream = false
	h.endHeaders = false
	h.priority = false
	h.exclusive = false
	h.rawHeaders = h.rawHeaders[:0]
}

// Deserialize decodes the HEADERS frame payload from frh, parsing padding and optional priority fields (RFC 9113 §6.2).
func (h *Headers) Deserialize(frh *FrameHeader) error {
	if frh.Stream() == 0 {
		return NewGoAwayError(ProtocolError, "HEADERS frame must be on a specific stream, not 0")
	}

	flags := frh.Flags()
	payload := frh.payload

	if flags.Has(FlagPadded) {
		var err error

		payload, err = cutPadding(payload, len(payload))
		if err != nil {
			return err
		}
	}

	if flags.Has(FlagPriority) {
		if len(payload) < 5 {
			return NewGoAwayError(FrameSizeError, "invalid HEADERS frame size for priority (RFC 9113 §6.2)")
		}

		h.priority = true
		h.exclusive = (payload[0] & 0x80) != 0

		h.stream = bytesToUint32(payload) & (1<<31 - 1)
		if h.stream == frh.Stream() {
			return NewGoAwayError(ProtocolError, "stream cannot depend on itself (RFC 9113 §5.3.1)")
		}

		h.weight = payload[4]
		payload = payload[5:]
	}

	h.endStream = flags.Has(FlagEndStream)
	h.endHeaders = flags.Has(FlagEndHeaders)
	h.rawHeaders = append(h.rawHeaders, payload...)

	return nil
}

// Serialize encodes the HEADERS frame payload, priority fields, and active flags into frh (RFC 9113 §6.2).
func (h *Headers) Serialize(frh *FrameHeader) {
	if h.endStream {
		frh.SetFlags(frh.Flags().Add(FlagEndStream))
	}

	if h.endHeaders {
		frh.SetFlags(frh.Flags().Add(FlagEndHeaders))
	}

	frh.payload = frh.payload[:0]

	if h.hasPadding {
		frh.SetFlags(frh.Flags().Add(FlagPadded))
		frh.payload = append(frh.payload, 0)
	}

	if h.priority {
		frh.SetFlags(frh.Flags().Add(FlagPriority))

		var priBuf [5]byte
		uint32ToBytes(priBuf[0:4], h.stream)

		if h.exclusive {
			priBuf[0] |= 0x80
		}

		priBuf[4] = h.weight
		frh.payload = append(frh.payload, priBuf[:]...)
	}

	frh.payload = append(frh.payload, h.rawHeaders...)

	if h.hasPadding {
		padLen := byte(0)
		if len(frh.payload) > 0 {
			frh.payload[0] = padLen
		}

		for i := byte(0); i < padLen; i++ {
			frh.payload = append(frh.payload, 0)
		}
	}
}
