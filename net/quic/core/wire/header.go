// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"encoding/binary"
	"fmt"

	"github.com/lemon4ksan/mach/net/quic/varint"
)

// PacketType identifies the type of a QUIC packet (RFC 9000 §17.2, RFC 9369 §4).
type PacketType byte

const (
	// PacketTypeInitial identifies an Initial packet.
	PacketTypeInitial PacketType = iota
	// PacketType0RTT identifies a 0-RTT protected packet.
	PacketType0RTT
	// PacketTypeHandshake identifies a Handshake packet.
	PacketTypeHandshake
	// PacketTypeRetry identifies a Retry packet.
	PacketTypeRetry
	// PacketTypeVersionNegotiation identifies a Version Negotiation packet.
	PacketTypeVersionNegotiation
	// PacketType1RTT identifies a 1-RTT Short Header packet.
	PacketType1RTT
)

// String returns the string representation of the packet type.
func (t PacketType) String() string {
	switch t {
	case PacketTypeInitial:
		return "Initial"
	case PacketType0RTT:
		return "0-RTT"
	case PacketTypeHandshake:
		return "Handshake"
	case PacketTypeRetry:
		return "Retry"
	case PacketTypeVersionNegotiation:
		return "Version Negotiation"
	case PacketType1RTT:
		return "1-RTT"
	default:
		return fmt.Sprintf("PacketType(%d)", byte(t))
	}
}

// IsLong reports whether the packet type uses a Long Header.
func (t PacketType) IsLong() bool {
	return t != PacketType1RTT
}

// Header represents a parsed QUIC packet header (RFC 9000 §17).
type Header struct {
	// IsLong indicates whether this is a Long Header (true) or Short Header (false).
	IsLong bool

	// Type is the QUIC packet type.
	Type PacketType

	// Version is the QUIC version (for Long Headers).
	Version Version

	// DestConnID is the destination connection ID (0 to 20 bytes).
	DestConnID []byte

	// SrcConnID is the source connection ID (0 to 20 bytes, Long Headers only).
	SrcConnID []byte

	// Token is the Initial token or Retry token.
	Token []byte

	// Length is the length of the packet number + payload + AEAD tag (varint, Long Headers only).
	Length uint64

	// PacketNumber is the full decoded packet number (RFC 9000 Appendix A).
	PacketNumber uint64

	// PacketNumberLen is the byte length of the encoded packet number (1 to 4).
	PacketNumberLen int

	// KeyPhase is the 1-RTT key phase bit (Short Headers only).
	KeyPhase bool

	// SpinBit is the 1-RTT latency spin bit (Short Headers only).
	SpinBit bool

	// Offsets and layout metadata for zero-copy packet protection and AEAD:
	HeaderLen          int // Total length of the header including packet number
	PacketNumberOffset int // Byte offset within packet where packet number begins
	PayloadLen         int // Total byte length of payload (ciphertext or plaintext)
}

// AppendHeader serializes the header into dst without heap allocations.
// It returns the updated dst slice, the byte offset where the packet number begins (pnOffset),
// and any error encountered.
func AppendHeader(dst []byte, h *Header) ([]byte, int, error) {
	if !h.IsLong {
		// Short Header (RFC 9000 §17.3)
		if len(h.DestConnID) > 20 {
			return dst, 0, ErrInvalidConnectionIDLen
		}
		if h.PacketNumberLen < 1 || h.PacketNumberLen > 4 {
			return dst, 0, ErrInvalidPacketNumber
		}

		firstByte := byte(0x40) // Fixed bit = 1
		if h.SpinBit {
			firstByte |= 0x20
		}
		if h.KeyPhase {
			firstByte |= 0x04
		}
		firstByte |= byte(h.PacketNumberLen - 1)

		dst = append(dst, firstByte)
		dst = append(dst, h.DestConnID...)
		pnOffset := len(dst)
		dst = AppendPacketNumber(dst, h.PacketNumber, h.PacketNumberLen)
		return dst, pnOffset, nil
	}

	// Long Header (RFC 9000 §17.2, RFC 9369 §4)
	if len(h.DestConnID) > 20 || len(h.SrcConnID) > 20 {
		return dst, 0, ErrInvalidConnectionIDLen
	}

	firstByte := byte(0xc0) // Header form (0x80) | Fixed bit (0x40)

	if h.Type == PacketTypeVersionNegotiation {
		firstByte = 0x80 | (firstByte & 0x0f) // Fixed bit arbitrary, RFC 9000 §17.2.1
		dst = append(dst, firstByte)
		dst = binary.BigEndian.AppendUint32(dst, 0) // Version 0
		dst = append(dst, byte(len(h.DestConnID)))
		dst = append(dst, h.DestConnID...)
		dst = append(dst, byte(len(h.SrcConnID)))
		dst = append(dst, h.SrcConnID...)
		return dst, 0, nil
	}

	var typeBits byte
	if h.Version == Version2 {
		// RFC 9369 §4 type bits: Initial=0b01, 0-RTT=0b10, Handshake=0b11, Retry=0b00
		switch h.Type {
		case PacketTypeRetry:
			typeBits = 0x00
		case PacketTypeInitial:
			typeBits = 0x10
		case PacketType0RTT:
			typeBits = 0x20
		case PacketTypeHandshake:
			typeBits = 0x30
		default:
			return dst, 0, ErrInvalidFrameType
		}
	} else {
		// RFC 9000 §17.2 type bits: Initial=0b00, 0-RTT=0b01, Handshake=0b10, Retry=0b11
		switch h.Type {
		case PacketTypeInitial:
			typeBits = 0x00
		case PacketType0RTT:
			typeBits = 0x10
		case PacketTypeHandshake:
			typeBits = 0x20
		case PacketTypeRetry:
			typeBits = 0x30
		default:
			return dst, 0, ErrInvalidFrameType
		}
	}

	firstByte |= typeBits

	if h.Type != PacketTypeRetry {
		if h.PacketNumberLen < 1 || h.PacketNumberLen > 4 {
			return dst, 0, ErrInvalidPacketNumber
		}
		firstByte |= byte(h.PacketNumberLen - 1)
	}

	dst = append(dst, firstByte)
	dst = binary.BigEndian.AppendUint32(dst, uint32(h.Version))
	dst = append(dst, byte(len(h.DestConnID)))
	dst = append(dst, h.DestConnID...)
	dst = append(dst, byte(len(h.SrcConnID)))
	dst = append(dst, h.SrcConnID...)

	switch h.Type {
	case PacketTypeInitial:
		dst = appendVarint(dst, uint64(len(h.Token)))
		dst = append(dst, h.Token...)
		dst = appendVarint(dst, h.Length)
		pnOffset := len(dst)
		dst = AppendPacketNumber(dst, h.PacketNumber, h.PacketNumberLen)
		return dst, pnOffset, nil

	case PacketType0RTT, PacketTypeHandshake:
		dst = appendVarint(dst, h.Length)
		pnOffset := len(dst)
		dst = AppendPacketNumber(dst, h.PacketNumber, h.PacketNumberLen)
		return dst, pnOffset, nil

	case PacketTypeRetry:
		dst = append(dst, h.Token...)
		return dst, 0, nil

	default:
		return dst, 0, ErrInvalidFrameType
	}
}

// ParseHeader parses an unprotected QUIC packet header from b.
// For Short Headers, destConnIDLen specifies the expected Destination Connection ID length.
// For Long Headers, destConnIDLen is ignored as length is read from the packet itself.
// largestAcked is used to decode the full packet number via DecodePacketNumber.
func ParseHeader(b []byte, destConnIDLen int, largestAcked uint64) (Header, error) {
	var h Header
	if len(b) < 1 {
		return h, ErrTruncatedPacket
	}

	firstByte := b[0]
	h.IsLong = (firstByte & 0x80) != 0

	if !h.IsLong {
		// Short Header (RFC 9000 §17.3)
		if (firstByte & 0x40) == 0 {
			return h, ErrInvalidFixedBit
		}
		if (firstByte & 0x18) != 0 {
			// Reserved bits must be 0
			return h, ErrInvalidReservedBits
		}

		h.Type = PacketType1RTT
		h.SpinBit = (firstByte & 0x20) != 0
		h.KeyPhase = (firstByte & 0x04) != 0
		h.PacketNumberLen = int(firstByte&0x03) + 1

		offset := 1
		if destConnIDLen > 20 || destConnIDLen < 0 {
			return h, ErrInvalidConnectionIDLen
		}
		if len(b) < offset+destConnIDLen+h.PacketNumberLen {
			return h, ErrTruncatedHeader
		}

		h.DestConnID = b[offset : offset+destConnIDLen]
		offset += destConnIDLen

		h.PacketNumberOffset = offset
		truncatedPN := ReadPacketNumber(b[offset:], h.PacketNumberLen)
		h.PacketNumber = DecodePacketNumber(largestAcked, truncatedPN, h.PacketNumberLen)
		offset += h.PacketNumberLen

		h.HeaderLen = offset
		h.PayloadLen = len(b) - offset
		return h, nil
	}

	// Long Header (RFC 9000 §17.2, RFC 9369 §4)
	if len(b) < 5 {
		return h, ErrTruncatedHeader
	}

	h.Version = Version(binary.BigEndian.Uint32(b[1:5]))
	offset := 5

	if h.Version == VersionNegotiation {
		h.Type = PacketTypeVersionNegotiation
		if len(b) < offset+2 {
			return h, ErrTruncatedHeader
		}
		dcidLen := int(b[offset])
		offset++
		if len(b) < offset+dcidLen+1 {
			return h, ErrTruncatedHeader
		}
		h.DestConnID = b[offset : offset+dcidLen]
		offset += dcidLen

		scidLen := int(b[offset])
		offset++
		if len(b) < offset+scidLen {
			return h, ErrTruncatedHeader
		}
		h.SrcConnID = b[offset : offset+scidLen]
		offset += scidLen

		h.HeaderLen = offset
		h.PayloadLen = len(b) - offset
		return h, nil
	}

	// For standard long headers, Fixed Bit must be 1
	if (firstByte & 0x40) == 0 {
		return h, ErrInvalidFixedBit
	}

	// Read Destination Connection ID
	if len(b) < offset+1 {
		return h, ErrTruncatedHeader
	}
	dcidLen := int(b[offset])
	offset++
	if dcidLen > 20 {
		return h, ErrInvalidConnectionIDLen
	}
	if len(b) < offset+dcidLen+1 {
		return h, ErrTruncatedHeader
	}
	h.DestConnID = b[offset : offset+dcidLen]
	offset += dcidLen

	// Read Source Connection ID
	scidLen := int(b[offset])
	offset++
	if scidLen > 20 {
		return h, ErrInvalidConnectionIDLen
	}
	if len(b) < offset+scidLen {
		return h, ErrTruncatedHeader
	}
	h.SrcConnID = b[offset : offset+scidLen]
	offset += scidLen

	typeBits := (firstByte & 0x30) >> 4
	if h.Version == Version2 {
		// RFC 9369 §4: Initial=0b01, 0-RTT=0b10, Handshake=0b11, Retry=0b00
		switch typeBits {
		case 0x0:
			h.Type = PacketTypeRetry
		case 0x1:
			h.Type = PacketTypeInitial
		case 0x2:
			h.Type = PacketType0RTT
		case 0x3:
			h.Type = PacketTypeHandshake
		}
	} else {
		// RFC 9000 §17.2: Initial=0b00, 0-RTT=0b01, Handshake=0b10, Retry=0b11
		switch typeBits {
		case 0x0:
			h.Type = PacketTypeInitial
		case 0x1:
			h.Type = PacketType0RTT
		case 0x2:
			h.Type = PacketTypeHandshake
		case 0x3:
			h.Type = PacketTypeRetry
		}
	}

	if h.Type == PacketTypeRetry {
		if len(b) < offset+16 { // Must at least contain 16-byte retry integrity tag
			return h, ErrTruncatedPacket
		}
		h.Token = b[offset : len(b)-16]
		h.HeaderLen = len(b) - 16
		h.PayloadLen = 16 // Retry integrity tag
		return h, nil
	}

	// For Initial, 0-RTT, Handshake: Reserved bits must be 0
	if (firstByte & 0x0c) != 0 {
		return h, ErrInvalidReservedBits
	}
	h.PacketNumberLen = int(firstByte&0x03) + 1

	if h.Type == PacketTypeInitial {
		tokenLen, n, err := varint.Decode(b[offset:])
		if err != nil {
			return h, ErrTruncatedHeader
		}
		offset += n
		if len(b) < offset+int(tokenLen) {
			return h, ErrTruncatedHeader
		}
		h.Token = b[offset : offset+int(tokenLen)]
		offset += int(tokenLen)
	}

	length, n, err := varint.Decode(b[offset:])
	if err != nil {
		return h, ErrTruncatedHeader
	}
	h.Length = length
	offset += n

	if len(b) < offset+h.PacketNumberLen {
		return h, ErrTruncatedHeader
	}

	h.PacketNumberOffset = offset
	truncatedPN := ReadPacketNumber(b[offset:], h.PacketNumberLen)
	h.PacketNumber = DecodePacketNumber(largestAcked, truncatedPN, h.PacketNumberLen)
	offset += h.PacketNumberLen

	h.HeaderLen = offset
	if uint64(len(b)-offset) < length-uint64(h.PacketNumberLen) {
		return h, ErrTruncatedPacket
	}
	h.PayloadLen = int(length) - h.PacketNumberLen
	return h, nil
}

// ParseHeaderOffsets parses the unprotected structural offsets of a protected packet header.
// It determines pnOffset (where packet number begins) and lengths without needing the header protection mask.
// This allows caller to extract the 16-byte sample and decrypt the packet number.
func ParseHeaderOffsets(b []byte, destConnIDLen int) (pnOffset int, isLong bool, typ PacketType, v Version, dcid, scid []byte, err error) {
	if len(b) < 1 {
		return 0, false, 0, 0, nil, nil, ErrTruncatedPacket
	}

	firstByte := b[0]
	isLong = (firstByte & 0x80) != 0

	if !isLong {
		// Short Header: 1 byte + destConnIDLen
		if destConnIDLen < 0 || destConnIDLen > 20 {
			return 0, false, 0, 0, nil, nil, ErrInvalidConnectionIDLen
		}
		if len(b) < 1+destConnIDLen+4 { // At least 1 byte first + CID + 4 bytes sample offset
			return 0, false, 0, 0, nil, nil, ErrTruncatedPacket
		}
		return 1 + destConnIDLen, false, PacketType1RTT, 0, b[1 : 1+destConnIDLen], nil, nil
	}

	if len(b) < 5 {
		return 0, true, 0, 0, nil, nil, ErrTruncatedHeader
	}
	v = Version(binary.BigEndian.Uint32(b[1:5]))
	offset := 5

	if v == VersionNegotiation {
		return 0, true, PacketTypeVersionNegotiation, v, nil, nil, nil
	}

	if len(b) < offset+1 {
		return 0, true, 0, v, nil, nil, ErrTruncatedHeader
	}
	dcidLen := int(b[offset])
	offset++
	if dcidLen > 20 || len(b) < offset+dcidLen+1 {
		return 0, true, 0, v, nil, nil, ErrTruncatedHeader
	}
	dcid = b[offset : offset+dcidLen]
	offset += dcidLen

	scidLen := int(b[offset])
	offset++
	if scidLen > 20 || len(b) < offset+scidLen {
		return 0, true, 0, v, nil, nil, ErrTruncatedHeader
	}
	scid = b[offset : offset+scidLen]
	offset += scidLen

	typeBits := (firstByte & 0x30) >> 4
	if v == Version2 {
		switch typeBits {
		case 0x0:
			typ = PacketTypeRetry
		case 0x1:
			typ = PacketTypeInitial
		case 0x2:
			typ = PacketType0RTT
		case 0x3:
			typ = PacketTypeHandshake
		}
	} else {
		switch typeBits {
		case 0x0:
			typ = PacketTypeInitial
		case 0x1:
			typ = PacketType0RTT
		case 0x2:
			typ = PacketTypeHandshake
		case 0x3:
			typ = PacketTypeRetry
		}
	}

	if typ == PacketTypeRetry {
		return 0, true, typ, v, dcid, scid, nil
	}

	if typ == PacketTypeInitial {
		tokenLen, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, true, typ, v, dcid, scid, ErrTruncatedHeader
		}
		offset += n + int(tokenLen)
		if len(b) < offset {
			return 0, true, typ, v, dcid, scid, ErrTruncatedHeader
		}
	}

	_, n, err := varint.Decode(b[offset:])
	if err != nil {
		return 0, true, typ, v, dcid, scid, ErrTruncatedHeader
	}
	offset += n

	return offset, true, typ, v, dcid, scid, nil
}

// appendVarint is an inlined, zero-allocation varint appender.
func appendVarint(dst []byte, val uint64) []byte {
	switch {
	case val <= 63:
		return append(dst, byte(val))
	case val <= 16383:
		return append(dst, byte(val>>8)|0x40, byte(val))
	case val <= 1073741823:
		return append(dst, byte(val>>24)|0x80, byte(val>>16), byte(val>>8), byte(val))
	case val <= varint.Max:
		return append(dst,
			byte(val>>56)|0xc0, byte(val>>48), byte(val>>40), byte(val>>32),
			byte(val>>24), byte(val>>16), byte(val>>8), byte(val))
	default:
		panic("wire: varint overflow")
	}
}
