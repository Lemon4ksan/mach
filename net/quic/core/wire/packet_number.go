// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import "encoding/binary"

const (
	// InvalidPacketNumber is a sentinel value indicating an uninitialized or invalid packet number.
	InvalidPacketNumber uint64 = ^uint64(0)

	// MaxPacketNumber is the maximum representable QUIC packet number (2^62 - 1) per RFC 9000 §17.1.
	MaxPacketNumber uint64 = (1 << 62) - 1
)

// DecodePacketNumber reconstructs the full 62-bit packet number from a truncated packet number,
// the largest packet number successfully authenticated so far, and the packet number length in bytes (RFC 9000 Appendix A).
func DecodePacketNumber(largestPN uint64, truncatedPN uint64, pnLen int) uint64 {
	pnNbits := uint(pnLen * 8)
	var expectedPN uint64
	if largestPN == InvalidPacketNumber {
		expectedPN = 0
	} else {
		expectedPN = largestPN + 1
	}

	pnWin := uint64(1) << pnNbits
	pnHwin := pnWin / 2
	pnMask := pnWin - 1

	candidatePN := (expectedPN &^ pnMask) | truncatedPN
	if candidatePN+pnHwin <= expectedPN && candidatePN < (uint64(1)<<62)-pnWin {
		return candidatePN + pnWin
	}
	if candidatePN > expectedPN+pnHwin && candidatePN >= pnWin {
		return candidatePN - pnWin
	}
	return candidatePN
}

// PacketNumberLength returns the minimum packet number length in bytes (1 to 4) required
// to safely encode pn given the largest acknowledged packet number (RFC 9000 §17.1).
func PacketNumberLength(pn uint64, largestAcked uint64) int {
	var diff uint64
	if largestAcked == InvalidPacketNumber || pn < largestAcked {
		diff = pn + 1
	} else {
		diff = pn - largestAcked
	}

	// Double the difference to accommodate the window requirement: 2 * diff < 2^(8*len)
	diff *= 2
	switch {
	case diff < (1 << 8):
		return 1
	case diff < (1 << 16):
		return 2
	case diff < (1 << 24):
		return 3
	default:
		return 4
	}
}

// EncodePacketNumber returns the truncated packet number and required byte length (1-4).
func EncodePacketNumber(pn uint64, largestAcked uint64) (uint64, int) {
	pnLen := PacketNumberLength(pn, largestAcked)
	var mask uint64
	switch pnLen {
	case 1:
		mask = 0xff
	case 2:
		mask = 0xffff
	case 3:
		mask = 0xffffff
	default:
		mask = 0xffffffff
	}
	return pn & mask, pnLen
}

// AppendPacketNumber appends a packet number of the specified byte length (1-4) in network byte order to dst.
func AppendPacketNumber(dst []byte, pn uint64, pnLen int) []byte {
	switch pnLen {
	case 1:
		return append(dst, byte(pn))
	case 2:
		return append(dst, byte(pn>>8), byte(pn))
	case 3:
		return append(dst, byte(pn>>16), byte(pn>>8), byte(pn))
	case 4:
		return append(dst, byte(pn>>24), byte(pn>>16), byte(pn>>8), byte(pn))
	default:
		panic("wire: invalid packet number length")
	}
}

// ReadPacketNumber decodes a truncated packet number of length pnLen bytes from b in network byte order.
func ReadPacketNumber(b []byte, pnLen int) uint64 {
	switch pnLen {
	case 1:
		return uint64(b[0])
	case 2:
		return uint64(binary.BigEndian.Uint16(b[:2]))
	case 3:
		return uint64(b[0])<<16 | uint64(b[1])<<8 | uint64(b[2])
	case 4:
		return uint64(binary.BigEndian.Uint32(b[:4]))
	default:
		panic("wire: invalid packet number length")
	}
}
