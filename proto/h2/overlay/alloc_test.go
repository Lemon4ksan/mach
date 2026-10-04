// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package overlay_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/h2/overlay"
)

func TestAlloc(t *testing.T) {
	raw := []byte{0x00, 0x00, 0x05, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 'h', 'e', 'l', 'l', 'o'}

	allocs := testing.AllocsPerRun(1000, func() {
		frame := overlay.Frame(raw)
		if !frame.IsValid() {
			t.Fatal("invalid frame")
		}
		_ = frame.Length()
		_ = frame.Type()
		_ = frame.Flags()
		_ = frame.StreamID()
		_ = frame.Payload()
	})
	if allocs != 0 {
		t.Errorf("expected 0 allocs, got %v", allocs)
	}
}
