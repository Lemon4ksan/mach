// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package raptor

import (
	"github.com/lemon4ksan/foundation/silicon/simd/gfni"
)

// N is the hardcoded block size for the RaptorQ/LT fountain codes stub.
const N = 32

// Encoder handles the generation of repair symbols for a block of packets.
type Encoder struct{}

// NewEncoder creates a new instance of Encoder.
func NewEncoder() *Encoder {
	return &Encoder{}
}

// Encode takes a block of packets and produces a repair symbol using Galois Field multiplication.
func (e *Encoder) Encode(packets [][]byte, symbolID uint32) []byte {
	if len(packets) == 0 {
		return nil
	}

	size := 0
	for _, p := range packets {
		if len(p) > size {
			size = len(p)
		}
	}

	if size == 0 {
		return nil
	}

	repair := make([]byte, size)
	temp := make([]byte, size)

	for i, p := range packets {
		if len(p) == 0 {
			continue
		}

		copy(temp, p)

		for j := len(p); j < size; j++ {
			temp[j] = 0 // padding
		}

		// Calculate a scalar in GF(2^8) based on symbolID and index. Avoid 0 scalar.
		scalar := byte((symbolID + uint32(i) + 1) % 256)
		if scalar == 0 {
			scalar = 1
		}

		// Multiply the packet by the scalar using the gfni library (stub logic).
		gfni.MultiplyGF2P8Vector(temp, temp, scalar)

		for j := 0; j < size; j++ {
			repair[j] ^= temp[j]
		}
	}

	return repair
}
