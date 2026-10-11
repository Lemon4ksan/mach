// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"errors"
	"io"

	"github.com/lemon4ksan/mach/net/quic/internal/protocol"
	"github.com/lemon4ksan/mach/net/quic/internal/utils"
)

// ParseShortHeader parses a 1-RTT Short Header packet (RFC 9000 §17.3).
//
// Short Header Format (RFC 9000 §17.3.1):
//   - Header Form (bit 7): 0 for Short Header (RFC 9000 §17.3.1)
//   - Fixed Bit (bit 6): MUST be 1 (RFC 9000 §17.3.1)
//   - Spin Bit (bit 5): Latency spin bit (RFC 9000 §17.4)
//   - Reserved Bits (bits 4-3): MUST be 00 (RFC 9000 §17.3.1)
//   - Key Phase (bit 2): 1-RTT key phase bit (RFC 9000 §17.3.1, RFC 9001 §6)
//   - Packet Number Length (bits 1-0): Packet number length (1 to 4 bytes, RFC 9000 §17.1)
//   - Destination Connection ID (0..160 bits): Length determined by connIDLen
//   - Packet Number (1..4 bytes): Truncated packet number (RFC 9000 §17.1)
//
// It must be called after header protection was removed (RFC 9001 §5.4).
// If the reserved bits are invalid, ErrInvalidReservedBits is returned while continuing parsing (RFC 9001 §5.4.1).
func ParseShortHeader(
	data []byte,
	connIDLen int,
) (length int, _ protocol.PacketNumber, _ protocol.PacketNumberLen, _ protocol.KeyPhaseBit, _ error) {
	if len(data) == 0 {
		return 0, 0, 0, 0, io.EOF
	}

	if data[0]&0x80 > 0 {
		return 0, 0, 0, 0, errors.New("not a short header packet")
	}

	if data[0]&0x40 == 0 {
		return 0, 0, 0, 0, errors.New("not a QUIC packet")
	}

	pnLen := protocol.PacketNumberLen(data[0]&0b11) + 1
	if len(data) < 1+int(pnLen)+connIDLen {
		return 0, 0, 0, 0, io.EOF
	}

	pos := 1 + connIDLen

	pn, err := readPacketNumber(data[pos:], pnLen)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	kp := protocol.KeyPhaseZero
	if data[0]&0b100 > 0 {
		kp = protocol.KeyPhaseOne
	}

	if data[0]&0x18 != 0 {
		err = ErrInvalidReservedBits
	}

	return 1 + connIDLen + int(pnLen), pn, pnLen, kp, err
}

// AppendShortHeader writes a short header.
func AppendShortHeader(
	b []byte,
	connID protocol.ConnectionID,
	pn protocol.PacketNumber,
	pnLen protocol.PacketNumberLen,
	kp protocol.KeyPhaseBit,
) ([]byte, error) {
	typeByte := 0x40 | uint8(pnLen-1)
	if kp == protocol.KeyPhaseOne {
		typeByte |= byte(1 << 2)
	}

	b = append(b, typeByte)
	b = append(b, connID.Bytes()...)

	return appendPacketNumber(b, pn, pnLen)
}

func ShortHeaderLen(dest protocol.ConnectionID, pnLen protocol.PacketNumberLen) protocol.ByteCount {
	return 1 + protocol.ByteCount(dest.Len()) + protocol.ByteCount(pnLen)
}

func LogShortHeader(
	logger utils.Logger,
	dest protocol.ConnectionID,
	pn protocol.PacketNumber,
	pnLen protocol.PacketNumberLen,
	kp protocol.KeyPhaseBit,
) {
	logger.Debugf(
		"\tShort Header{DestConnectionID: %s, PacketNumber: %d, PacketNumberLen: %d, KeyPhase: %s}",
		dest,
		pn,
		pnLen,
		kp,
	)
}
