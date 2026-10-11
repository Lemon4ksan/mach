// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"testing"
)

func FuzzVarint(f *testing.F) {
	f.Add([]byte{0x00}, byte(8))
	f.Add([]byte{0xff, 0x01}, byte(6))
	f.Add([]byte{0x1f, 0x9a, 0x0a}, byte(5))

	f.Fuzz(func(t *testing.T, data []byte, n byte) {
		if n == 0 || n > 8 {
			return
		}

		_, _, _ = readInt(n, data)
	})
}
