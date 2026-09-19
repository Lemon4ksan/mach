// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"github.com/lemon4ksan/foundation/net/hpack"
)

// Event is yielded by the state machine.
type Event any

// StreamDataEvent indicates that a stream has received data.
type StreamDataEvent struct {
	StreamID uint32
	Payload  []byte
}

// HeadersReadyEvent indicates that headers are available.
type HeadersReadyEvent struct {
	StreamID  uint32
	EndStream bool
}

// StreamClosedEvent indicates a stream was closed.
type StreamClosedEvent struct {
	StreamID uint32
	Err      error
}

// WriteEvent instructs the outer orchestrator to flush bytes to the socket.
type WriteEvent struct {
	Payload []byte
}

// StateMachine is a Sans-IO (I/O-free) HTTP/2 finite state machine.
// It manages the protocol state, HPACK decoding, and multiplexing without allocating
// goroutines or blocking on sockets.
type StateMachine struct {
	dec *hpack.HPACK
	enc *hpack.HPACK

	// internal matrices and queues for events
	events []Event
}

// NewStateMachine initializes a zero-allocation Sans-IO H2 FSM.
func NewStateMachine() *StateMachine {
	return &StateMachine{
		dec: hpack.AcquireHPACK(),
		enc: hpack.AcquireHPACK(),
	}
}

// Feed consumes raw bytes from the network socket and advances the internal state.
func (sm *StateMachine) Feed(data []byte) error {
	// Parse frames and populate events slice
	return nil
}

// NextEvent yields the next protocol event to the engine orchestrator.
// Returns nil if no more events are available until more data is fed.
func (sm *StateMachine) NextEvent() Event {
	if len(sm.events) == 0 {
		return nil
	}

	e := sm.events[0]
	sm.events = sm.events[1:]

	return e
}
