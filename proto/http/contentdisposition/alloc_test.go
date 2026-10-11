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

	t.Run("AppendFormatContentDisposition", func(t *testing.T) {
		var buf [256]byte
		allocs := testing.AllocsPerRun(1000, func() {
			_ = contentdisposition.AppendFormatContentDisposition(buf[:0], "attachment", "report.pdf")
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for AppendFormatContentDisposition, got %v", allocs)
		}
	})

	t.Run("AppendEncodeRFC8187", func(t *testing.T) {
		var buf [256]byte
		allocs := testing.AllocsPerRun(1000, func() {
			_ = contentdisposition.AppendEncodeRFC8187(buf[:0], "отчет.pdf", "ru")
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for AppendEncodeRFC8187, got %v", allocs)
		}
	})

	t.Run("IsWindowsReservedName", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			if !contentdisposition.IsWindowsReservedName("CON.txt") {
				t.Fatal("expected CON.txt to be reserved")
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for IsWindowsReservedName, got %v", allocs)
		}
	})

	t.Run("IsRFC8187AttrChar", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			if !contentdisposition.IsRFC8187AttrChar('a') {
				t.Fatal("expected valid attr char")
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for IsRFC8187AttrChar, got %v", allocs)
		}
	})
}
