// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"unsafe"
)

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

	// For tiny payloads (< 8 bytes), run unrolled scalar XOR directly to avoid SWAR setup overhead.
	if n < 8 {
		switch n {
		case 7:
			payload[6] ^= key[(off+6)&3]
			fallthrough
		case 6:
			payload[5] ^= key[(off+5)&3]
			fallthrough
		case 5:
			payload[4] ^= key[(off+4)&3]
			fallthrough
		case 4:
			payload[3] ^= key[(off+3)&3]
			fallthrough
		case 3:
			payload[2] ^= key[(off+2)&3]
			fallthrough
		case 2:
			payload[1] ^= key[(off+1)&3]
			fallthrough
		case 1:
			payload[0] ^= key[off]
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

	// Construct repeated 8-byte mask word for current key phase purely in registers.
	b0 := uint32(key[off])
	b1 := uint32(key[(off+1)&3])
	b2 := uint32(key[(off+2)&3])
	b3 := uint32(key[(off+3)&3])

	k32 := b0 | (b1 << 8) | (b2 << 16) | (b3 << 24)
	k64 := uint64(k32) | (uint64(k32) << 32)

	// 128-byte unrolled SWAR core loop (16 uint64 words per iteration)
	for len(payload) >= 128 {
		*(*uint64)(unsafe.Pointer(&payload[0])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[8])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[16])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[24])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[32])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[40])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[48])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[56])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[64])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[72])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[80])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[88])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[96])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[104])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[112])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[120])) ^= k64
		payload = payload[128:]
	}

	// 64-byte block
	if len(payload) >= 64 {
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

	// 32-byte block
	if len(payload) >= 32 {
		*(*uint64)(unsafe.Pointer(&payload[0])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[8])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[16])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[24])) ^= k64
		payload = payload[32:]
	}

	// 16-byte block
	if len(payload) >= 16 {
		*(*uint64)(unsafe.Pointer(&payload[0])) ^= k64
		*(*uint64)(unsafe.Pointer(&payload[8])) ^= k64
		payload = payload[16:]
	}

	// 8-byte block
	if len(payload) >= 8 {
		*(*uint64)(unsafe.Pointer(&payload[0])) ^= k64
		payload = payload[8:]
	}

	// 4-byte chunk
	if len(payload) >= 4 {
		*(*uint32)(unsafe.Pointer(&payload[0])) ^= k32
		payload = payload[4:]
	}

	// Remaining 0-3 tail bytes
	switch len(payload) {
	case 3:
		payload[0] ^= byte(b0)
		payload[1] ^= byte(b1)
		payload[2] ^= byte(b2)
	case 2:
		payload[0] ^= byte(b0)
		payload[1] ^= byte(b1)
	case 1:
		payload[0] ^= byte(b0)
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
