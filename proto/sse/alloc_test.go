// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sse_test

import (
	"strings"
	"testing"

	"github.com/lemon4ksan/mach/proto/sse"
)

func TestAlloc(t *testing.T) {
	raw := "event: update\ndata: payload\nid: 42\n\n"

	t.Run("ReaderNext", func(t *testing.T) {
		r := strings.NewReader(raw)
		reader := sse.NewReader[sse.Event](r)

		allocs := testing.AllocsPerRun(1000, func() {
			r.Reset(raw)
			ev, err := reader.Next()
			if err != nil || ev.Event != "update" || ev.Data != "payload" {
				t.Fatalf("unexpected event: %v, err=%v", ev, err)
			}
		})
		// Small number of allocs due to string conversions
		if allocs > 10 {
			t.Errorf("expected minimal allocs, got %v", allocs)
		}
	})
}
