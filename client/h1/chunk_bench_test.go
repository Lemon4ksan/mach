// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/lemon4ksan/mach/core/bytesutil"
)

func BenchmarkH1_ParseHexUint(b *testing.B) {
	src := []byte("1a4f")
	b.ReportAllocs()

	var total int
	for b.Loop() {
		val, _, _ := ParseHexUint(src)
		total += val
	}
	_ = total
}

func BenchmarkH1_bytesutil_FormatHexUint(b *testing.B) {
	var buf [16]byte
	b.ReportAllocs()

	var total int
	for b.Loop() {
		n := bytesutil.FormatHexUint(&buf, 6725)
		total += n
	}
	_ = total
}

func BenchmarkH1_bytesutil_WriteHexInt(b *testing.B) {
	var out bytes.Buffer
	w := bufio.NewWriter(&out)
	b.ReportAllocs()

	for b.Loop() {
		out.Reset()
		_ = bytesutil.WriteHexInt(w, 6725)
	}
}
