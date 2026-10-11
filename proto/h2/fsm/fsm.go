// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fsm

import (
	"github.com/lemon4ksan/mach/proto/h2/overlay"
)

// EventHandler receives protocol events from the state machine.
// Implementing this interface avoids interface{} allocations on the hot path.
type EventHandler interface {
	OnStreamData(streamID uint32, payload []byte, endStream bool)
	OnHeaders(streamID uint32, payload []byte, endHeaders, endStream bool)
	OnContinuation(streamID uint32, payload []byte, endHeaders bool)
	OnSettings(payload []byte, ack bool)
	OnPing(payload []byte, ack bool)
	OnWindowUpdate(streamID, increment uint32)
	OnStreamReset(streamID, errorCode uint32)
	OnGoAway(lastStreamID, errorCode uint32)
	OnWrite(payload []byte)
}

// StateMachine is a Sans-IO (I/O-free) HTTP/2 finite state machine.
// It manages the protocol state, HPACK decoding, and multiplexing without allocating
// goroutines or blocking on sockets.
type StateMachine struct {
	Handler EventHandler
}

// NewStateMachine initializes a zero-allocation Sans-IO H2 FSM.
func NewStateMachine(handler EventHandler) *StateMachine {
	return &StateMachine{
		Handler: handler,
	}
}

// Feed consumes raw bytes from the network socket and advances the internal state.
// It will invoke methods on the registered EventHandler synchronously.
func (sm *StateMachine) Feed(data []byte) (int, error) {
	consumed := 0

	for len(data) >= 9 {
		f := overlay.Frame(data)
		if !f.IsValid() {
			break // Need more data
		}

		payloadLen := int(f.Length())
		frameLen := 9 + payloadLen
		streamID := f.StreamID()
		flags := f.Flags()

		switch f.Type() {
		case 0x00: // DATA
			df := overlay.DataFrame(f)
			sm.Handler.OnStreamData(streamID, df.Data(), df.IsEndStream())
		case 0x01: // HEADERS
			hf := overlay.HeadersFrame(f)
			sm.Handler.OnHeaders(streamID, hf.HeaderBlockFragment(), hf.IsEndHeaders(), hf.IsEndStream())
		case 0x02: // PRIORITY
			// Ignore
		case 0x03: // RST_STREAM
			if payloadLen == 4 {
				errorCode := uint32(
					f.Payload()[0],
				)<<24 | uint32(
					f.Payload()[1],
				)<<16 | uint32(
					f.Payload()[2],
				)<<8 | uint32(
					f.Payload()[3],
				)
				sm.Handler.OnStreamReset(streamID, errorCode)
			}

		case 0x04: // SETTINGS
			ack := flags&0x1 != 0
			sm.Handler.OnSettings(f.Payload(), ack)
		case 0x05: // PUSH_PROMISE
			// Ignore
		case 0x06: // PING
			ack := flags&0x1 != 0
			sm.Handler.OnPing(f.Payload(), ack)
		case 0x07: // GOAWAY
			if payloadLen >= 8 {
				lastStreamID := uint32(
					f.Payload()[0]&0x7F,
				)<<24 | uint32(
					f.Payload()[1],
				)<<16 | uint32(
					f.Payload()[2],
				)<<8 | uint32(
					f.Payload()[3],
				)
				errorCode := uint32(
					f.Payload()[4],
				)<<24 | uint32(
					f.Payload()[5],
				)<<16 | uint32(
					f.Payload()[6],
				)<<8 | uint32(
					f.Payload()[7],
				)
				sm.Handler.OnGoAway(lastStreamID, errorCode)
			}

		case 0x08: // WINDOW_UPDATE
			if payloadLen == 4 {
				increment := uint32(
					f.Payload()[0]&0x7F,
				)<<24 | uint32(
					f.Payload()[1],
				)<<16 | uint32(
					f.Payload()[2],
				)<<8 | uint32(
					f.Payload()[3],
				)
				sm.Handler.OnWindowUpdate(streamID, increment)
			}

		case 0x09: // CONTINUATION
			endHeaders := flags&0x4 != 0
			sm.Handler.OnContinuation(streamID, f.Payload(), endHeaders)
		}

		data = data[frameLen:]
		consumed += frameLen
	}

	return consumed, nil
}
