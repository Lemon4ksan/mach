// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package protocol

// A PacketNumber in QUIC
type PacketNumber int64

// InvalidPacketNumber is a packet number that is never sent.
// In QUIC, 0 is a valid packet number.
const InvalidPacketNumber PacketNumber = -1

// PacketNumberLen is the length of the packet number in bytes
type PacketNumberLen uint8

const (
	// PacketNumberLen1 is a packet number length of 1 byte
	PacketNumberLen1 PacketNumberLen = 1
	// PacketNumberLen2 is a packet number length of 2 bytes
	PacketNumberLen2 PacketNumberLen = 2
	// PacketNumberLen3 is a packet number length of 3 bytes
	PacketNumberLen3 PacketNumberLen = 3
	// PacketNumberLen4 is a packet number length of 4 bytes
	PacketNumberLen4 PacketNumberLen = 4
)

// DecodePacketNumber calculates the full packet number based on its truncated length and the largest acknowledged packet number (RFC 9000 §17.1 & Appendix A.3).
func DecodePacketNumber(length PacketNumberLen, largest, truncated PacketNumber) PacketNumber {
	expected := largest + 1
	win := PacketNumber(1 << (length * 8))
	hwin := win / 2
	mask := win - 1

	candidate := (expected & ^mask) | truncated
	if candidate <= expected-hwin && candidate < 1<<62-win {
		return candidate + win
	}

	if candidate > expected+hwin && candidate >= win {
		return candidate - win
	}

	return candidate
}

// PacketNumberLengthForHeader determines the minimum packet number length needed to prevent ambiguity (RFC 9000 §17.1 & Appendix A.2).
// It never chooses a PacketNumberLen of 1 byte, since this is too short under certain loss recovery circumstances.
func PacketNumberLengthForHeader(pn, largestAcked PacketNumber) PacketNumberLen {
	var numUnacked PacketNumber
	if largestAcked == InvalidPacketNumber {
		numUnacked = pn + 1
	} else {
		numUnacked = pn - largestAcked
	}

	if numUnacked < 1<<(16-1) {
		return PacketNumberLen2
	}

	if numUnacked < 1<<(24-1) {
		return PacketNumberLen3
	}

	return PacketNumberLen4
}
