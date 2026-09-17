// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/mach/quic/internal/protocol"
)

var streamFramePool = generic.NewPool(func() *StreamFrame {
	return &StreamFrame{
		Data:     make([]byte, 0, protocol.MaxPacketBufferSize),
		fromPool: true,
	}
})

func GetStreamFrame() *StreamFrame {
	return streamFramePool.Get()
}

func putStreamFrame(f *StreamFrame) {
	if !f.fromPool {
		return
	}

	if protocol.ByteCount(cap(f.Data)) != protocol.MaxPacketBufferSize {
		panic("wire.PutStreamFrame called with packet of wrong size!")
	}

	streamFramePool.Put(f)
}
