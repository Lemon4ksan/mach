// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/foundation/encoding/varint"
)

func TestSettingsEncodeAndParse(t *testing.T) {
	st := &Settings{
		MaxFieldSectionSize: 65536,
		QpackMaxTableCap:    4096,
		QpackBlockedStreams: 100,
		EnableDatagrams:     true,
		EnableConnect:       true,
		Other: map[uint64]uint64{
			0x1234: 5678,
		},
	}

	encoded := st.Encode()
	r := bytes.NewReader(encoded)

	frameType, payloadLen, err := ReadFrameHeader(r)
	if err != nil {
		t.Fatalf("failed to read frame header: %v", err)
	}

	if frameType != FrameTypeSettings {
		t.Fatalf("expected FrameTypeSettings (%d), got %d", FrameTypeSettings, frameType)
	}

	if payloadLen != uint64(r.Len()) { //nolint:gosec // buffer length is non-negative
		t.Fatalf("payload length mismatch: header specifies %d, remaining buffer is %d", payloadLen, r.Len())
	}

	parsed, err := DecodeSettings(r, payloadLen)
	if err != nil {
		t.Fatalf("DecodeSettings failed: %v", err)
	}

	if !parsed.EnableConnect {
		t.Errorf("expected EnableConnect to be true per RFC 9220 §3 (SETTINGS_ENABLE_CONNECT_PROTOCOL=0x08)")
	}
}

func TestReadFrameHeader(t *testing.T) {
	var buf []byte

	buf = varint.Append(buf, FrameTypeHeaders)
	buf = varint.Append(buf, 1024)

	r := bytes.NewReader(buf)

	fType, pLen, err := ReadFrameHeader(r)
	if err != nil {
		t.Fatalf("ReadFrameHeader failed: %v", err)
	}

	if fType != FrameTypeHeaders {
		t.Errorf("got frame type %d, want %d", fType, FrameTypeHeaders)
	}

	if pLen != 1024 {
		t.Errorf("got payload length %d, want 1024", pLen)
	}
}

func TestAppendFrameHeaders(t *testing.T) {
	hHeader := AppendHeadersHeader(nil, 512)
	rH := bytes.NewReader(hHeader)

	fType, pLen, err := ReadFrameHeader(rH)
	if err != nil || fType != FrameTypeHeaders || pLen != 512 {
		t.Fatalf("appendHeadersHeader failed: type=%d, len=%d, err=%v", fType, pLen, err)
	}

	dHeader := AppendDataHeader(nil, 2048)
	rD := bytes.NewReader(dHeader)

	fType, pLen, err = ReadFrameHeader(rD)
	if err != nil || fType != FrameTypeData || pLen != 2048 {
		t.Fatalf("appendDataHeader failed: type=%d, len=%d, err=%v", fType, pLen, err)
	}
}

func TestReservedH2FrameTypes(t *testing.T) {
	// RFC 9114 §7.2.8 & §11.2.1 Table 2: 0x02, 0x06, 0x08, 0x09 are reserved from HTTP/2
	reserved := []uint64{
		FrameTypeReservedPriority,
		FrameTypeReservedPing,
		FrameTypeReservedWindowUpdate,
		FrameTypeReservedContinuation,
	}

	for _, ft := range reserved {
		if !IsReservedH2FrameType(ft) {
			t.Errorf("expected frame type 0x%02x to be identified as reserved H2 frame type (RFC 9114 §7.2.8)", ft)
		}
	}

	nonReserved := []uint64{
		FrameTypeData,
		FrameTypeHeaders,
		FrameTypeCancelPush,
		FrameTypeSettings,
		FrameTypePushPromise,
		FrameTypeGoAway,
		FrameTypeMaxPushID,
		0x21,
		0x40,
	}

	for _, ft := range nonReserved {
		if IsReservedH2FrameType(ft) {
			t.Errorf("frame type 0x%02x should not be identified as reserved H2 frame type", ft)
		}
	}
}

func TestGreaseFrameAndSettingTypes(t *testing.T) {
	// RFC 9114 §7.2.8 & §7.2.4.1: 0x1f * N + 0x21 are reserved GREASE points
	greaseValues := []uint64{
		0x21,
		0x40,
		0x1f*2 + 0x21,
		0x1f*100 + 0x21,
	}

	for _, g := range greaseValues {
		if !IsGreaseFrameType(g) {
			t.Errorf("expected 0x%x to be GREASE frame type (RFC 9114 §7.2.8)", g)
		}
		if !IsGreaseSetting(g) {
			t.Errorf("expected 0x%x to be GREASE setting (RFC 9114 §7.2.4.1)", g)
		}
	}

	nonGreaseValues := []uint64{0x00, 0x01, 0x02, 0x04, 0x06, 0x20, 0x22, 0x3f}
	for _, ng := range nonGreaseValues {
		if IsGreaseFrameType(ng) {
			t.Errorf("0x%x should not be GREASE frame type", ng)
		}
		if IsGreaseSetting(ng) {
			t.Errorf("0x%x should not be GREASE setting", ng)
		}
	}
}
