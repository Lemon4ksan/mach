// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"errors"
	"io"

	"github.com/lemon4ksan/foundation/encoding/varint"
)

var ErrH3SettingsError = errors.New("aoni/h3engine: reserved H2 setting ID in H3 SETTINGS frame (RFC 9114 §7.2.4.1)")

// HTTP/3 Frame Types (RFC 9114 §7.2 & §11.2.1 Table 2).
const (
	// FrameTypeData conveys arbitrary sequences of bytes for request/response content (RFC 9114 §7.2.1: 0x00).
	FrameTypeData uint64 = 0x00

	// FrameTypeHeaders carries QPACK-compressed HTTP field sections (RFC 9114 §7.2.2: 0x01).
	FrameTypeHeaders uint64 = 0x01

	// FrameTypeCancelPush requests cancellation of a server push prior to stream receipt (RFC 9114 §7.2.3: 0x03).
	FrameTypeCancelPush uint64 = 0x03

	// FrameTypeSettings conveys connection configuration parameters on control stream (RFC 9114 §7.2.4: 0x04).
	FrameTypeSettings uint64 = 0x04

	// FrameTypePushPromise carries promised request headers from server to client (RFC 9114 §7.2.5: 0x05).
	FrameTypePushPromise uint64 = 0x05

	// FrameTypeGoAway initiates graceful connection shutdown (RFC 9114 §7.2.6: 0x07).
	FrameTypeGoAway uint64 = 0x07

	// FrameTypeMaxPushID controls the maximum server push ID allowed by client (RFC 9114 §7.2.7: 0x0d).
	FrameTypeMaxPushID uint64 = 0x0D

	// FrameTypeWebTransportStream establishes a WebTransport stream (WebTransport over HTTP/3 §4.1: 0x41).
	FrameTypeWebTransportStream uint64 = 0x41
)

// HTTP/3 Unidirectional Stream Types (RFC 9114 §6.2 & §11.2.4 Table 5).
const (
	// StreamTypeControl establishes the unidirectional HTTP/3 control stream (RFC 9114 §6.2.1: 0x00).
	StreamTypeControl uint64 = 0x00

	// StreamTypePush establishes a server-initiated push stream (RFC 9114 §6.2.2: 0x01).
	StreamTypePush uint64 = 0x01

	// StreamTypeQPACKEncoder establishes the QPACK encoder unidirectional stream (RFC 9204 §4.2: 0x02).
	StreamTypeQPACKEncoder uint64 = 0x02

	// StreamTypeQPACKDecoder establishes the QPACK decoder unidirectional stream (RFC 9204 §4.2: 0x03).
	StreamTypeQPACKDecoder uint64 = 0x03

	// StreamTypeWebTransport establishes a WebTransport unidirectional stream (WebTransport over HTTP/3 §4.2: 0x54).
	StreamTypeWebTransport uint64 = 0x54
)

// HTTP/3 SETTINGS Parameters (RFC 9114 §7.2.4.1 & §11.2.2 Table 3).
const (
	// SettingQpackMaxTableCapacity defines maximum dynamic table capacity for QPACK (RFC 9204 §5: 0x01).
	SettingQpackMaxTableCapacity uint64 = 0x01

	// SettingMaxFieldSectionSize defines maximum field section size in octets (RFC 9114 §7.2.4.1: 0x06).
	SettingMaxFieldSectionSize uint64 = 0x06

	// SettingQpackBlockedStreams defines maximum streams that can be blocked by QPACK (RFC 9204 §5: 0x07).
	SettingQpackBlockedStreams uint64 = 0x07

	// SettingEnableConnectProtocol enables Extended CONNECT for WebSockets over H3 (RFC 9220 §3: 0x08).
	SettingEnableConnectProtocol uint64 = 0x08

	// SettingH3Datagram enables HTTP/3 datagram support over QUIC (RFC 9297: 0x33).
	SettingH3Datagram uint64 = 0x33
)

// Settings encapsulates HTTP/3 connection parameters negotiated during control stream setup (RFC 9114 §7.2.4).
type Settings struct {
	Other               map[uint64]uint64
	MaxFieldSectionSize int64
	QpackMaxTableCap    uint64
	QpackBlockedStreams uint64
	EnableDatagrams     bool
	EnableConnect       bool
}

// Encode serializes Settings into a binary H3 SETTINGS frame payload.
func (s *Settings) Encode() []byte {
	var payload []byte

	if s.MaxFieldSectionSize >= 0 {
		payload = varint.Append(payload, SettingMaxFieldSectionSize)
		payload = varint.Append(payload, uint64(s.MaxFieldSectionSize))
	}

	if s.QpackMaxTableCap > 0 {
		payload = varint.Append(payload, SettingQpackMaxTableCapacity)
		payload = varint.Append(payload, s.QpackMaxTableCap)
	}

	if s.QpackBlockedStreams > 0 {
		payload = varint.Append(payload, SettingQpackBlockedStreams)
		payload = varint.Append(payload, s.QpackBlockedStreams)
	}

	if s.EnableDatagrams {
		payload = varint.Append(payload, SettingH3Datagram)
		payload = varint.Append(payload, 1)
	}

	if s.EnableConnect {
		payload = varint.Append(payload, SettingEnableConnectProtocol)
		payload = varint.Append(payload, 1)
	}

	for k, v := range s.Other {
		payload = varint.Append(payload, k)
		payload = varint.Append(payload, v)
	}

	var frame []byte

	frame = varint.Append(frame, FrameTypeSettings)
	frame = varint.Append(frame, uint64(len(payload)))

	return append(frame, payload...)
}

// DecodeSettings decodes an incoming H3 SETTINGS frame and verifies that no reserved HTTP/2 settings are present (RFC 9114 §7.2.4 & §7.2.4.1).
func DecodeSettings(r io.Reader, payloadLen uint64) (*Settings, error) {
	lr := io.LimitReader(r, int64(payloadLen))
	qr := varint.NewReader(lr)

	st := &Settings{
		Other: make(map[uint64]uint64),
	}

	seen := make(map[uint64]bool)

	for {
		id, err := varint.Read(qr)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, err
		}

		if seen[id] {
			return nil, ErrH3SettingsError
		}
		seen[id] = true

		val, err := varint.Read(qr)
		if err != nil {
			return nil, err
		}

		// RFC 9114 Section 7.2.4.1 & Appendix A.3: reserved H2 settings (0x00, 0x02, 0x03, 0x04, 0x05)
		switch id {
		case 0x00, 0x02, 0x03, 0x04, 0x05:
			return nil, ErrH3SettingsError

		case SettingMaxFieldSectionSize:
			st.MaxFieldSectionSize = int64(val)
		case SettingQpackMaxTableCapacity:
			st.QpackMaxTableCap = val
		case SettingQpackBlockedStreams:
			st.QpackBlockedStreams = val
		case SettingH3Datagram:
			if val != 1 {
				return nil, ErrH3SettingsError
			}
			st.EnableDatagrams = true
		case SettingEnableConnectProtocol:
			if val != 0 && val != 1 {
				return nil, ErrH3SettingsError
			}
			st.EnableConnect = (val == 1)
		default:
			st.Other[id] = val
		}
	}

	return st, nil
}

// ReadFrameHeader decodes the QUIC variable-length integer frame type and payload length (RFC 9114 §7.1).
func ReadFrameHeader(r varint.Reader) (frameType, payloadLen uint64, err error) {
	frameType, err = varint.Read(r)
	if err != nil {
		return 0, 0, err
	}

	payloadLen, err = varint.Read(r)
	if err != nil {
		return 0, 0, err
	}

	return frameType, payloadLen, nil
}

func AppendHeadersHeader(b []byte, length uint64) []byte {
	b = varint.Append(b, FrameTypeHeaders)
	return varint.Append(b, length)
}

func AppendDataHeader(b []byte, length uint64) []byte {
	b = varint.Append(b, FrameTypeData)
	return varint.Append(b, length)
}
