// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

const minWindowUpdateBatch = 32768

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
	for {
		old := st.sendWindow.Load()
		if int64(old)+int64(inc) > int64(1<<31-1) {
			return coreh2.FlowControlError
		}

		if st.sendWindow.CompareAndSwap(old, old+inc) {
			return nil
		}
	}
}

func (sc *ServerConn) replenishReceiveWindow(st *serverStream, consumed int) {
	if consumed <= 0 || sc.Closed() {
		return
	}

	var (
		connInc uint32
		stInc   uint32
	)

	// Batched connection-level window update accounting
	connTotal := sc.connRecvUnacked.Add(int32(consumed))
	if connTotal >= minWindowUpdateBatch {
		if sc.connRecvUnacked.CompareAndSwap(connTotal, 0) {
			connInc = uint32(connTotal)
		}
	}

	// Batched stream-level window update accounting
	if st != nil && !st.endStream {
		st.recvUnacked += int32(consumed)
		if st.recvUnacked >= minWindowUpdateBatch {
			stInc = uint32(st.recvUnacked)
			st.recvUnacked = 0
		}
	}

	if connInc > 0 || stInc > 0 {
		_ = sc.sendBatchedWindowUpdates(connInc, st, stInc)
	}
}
