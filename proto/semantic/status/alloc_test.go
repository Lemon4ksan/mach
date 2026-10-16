// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/semantic/status"
)

func TestAlloc(t *testing.T) {
	dst := make([]byte, 0, 128)
	protocol := []byte("HTTP/1.1")

	allocs := testing.AllocsPerRun(1000, func() {
		_ = status.Message(status.OK)
		_ = status.Message(status.NotFound)
		_ = status.FormatLine(dst[:0], protocol, status.OK, nil)
	})
	if allocs != 0 {
		t.Errorf("expected 0 allocs, got %v", allocs)
	}
}
