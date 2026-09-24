// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sse_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/proto/sse"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
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
		"data: [DONE]\n\n"

	closer := &customCloserReader{Reader: strings.NewReader(raw)}
	r := sse.NewReader[sse.Event](closer)

	ev1, err := r.NextEvent()
	require.NoError(t, err)
	assert.Equal(t, "message", ev1.Event)
	assert.Equal(t, "hello world", ev1.Data)
	assert.Equal(t, "1", ev1.ID)
	assert.Equal(t, 5000, ev1.Retry)

	ev2, err := r.NextEvent()
	require.NoError(t, err)
	assert.Equal(t, "update", ev2.Event)
	assert.Equal(t, "first line\nsecond line", ev2.Data)

	// [DONE] triggers io.EOF
	_, err = r.NextEvent()
	assert.ErrorIs(t, err, io.EOF)

	assert.NoError(t, r.Close())
	assert.True(t, closer.closed)
}

func TestSSE_TypesAndDecoders(t *testing.T) {
	t.Parallel()

	// 1. String reader
	rawStr := "data: pure-string\n\n"
	rStr := sse.NewReader[string](strings.NewReader(rawStr))
	val, err := rStr.Next()
	require.NoError(t, err)
	assert.Equal(t, "pure-string", val)

	// 2. Struct JSON decoding
	type Item struct {
		Name string `json:"name"`
		Val  int    `json:"val"`
	}

	rawJSON := "data: {\"name\":\"first\",\"val\":1}\n\n" +
		"data: invalid-json-payload\n\n"

	rJSON := sse.NewReader[Item](strings.NewReader(rawJSON))
	item1, err := rJSON.Next()
	require.NoError(t, err)
	assert.Equal(t, "first", item1.Name)
	assert.Equal(t, 1, item1.Val)

	// Invalid JSON returns error
	_, err = rJSON.Next()
	assert.Error(t, err)

	// 3. Trailing event before EOF without double newline
	rawTrailing := "event: ping\ndata: pong"
	rTrailing := sse.NewReader[sse.Event](strings.NewReader(rawTrailing))
	evTrailing, err := rTrailing.NextEvent()
	require.NoError(t, err)
	assert.Equal(t, "ping", evTrailing.Event)
	assert.Equal(t, "pong", evTrailing.Data)

	// Next on empty reader
	_, err = rTrailing.NextEvent()
	assert.ErrorIs(t, err, io.EOF)
}

func TestSSE_Channel_And_Iterator(t *testing.T) {
	t.Parallel()

	type Item struct {
		Name string `json:"name"`
		Val  int    `json:"val"`
	}

	raw := "data: {\"name\":\"first\",\"val\":1}\n\n" +
		"data: {\"name\":\"second\",\"val\":2}\n\n"

	// 1. Iterator range-over-func
	rIter := sse.NewReader[Item](strings.NewReader(raw))
	var items []Item
	for item, err := range rIter.All() {
		require.NoError(t, err)
		items = append(items, item)
	}
	assert.Len(t, items, 2)

	// 2. Channel reading
	rChan := sse.NewReader[Item](strings.NewReader(raw))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	outCh, errCh := rChan.Channel(ctx)
	var chanItems []Item
	for item := range outCh {
		chanItems = append(chanItems, item)
	}

	for err := range errCh {
		t.Fatalf("unexpected channel error: %v", err)
	}
	assert.Len(t, chanItems, 2)
}
