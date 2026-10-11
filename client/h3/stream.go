// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"io"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/mach"
	"github.com/lemon4ksan/mach/net/quic"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

var (
	_ mach.Stream = (*Stream)(nil)
	_ mach.Frame  = (*Frame)(nil)
)

// Stream represents an HTTP/3 bidirectional stream satisfying mach.Stream.
type Stream struct {
	str   *quic.Stream
	frame Frame
	vr    varint.Reader
}

// NewStream creates a new HTTP/3 Stream wrapping a QUIC stream.
func NewStream(str *quic.Stream) *Stream {
	return &Stream{
		str: str,
		vr:  varint.NewReader(str),
	}
}

func (s *Stream) Read(p []byte) (int, error) {
	return s.str.Read(p)
}

func (s *Stream) Write(p []byte) (int, error) {
	return s.str.Write(p)
}

func (s *Stream) Close() error {
	return s.str.Close()
}

func (s *Stream) ReadFrame() (mach.Frame, error) {
	frameType, payloadLen, err := coreh3.ReadFrameHeader(s.vr)
	if err != nil {
		return nil, err
	}

	if cap(s.frame.payload) < int(payloadLen) {
		s.frame.payload = make([]byte, payloadLen)
	} else {
		s.frame.payload = s.frame.payload[:payloadLen]
	}

	if _, err := io.ReadFull(s.str, s.frame.payload); err != nil {
		return nil, err
	}

	s.frame.frameType = uint8(frameType)

	return &s.frame, nil
}

// Frame implements mach.Frame for HTTP/3 frames.
type Frame struct {
	frameType uint8
	payload   []byte
}

func (f *Frame) Type() uint8 {
	return f.frameType
}

func (f *Frame) Payload() []byte {
	return f.payload
}
