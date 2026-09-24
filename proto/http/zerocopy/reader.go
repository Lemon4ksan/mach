// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"bytes"
	"errors"
	"io"
)

var ErrBufferFull = errors.New("zerocopy: buffer full")

type StreamReader struct {
	buf []byte
	rd  io.Reader
	r   int
	w   int
	err error
}

func NewStreamReader(rd io.Reader, size int) *StreamReader {
	if size < 4096 {
		size = 4096
	}

	return &StreamReader{
		buf: make([]byte, size),
		rd:  rd,
	}
}

func (b *StreamReader) Reset(rd io.Reader) {
	b.rd = rd
	b.r = 0
	b.w = 0
	b.err = nil
}

func (b *StreamReader) Buffered() int {
	return b.w - b.r
}

func (b *StreamReader) Peek(n int) ([]byte, error) {
	if n < 0 {
		return nil, errors.New("zerocopy: negative count")
	}

	for b.w-b.r < n && b.err == nil {
		b.fill()
	}

	if n > len(b.buf) {
		return nil, ErrBufferFull
	}

	var err error
	if avail := b.w - b.r; avail < n {
		n = avail

		err = b.err
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
	}

	return b.buf[b.r : b.r+n], err
}

func (b *StreamReader) Discard(n int) (int, error) {
	if n < 0 {
		return 0, errors.New("zerocopy: negative count")
	}

	if n == 0 {
		return 0, nil
	}

	remain := b.w - b.r
	if remain == 0 && b.err != nil {
		return 0, b.err
	}

	if n <= remain {
		b.r += n
		return n, nil
	}

	b.r = b.w

	return remain, io.EOF
}

func (b *StreamReader) ReadSlice(delim byte) ([]byte, error) {
	s := 0
	for {
		if i := bytes.IndexByte(b.buf[b.r+s:b.w], delim); i >= 0 {
			i += s
			res := b.buf[b.r : b.r+i+1]
			b.r += i + 1

			return res, nil
		}

		if b.err != nil {
			res := b.buf[b.r:b.w]
			b.r = b.w
			return res, b.err
		}

		s = b.w - b.r
		b.fill()
	}
}

func (b *StreamReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	if b.r == b.w {
		if b.err != nil {
			return 0, b.err
		}

		if len(p) >= len(b.buf) {
			n, b.err = b.rd.Read(p)
			return n, b.err
		}

		b.fill()

		if b.r == b.w {
			return 0, b.err
		}
	}

	n = copy(p, b.buf[b.r:b.w])
	b.r += n

	return n, nil
}

func (b *StreamReader) ReadByte() (byte, error) {
	for b.r == b.w {
		if b.err != nil {
			return 0, b.err
		}

		b.fill()
	}

	c := b.buf[b.r]
	b.r++

	return c, nil
}

func (b *StreamReader) fill() {
	// ALWAYS shift to align memory for AVX2 instructions!
	if b.r > 0 {
		copy(b.buf, b.buf[b.r:b.w])
		b.w -= b.r
		b.r = 0
	}

	if b.w >= len(b.buf) {
		return
	}

	n, err := b.rd.Read(b.buf[b.w:])
	b.w += n
	b.err = err
}
