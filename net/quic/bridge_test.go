// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quic

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/encoding/varint"

	"github.com/lemon4ksan/mach/net/quic/core/handshake"
	"github.com/lemon4ksan/mach/net/quic/core/wire"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

func generateTestBridgeCert(t *testing.T) tls.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Clean-Room QUIC Bridge Test"},
			CommonName:   "localhost",
		},
		DNSNames:              []string{"localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	return tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}
}

func setupTestBridges(t *testing.T, ctx context.Context) (*CoreSessionBridge, *CoreSessionBridge) {
	t.Helper()

	cert := generateTestBridgeCert(t)
	alpn := []string{"h3"}

	destCID := handshake.NewConnectionID([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})
	clientSrcCID := handshake.NewConnectionID([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x11, 0x22})
	serverSrcCID := handshake.NewConnectionID([]byte{0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0x00})

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

	// Start handshakes
	if err := clientBridge.Start(ctx); err != nil {
		t.Fatalf("clientBridge.Start failed: %v", err)
	}
	if err := serverBridge.Start(ctx); err != nil {
		t.Fatalf("serverBridge.Start failed: %v", err)
	}

	// Drive handshake packets across loopback
	for round := 0; round < 30; round++ {
		if clientBridge.IsHandshakeDone() && serverBridge.IsHandshakeDone() {
			break
		}

		progress := false
		// Drain client -> server
		cPackets := clientBridge.DrainOutgoingPackets()
		for _, pkt := range cPackets {
			if err := serverBridge.HandleIncomingPacket(pkt); err != nil {
				t.Fatalf("serverBridge.HandleIncomingPacket error: %v", err)
			}
			progress = true
		}

		// Drain server -> client
		sPackets := serverBridge.DrainOutgoingPackets()
		for _, pkt := range sPackets {
			if err := clientBridge.HandleIncomingPacket(pkt); err != nil {
				t.Fatalf("clientBridge.HandleIncomingPacket error: %v", err)
			}
			progress = true
		}

		if !progress && (!clientBridge.IsHandshakeDone() || !serverBridge.IsHandshakeDone()) {
			t.Fatal("handshake deadlocked without progress")
		}
	}

	if !clientBridge.IsHandshakeDone() || !serverBridge.IsHandshakeDone() {
		t.Fatal("handshake did not reach completion")
	}

	clientBridge.SetDestConnID(serverSrcCID)
	serverBridge.SetDestConnID(clientSrcCID)

	return clientBridge, serverBridge
}

func TestCoreSessionBridge_HandshakeAndStreamMultiplexing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientBridge, serverBridge := setupTestBridges(t, ctx)
	defer clientBridge.Close()
	defer serverBridge.Close()

	// 1. Establish HTTP/3 Unidirectional Control Stream
	clientCtrl, err := clientBridge.StreamConn().OpenUniStream()
	if err != nil {
		t.Fatalf("OpenUniStream error: %v", err)
	}

	settings := &coreh3.Settings{
		MaxFieldSectionSize: 262144,
		EnableDatagrams:     true,
	}
	var ctrlBuf []byte
	ctrlBuf = varint.Append(ctrlBuf, coreh3.StreamTypeControl)
	ctrlBuf = append(ctrlBuf, settings.Encode()...)

	_, err = clientCtrl.Write(ctrlBuf)
	if err != nil {
		t.Fatalf("clientCtrl.Write error: %v", err)
	}

	cData, cOffset, cFin, ok := clientCtrl.PopStreamFrame(4096)
	if !ok {
		t.Fatal("expected control frame to pop")
	}

	// Pack 1-RTT packet from client and deliver to server
	pkt, err := clientBridge.PackStreamPacket(clientCtrl.StreamID(), cOffset, cData, cFin)
	if err != nil {
		t.Fatalf("PackStreamPacket error: %v", err)
	}
	if err := serverBridge.HandleIncomingPacket(pkt); err != nil {
		t.Fatalf("server HandleIncomingPacket error: %v", err)
	}

	serverCtrl, err := serverBridge.StreamConn().AcceptUniStream(ctx)
	if err != nil {
		t.Fatalf("AcceptUniStream error: %v", err)
	}

	r := varint.NewReader(serverCtrl)
	st, err := varint.Read(r)
	if err != nil {
		t.Fatalf("varint.Read stream type error: %v", err)
	}
	if st != coreh3.StreamTypeControl {
		t.Fatalf("expected control stream type 0x00, got 0x%x", st)
	}

	ft, pl, err := coreh3.ReadFrameHeader(r)
	if err != nil {
		t.Fatalf("ReadFrameHeader error: %v", err)
	}
	if ft != coreh3.FrameTypeSettings {
		t.Fatalf("expected SETTINGS frame, got 0x%x", ft)
	}
	payload := make([]byte, pl)
	if _, err := io.ReadFull(r, payload); err != nil {
		t.Fatalf("read settings payload error: %v", err)
	}

	// 2. Open Bidirectional Request Stream
	reqStr, err := clientBridge.StreamConn().OpenStream()
	if err != nil {
		t.Fatalf("OpenStream error: %v", err)
	}

	reqPayload := []byte("GET /hello HTTP/3\r\n")
	if _, err := reqStr.Write(reqPayload); err != nil {
		t.Fatalf("reqStr.Write error: %v", err)
	}
	_ = reqStr.CloseWrite()

	rData, rOffset, rFin, ok := reqStr.SendStream().PopStreamFrame(4096)
	if !ok {
		t.Fatal("expected request frame to pop")
	}

	pkt, err = clientBridge.PackStreamPacket(reqStr.StreamID(), rOffset, rData, rFin)
	if err != nil {
		t.Fatalf("PackStreamPacket request error: %v", err)
	}
	if err := serverBridge.HandleIncomingPacket(pkt); err != nil {
		t.Fatalf("server HandleIncomingPacket error: %v", err)
	}

	srvReqStr, err := serverBridge.StreamConn().AcceptStream(ctx)
	if err != nil {
		t.Fatalf("AcceptStream error: %v", err)
	}

	rcvdReq := make([]byte, len(reqPayload))
	if _, err := io.ReadFull(srvReqStr, rcvdReq); err != nil {
		t.Fatalf("ReadFull server request error: %v", err)
	}
	if string(rcvdReq) != string(reqPayload) {
		t.Fatalf("expected %q, got %q", string(reqPayload), string(rcvdReq))
	}
}

func TestCoreSessionBridge_AsymmetricIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientBridge, serverBridge := setupTestBridges(t, ctx)
	defer clientBridge.Close()
	defer serverBridge.Close()

	// 1. Send completely corrupted garbage bytes
	garbage := []byte{0xff, 0x00, 0x12, 0x34, 0xde, 0xad, 0xbe, 0xef}
	err := serverBridge.HandleIncomingPacket(garbage)
	// Server must cleanly return error and NOT panic
	if err == nil {
		t.Fatal("expected error on malformed packet")
	}

	// 2. Normal stream operations must continue functioning unaffected (strict asymmetric isolation)
	reqStr, err := clientBridge.StreamConn().OpenStream()
	if err != nil {
		t.Fatalf("OpenStream error: %v", err)
	}

	validData := []byte("healthy isolated stream payload")
	if _, err := reqStr.Write(validData); err != nil {
		t.Fatalf("write error: %v", err)
	}

	vData, vOffset, vFin, ok := reqStr.SendStream().PopStreamFrame(4096)
	if !ok {
		t.Fatal("expected stream data to pop")
	}

	validPkt, err := clientBridge.PackStreamPacket(reqStr.StreamID(), vOffset, vData, vFin)
	if err != nil {
		t.Fatalf("PackStreamPacket error: %v", err)
	}

	if err := serverBridge.HandleIncomingPacket(validPkt); err != nil {
		t.Fatalf("server failed to handle valid packet after garbage: %v", err)
	}

	srvStr, err := serverBridge.StreamConn().AcceptStream(ctx)
	if err != nil {
		t.Fatalf("server AcceptStream error: %v", err)
	}

	buf := make([]byte, len(validData))
	if _, err := io.ReadFull(srvStr, buf); err != nil {
		t.Fatalf("read error: %v", err)
	}
	if string(buf) != string(validData) {
		t.Fatalf("expected %q, got %q", string(validData), string(buf))
	}
}
