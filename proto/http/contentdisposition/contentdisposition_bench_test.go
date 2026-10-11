// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package contentdisposition_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http/contentdisposition"
)

func BenchmarkParseContentDisposition(b *testing.B) {
	header := `attachment; filename="document.pdf"; filename*=UTF-8''document_%D1%82%D0%B5%D1%81%D1%82.pdf`
	for b.Loop() {
		_ = contentdisposition.ParseContentDisposition(header)
	}
}

func BenchmarkFormatContentDisposition(b *testing.B) {
	for b.Loop() {
		_ = contentdisposition.FormatContentDisposition("attachment", "отчет_2026_финансовый.pdf")
	}
}

func BenchmarkFileName(b *testing.B) {
	path := `../../Windows/System32/../documents/report.pdf`
	for b.Loop() {
		_ = contentdisposition.FileName(path)
	}
}

func BenchmarkEncodeRFC8187(b *testing.B) {
	val := "letztes Kapitel für Report 2026"
	for b.Loop() {
		_ = contentdisposition.EncodeRFC8187(val, "de")
	}
}

func BenchmarkDecodeRFC8187(b *testing.B) {
	encoded := "UTF-8'de'letztes%20Kapitel%20f%C3%BCr%20Report%202026"
	for b.Loop() {
		_, _, _, _ = contentdisposition.DecodeRFC8187(encoded)
	}
}
