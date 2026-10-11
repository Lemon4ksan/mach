// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/mach/net/quic/internal/protocol"
)

// A MaxStreamDataFrame is a MAX_STREAM_DATA frame (RFC 9000 §19.10).
type MaxStreamDataFrame struct {
	StreamID          protocol.StreamID
	MaximumStreamData protocol.ByteCount
}

// parseMaxStreamDataFrame parses a MAX_STREAM_DATA frame (RFC 9000 §19.10).
func parseMaxStreamDataFrame(b []byte, _ protocol.Version) (*MaxStreamDataFrame, int, error) {
	startLen := len(b)

	sid, l, err := varint.Parse(b)
	if err != nil {
		return nil, 0, replaceUnexpectedEOF(err)
	}

	b = b[l:]

	offset, l, err := varint.Parse(b)
	if err != nil {
		return nil, 0, replaceUnexpectedEOF(err)
	}

	b = b[l:]

	return &MaxStreamDataFrame{
		StreamID:          protocol.StreamID(sid),
		MaximumStreamData: protocol.ByteCount(offset),
	}, startLen - len(b), nil
}

func (f *MaxStreamDataFrame) Append(b []byte, _ protocol.Version) ([]byte, error) {
	b = append(b, byte(FrameTypeMaxStreamData))
	b = varint.Append(b, uint64(f.StreamID))
	b = varint.Append(b, uint64(f.MaximumStreamData))

	return b, nil
}

// Length of a written frame
func (f *MaxStreamDataFrame) Length(protocol.Version) protocol.ByteCount {
	return 1 + protocol.ByteCount(varint.Len(uint64(f.StreamID))+varint.Len(uint64(f.MaximumStreamData)))
}
