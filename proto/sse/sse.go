// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sse implements the WHATWG HTML Living Standard Server-Sent Events (SSE) protocol (formerly W3C).
package sse

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"strconv"
	"strings"
)

// Event represents a single parsed W3C Server-Sent Event frame.
type Event struct {
	// Event is the custom event type name (event: field).
	Event string

	// Data is the accumulated event payload (data: fields).
	Data string

	// ID is the event stream identifier (id: field).
	ID string

	// Retry is the reconnect interval in milliseconds (retry: field).
	Retry int
}

// Reader provides sequential reading over a W3C Server-Sent Event stream.
type Reader[T any] struct {
	br     *bufio.Reader
	closer io.Closer
}

// NewReader creates a new SSE [Reader] from r.
func NewReader[T any](r io.Reader) *Reader[T] {
	var closer io.Closer
	if c, ok := r.(io.Closer); ok {
		closer = c
	}

	return &Reader[T]{
		br:     bufio.NewReader(r),
		closer: closer,
	}
}

// Close closes the underlying reader if it implements [io.Closer].
func (r *Reader[T]) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}

	return nil
}

// NextEvent reads the next raw [Event] from the stream according to the W3C specification.
func (r *Reader[T]) NextEvent() (Event, error) {
	if r == nil || r.br == nil {
		return Event{}, io.EOF
	}

	var currentEvent Event

	for {
		lineBytes, err := r.br.ReadSlice('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(lineBytes) > 0 {
					parseSSELine(lineBytes, &currentEvent)

					if currentEvent.Data != "" || currentEvent.Event != "" {
						if strings.EqualFold(strings.TrimSpace(currentEvent.Data), "[DONE]") {
							return Event{}, io.EOF
						}

						return currentEvent, nil
					}
				}

				return Event{}, io.EOF
			}

			return Event{}, err
		}

		// Empty line indicates event boundary
		if len(bytes.TrimSpace(lineBytes)) == 0 {
			if currentEvent.Data != "" || currentEvent.Event != "" {
				if strings.EqualFold(strings.TrimSpace(currentEvent.Data), "[DONE]") {
					return Event{}, io.EOF
				}

				return currentEvent, nil
			}

			continue
		}

		parseSSELine(lineBytes, &currentEvent)
	}
}

// Next reads and decodes the next event payload into T.
func (r *Reader[T]) Next() (T, error) {
	ev, err := r.NextEvent()
	if err != nil {
		var zero T
		return zero, err
	}

	return decodePayload[T](ev)
}

// All returns an iter.Seq2 range-over-func iterator yielding decoded values (Go 1.23+).
func (r *Reader[T]) All() iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for {
			val, err := r.Next()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					yield(val, err)
				}

				return
			}

			if !yield(val, nil) {
				return
			}
		}
	}
}

// Channel pushes decoded stream values to output channels.
func (r *Reader[T]) Channel(ctx context.Context) (<-chan T, <-chan error) {
	out := make(chan T, 16)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)
		defer r.Close()

		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
				val, err := r.Next()
				if err != nil {
					if !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
						errCh <- err
					}

					return
				}

				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				case out <- val:
				}
			}
		}
	}()

	return out, errCh
}

func parseSSELine(line []byte, ev *Event) {
	line = bytes.TrimRight(line, "\r\n")
	if len(line) == 0 || line[0] == ':' {
		return
	}

	key, value, ok := bytes.Cut(line, []byte{':'})
	if ok {
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}
	} else {
		key = line
	}

	switch string(key) {
	case "event":
		ev.Event = string(value)
	case "data":
		if ev.Data != "" {
			ev.Data += "\n" + string(value)
			return
		}

		ev.Data = string(value)

	case "id":
		ev.ID = string(value)
	case "retry":
		if r, err := strconv.Atoi(string(bytes.TrimSpace(value))); err == nil {
			ev.Retry = r
		}
	}
}

func decodePayload[T any](ev Event) (T, error) {
	if sse, ok := any(ev).(T); ok {
		return sse, nil
	}

	if s, ok := any(ev.Data).(T); ok {
		return s, nil
	}

	var val T
	if err := json.Unmarshal([]byte(ev.Data), &val); err != nil {
		return val, fmt.Errorf("sse: unmarshal failed: %w", err)
	}

	return val, nil
}
