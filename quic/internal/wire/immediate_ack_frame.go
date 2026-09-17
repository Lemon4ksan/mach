// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"github.com/lemon4ksan/foundation/encoding/varint"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
)

// An ImmediateAckFrame is an IMMEDIATE_ACK frame
type ImmediateAckFrame struct{}

func (f *ImmediateAckFrame) Append(b []byte, _ protocol.Version) ([]byte, error) {
	return varint.Append(b, uint64(FrameTypeImmediateAck)), nil
}

// Length of a written frame
func (f *ImmediateAckFrame) Length(_ protocol.Version) protocol.ByteCount {
	return protocol.ByteCount(varint.Len(uint64(FrameTypeImmediateAck)))
}
