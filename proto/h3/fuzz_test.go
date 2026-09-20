// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"bytes"
	"testing"
)

// FuzzH3FrameHeaderRead tests HTTP/3 varint frame header reading against arbitrary input bytes.
func FuzzH3FrameHeaderRead(f *testing.F) {
	seeds := [][]byte{
		{0x01, 0x04, 0x00, 0x00, 0x80, 0x01},
		{0x00, 0x0a, 't', 'e', 's', 't', 'b', 'o', 'd', 'y', '1', '2'},
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		r := bytes.NewReader(data)
		_, _, _ = ReadFrameHeader(r)
	})
}
