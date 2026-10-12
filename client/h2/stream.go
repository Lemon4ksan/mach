// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/lemon4ksan/mach"
	"github.com/lemon4ksan/mach/proto/http"
)

var (
	_ mach.Stream = (*Stream)(nil)
	_ mach.Frame  = (*WireFrame)(nil)
)

// Stream represents an HTTP/2 stream satisfying mach.Stream.
type Stream struct {
	c        *Conn
	ctx      context.Context
	reqBuf   bytes.Buffer
	resBytes []byte
	resOff   int
	res      *http.Response
	executed bool
	mu       sync.Mutex
	frameIdx int
	frames   []WireFrame
}

// NewStream creates a new HTTP/2 Stream wrapping c.
func NewStream(c *Conn, ctx context.Context) *Stream {
	return &Stream{
		c:   c,
		ctx: ctx,
	}
}

func (s *Stream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.reqBuf.Write(p)
}

func (s *Stream) ensureExecuted() error {
	if s.executed {
		return nil
	}

	s.executed = true

	req := http.AcquireRequest()
	defer http.ReleaseRequest(req)

	if s.reqBuf.Len() > 0 {
		br := bufio.NewReader(&s.reqBuf)
		if err := req.Read(br); err != nil {
			req.Header.SetMethod("POST")
			req.SetRequestURI("/")
			req.SetBody(s.reqBuf.Bytes())
		}
	} else {
		req.Header.SetMethod("GET")
		req.SetRequestURI("/")
	}

	s.res = http.AcquireResponse()

	err := s.c.Do(s.ctx, req, s.res)
	if err != nil {
		return err
	}

	var resBuf bytes.Buffer

	bw := bufio.NewWriter(&resBuf)
	_ = s.res.Write(bw)
	_ = bw.Flush()
	s.resBytes = resBuf.Bytes()

	var hdrBuf bytes.Buffer

	hw := bufio.NewWriter(&hdrBuf)
	_ = s.res.Header.Write(hw)
	_ = hw.Flush()

	s.frames = []WireFrame{
		{frameType: mach.FrameHeaders, payload: hdrBuf.Bytes()},
		{frameType: mach.FrameData, payload: s.res.Body()},
	}

	return nil
}

func (s *Stream) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureExecuted(); err != nil {
		return 0, err
	}

	if s.resOff >= len(s.resBytes) {
		return 0, io.EOF
	}

	n := copy(p, s.resBytes[s.resOff:])
	s.resOff += n

	return n, nil
}

func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.res != nil {
		http.ReleaseResponse(s.res)
		s.res = nil
	}

	return nil
}

func (s *Stream) ReadFrame() (mach.Frame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureExecuted(); err != nil {
		return nil, err
	}

	if s.frameIdx >= len(s.frames) {
		return nil, io.EOF
	}

	f := &s.frames[s.frameIdx]
	s.frameIdx++

	return f, nil
}

// WireFrame implements mach.Frame for HTTP/2 wire frames.
type WireFrame struct {
	frameType uint8
	payload   []byte
}

func (f *WireFrame) Type() uint8 {
	return f.frameType
}

func (f *WireFrame) Payload() []byte {
	return f.payload
}
