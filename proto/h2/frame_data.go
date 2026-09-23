// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

// Data conveys arbitrary, variable-length sequences of octets associated with a stream (RFC 9113 §6.1).
//
// DATA frames MUST be associated with a stream. If a DATA frame is received whose stream
// identifier field is 0x00, the recipient MUST respond with a connection error of type PROTOCOL_ERROR.
//
// Concurrency: Not thread-safe; instances are pooled and must be owned by a single goroutine.
// Lifecycle: Instances are managed by [framePools] (sync.Pool). Call [Data.Reset] prior to reuse.
type Data struct {
	endStream  bool
	hasPadding bool
	b          []byte
}

// Type returns the FrameType identifier FrameData (0x0, RFC 9113 §6.1).
func (d *Data) Type() FrameType { return FrameData }

// Reset clears payload buffers, padding flags, and stream termination state for pool reuse.
func (d *Data) Reset() { d.endStream = false; d.hasPadding = false; d.b = d.b[:0] }

// SetEndStream sets the END_STREAM flag bit (0x1), signaling this is the last frame sent on the stream (RFC 9113 §6.1).
func (d *Data) SetEndStream(v bool) { d.endStream = v }

// EndStream reports whether the END_STREAM flag bit (0x1) is set on this frame (RFC 9113 §6.1).
func (d *Data) EndStream() bool { return d.endStream }

// Data returns the raw unpadded payload octets carried by the DATA frame (RFC 9113 §6.1).
func (d *Data) Data() []byte { return d.b }

// SetData replaces the frame payload with the provided octet slice, reusing existing capacity when possible.
func (d *Data) SetData(b []byte) { d.b = append(d.b[:0], b...) }

// Padding reports whether the PADDED flag bit (0x8) is enabled on this frame (RFC 9113 §6.1).
func (d *Data) Padding() bool { return d.hasPadding }

// SetPadding configures whether the PADDED flag bit (0x8) and trailing zero octets are emitted on serialization (RFC 9113 §6.1).
func (d *Data) SetPadding(v bool) { d.hasPadding = v }

// Append appends payload octets to the existing buffer without reallocating if within slice capacity.
func (d *Data) Append(b []byte) { d.b = append(d.b, b...) }

// Len returns the current length in octets of the application data payload.
func (d *Data) Len() int { return len(d.b) }

// Write appends b to the internal payload buffer, satisfying the [io.Writer] interface.
func (d *Data) Write(b []byte) (int, error) { d.Append(b); return len(b), nil }

// Deserialize decodes the DATA frame payload from fr, validating stream binding and stripping padding (RFC 9113 §6.1).
func (d *Data) Deserialize(fr *FrameHeader) error {
	if fr.Stream() == 0 {
		return NewGoAwayError(ProtocolError, "DATA frame must be on a specific stream, not 0")
	}

	payload := fr.payload

	if fr.Flags().Has(FlagPadded) {
		var err error

		payload, err = cutPadding(payload, fr.Len())
		if err != nil {
			return err
		}
	}

	d.endStream = fr.Flags().Has(FlagEndStream)
	d.b = append(d.b[:0], payload...)

	return nil
}

// Serialize encodes the DATA frame payload and active flags into the destination FrameHeader (RFC 9113 §6.1).
func (d *Data) Serialize(fr *FrameHeader) {
	if d.endStream {
		fr.SetFlags(fr.Flags().Add(FlagEndStream))
	}

	fr.payload = fr.payload[:0]

	if d.hasPadding {
		fr.SetFlags(fr.Flags().Add(FlagPadded))

		padLen := byte(0)
		fr.payload = append(fr.payload, padLen)

		fr.payload = append(fr.payload, d.b...)
		for i := byte(0); i < padLen; i++ {
			fr.payload = append(fr.payload, 0)
		}
	} else {
		fr.payload = append(fr.payload, d.b...)
	}
}
