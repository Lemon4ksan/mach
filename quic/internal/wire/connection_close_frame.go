// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"io"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
	"github.com/lemon4ksan/foundation/encoding/varint"
)

// A ConnectionCloseFrame is a CONNECTION_CLOSE frame
type ConnectionCloseFrame struct {
	ReasonPhrase       string
	ErrorCode          uint64
	FrameType          uint64
	IsApplicationError bool
}

func parseConnectionCloseFrame(b []byte, typ FrameType, _ protocol.Version) (*ConnectionCloseFrame, int, error) {
	startLen := len(b)
	f := &ConnectionCloseFrame{IsApplicationError: typ == FrameTypeApplicationClose}

	ec, l, err := varint.Parse(b)
	if err != nil {
		return nil, 0, replaceUnexpectedEOF(err)
	}

	b = b[l:]
	f.ErrorCode = ec
	// read the Frame Type, if this is not an application error
	if !f.IsApplicationError {
		ft, l, err := varint.Parse(b)
		if err != nil {
			return nil, 0, replaceUnexpectedEOF(err)
		}

		b = b[l:]
		f.FrameType = ft
	}

	var reasonPhraseLen uint64

	reasonPhraseLen, l, err = varint.Parse(b)
	if err != nil {
		return nil, 0, replaceUnexpectedEOF(err)
	}

	b = b[l:]
	if int(reasonPhraseLen) > len(b) {
		return nil, 0, io.EOF
	}

	f.ReasonPhrase = string(b[:reasonPhraseLen])
	// Advance b by reasonPhraseLen is not strictly necessary here because we are returning, 
	// but let's just do it cleanly if needed. Actually the next line uses startLen - len(b) + int(reasonPhraseLen)

	return f, startLen - len(b) + int(reasonPhraseLen), nil
}

// Length of a written frame
func (f *ConnectionCloseFrame) Length(protocol.Version) protocol.ByteCount {
	length := 1 + protocol.ByteCount(
		varint.Len(f.ErrorCode)+varint.Len(uint64(len(f.ReasonPhrase))),
	) + protocol.ByteCount(
		len(f.ReasonPhrase),
	)
	if !f.IsApplicationError {
		length += protocol.ByteCount(varint.Len(f.FrameType)) // for the frame type
	}

	return length
}

func (f *ConnectionCloseFrame) Append(b []byte, _ protocol.Version) ([]byte, error) {
	if f.IsApplicationError {
		b = append(b, byte(FrameTypeApplicationClose))
	} else {
		b = append(b, byte(FrameTypeConnectionClose))
	}

	b = varint.Append(b, f.ErrorCode)
	if !f.IsApplicationError {
		b = varint.Append(b, f.FrameType)
	}

	b = varint.Append(b, uint64(len(f.ReasonPhrase)))
	b = append(b, []byte(f.ReasonPhrase)...)

	return b, nil
}
