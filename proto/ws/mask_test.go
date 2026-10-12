// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws_test

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/lemon4ksan/mach/proto/ws"
)

func TestMask_Involution(t *testing.T) {
	lengths := []int{0, 1, 3, 4, 7, 8, 9, 15, 16, 31, 32, 63, 64, 65, 127, 128, 255, 1024, 65536}
	key := ws.MaskKey{0xCA, 0xFE, 0xBA, 0xBE}

	for _, n := range lengths {
		original := make([]byte, n)
		for i := range original {
			original[i] = byte(i*17 + 5)
		}

		data := make([]byte, n)
		copy(data, original)

		// First call masks
		ws.Mask(data, key)
		// Second call unmasks
		ws.Mask(data, key)

		if !bytes.Equal(data, original) {
			t.Fatalf("Mask involution failed for len=%d", n)
		}
	}
}

func TestMask_Differential_AllLengthsAndOffsets(t *testing.T) {
	key := ws.MaskKey{0x37, 0xA2, 0xF9, 0x11}

	// Buffer with head room to test varied pointer alignments
	const maxLen = 512
	raw := make([]byte, maxLen+32)
	for i := range raw {
		raw[i] = byte(i * 3)
	}

	for align := 0; align < 8; align++ {
		for length := 0; length <= 256; length++ {
			for offset := 0; offset < 8; offset++ {
				swarBuf := make([]byte, length)
				scalarBuf := make([]byte, length)

				copy(swarBuf, raw[align:align+length])
				copy(scalarBuf, raw[align:align+length])

				ws.MaskWithOffset(swarBuf, key, offset)
				ws.MaskScalar(scalarBuf, key, offset)

				if !bytes.Equal(swarBuf, scalarBuf) {
					t.Fatalf("Differential mismatch: align=%d, length=%d, offset=%d\nswar:   %x\nscalar: %x",
						align, length, offset, swarBuf, scalarBuf)
				}
			}
		}
	}
}

func TestMask_FuzzRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(123456789))

	for iter := 0; iter < 1000; iter++ {
		length := rng.Intn(4096)
		offset := rng.Intn(16)
		align := rng.Intn(8)

		var key ws.MaskKey
		rng.Read(key[:])

		raw := make([]byte, length+align)
		rng.Read(raw)

		swarData := make([]byte, length)
		copy(swarData, raw[align:])

		scalarData := make([]byte, length)
		copy(scalarData, raw[align:])

		ws.MaskWithOffset(swarData, key, offset)
		ws.MaskScalar(scalarData, key, offset)

		if !bytes.Equal(swarData, scalarData) {
			t.Fatalf("Fuzz mismatch at iter %d: len=%d, offset=%d, align=%d", iter, length, offset, align)
		}
	}
}
