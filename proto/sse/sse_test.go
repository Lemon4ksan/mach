// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sse_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/sse"
)

type customCloserReader struct {
	io.Reader
	closed bool
}

func (c *customCloserReader) Close() error {
	c.closed = true
	return nil
}

func TestSSE_Parser_Basic(t *testing.T) {
	t.Parallel()

	raw := ": comment line\n" +
		"event: message\n" +
		"data: hello world\n" +
		"id: 1\n" +
		"retry: 5000\n\n" +
		"event: update\n" +
		"data: first line\n" +
		"data: second line\n\n" +
		": another comment\n\n" +
		"data: final event\n\n"

	closer := &customCloserReader{Reader: strings.NewReader(raw)}
	r := sse.NewReader(closer)

	ev1, err := r.NextEvent()
	require.NoError(t, err)
	assert.Equal(t, "message", ev1.EventString())
	assert.Equal(t, "hello world", ev1.DataString())
	assert.Equal(t, "1", ev1.IDString())
	assert.Equal(t, 5000, ev1.Retry)

	ev2, err := r.NextEvent()
	require.NoError(t, err)
	assert.Equal(t, "update", ev2.EventString())
	assert.Equal(t, "first line\nsecond line", ev2.DataString())

	ev3, err := r.NextEvent()
	require.NoError(t, err)
	assert.Equal(t, "", ev3.EventString())
	assert.Equal(t, "final event", ev3.DataString())

	_, err = r.NextEvent()
	assert.ErrorIs(t, err, io.EOF)

	assert.NoError(t, r.Close())
	assert.True(t, closer.closed)
}

func TestSSE_Scanner_And_Iterator(t *testing.T) {
	t.Parallel()

	raw := "data: msg1\n\n" +
		"data: msg2\n\n"

	// 1. Scan() loop
	r := sse.NewReader(strings.NewReader(raw))
	var events []string
	for r.Scan() {
		events = append(events, string(r.Event().Data))
	}
	require.NoError(t, r.Err())
	assert.Equal(t, []string{"msg1", "msg2"}, events)

	// 2. Iterator All()
	r.Reset(strings.NewReader(raw))
	var iterEvents []string
	for ev, err := range r.All() {
		require.NoError(t, err)
		iterEvents = append(iterEvents, string(ev.Data))
	}
	assert.Equal(t, []string{"msg1", "msg2"}, iterEvents)
}

func TestSSE_TrailingEvent_BeforeEOF(t *testing.T) {
	t.Parallel()

	raw := "event: ping\ndata: pong"
	r := sse.NewReader(strings.NewReader(raw))

	ev, err := r.NextEvent()
	require.NoError(t, err)
	assert.Equal(t, "ping", ev.EventString())
	assert.Equal(t, "pong", ev.DataString())

	_, err = r.NextEvent()
	assert.ErrorIs(t, err, io.EOF)
}

func TestSSE_LongLine_BufferGrowth(t *testing.T) {
	t.Parallel()

	// Data line larger than default bufio 4096 bytes buffer
	longPayload := strings.Repeat("A", 10000)
	raw := "event: large\ndata: " + longPayload + "\n\n"

	r := sse.NewReader(strings.NewReader(raw))
	ev, err := r.NextEvent()
	require.NoError(t, err)
	assert.Equal(t, "large", ev.EventString())
	assert.Equal(t, longPayload, ev.DataString())
}

func TestSSE_SpecEdgeCases(t *testing.T) {
	t.Parallel()

	// 1. Null byte in ID must be ignored (WHATWG §9.2)
	rawNullID := "id: invalid\x00id\ndata: test\n\n"
	r := sse.NewReader(strings.NewReader(rawNullID))
	ev, err := r.NextEvent()
	require.NoError(t, err)
	assert.Empty(t, ev.ID)
	assert.Equal(t, "test", ev.DataString())

	// 2. Line without colon treated as field with empty value
	rawNoColon := "data\n\n"
	r.Reset(strings.NewReader(rawNoColon))
	ev, err = r.NextEvent()
	require.NoError(t, err)
	assert.Empty(t, ev.Data)

	// 3. Leading space after colon stripped, subsequent spaces preserved
	rawSpaces := "data:   three spaces\n\n"
	r.Reset(strings.NewReader(rawSpaces))
	ev, err = r.NextEvent()
	require.NoError(t, err)
	assert.Equal(t, "  three spaces", ev.DataString())
}

func TestSSE_AppendAndWriteEvent(t *testing.T) {
	t.Parallel()

	// 1. Single-line event
	out := sse.AppendEvent(nil, "greeting", "hello", "1", 1000)
	expected := "id: 1\nevent: greeting\nretry: 1000\ndata: hello\n\n"
	assert.Equal(t, expected, string(out))

	// 2. Multi-line data payload
	outMulti := sse.AppendEvent(nil, "", "line1\nline2\r\nline3", "", 0)
	expectedMulti := "data: line1\ndata: line2\ndata: line3\n\n"
	assert.Equal(t, expectedMulti, string(outMulti))

	// 3. WriteEvent to buffer
	var buf bytes.Buffer
	n, err := sse.WriteEvent(&buf, "update", "value", "id-2", 0)
	require.NoError(t, err)
	assert.Equal(t, buf.Len(), n)
	assert.Equal(t, "id: id-2\nevent: update\ndata: value\n\n", buf.String())

	// 4. Writer struct
	buf.Reset()
	w := sse.NewWriter(&buf)
	n2, err := w.WriteEvent([]byte("alert"), []byte("fired"), nil, 0)
	require.NoError(t, err)
	assert.Equal(t, buf.Len(), n2)
	assert.Equal(t, "event: alert\ndata: fired\n\n", buf.String())
}
