// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"

	"github.com/lemon4ksan/mach/hpack"
)

var huffmanDecStorage = pool.NewPerPStorage(func() *[]byte {
	b := make([]byte, 0, 512)
	return &b
})

func decodeHuffman(src []byte, arena *[]byte) (string, error) {
	if arena == nil {
		bufPtr := huffmanDecStorage.Get()
		defer huffmanDecStorage.Put(bufPtr)

		dst := hpack.HuffmanDecode((*bufPtr)[:0], src)
		*bufPtr = dst

		return string(dst), nil
	}

	start := len(*arena)
	dst := hpack.HuffmanDecode(*arena, src)

	*arena = dst

	return bytesconv.B2S((*arena)[start:]), nil
}
