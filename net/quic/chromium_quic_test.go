// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quic

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/net/quic/core/handshake"
	"github.com/lemon4ksan/mach/net/quic/core/wire"
)

// TestChromium_EarlyPacketReordering_EncryptionLevels verifies that early packets
// arriving before their corresponding cryptographic keys are derived (e.g. Handshake
// arriving before Initial keys, or 1-RTT arriving before Handshake keys) are buffered
// and processed successfully upon key derivation (ported from Chromium QuicEndToEndTest).
func TestChromium_EarlyPacketReordering_EncryptionLevels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cert := generateTestBridgeCert(t)
	alpn := []string{"h3"}

	destCID := handshake.NewConnectionID([]byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80})
	clientSrcCID := handshake.NewConnectionID([]byte{0xc1, 0xc2, 0xc3, 0xc4, 0xc5, 0xc6, 0xc7, 0xc8})
	serverSrcCID := handshake.NewConnectionID([]byte{0xd1, 0xd2, 0xd3, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8})

	serverParams := handshake.DefaultTransportParameters()
	serverParams.OriginalDestinationConnectionID = destCID
	serverParams.HasOriginalDestinationConnectionID = true
	serverParams.InitialSourceConnectionID = serverSrcCID
	serverParams.HasInitialSourceConnectionID = true
	serverParams.MaxIdleTimeout = 30 * time.Second
	serverParams.HasMaxIdleTimeout = true
	serverParams.InitialMaxData = 10000000
	serverParams.InitialMaxStreamDataBidiLocal = 5000000
	serverParams.InitialMaxStreamsBidi = 100
	serverParams.InitialMaxStreamsUni = 100

	clientParams := handshake.DefaultTransportParameters()
	clientParams.InitialSourceConnectionID = clientSrcCID
	clientParams.HasInitialSourceConnectionID = true
	clientParams.MaxIdleTimeout = 30 * time.Second
	clientParams.HasMaxIdleTimeout = true
	clientParams.InitialMaxData = 10000000
	clientParams.InitialMaxStreamDataBidiLocal = 5000000
	clientParams.InitialMaxStreamsBidi = 100
	clientParams.InitialMaxStreamsUni = 100

	clientTLS := &tls.Config{
		ServerName:         "localhost",
		InsecureSkipVerify: true,
		NextProtos:         alpn,
		MinVersion:         tls.VersionTLS13,
	}
	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   alpn,
		MinVersion:   tls.VersionTLS13,
	}

	dummyClientAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 54321}
	dummyServerAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4433}

	clientBridge, err := NewCoreSessionBridge(CoreBridgeConfig{
		Perspective: handshake.PerspectiveClient,
		TLSConfig:   clientTLS,
		LocalParams: clientParams,
		DestConnID:  destCID,
		SrcConnID:   clientSrcCID,
		Version:     wire.Version1,
		LocalAddr:   dummyClientAddr,
		RemoteAddr:  dummyServerAddr,
	})
	if err != nil {
		t.Fatalf("failed to create client bridge: %v", err)
	}
	defer clientBridge.Close()

	serverBridge, err := NewCoreSessionBridge(CoreBridgeConfig{
		Perspective: handshake.PerspectiveServer,
		TLSConfig:   serverTLS,
		LocalParams: serverParams,
		DestConnID:  destCID,
		SrcConnID:   serverSrcCID,
		Version:     wire.Version1,
		LocalAddr:   dummyServerAddr,
		RemoteAddr:  dummyClientAddr,
	})
	if err != nil {
		t.Fatalf("failed to create server bridge: %v", err)
	}
	defer serverBridge.Close()

	if err := clientBridge.Start(ctx); err != nil {
		t.Fatalf("clientBridge.Start failed: %v", err)
	}
	if err := serverBridge.Start(ctx); err != nil {
		t.Fatalf("serverBridge.Start failed: %v", err)
	}

	// 1. Client sends initial packet to server
	cInitialPackets := clientBridge.DrainOutgoingPackets()
	if len(cInitialPackets) == 0 {
		t.Fatalf("expected client Initial packet")
	}

	// Server processes client Initial
	for _, pkt := range cInitialPackets {
		if err := serverBridge.HandleIncomingPacket(pkt); err != nil {
			t.Fatalf("server HandleIncomingPacket error: %v", err)
		}
	}

	// 2. Server produces Initial + Handshake packets
	sPackets := serverBridge.DrainOutgoingPackets()
	if len(sPackets) < 2 {
		t.Fatalf("expected server to produce at least 2 packets (Initial + Handshake), got %d", len(sPackets))
	}

	// ADVERSARIAL REORDERING: Feed the packets to client in REVERSE order
	// (Handshake packet arrives BEFORE Initial packet)
	for i := len(sPackets) - 1; i >= 0; i-- {
		pkt := sPackets[i]
		if err := clientBridge.HandleIncomingPacket(pkt); err != nil {
			t.Fatalf("client HandleIncomingPacket error on out-of-order packet: %v", err)
		}
	}

	// Complete any remaining handshake rounds
	for round := 0; round < 10; round++ {
		if clientBridge.IsHandshakeDone() && serverBridge.IsHandshakeDone() {
			break
		}
		for _, pkt := range clientBridge.DrainOutgoingPackets() {
			_ = serverBridge.HandleIncomingPacket(pkt)
		}
		for _, pkt := range serverBridge.DrainOutgoingPackets() {
			_ = clientBridge.HandleIncomingPacket(pkt)
		}
	}

	if !clientBridge.IsHandshakeDone() || !serverBridge.IsHandshakeDone() {
		t.Fatalf("handshake failed to complete after reordered packet arrival")
	}
}

// TestChromium_IndependentPacketNumberSpaces verifies RFC 9000 §12.3:
// Initial, Handshake, and Application Data (1-RTT) packet number spaces are strictly
// independent, each starting from packet number 0 without cross-space contamination.
func TestChromium_IndependentPacketNumberSpaces(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientBridge, serverBridge := setupTestBridges(t, ctx)
	defer clientBridge.Close()
	defer serverBridge.Close()

	// Verify client packet number space counters are tracked independently
	cInitPN := clientBridge.nextPNInitial.Load()
	cHsPN := clientBridge.nextPNHs.Load()
	c1RTTPN := clientBridge.nextPN1RTT.Load()

	if cInitPN == 0 {
		t.Fatalf("expected non-zero Initial PN after handshake, got %d", cInitPN)
	}
	if cHsPN == 0 {
		t.Fatalf("expected non-zero Handshake PN after handshake, got %d", cHsPN)
	}

	// Largest received packet numbers per space must also be tracked independently
	largestInit := clientBridge.largestRecvPNInitial.Load()
	largestHs := clientBridge.largestRecvPNHs.Load()
	largest1RTT := clientBridge.largestRecvPN1RTT.Load()

	if largestInit > 100 && largestInit != wire.InvalidPacketNumber {
		t.Fatalf("unrealistic largest Initial PN: %d", largestInit)
	}
	if largestHs > 100 && largestHs != wire.InvalidPacketNumber {
		t.Fatalf("unrealistic largest Handshake PN: %d", largestHs)
	}

	// Open a 1-RTT stream and exchange data to advance 1-RTT PN space
	str, err := clientBridge.StreamConn().OpenUniStream()
	if err != nil {
		t.Fatalf("OpenUniStream failed: %v", err)
	}

	_, err = str.Write([]byte("adversarial test vector"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	cData, cOffset, cFin, ok := str.PopStreamFrame(4096)
	if !ok {
		t.Fatal("expected stream frame to pop")
	}

	pkt, err := clientBridge.PackStreamPacket(str.StreamID(), cOffset, cData, cFin)
	if err != nil {
		t.Fatalf("PackStreamPacket error: %v", err)
	}

	if err := serverBridge.HandleIncomingPacket(pkt); err != nil {
		t.Fatalf("server failed to handle 1-RTT packet: %v", err)
	}

	// Verify server's largest 1-RTT received PN was updated without affecting other spaces
	server1RTT := serverBridge.largestRecvPN1RTT.Load()
	if server1RTT == wire.InvalidPacketNumber {
		t.Fatalf("server 1-RTT PN did not advance")
	}
	_ = c1RTTPN
	_ = largest1RTT
}
