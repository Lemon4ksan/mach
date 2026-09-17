// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"github.com/lemon4ksan/mach/quic/internal/protocol"
	"github.com/lemon4ksan/foundation/encoding/varint"
)

// A RetireConnectionIDFrame is a RETIRE_CONNECTION_ID frame
type RetireConnectionIDFrame struct {
	SequenceNumber uint64
}

func parseRetireConnectionIDFrame(b []byte, _ protocol.Version) (*RetireConnectionIDFrame, int, error) {
	seq, l, err := varint.Parse(b)
	if err != nil {
		return nil, 0, replaceUnexpectedEOF(err)
	}

	return &RetireConnectionIDFrame{SequenceNumber: seq}, l, nil
}

func (f *RetireConnectionIDFrame) Append(b []byte, _ protocol.Version) ([]byte, error) {
	b = append(b, byte(FrameTypeRetireConnectionID))
	b = varint.Append(b, f.SequenceNumber)
	return b, nil
}

// Length of a written frame
func (f *RetireConnectionIDFrame) Length(protocol.Version) protocol.ByteCount {
	return 1 + protocol.ByteCount(varint.Len(f.SequenceNumber))
}
