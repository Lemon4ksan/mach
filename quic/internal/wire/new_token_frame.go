// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"errors"
	"io"
	"slices"

	"github.com/lemon4ksan/foundation/encoding/varint"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
)

// A NewTokenFrame is a NEW_TOKEN frame
type NewTokenFrame struct {
	Token []byte
}

func parseNewTokenFrame(b []byte, _ protocol.Version) (*NewTokenFrame, int, error) {
	tokenLen, l, err := varint.Parse(b)
	if err != nil {
		return nil, 0, replaceUnexpectedEOF(err)
	}

	b = b[l:]

	if tokenLen == 0 {
		return nil, 0, errors.New("token must not be empty")
	}

	if uint64(len(b)) < tokenLen {
		return nil, 0, io.EOF
	}

	token := slices.Clone(b[:tokenLen])

	return &NewTokenFrame{Token: token}, l + int(tokenLen), nil
}

func (f *NewTokenFrame) Append(b []byte, _ protocol.Version) ([]byte, error) {
	b = append(b, byte(FrameTypeNewToken))
	b = varint.Append(b, uint64(len(f.Token)))
	b = append(b, f.Token...)

	return b, nil
}

// Length of a written frame
func (f *NewTokenFrame) Length(protocol.Version) protocol.ByteCount {
	return 1 + protocol.ByteCount(varint.Len(uint64(len(f.Token)))+len(f.Token))
}
