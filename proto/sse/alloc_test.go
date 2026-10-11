// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sse_test

import (
	"io"
	"strings"
	"testing"

	"github.com/lemon4ksan/mach/proto/sse"
)

func TestAlloc(t *testing.T) {
	raw := "event: update\ndata: payload\nid: 42\nretry: 5000\n\n"

	t.Run("ReaderNextEvent", func(t *testing.T) {
		r := strings.NewReader(raw)
		reader := sse.NewReader(r)

		allocs := testing.AllocsPerRun(1000, func() {
			r.Reset(raw)
			reader.Reset(r)

			ev, err := reader.NextEvent()
			if err != nil || string(ev.Event) != "update" || string(ev.Data) != "payload" {
				t.Fatalf("unexpected event: %v, err=%v", ev, err)
			}
		})
		if allocs != 0 {
			t.Errorf("ReaderNextEvent expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("ReaderScan", func(t *testing.T) {
		r := strings.NewReader(raw)
		reader := sse.NewReader(r)

		allocs := testing.AllocsPerRun(1000, func() {
			r.Reset(raw)
			reader.Reset(r)

			if !reader.Scan() {
				t.Fatalf("expected Scan to succeed, err=%v", reader.Err())
			}
			ev := reader.Event()
			if string(ev.Event) != "update" || string(ev.Data) != "payload" {
				t.Fatalf("unexpected event: %v", ev)
			}
		})
		if allocs != 0 {
			t.Errorf("ReaderScan expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("AppendEvent", func(t *testing.T) {
		buf := make([]byte, 0, 128)
		event := "update"
		data := "first line\nsecond line"
		id := "100"

		allocs := testing.AllocsPerRun(1000, func() {
			out := sse.AppendEvent(buf[:0], event, data, id, 3000)
			if len(out) == 0 {
				t.Fatal("empty out")
			}
		})
		if allocs != 0 {
			t.Errorf("AppendEvent expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("WriteEvent", func(t *testing.T) {
		var w io.Writer = io.Discard
		event := "update"
		data := "payload"
		id := "100"

		allocs := testing.AllocsPerRun(1000, func() {
			n, err := sse.WriteEvent(w, event, data, id, 1000)
			if err != nil || n == 0 {
				t.Fatalf("WriteEvent failed: %d, %v", n, err)
			}
		})
		if allocs != 0 {
			t.Errorf("WriteEvent expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("WriterStruct", func(t *testing.T) {
		w := sse.NewWriter(io.Discard)
		event := []byte("update")
		data := []byte("payload")
		id := []byte("100")

		allocs := testing.AllocsPerRun(1000, func() {
			n, err := w.WriteEvent(event, data, id, 1000)
			if err != nil || n == 0 {
				t.Fatalf("Writer.WriteEvent failed: %d, %v", n, err)
			}
		})
		if allocs != 0 {
			t.Errorf("Writer.WriteEvent expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("EventStringHelpers", func(t *testing.T) {
		ev := sse.Event{
			Event: []byte("alert"),
			Data:  []byte("system check"),
			ID:    []byte("id-99"),
		}

		allocs := testing.AllocsPerRun(1000, func() {
			_ = ev.EventString()
			_ = ev.DataString()
			_ = ev.IDString()
		})
		if allocs != 0 {
			t.Errorf("EventStringHelpers expected 0 allocs, got %v", allocs)
		}
	})
}
