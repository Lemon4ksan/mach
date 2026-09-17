// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"math"
	"time"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
	"github.com/lemon4ksan/foundation/encoding/varint"
)

type AckFrequencyFrame struct {
	SequenceNumber        uint64
	AckElicitingThreshold uint64
	RequestMaxAckDelay    time.Duration
	ReorderingThreshold   protocol.PacketNumber
}

func parseAckFrequencyFrame(b []byte, _ protocol.Version) (*AckFrequencyFrame, int, error) {
	startLen := len(b)

	seq, l, err := varint.Parse(b)
	if err != nil {
		return nil, 0, replaceUnexpectedEOF(err)
	}

	b = b[l:]

	aeth, l, err := varint.Parse(b)
	if err != nil {
		return nil, 0, replaceUnexpectedEOF(err)
	}

	b = b[l:]

	mad, l, err := varint.Parse(b)
	if err != nil {
		return nil, 0, replaceUnexpectedEOF(err)
	}

	// prevents overflows if the peer sends a very large value
	maxAckDelay := time.Duration(mad) * time.Microsecond
	if maxAckDelay < 0 {
		maxAckDelay = math.MaxInt64
	}

	b = b[l:]

	rth, l, err := varint.Parse(b)
	if err != nil {
		return nil, 0, replaceUnexpectedEOF(err)
	}

	b = b[l:]

	return &AckFrequencyFrame{
		SequenceNumber:        seq,
		AckElicitingThreshold: aeth,
		RequestMaxAckDelay:    maxAckDelay,
		ReorderingThreshold:   protocol.PacketNumber(rth),
	}, startLen - len(b), nil
}

func (f *AckFrequencyFrame) Append(b []byte, _ protocol.Version) ([]byte, error) {
	b = varint.Append(b, uint64(FrameTypeAckFrequency))
	b = varint.Append(b, f.SequenceNumber)
	b = varint.Append(b, f.AckElicitingThreshold)
	b = varint.Append(b, uint64(f.RequestMaxAckDelay/time.Microsecond))

	return varint.Append(b, uint64(f.ReorderingThreshold)), nil
}

func (f *AckFrequencyFrame) Length(_ protocol.Version) protocol.ByteCount {
	return protocol.ByteCount(2 + varint.Len(f.SequenceNumber) + varint.Len(f.AckElicitingThreshold) +
		varint.Len(uint64(f.RequestMaxAckDelay/time.Microsecond)) + varint.Len(uint64(f.ReorderingThreshold)))
}
