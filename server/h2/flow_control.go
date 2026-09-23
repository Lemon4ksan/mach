// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

func (sc *ServerConn) handleWindowUpdate(fr *coreh2.FrameHeader) error {
	defer coreh2.ReleaseFrameHeader(fr)

	wu := fr.Body().(*coreh2.WindowUpdate)
	inc := int32(wu.Increment()) //nolint:gosec

	// RFC 9113 §6.9: A receiver MUST treat the receipt of a WINDOW_UPDATE
	// frame with an increment of 0 as a stream error of type PROTOCOL_ERROR
	if inc <= 0 {
		return coreh2.ProtocolError
	}

	streamID := fr.Stream()
	if streamID == 0 {
		return sc.updateConnSendWindow(inc)
	}

	sc.streamsMu.RLock()
	st, ok := sc.streams[streamID]
	sc.streamsMu.RUnlock()

	if !ok {
		return nil
	}

	return sc.updateStreamSendWindow(st, inc)
}

func (sc *ServerConn) updateConnSendWindow(inc int32) error {
	for {
		old := sc.connSendWindow.Load()
		if int64(old)+int64(inc) > int64(1<<31-1) {
			// RFC 9113 §6.9.1: A sender MUST NOT allow a flow-control window to exceed 2^31-1
			return coreh2.FlowControlError
		}

		if sc.connSendWindow.CompareAndSwap(old, old+inc) {
			return nil
		}
	}
}

func (sc *ServerConn) updateStreamSendWindow(st *serverStream, inc int32) error {
	sc.streamsMu.Lock()
	defer sc.streamsMu.Unlock()

	if int64(st.sendWindow)+int64(inc) > int64(1<<31-1) {
		return coreh2.FlowControlError
	}

	st.sendWindow += inc

	return nil
}

func (sc *ServerConn) replenishReceiveWindow(streamID uint32, consumed int) {
	if consumed <= 0 || sc.Closed() {
		return
	}

	// Send WINDOW_UPDATE for stream and connection (RFC 9113 §6.9)
	_ = sc.sendWindowUpdateFrame(0, uint32(consumed))        //nolint:gosec // bounds checked
	_ = sc.sendWindowUpdateFrame(streamID, uint32(consumed)) //nolint:gosec // bounds checked
}
