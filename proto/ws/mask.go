// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"encoding/binary"
	"unsafe"
)

var isLittleEndian = binary.NativeEndian.Uint16([]byte{1, 0}) == 1

// Mask applies the RFC 6455 payload masking algorithm in place using key (§5.3).
// It is vectorized using a 64-bit SWAR (SIMD-Within-A-Register) kernel with
// unaligned prefix alignment, 64-byte unrolled core loop, and tail processing.
//
// Mask is an involution: Mask(payload, key) followed by Mask(payload, key)
// restores the original payload bytes.
//
// Zero-allocation: testing.AllocsPerRun == 0.
func Mask(payload []byte, key [4]byte) {
	MaskWithOffset(payload, key, 0)
}

// MaskWithOffset applies the RFC 6455 masking algorithm in place starting at
// the specified frame byte offset. This is essential when streaming or chunking
// frames across TCP packet boundaries.
//
// Zero-allocation: testing.AllocsPerRun == 0.
func MaskWithOffset(payload []byte, key [4]byte, offset int) {
	n := len(payload)
	if n == 0 {
		return
	}

	off := offset & 3

	// For tiny payloads (< 8 bytes), run scalar XOR directly to avoid SWAR setup overhead.
	if n < 8 {
		for i := 0; i < n; i++ {
			payload[i] ^= key[(off+i)&3]
		}

		return
	}

	// Align to 8-byte pointer boundary for peak hardware throughput.
	ptr := uintptr(unsafe.Pointer(&payload[0]))

	misalign := int(ptr & 7)
	if misalign != 0 {
		prefix := 8 - misalign
		if prefix > n {
			prefix = n
		}

		for i := 0; i < prefix; i++ {
			payload[i] ^= key[(off+i)&3]
		}

		payload = payload[prefix:]
		off = (off + prefix) & 3

		n = len(payload)
		if n < 8 {
			for i := 0; i < n; i++ {
				payload[i] ^= key[(off+i)&3]
			}

			return
		}
	}

	// Construct repeated 8-byte mask word for current key phase.
	b0 := key[off]
	b1 := key[(off+1)&3]
	b2 := key[(off+2)&3]
	b3 := key[(off+3)&3]

	var k64 uint64
	if isLittleEndian {
		k32 := uint32(b0) | (uint32(b1) << 8) | (uint32(b2) << 16) | (uint32(b3) << 24)
		k64 = uint64(k32) | (uint64(k32) << 32)
	} else {
		k32 := (uint32(b0) << 24) | (uint32(b1) << 16) | (uint32(b2) << 8) | uint32(b3)
		k64 = (uint64(k32) << 32) | uint64(k32)
	}

	// 64-byte unrolled SWAR core loop (8 uint64 words per iteration)
	for len(payload) >= 64 {
		*(*uint64)(unsafe.Pointer(&payload[0])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[8])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[16])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[24])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[32])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[40])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[48])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[56])) ^= k64
		payload = payload[64:]
	}

	// 8-byte SWAR loop
	for len(payload) >= 8 {
		*(*uint64)(unsafe.Pointer(&payload[0])) ^= k64
		payload = payload[8:]
	}

	// 4-byte chunk
	if len(payload) >= 4 {
		if isLittleEndian {
			*(*uint32)(unsafe.Pointer(&payload[0])) ^= uint32(k64)
		} else {
			*(*uint32)(unsafe.Pointer(&payload[0])) ^= uint32(k64 >> 32)
		}

		payload = payload[4:]
	}

	// Remaining 0-3 tail bytes
	for i := 0; i < len(payload); i++ {
		if isLittleEndian {
			payload[i] ^= byte(k64 >> (i * 8))
		} else {
			payload[i] ^= byte(k64 >> (56 - i*8))
		}
	}
}

// MaskScalar is the reference byte-by-byte masking implementation used for
// validation and benchmarking.
func MaskScalar(payload []byte, key [4]byte, offset int) {
	off := offset & 3
	for i := 0; i < len(payload); i++ {
		payload[i] ^= key[(off+i)&3]
	}
}
