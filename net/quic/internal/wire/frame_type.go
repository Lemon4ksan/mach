// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import "github.com/lemon4ksan/mach/net/quic/internal/protocol"

type FrameType uint64

// These constants correspond to those defined in RFC 9000 §12.4 Table 3 and §19.
// Stream frame types (0x08-0x0f) are defined in RFC 9000 §19.8; use FrameType.IsStreamFrameType() to identify them.
const (
	FrameTypePing        FrameType = 0x1 // RFC 9000 §19.2
	FrameTypeAck         FrameType = 0x2 // RFC 9000 §19.3
	FrameTypeAckECN      FrameType = 0x3 // RFC 9000 §19.3 & §19.3.2
	FrameTypeResetStream FrameType = 0x4 // RFC 9000 §19.4
	FrameTypeStopSending FrameType = 0x5 // RFC 9000 §19.5
	FrameTypeCrypto      FrameType = 0x6 // RFC 9000 §19.6
	FrameTypeNewToken    FrameType = 0x7 // RFC 9000 §19.7

	FrameTypeMaxData            FrameType = 0x10 // RFC 9000 §19.9
	FrameTypeMaxStreamData      FrameType = 0x11 // RFC 9000 §19.10
	FrameTypeBidiMaxStreams     FrameType = 0x12 // RFC 9000 §19.11
	FrameTypeUniMaxStreams      FrameType = 0x13 // RFC 9000 §19.11
	FrameTypeDataBlocked        FrameType = 0x14 // RFC 9000 §19.12
	FrameTypeStreamDataBlocked  FrameType = 0x15 // RFC 9000 §19.13
	FrameTypeBidiStreamBlocked  FrameType = 0x16 // RFC 9000 §19.14
	FrameTypeUniStreamBlocked   FrameType = 0x17 // RFC 9000 §19.14
	FrameTypeNewConnectionID    FrameType = 0x18 // RFC 9000 §19.15
	FrameTypeRetireConnectionID FrameType = 0x19 // RFC 9000 §19.16
	FrameTypePathChallenge      FrameType = 0x1a // RFC 9000 §19.17
	FrameTypePathResponse       FrameType = 0x1b // RFC 9000 §19.18
	FrameTypePathAbandon        FrameType = 0x152
	FrameTypeConnectionClose    FrameType = 0x1c // RFC 9000 §19.19
	FrameTypeApplicationClose   FrameType = 0x1d // RFC 9000 §19.19
	FrameTypeHandshakeDone      FrameType = 0x1e // RFC 9000 §19.20
	// https://datatracker.ietf.org/doc/draft-ietf-quic-reliable-stream-reset/09/
	FrameTypeResetStreamAt FrameType = 0x24
	FrameTypeAckFrequency  FrameType = 0xaf
	FrameTypeImmediateAck  FrameType = 0x1f

	FrameTypeDatagramNoLength   FrameType = 0x30 // RFC 9221 §4
	FrameTypeDatagramWithLength FrameType = 0x31 // RFC 9221 §4
)

func (t FrameType) IsStreamFrameType() bool {
	return t >= 0x8 && t <= 0xf
}

func (t FrameType) isValidRFC9000() bool {
	return t <= 0x1e
}

func (t FrameType) IsAckFrameType() bool {
	return t == FrameTypeAck || t == FrameTypeAckECN
}

func (t FrameType) IsDatagramFrameType() bool {
	return t == FrameTypeDatagramNoLength || t == FrameTypeDatagramWithLength
}

func (t FrameType) isAllowedAtEncLevel(encLevel protocol.EncryptionLevel) bool {
	//nolint:exhaustive
	switch encLevel {
	case protocol.EncryptionInitial, protocol.EncryptionHandshake:
		switch t {
		case FrameTypeCrypto, FrameTypeAck, FrameTypeAckECN, FrameTypeConnectionClose, FrameTypePing:
			return true
		default:
			return false
		}

	case protocol.Encryption0RTT:
		switch t {
		case FrameTypeCrypto,
			FrameTypeAck,
			FrameTypeAckECN,
			FrameTypeConnectionClose,
			FrameTypeNewToken,
			FrameTypePathResponse,
			FrameTypeRetireConnectionID,
			FrameTypePathAbandon:
			return false
		default:
			return true
		}

	case protocol.Encryption1RTT:
		return true
	default:
		panic("unknown encryption level")
	}
}
