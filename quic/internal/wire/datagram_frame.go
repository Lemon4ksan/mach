// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"io"
	"slices"

	"github.com/lemon4ksan/foundation/encoding/varint"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
)

// MaxDatagramSize is the maximum size of a DATAGRAM frame (RFC 9221).
// By setting it to a large value, we allow all datagrams that fit into a QUIC packet.
// The value is chosen such that it can still be encoded as a 2 byte varint.
// This is a var and not a const so it can be set in tests.
var MaxDatagramSize protocol.ByteCount = 16383

// A DatagramFrame is a DATAGRAM frame
type DatagramFrame struct {
	Data           []byte
	DataLenPresent bool
}

func parseDatagramFrame(b []byte, typ FrameType, _ protocol.Version) (*DatagramFrame, int, error) {
	startLen := len(b)
	f := &DatagramFrame{}
	f.DataLenPresent = uint64(typ)&0x1 > 0

	var length uint64
	if f.DataLenPresent {
		var (
			err error
			l   int
		)

		length, l, err = varint.Parse(b)
		if err != nil {
			return nil, 0, replaceUnexpectedEOF(err)
		}

		b = b[l:]
		if length > uint64(len(b)) {
			return nil, 0, io.EOF
		}
	} else {
		length = uint64(len(b))
	}

	f.Data = slices.Clone(b[:length])

	return f, startLen - len(b) + int(length), nil
}

func (f *DatagramFrame) Append(b []byte, _ protocol.Version) ([]byte, error) {
	typ := uint8(0x30)
	if f.DataLenPresent {
		typ ^= 0b1
	}

	b = append(b, typ)
	if f.DataLenPresent {
		b = varint.Append(b, uint64(len(f.Data)))
	}

	b = append(b, f.Data...)

	return b, nil
}

// MaxDataLen returns the maximum data length
func (f *DatagramFrame) MaxDataLen(maxSize protocol.ByteCount, version protocol.Version) protocol.ByteCount {
	headerLen := protocol.ByteCount(1)
	if f.DataLenPresent {
		// pretend that the data size will be 1 bytes
		// if it turns out that varint encoding the length will consume 2 bytes, we need to adjust the data length afterwards
		headerLen++
	}

	if headerLen > maxSize {
		return 0
	}

	maxDataLen := maxSize - headerLen
	if f.DataLenPresent && varint.Len(uint64(maxDataLen)) != 1 {
		maxDataLen--
	}

	return maxDataLen
}

// Length of a written frame
func (f *DatagramFrame) Length(_ protocol.Version) protocol.ByteCount {
	length := 1 + protocol.ByteCount(len(f.Data))
	if f.DataLenPresent {
		length += protocol.ByteCount(varint.Len(uint64(len(f.Data))))
	}

	return length
}
