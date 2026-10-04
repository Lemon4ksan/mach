// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package contentdisposition_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http/contentdisposition"
)

func TestAlloc(t *testing.T) {
	header := `attachment; filename="report.pdf"`

	t.Run("Parse", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			cd := contentdisposition.ParseContentDisposition(header)
			if cd.Type != "attachment" || cd.Filename != "report.pdf" {
				t.Fatalf("unexpected parse: cd=%v", cd)
			}
		})
		if allocs > 10 {
			t.Errorf("expected minimal allocs, got %v", allocs)
		}
	})
}
