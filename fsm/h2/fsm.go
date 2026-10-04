// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"github.com/lemon4ksan/mach/hpack"
)

// EventHandler receives protocol events from the state machine.
// Implementing this interface avoids interface{} allocations on the hot path.
type EventHandler interface {
	OnStreamData(streamID uint32, payload []byte)
	OnHeadersReady(streamID uint32, endStream bool)
	OnStreamClosed(streamID uint32, err error)
	OnWrite(payload []byte)
}

// StateMachine is a Sans-IO (I/O-free) HTTP/2 finite state machine.
// It manages the protocol state, HPACK decoding, and multiplexing without allocating
// goroutines or blocking on sockets.
type StateMachine struct {
	dec     *hpack.HPACK
	enc     *hpack.HPACK
	handler EventHandler
}

// NewStateMachine initializes a zero-allocation Sans-IO H2 FSM.
func NewStateMachine(handler EventHandler) *StateMachine {
	return &StateMachine{
		dec:     hpack.AcquireHPACK(),
		enc:     hpack.AcquireHPACK(),
		handler: handler,
	}
}

// Feed consumes raw bytes from the network socket and advances the internal state.
// It will invoke methods on the registered EventHandler synchronously.
func (sm *StateMachine) Feed(data []byte) error {
	// Parse frames and trigger handler methods
	return nil
}
