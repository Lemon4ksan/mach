// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"bytes"
	"net/netip"
	"testing"
	"time"
)

func TestTransportParametersRoundTripClient(t *testing.T) {
	orig := DefaultTransportParameters()
	orig.InitialSourceConnectionID = NewConnectionID([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	orig.HasInitialSourceConnectionID = true
	orig.MaxIdleTimeout = 45 * time.Second
	orig.HasMaxIdleTimeout = true
	orig.MaxUDPPayloadSize = 1450
	orig.InitialMaxData = 1000000
	orig.InitialMaxStreamDataBidiLocal = 500000
	orig.InitialMaxStreamDataBidiRemote = 400000
	orig.InitialMaxStreamDataUni = 300000
	orig.InitialMaxStreamsBidi = 128
	orig.InitialMaxStreamsUni = 64
	orig.AckDelayExponent = 5
	orig.MaxAckDelay = 50 * time.Millisecond
	orig.DisableActiveMigration = true
	orig.ActiveConnectionIDLimit = 4
	orig.MaxDatagramFrameSize = 1200
	orig.HasMaxDatagramFrameSize = true

	// Marshal as Client
	data := orig.Marshal(PerspectiveClient)
	if len(data) == 0 {
		t.Fatalf("marshaled data is empty")
	}

	// Unmarshal on Server side
	var parsed TransportParameters
	if err := parsed.Unmarshal(data, PerspectiveClient); err != nil {
		t.Fatalf("failed to unmarshal client transport parameters: %v", err)
	}

	if !parsed.InitialSourceConnectionID.Equal(orig.InitialSourceConnectionID) {
		t.Errorf("InitialSourceConnectionID mismatch: got %v, want %v", parsed.InitialSourceConnectionID, orig.InitialSourceConnectionID)
	}
	if parsed.MaxIdleTimeout != orig.MaxIdleTimeout {
		t.Errorf("MaxIdleTimeout mismatch: got %v, want %v", parsed.MaxIdleTimeout, orig.MaxIdleTimeout)
	}
	if parsed.MaxUDPPayloadSize != orig.MaxUDPPayloadSize {
		t.Errorf("MaxUDPPayloadSize mismatch: got %v, want %v", parsed.MaxUDPPayloadSize, orig.MaxUDPPayloadSize)
	}
	if parsed.InitialMaxData != orig.InitialMaxData {
		t.Errorf("InitialMaxData mismatch: got %v, want %v", parsed.InitialMaxData, orig.InitialMaxData)
	}
	if parsed.InitialMaxStreamDataBidiLocal != orig.InitialMaxStreamDataBidiLocal {
		t.Errorf("InitialMaxStreamDataBidiLocal mismatch: got %v, want %v", parsed.InitialMaxStreamDataBidiLocal, orig.InitialMaxStreamDataBidiLocal)
	}
	if parsed.InitialMaxStreamDataBidiRemote != orig.InitialMaxStreamDataBidiRemote {
		t.Errorf("InitialMaxStreamDataBidiRemote mismatch: got %v, want %v", parsed.InitialMaxStreamDataBidiRemote, orig.InitialMaxStreamDataBidiRemote)
	}
	if parsed.InitialMaxStreamDataUni != orig.InitialMaxStreamDataUni {
		t.Errorf("InitialMaxStreamDataUni mismatch: got %v, want %v", parsed.InitialMaxStreamDataUni, orig.InitialMaxStreamDataUni)
	}
	if parsed.InitialMaxStreamsBidi != orig.InitialMaxStreamsBidi {
		t.Errorf("InitialMaxStreamsBidi mismatch: got %v, want %v", parsed.InitialMaxStreamsBidi, orig.InitialMaxStreamsBidi)
	}
	if parsed.InitialMaxStreamsUni != orig.InitialMaxStreamsUni {
		t.Errorf("InitialMaxStreamsUni mismatch: got %v, want %v", parsed.InitialMaxStreamsUni, orig.InitialMaxStreamsUni)
	}
	if parsed.AckDelayExponent != orig.AckDelayExponent {
		t.Errorf("AckDelayExponent mismatch: got %v, want %v", parsed.AckDelayExponent, orig.AckDelayExponent)
	}
	if parsed.MaxAckDelay != orig.MaxAckDelay {
		t.Errorf("MaxAckDelay mismatch: got %v, want %v", parsed.MaxAckDelay, orig.MaxAckDelay)
	}
	if parsed.DisableActiveMigration != orig.DisableActiveMigration {
		t.Errorf("DisableActiveMigration mismatch: got %v, want %v", parsed.DisableActiveMigration, orig.DisableActiveMigration)
	}
	if parsed.ActiveConnectionIDLimit != orig.ActiveConnectionIDLimit {
		t.Errorf("ActiveConnectionIDLimit mismatch: got %v, want %v", parsed.ActiveConnectionIDLimit, orig.ActiveConnectionIDLimit)
	}
	if parsed.MaxDatagramFrameSize != orig.MaxDatagramFrameSize {
		t.Errorf("MaxDatagramFrameSize mismatch: got %v, want %v", parsed.MaxDatagramFrameSize, orig.MaxDatagramFrameSize)
	}
}

func TestTransportParametersRoundTripServer(t *testing.T) {
	orig := DefaultTransportParameters()
	orig.OriginalDestinationConnectionID = NewConnectionID([]byte{0xaa, 0xbb, 0xcc, 0xdd})
	orig.HasOriginalDestinationConnectionID = true
	orig.InitialSourceConnectionID = NewConnectionID([]byte{0x11, 0x22, 0x33, 0x44})
	orig.HasInitialSourceConnectionID = true
	orig.RetrySourceConnectionID = NewConnectionID([]byte{0x55, 0x66, 0x77, 0x88})
	orig.HasRetrySourceConnectionID = true

	var token [16]byte
	copy(token[:], []byte("statelessreset16"))
	orig.StatelessResetToken = token
	orig.HasStatelessResetToken = true

	orig.PreferredAddress = PreferredAddress{
		IPv4:                netip.MustParseAddrPort("192.0.2.1:4433"),
		IPv6:                netip.MustParseAddrPort("[2001:db8::1]:4433"),
		ConnectionID:        NewConnectionID([]byte{0x99, 0x88, 0x77}),
		StatelessResetToken: token,
	}
	orig.HasPreferredAddress = true

	orig.MaxIdleTimeout = 30 * time.Second
	orig.HasMaxIdleTimeout = true
	orig.InitialMaxData = 2000000
	orig.InitialMaxStreamDataBidiLocal = 1000000
	orig.InitialMaxStreamsBidi = 256
	orig.DisableActiveMigration = false

	// Marshal as Server
	data := orig.Marshal(PerspectiveServer)

	// Unmarshal on Client side
	var parsed TransportParameters
	if err := parsed.Unmarshal(data, PerspectiveServer); err != nil {
		t.Fatalf("failed to unmarshal server transport parameters: %v", err)
	}

	if !parsed.OriginalDestinationConnectionID.Equal(orig.OriginalDestinationConnectionID) {
		t.Errorf("OriginalDestinationConnectionID mismatch")
	}
	if !parsed.InitialSourceConnectionID.Equal(orig.InitialSourceConnectionID) {
		t.Errorf("InitialSourceConnectionID mismatch")
	}
	if !parsed.RetrySourceConnectionID.Equal(orig.RetrySourceConnectionID) {
		t.Errorf("RetrySourceConnectionID mismatch")
	}
	if !bytes.Equal(parsed.StatelessResetToken[:], orig.StatelessResetToken[:]) {
		t.Errorf("StatelessResetToken mismatch")
	}
	if !parsed.HasPreferredAddress {
		t.Fatalf("expected HasPreferredAddress true")
	}
	if parsed.PreferredAddress.IPv4 != orig.PreferredAddress.IPv4 {
		t.Errorf("IPv4 mismatch: got %v, want %v", parsed.PreferredAddress.IPv4, orig.PreferredAddress.IPv4)
	}
	if parsed.PreferredAddress.IPv6 != orig.PreferredAddress.IPv6 {
		t.Errorf("IPv6 mismatch: got %v, want %v", parsed.PreferredAddress.IPv6, orig.PreferredAddress.IPv6)
	}
	if !parsed.PreferredAddress.ConnectionID.Equal(orig.PreferredAddress.ConnectionID) {
		t.Errorf("PreferredAddress ConnectionID mismatch")
	}
	if !bytes.Equal(parsed.PreferredAddress.StatelessResetToken[:], orig.PreferredAddress.StatelessResetToken[:]) {
		t.Errorf("PreferredAddress StatelessResetToken mismatch")
	}
}

func TestTransportParametersRFC9000Validation(t *testing.T) {
	t.Run("client sending server-only parameter", func(t *testing.T) {
		serverOnlyIDs := []uint64{
			paramOriginalDestinationConnectionID,
			paramStatelessResetToken,
			paramPreferredAddress,
			paramRetrySourceConnectionID,
		}
		for _, id := range serverOnlyIDs {
			buf := appendVarint(nil, id)
			buf = appendVarint(buf, 4)
			buf = append(buf, 1, 2, 3, 4)

			var tp TransportParameters
			err := tp.Unmarshal(buf, PerspectiveClient)
			if err == nil {
				t.Fatalf("expected error when client sends server-only parameter 0x%x, got nil", id)
			}
		}
	})

	t.Run("duplicate parameter ID", func(t *testing.T) {
		buf := appendVarintParam(nil, paramInitialMaxData, 1000)
		buf = appendVarintParam(buf, paramInitialMaxData, 2000)

		var tp TransportParameters
		err := tp.Unmarshal(buf, PerspectiveServer)
		if err == nil {
			t.Fatalf("expected error for duplicate parameter ID, got nil")
		}
	})

	t.Run("max_udp_payload_size < 1200", func(t *testing.T) {
		buf := appendVarintParam(nil, paramMaxUDPPayloadSize, 1199)
		var tp TransportParameters
		err := tp.Unmarshal(buf, PerspectiveServer)
		if err == nil {
			t.Fatalf("expected error for max_udp_payload_size < 1200, got nil")
		}
	})

	t.Run("ack_delay_exponent > 20", func(t *testing.T) {
		buf := appendVarintParam(nil, paramAckDelayExponent, 21)
		var tp TransportParameters
		err := tp.Unmarshal(buf, PerspectiveServer)
		if err == nil {
			t.Fatalf("expected error for ack_delay_exponent > 20, got nil")
		}
	})

	t.Run("max_ack_delay >= 2^14", func(t *testing.T) {
		buf := appendVarintParam(nil, paramMaxAckDelay, 16384)
		var tp TransportParameters
		err := tp.Unmarshal(buf, PerspectiveServer)
		if err == nil {
			t.Fatalf("expected error for max_ack_delay >= 16384, got nil")
		}
	})

	t.Run("active_connection_id_limit < 2", func(t *testing.T) {
		buf := appendVarintParam(nil, paramActiveConnectionIDLimit, 1)
		var tp TransportParameters
		err := tp.Unmarshal(buf, PerspectiveServer)
		if err == nil {
			t.Fatalf("expected error for active_connection_id_limit < 2, got nil")
		}
	})

	t.Run("unknown parameter ignored per RFC 9000", func(t *testing.T) {
		// Parameter 0x1234 with 4 bytes value
		buf := appendBytesParam(nil, 0x1234, []byte{0xde, 0xad, 0xbe, 0xef})
		buf = appendVarintParam(buf, paramInitialMaxData, 55555)

		var tp TransportParameters
		err := tp.Unmarshal(buf, PerspectiveServer)
		if err != nil {
			t.Fatalf("expected unknown parameter to be ignored, got error: %v", err)
		}
		if tp.InitialMaxData != 55555 {
			t.Errorf("expected InitialMaxData 55555, got %d", tp.InitialMaxData)
		}
	})
}

func TestTransportParametersZeroAlloc(t *testing.T) {
	orig := DefaultTransportParameters()
	orig.InitialSourceConnectionID = NewConnectionID([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	orig.HasInitialSourceConnectionID = true
	orig.MaxIdleTimeout = 30 * time.Second
	orig.HasMaxIdleTimeout = true
	orig.InitialMaxData = 1000000
	orig.InitialMaxStreamDataBidiLocal = 500000
	orig.InitialMaxStreamsBidi = 100

	buf := make([]byte, 512)
	encoded := orig.Append(buf[:0], PerspectiveClient)

	// Test zero alloc on Append
	allocsAppend := testing.AllocsPerRun(100, func() {
		_ = orig.Append(buf[:0], PerspectiveClient)
	})
	if allocsAppend > 0 {
		t.Errorf("expected 0 allocs for Append, got %f", allocsAppend)
	}

	// Test zero alloc on Unmarshal
	var parsed TransportParameters
	allocsUnmarshal := testing.AllocsPerRun(100, func() {
		_ = parsed.Unmarshal(encoded, PerspectiveClient)
	})
	if allocsUnmarshal > 0 {
		t.Errorf("expected 0 allocs for Unmarshal, got %f", allocsUnmarshal)
	}
}
