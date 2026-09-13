// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"

	"github.com/lemon4ksan/mach/core/h2"
)

var huffmanDecStorage = pool.NewPerPStorage(func() *[]byte {
	b := make([]byte, 0, 512)
	return &b
})

func appendHuffman(dst []byte, s string) []byte {
	return h2.HuffmanEncode(dst, bytesconv.S2B(s))
}

func huffmanLen(s string) int {
	return h2.HuffmanEncodeLength(bytesconv.S2B(s))
}

func decodeHuffman(src []byte) (string, error) {
	bufPtr := huffmanDecStorage.Get()
	defer huffmanDecStorage.Put(bufPtr)

	dst := h2.HuffmanDecode((*bufPtr)[:0], src)
	*bufPtr = dst

	return string(dst), nil
}
