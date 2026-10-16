// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sse implements the WHATWG HTML Living Standard Server-Sent Events (SSE) wire protocol (§9.2).
package sse

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"iter"
	"strconv"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"
)

// Event represents a single parsed Server-Sent Event frame (WHATWG §9.2).
//
// Byte slices reference internal Reader buffers and remain valid until the next
// call to [Reader.NextEvent] or [Reader.Scan].
type Event struct {
	// Event is the custom event type name (event: field).
	Event []byte

	// Data is the accumulated event payload (data: fields).
	Data []byte

	// ID is the event stream identifier (id: field).
	ID []byte

	// Retry is the reconnect interval in milliseconds (retry: field).
	Retry int
}

// Reset clears all event fields while preserving buffer capacities.
func (e *Event) Reset() {
	e.Event = e.Event[:0]
	e.Data = e.Data[:0]
	e.ID = e.ID[:0]
	e.Retry = 0
}

// EventString returns the event type as a string without allocation.
func (e *Event) EventString() string {
	return bytesconv.B2S(e.Event)
}

// DataString returns the data payload as a string without allocation.
func (e *Event) DataString() string {
	return bytesconv.B2S(e.Data)
}

// IDString returns the event identifier as a string without allocation.
func (e *Event) IDString() string {
	return bytesconv.B2S(e.ID)
}

// Reader provides zero-allocation sequential parsing over a WHATWG Server-Sent Event stream.
type Reader struct {
	br     *bufio.Reader
	closer io.Closer

	eventBuf []byte
	dataBuf  []byte
	idBuf    []byte
	lineBuf  []byte

	hasData bool
	cur     Event
	err     error
}

// NewReader creates a new SSE [Reader] from r.
func NewReader(r io.Reader) *Reader {
	var closer io.Closer
	if c, ok := r.(io.Closer); ok {
		closer = c
	}

	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}

	return &Reader{
		br:     br,
		closer: closer,
	}
}

// Reset reinitializes the Reader with a new underlying stream, retaining buffer capacities.
func (r *Reader) Reset(rd io.Reader) {
	var closer io.Closer
	if c, ok := rd.(io.Closer); ok {
		closer = c
	}

	r.closer = closer

	if br, ok := rd.(*bufio.Reader); ok {
		r.br = br
	} else if r.br != nil {
		r.br.Reset(rd)
	} else {
		r.br = bufio.NewReader(rd)
	}

	r.eventBuf = r.eventBuf[:0]
	r.dataBuf = r.dataBuf[:0]
	r.idBuf = r.idBuf[:0]
	r.lineBuf = r.lineBuf[:0]
	r.hasData = false
	r.cur.Reset()
	r.err = nil
}

// Close closes the underlying reader if it implements [io.Closer].
func (r *Reader) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}

	return nil
}

// NextEvent reads and returns the next [Event] according to WHATWG HTML §9.2.
// It returns [io.EOF] when the stream has terminated.
func (r *Reader) NextEvent() (Event, error) {
	if r == nil || r.br == nil {
		return Event{}, io.EOF
	}

	r.cur.Reset()
	r.eventBuf = r.eventBuf[:0]
	r.dataBuf = r.dataBuf[:0]
	r.idBuf = r.idBuf[:0]
	r.hasData = false

	for {
		line, err := r.readLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if r.hasDispatchedData() {
					return r.buildEvent(), nil
				}

				return Event{}, io.EOF
			}

			return Event{}, err
		}

		line = bytes.TrimRight(line, "\r\n")

		// Empty line indicates event boundary / dispatch
		if len(line) == 0 {
			if r.hasDispatchedData() {
				return r.buildEvent(), nil
			}

			continue
		}

		// Comment line per WHATWG spec: starts with U+003A COLON (:)
		if line[0] == ':' {
			continue
		}

		field, value, ok := bytes.Cut(line, []byte{':'})
		if ok && len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}

		switch {
		case bytes.Equal(field, []byte("data")):
			if r.hasData {
				r.dataBuf = append(r.dataBuf, '\n')
			}

			r.dataBuf = append(r.dataBuf, value...)
			r.hasData = true

		case bytes.Equal(field, []byte("event")):
			r.eventBuf = append(r.eventBuf[:0], value...)

		case bytes.Equal(field, []byte("id")):
			// WHATWG: if value contains null character (U+0000), ignore it
			if bytes.IndexByte(value, 0) == -1 {
				r.idBuf = append(r.idBuf[:0], value...)
			}

		case bytes.Equal(field, []byte("retry")):
			if v, parseErr := parseASCIIUint(value); parseErr == nil {
				r.cur.Retry = v
			}
		}
	}
}

func (r *Reader) hasDispatchedData() bool {
	return r.hasData || len(r.eventBuf) > 0 || len(r.idBuf) > 0 || r.cur.Retry > 0
}

func (r *Reader) buildEvent() Event {
	r.cur.Event = r.eventBuf
	r.cur.Data = r.dataBuf
	r.cur.ID = r.idBuf
	ev := r.cur

	r.eventBuf = r.eventBuf[:0]
	r.dataBuf = r.dataBuf[:0]
	r.idBuf = r.idBuf[:0]
	r.hasData = false

	return ev
}

func (r *Reader) readLine() ([]byte, error) {
	r.lineBuf = r.lineBuf[:0]

	for {
		chunk, err := r.br.ReadSlice('\n')
		if err == nil {
			if len(r.lineBuf) == 0 {
				return chunk, nil
			}

			r.lineBuf = append(r.lineBuf, chunk...)

			return r.lineBuf, nil
		}

		if errors.Is(err, bufio.ErrBufferFull) {
			r.lineBuf = append(r.lineBuf, chunk...)
			continue
		}

		if errors.Is(err, io.EOF) {
			if len(chunk) > 0 {
				r.lineBuf = append(r.lineBuf, chunk...)
				return r.lineBuf, nil
			}

			if len(r.lineBuf) > 0 {
				return r.lineBuf, nil
			}
		}

		return nil, err
	}
}

// Scan advances the Reader to the next event, which is accessible via [Reader.Event].
// Returns false on EOF or stream transport error.
func (r *Reader) Scan() bool {
	ev, err := r.NextEvent()
	if err != nil {
		if !errors.Is(err, io.EOF) {
			r.err = err
		}

		return false
	}

	r.cur = ev

	return true
}

// Event returns the most recent [Event] read by [Reader.Scan].
func (r *Reader) Event() Event {
	return r.cur
}

// Err returns the first non-EOF error encountered by [Reader.Scan].
func (r *Reader) Err() error {
	return r.err
}

// All returns an iter.Seq2 range-over-func iterator yielding decoded events (Go 1.23+).
func (r *Reader) All() iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		for {
			ev, err := r.NextEvent()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					yield(ev, err)
				}

				return
			}

			if !yield(ev, nil) {
				return
			}
		}
	}
}

// AppendEvent appends a formatted Server-Sent Event frame to dst according to WHATWG §9.2.
// Multi-line data payloads are split into individual data: lines.
func AppendEvent[S ~string | ~[]byte](dst []byte, event, data, id S, retry int) []byte {
	if len(id) > 0 {
		dst = append(dst, "id: "...)
		dst = append(dst, id...)
		dst = append(dst, '\n')
	}

	if len(event) > 0 {
		dst = append(dst, "event: "...)
		dst = append(dst, event...)
		dst = append(dst, '\n')
	}

	if retry > 0 {
		dst = append(dst, "retry: "...)
		dst = strconv.AppendInt(dst, int64(retry), 10)
		dst = append(dst, '\n')
	}

	if len(data) > 0 {
		raw := data
		for len(raw) > 0 {
			dst = append(dst, "data: "...)

			idx := indexNewline(raw)
			if idx >= 0 {
				dst = append(dst, raw[:idx]...)
				dst = append(dst, '\n')

				if idx+1 < len(raw) && raw[idx] == '\r' && raw[idx+1] == '\n' {
					raw = raw[idx+2:]
				} else {
					raw = raw[idx+1:]
				}
			} else {
				dst = append(dst, raw...)
				dst = append(dst, '\n')
				break
			}
		}
	}

	dst = append(dst, '\n')

	return dst
}

func indexNewline[S ~string | ~[]byte](s S) int {
	for i := range len(s) {
		if s[i] == '\n' || s[i] == '\r' {
			return i
		}
	}

	return -1
}

var sseBufferPool = pool.NewPerCorePool[*bytesconv.ByteBuffer](func() *bytesconv.ByteBuffer {
	return &bytesconv.ByteBuffer{B: make([]byte, 0, 512)}
})

// WriteEvent formats and writes a Server-Sent Event to w using a pooled zero-allocation buffer.
func WriteEvent[S ~string | ~[]byte](w io.Writer, event, data, id S, retry int) (int, error) {
	bb := sseBufferPool.Get()
	bb.B = AppendEvent(bb.B[:0], event, data, id, retry)
	n, err := w.Write(bb.B)
	sseBufferPool.Put(bb)

	return n, err
}

// Writer provides sequential, zero-allocation serialization of SSE events to an io.Writer.
type Writer struct {
	w   io.Writer
	buf []byte
}

// NewWriter creates a new SSE [Writer] targeting w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteEvent formats and writes an SSE event to the underlying stream.
func (w *Writer) WriteEvent(event, data, id []byte, retry int) (int, error) {
	w.buf = AppendEvent(w.buf[:0], event, data, id, retry)
	return w.w.Write(w.buf)
}

func parseASCIIUint(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, errors.New("empty uint")
	}

	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid digit")
		}

		n = n*10 + int(c-'0')
	}

	return n, nil
}
