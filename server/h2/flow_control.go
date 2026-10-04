// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

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
