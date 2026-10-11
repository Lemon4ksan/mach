// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"time"
)

// Transport Parameter IDs defined in RFC 9000 Section 18.2 and RFC 9221.
const (
	paramOriginalDestinationConnectionID = 0x00
	paramMaxIdleTimeout                  = 0x01
	paramStatelessResetToken             = 0x02
	paramMaxUDPPayloadSize               = 0x03
	paramInitialMaxData                  = 0x04
	paramInitialMaxStreamDataBidiLocal   = 0x05
	paramInitialMaxStreamDataBidiRemote  = 0x06
	paramInitialMaxStreamDataUni         = 0x07
	paramInitialMaxStreamsBidi           = 0x08
	paramInitialMaxStreamsUni            = 0x09
	paramAckDelayExponent                = 0x0a
	paramMaxAckDelay                     = 0x0b
	paramDisableActiveMigration          = 0x0c
	paramPreferredAddress                = 0x0d
	paramActiveConnectionIDLimit         = 0x0e
	paramInitialSourceConnectionID       = 0x0f
	paramRetrySourceConnectionID         = 0x10
	paramMaxDatagramFrameSize            = 0x20
)

// RFC 9000 default values for transport parameters.
const (
	defaultMaxUDPPayloadSize       uint64 = 65527
	defaultAckDelayExponent        uint8  = 3
	defaultMaxAckDelay                    = 25 * time.Millisecond
	defaultActiveConnectionIDLimit uint64 = 2
)

// PreferredAddress is the preferred_address transport parameter (RFC 9000 Section 18.2).
type PreferredAddress struct {
	IPv4                netip.AddrPort
	IPv6                netip.AddrPort
	ConnectionID        ConnectionID
	StatelessResetToken [16]byte
}

// TransportParameters contains QUIC transport parameters exchanged in the TLS handshake (RFC 9000 §18.2).
type TransportParameters struct {
	// Connection IDs
	OriginalDestinationConnectionID ConnectionID
	InitialSourceConnectionID       ConnectionID
	RetrySourceConnectionID         ConnectionID

	// Server-only security tokens & addressing
	StatelessResetToken [16]byte
	PreferredAddress    PreferredAddress

	// Flow control & resource limits
	MaxIdleTimeout                 time.Duration
	MaxUDPPayloadSize              uint64 // default 65527, min 1200
	InitialMaxData                 uint64
	InitialMaxStreamDataBidiLocal  uint64
	InitialMaxStreamDataBidiRemote uint64
	InitialMaxStreamDataUni        uint64
	InitialMaxStreamsBidi          uint64
	InitialMaxStreamsUni           uint64
	AckDelayExponent               uint8         // default 3, max 20
	MaxAckDelay                    time.Duration // default 25ms, max 16383ms
	ActiveConnectionIDLimit        uint64        // default 2, min 2
	MaxDatagramFrameSize           uint64        // RFC 9221

	DisableActiveMigration bool

	// Presence flags
	HasOriginalDestinationConnectionID bool
	HasInitialSourceConnectionID       bool
	HasRetrySourceConnectionID         bool
	HasStatelessResetToken             bool
	HasPreferredAddress                bool
	HasMaxIdleTimeout                  bool
	HasMaxDatagramFrameSize            bool
}

// DefaultTransportParameters returns transport parameters initialized to RFC 9000 defaults.
func DefaultTransportParameters() TransportParameters {
	return TransportParameters{
		MaxUDPPayloadSize:       defaultMaxUDPPayloadSize,
		AckDelayExponent:        defaultAckDelayExponent,
		MaxAckDelay:             defaultMaxAckDelay,
		ActiveConnectionIDLimit: defaultActiveConnectionIDLimit,
	}
}

// TransportParameterError represents a QUIC TRANSPORT_PARAMETER_ERROR (0x08).
type TransportParameterError struct {
	Reason string
}

func (e *TransportParameterError) Error() string {
	return "transport parameter error: " + e.Reason
}

func tpError(format string, args ...any) error {
	return &TransportParameterError{Reason: fmt.Sprintf(format, args...)}
}

// Append serializes transport parameters to b with zero allocations.
func (p *TransportParameters) Append(b []byte, pers Perspective) []byte {
	// Server-only parameters
	if pers == PerspectiveServer {
		if p.HasOriginalDestinationConnectionID {
			b = appendBytesParam(b, paramOriginalDestinationConnectionID, p.OriginalDestinationConnectionID.Bytes())
		}
		if p.HasStatelessResetToken {
			b = appendBytesParam(b, paramStatelessResetToken, p.StatelessResetToken[:])
		}
		if p.HasPreferredAddress {
			b = p.appendPreferredAddress(b)
		}
		if p.HasRetrySourceConnectionID {
			b = appendBytesParam(b, paramRetrySourceConnectionID, p.RetrySourceConnectionID.Bytes())
		}
	}

	// Parameters sent by both client and server
	if p.HasInitialSourceConnectionID {
		b = appendBytesParam(b, paramInitialSourceConnectionID, p.InitialSourceConnectionID.Bytes())
	}
	if p.HasMaxIdleTimeout && p.MaxIdleTimeout > 0 {
		b = appendVarintParam(b, paramMaxIdleTimeout, uint64(p.MaxIdleTimeout.Milliseconds()))
	}
	if p.MaxUDPPayloadSize != 0 && p.MaxUDPPayloadSize != defaultMaxUDPPayloadSize {
		b = appendVarintParam(b, paramMaxUDPPayloadSize, p.MaxUDPPayloadSize)
	}
	if p.InitialMaxData > 0 {
		b = appendVarintParam(b, paramInitialMaxData, p.InitialMaxData)
	}
	if p.InitialMaxStreamDataBidiLocal > 0 {
		b = appendVarintParam(b, paramInitialMaxStreamDataBidiLocal, p.InitialMaxStreamDataBidiLocal)
	}
	if p.InitialMaxStreamDataBidiRemote > 0 {
		b = appendVarintParam(b, paramInitialMaxStreamDataBidiRemote, p.InitialMaxStreamDataBidiRemote)
	}
	if p.InitialMaxStreamDataUni > 0 {
		b = appendVarintParam(b, paramInitialMaxStreamDataUni, p.InitialMaxStreamDataUni)
	}
	if p.InitialMaxStreamsBidi > 0 {
		b = appendVarintParam(b, paramInitialMaxStreamsBidi, p.InitialMaxStreamsBidi)
	}
	if p.InitialMaxStreamsUni > 0 {
		b = appendVarintParam(b, paramInitialMaxStreamsUni, p.InitialMaxStreamsUni)
	}
	if p.AckDelayExponent != defaultAckDelayExponent && p.AckDelayExponent != 0 {
		b = appendVarintParam(b, paramAckDelayExponent, uint64(p.AckDelayExponent))
	}
	if p.MaxAckDelay != defaultMaxAckDelay && p.MaxAckDelay > 0 {
		b = appendVarintParam(b, paramMaxAckDelay, uint64(p.MaxAckDelay.Milliseconds()))
	}
	if p.DisableActiveMigration {
		b = appendVarint(b, paramDisableActiveMigration)
		b = appendVarint(b, 0) // zero length
	}
	if p.ActiveConnectionIDLimit != defaultActiveConnectionIDLimit && p.ActiveConnectionIDLimit != 0 {
		b = appendVarintParam(b, paramActiveConnectionIDLimit, p.ActiveConnectionIDLimit)
	}
	if p.HasMaxDatagramFrameSize {
		b = appendVarintParam(b, paramMaxDatagramFrameSize, p.MaxDatagramFrameSize)
	}

	return b
}

// Marshal serializes transport parameters into a newly allocated slice.
func (p *TransportParameters) Marshal(pers Perspective) []byte {
	return p.Append(make([]byte, 0, 128), pers)
}

func (p *TransportParameters) appendPreferredAddress(b []byte) []byte {
	cidLen := p.PreferredAddress.ConnectionID.Len()
	paramLen := 4 + 2 + 16 + 2 + 1 + cidLen + 16

	b = appendVarint(b, paramPreferredAddress)
	b = appendVarint(b, uint64(paramLen))

	// IPv4 Address + Port
	ipv4 := p.PreferredAddress.IPv4.Addr().As4()
	b = append(b, ipv4[:]...)
	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], p.PreferredAddress.IPv4.Port())
	b = append(b, portBuf[:]...)

	// IPv6 Address + Port
	ipv6 := p.PreferredAddress.IPv6.Addr().As16()
	b = append(b, ipv6[:]...)
	binary.BigEndian.PutUint16(portBuf[:], p.PreferredAddress.IPv6.Port())
	b = append(b, portBuf[:]...)

	// Connection ID
	b = append(b, byte(cidLen))
	b = append(b, p.PreferredAddress.ConnectionID.Bytes()...)

	// Stateless reset token
	b = append(b, p.PreferredAddress.StatelessResetToken[:]...)

	return b
}

func appendVarintParam(b []byte, id uint64, val uint64) []byte {
	vLen := varintLen(val)
	b = appendVarint(b, id)
	b = appendVarint(b, uint64(vLen))
	b = appendVarint(b, val)
	return b
}

func appendBytesParam(b []byte, id uint64, data []byte) []byte {
	b = appendVarint(b, id)
	b = appendVarint(b, uint64(len(data)))
	b = append(b, data...)
	return b
}

// Unmarshal decodes transport parameters from data with zero heap allocations.
func (p *TransportParameters) Unmarshal(data []byte, sentBy Perspective) error {
	*p = DefaultTransportParameters()

	// Bitmask for duplicate detection of IDs < 64 (zero heap allocations)
	var seenMask uint64
	// Stack array for duplicate detection of extension IDs >= 64
	var seenExt [8]uint64
	var numExt int

	for len(data) > 0 {
		id, n, err := readVarint(data)
		if err != nil {
			return tpError("failed to read parameter ID: %v", err)
		}
		data = data[n:]

		// Duplicate detection
		if id < 64 {
			bit := uint64(1) << id
			if (seenMask & bit) != 0 {
				return tpError("duplicate transport parameter 0x%x", id)
			}
			seenMask |= bit
		} else {
			for i := 0; i < numExt; i++ {
				if seenExt[i] == id {
					return tpError("duplicate transport parameter 0x%x", id)
				}
			}
			if numExt < len(seenExt) {
				seenExt[numExt] = id
				numExt++
			}
		}

		length, n, err := readVarint(data)
		if err != nil {
			return tpError("failed to read parameter length: %v", err)
		}
		data = data[n:]

		if uint64(len(data)) < length {
			return tpError("unexpected EOF for parameter 0x%x (needed %d, have %d)", id, length, len(data))
		}
		paramData := data[:length]
		data = data[length:]

		// Server-only parameter validation: client MUST NOT send them
		if sentBy == PerspectiveClient {
			switch id {
			case paramOriginalDestinationConnectionID,
				paramStatelessResetToken,
				paramPreferredAddress,
				paramRetrySourceConnectionID:
				return tpError("client sent server-only parameter 0x%x", id)
			}
		}

		switch id {
		case paramOriginalDestinationConnectionID:
			if len(paramData) > MaxConnectionIDLen {
				return tpError("original destination connection ID too long: %d", len(paramData))
			}
			p.OriginalDestinationConnectionID = NewConnectionID(paramData)
			p.HasOriginalDestinationConnectionID = true

		case paramInitialSourceConnectionID:
			if len(paramData) > MaxConnectionIDLen {
				return tpError("initial source connection ID too long: %d", len(paramData))
			}
			p.InitialSourceConnectionID = NewConnectionID(paramData)
			p.HasInitialSourceConnectionID = true

		case paramRetrySourceConnectionID:
			if len(paramData) > MaxConnectionIDLen {
				return tpError("retry source connection ID too long: %d", len(paramData))
			}
			p.RetrySourceConnectionID = NewConnectionID(paramData)
			p.HasRetrySourceConnectionID = true

		case paramStatelessResetToken:
			if len(paramData) != 16 {
				return tpError("invalid stateless reset token length: %d (expected 16)", len(paramData))
			}
			copy(p.StatelessResetToken[:], paramData)
			p.HasStatelessResetToken = true

		case paramMaxIdleTimeout:
			v, _, err := readVarint(paramData)
			if err != nil {
				return tpError("malformed max_idle_timeout: %v", err)
			}
			p.MaxIdleTimeout = time.Duration(v) * time.Millisecond
			p.HasMaxIdleTimeout = true

		case paramMaxUDPPayloadSize:
			v, _, err := readVarint(paramData)
			if err != nil {
				return tpError("malformed max_udp_payload_size: %v", err)
			}
			if v < 1200 {
				return tpError("max_udp_payload_size must be >= 1200, got %d", v)
			}
			p.MaxUDPPayloadSize = v

		case paramInitialMaxData:
			v, _, err := readVarint(paramData)
			if err != nil {
				return tpError("malformed initial_max_data: %v", err)
			}
			p.InitialMaxData = v

		case paramInitialMaxStreamDataBidiLocal:
			v, _, err := readVarint(paramData)
			if err != nil {
				return tpError("malformed initial_max_stream_data_bidi_local: %v", err)
			}
			p.InitialMaxStreamDataBidiLocal = v

		case paramInitialMaxStreamDataBidiRemote:
			v, _, err := readVarint(paramData)
			if err != nil {
				return tpError("malformed initial_max_stream_data_bidi_remote: %v", err)
			}
			p.InitialMaxStreamDataBidiRemote = v

		case paramInitialMaxStreamDataUni:
			v, _, err := readVarint(paramData)
			if err != nil {
				return tpError("malformed initial_max_stream_data_uni: %v", err)
			}
			p.InitialMaxStreamDataUni = v

		case paramInitialMaxStreamsBidi:
			v, _, err := readVarint(paramData)
			if err != nil {
				return tpError("malformed initial_max_streams_bidi: %v", err)
			}
			p.InitialMaxStreamsBidi = v

		case paramInitialMaxStreamsUni:
			v, _, err := readVarint(paramData)
			if err != nil {
				return tpError("malformed initial_max_streams_uni: %v", err)
			}
			p.InitialMaxStreamsUni = v

		case paramAckDelayExponent:
			v, _, err := readVarint(paramData)
			if err != nil {
				return tpError("malformed ack_delay_exponent: %v", err)
			}
			if v > 20 {
				return tpError("ack_delay_exponent must be <= 20, got %d", v)
			}
			p.AckDelayExponent = uint8(v)

		case paramMaxAckDelay:
			v, _, err := readVarint(paramData)
			if err != nil {
				return tpError("malformed max_ack_delay: %v", err)
			}
			if v >= (1 << 14) {
				return tpError("max_ack_delay must be < 2^14 ms, got %d", v)
			}
			p.MaxAckDelay = time.Duration(v) * time.Millisecond

		case paramDisableActiveMigration:
			if len(paramData) != 0 {
				return tpError("disable_active_migration must have length 0, got %d", len(paramData))
			}
			p.DisableActiveMigration = true

		case paramActiveConnectionIDLimit:
			v, _, err := readVarint(paramData)
			if err != nil {
				return tpError("malformed active_connection_id_limit: %v", err)
			}
			if v < 2 {
				return tpError("active_connection_id_limit must be >= 2, got %d", v)
			}
			p.ActiveConnectionIDLimit = v

		case paramPreferredAddress:
			if err := p.unmarshalPreferredAddress(paramData); err != nil {
				return err
			}
			p.HasPreferredAddress = true

		case paramMaxDatagramFrameSize:
			v, _, err := readVarint(paramData)
			if err != nil {
				return tpError("malformed max_datagram_frame_size: %v", err)
			}
			p.MaxDatagramFrameSize = v
			p.HasMaxDatagramFrameSize = true

		default:
			// RFC 9000 Section 18: Endpoints MUST ignore unknown transport parameters
		}
	}

	return nil
}

func (p *TransportParameters) unmarshalPreferredAddress(b []byte) error {
	// 4 (IPv4) + 2 (port) + 16 (IPv6) + 2 (port) + 1 (cidLen) + 16 (stateless reset token) = 41
	if len(b) < 41 {
		return tpError("preferred_address parameter too short: %d", len(b))
	}

	// IPv4
	ipv4Bytes := [4]byte(b[0:4])
	ipv4Port := binary.BigEndian.Uint16(b[4:6])
	p.PreferredAddress.IPv4 = netip.AddrPortFrom(netip.AddrFrom4(ipv4Bytes), ipv4Port)

	// IPv6
	ipv6Bytes := [16]byte(b[6:22])
	ipv6Port := binary.BigEndian.Uint16(b[22:24])
	p.PreferredAddress.IPv6 = netip.AddrPortFrom(netip.AddrFrom16(ipv6Bytes), ipv6Port)

	// Connection ID
	cidLen := int(b[24])
	if 41+cidLen != len(b) {
		return tpError("preferred_address length mismatch: total %d, expected %d", len(b), 41+cidLen)
	}
	if cidLen > MaxConnectionIDLen {
		return tpError("preferred_address connection ID too long: %d", cidLen)
	}
	p.PreferredAddress.ConnectionID = NewConnectionID(b[25 : 25+cidLen])

	// Stateless Reset Token
	copy(p.PreferredAddress.StatelessResetToken[:], b[25+cidLen:41+cidLen])

	return nil
}
