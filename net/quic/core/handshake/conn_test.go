// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

func generateTestCert(t *testing.T) tls.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Clean-Room QUIC Core"},
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

func TestLoopbackHandshakeSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cert := generateTestCert(t)
	alpn := []string{"quic-clean-room-v1"}

	destCID := NewConnectionID([]byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04})
	clientSrcCID := NewConnectionID([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})
	serverSrcCID := NewConnectionID([]byte{0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10})

	// Server parameters
	serverParams := DefaultTransportParameters()
	serverParams.OriginalDestinationConnectionID = destCID
	serverParams.HasOriginalDestinationConnectionID = true
	serverParams.InitialSourceConnectionID = serverSrcCID
	serverParams.HasInitialSourceConnectionID = true
	serverParams.MaxIdleTimeout = 45 * time.Second
	serverParams.HasMaxIdleTimeout = true
	serverParams.InitialMaxData = 10000000
	serverParams.InitialMaxStreamDataBidiLocal = 5000000
	serverParams.InitialMaxStreamsBidi = 100
	serverParams.DisableActiveMigration = true

	// Client parameters
	clientParams := DefaultTransportParameters()
	clientParams.InitialSourceConnectionID = clientSrcCID
	clientParams.HasInitialSourceConnectionID = true
	clientParams.MaxIdleTimeout = 30 * time.Second
	clientParams.HasMaxIdleTimeout = true
	clientParams.InitialMaxData = 8000000
	clientParams.InitialMaxStreamDataBidiLocal = 4000000
	clientParams.InitialMaxStreamsBidi = 50

	serverCfg := &Config{
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   alpn,
			MinVersion:   tls.VersionTLS13,
		},
		LocalParams: serverParams,
		DestConnID:  destCID,
		Version:     Version1,
	}
	server, err := NewServerHandshake(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server handshake driver: %v", err)
	}
	defer server.Close()

	clientCfg := &Config{
		TLSConfig: &tls.Config{
			ServerName:         "localhost",
			InsecureSkipVerify: true,
			NextProtos:         alpn,
			MinVersion:         tls.VersionTLS13,
		},
		LocalParams: clientParams,
		DestConnID:  destCID,
		Version:     Version1,
	}
	client, err := NewClientHandshake(clientCfg)
	if err != nil {
		t.Fatalf("failed to create client handshake driver: %v", err)
	}
	defer client.Close()

	// Start both endpoints
	if err := client.Start(ctx); err != nil {
		t.Fatalf("client.Start failed: %v", err)
	}
	if err := server.Start(ctx); err != nil {
		t.Fatalf("server.Start failed: %v", err)
	}

	levels := []EncryptionLevel{
		EncryptionLevelInitial,
		EncryptionLevelHandshake,
		EncryptionLevel1RTT,
	}

	// Loopback message exchange
	rounds := 0
	const maxRounds = 25
	for ; rounds < maxRounds; rounds++ {
		if client.IsHandshakeDone() && server.IsHandshakeDone() {
			break
		}

		progress := false
		for _, lvl := range levels {
			if client.HasCryptoData(lvl) {
				data := client.DrainCryptoData(lvl)
				if err := server.HandleCryptoData(lvl, data); err != nil {
					t.Fatalf("server.HandleCryptoData failed at %s: %v", lvl, err)
				}
				progress = true
			}
			if server.HasCryptoData(lvl) {
				data := server.DrainCryptoData(lvl)
				if err := client.HandleCryptoData(lvl, data); err != nil {
					t.Fatalf("client.HandleCryptoData failed at %s: %v", lvl, err)
				}
				progress = true
			}
		}

		if !progress {
			break
		}
	}

	if !client.IsHandshakeDone() {
		t.Fatalf("client handshake did not complete to 1-RTT after %d rounds", rounds)
	}
	if !server.IsHandshakeDone() {
		t.Fatalf("server handshake did not complete to 1-RTT after %d rounds", rounds)
	}

	t.Logf("Handshake completed successfully in %d exchange rounds!", rounds)

	// Verify ALPN
	clientConnState := client.ConnectionState()
	serverConnState := server.ConnectionState()
	if clientConnState.NegotiatedProtocol != "quic-clean-room-v1" {
		t.Errorf("unexpected negotiated protocol on client: %q", clientConnState.NegotiatedProtocol)
	}
	if serverConnState.NegotiatedProtocol != "quic-clean-room-v1" {
		t.Errorf("unexpected negotiated protocol on server: %q", serverConnState.NegotiatedProtocol)
	}

	// Verify Transport Parameters received by peer
	clientPeerParams := client.PeerTransportParameters()
	if clientPeerParams == nil {
		t.Fatalf("client did not receive peer transport parameters")
	}
	if !clientPeerParams.OriginalDestinationConnectionID.Equal(serverParams.OriginalDestinationConnectionID) {
		t.Errorf("client got wrong OriginalDestinationConnectionID")
	}
	if !clientPeerParams.InitialSourceConnectionID.Equal(serverParams.InitialSourceConnectionID) {
		t.Errorf("client got wrong InitialSourceConnectionID")
	}
	if clientPeerParams.InitialMaxData != serverParams.InitialMaxData {
		t.Errorf(
			"client got wrong InitialMaxData: %d != %d",
			clientPeerParams.InitialMaxData,
			serverParams.InitialMaxData,
		)
	}
	if clientPeerParams.DisableActiveMigration != serverParams.DisableActiveMigration {
		t.Errorf("client got wrong DisableActiveMigration")
	}

	serverPeerParams := server.PeerTransportParameters()
	if serverPeerParams == nil {
		t.Fatalf("server did not receive peer transport parameters")
	}
	if !serverPeerParams.InitialSourceConnectionID.Equal(clientParams.InitialSourceConnectionID) {
		t.Errorf("server got wrong InitialSourceConnectionID")
	}
	if serverPeerParams.InitialMaxData != clientParams.InitialMaxData {
		t.Errorf(
			"server got wrong InitialMaxData: %d != %d",
			serverPeerParams.InitialMaxData,
			clientParams.InitialMaxData,
		)
	}

	// Verify Key Agreement:
	// Handshake keys: Client Write == Server Read, Server Write == Client Read
	clientHSWrite := client.WriteSecret(EncryptionLevelHandshake)
	serverHSRead := server.ReadSecret(EncryptionLevelHandshake)
	if !bytes.Equal(clientHSWrite, serverHSRead) {
		t.Errorf("Handshake secret mismatch: client write != server read")
	}

	serverHSWrite := server.WriteSecret(EncryptionLevelHandshake)
	clientHSRead := client.ReadSecret(EncryptionLevelHandshake)
	if !bytes.Equal(serverHSWrite, clientHSRead) {
		t.Errorf("Handshake secret mismatch: server write != client read")
	}

	// 1-RTT keys: Client Write == Server Read, Server Write == Client Read
	client1RTTWrite := client.WriteSecret(EncryptionLevel1RTT)
	server1RTTRead := server.ReadSecret(EncryptionLevel1RTT)
	if !bytes.Equal(client1RTTWrite, server1RTTRead) {
		t.Errorf("1-RTT secret mismatch: client write != server read")
	}

	server1RTTWrite := server.WriteSecret(EncryptionLevel1RTT)
	client1RTTRead := client.ReadSecret(EncryptionLevel1RTT)
	if !bytes.Equal(server1RTTWrite, client1RTTRead) {
		t.Errorf("1-RTT secret mismatch: server write != client read")
	}

	// Verify derived packet keys and header protection keys exist
	client1RTTKeys := client.WriteKeys(EncryptionLevel1RTT)
	if client1RTTKeys == nil || len(client1RTTKeys.Key) == 0 || len(client1RTTKeys.IV) == 0 ||
		len(client1RTTKeys.HPKey) == 0 {
		t.Fatalf("client 1-RTT write traffic keys are missing or invalid")
	}
	server1RTTKeys := server.WriteKeys(EncryptionLevel1RTT)
	if server1RTTKeys == nil || len(server1RTTKeys.Key) == 0 || len(server1RTTKeys.IV) == 0 ||
		len(server1RTTKeys.HPKey) == 0 {
		t.Fatalf("server 1-RTT write traffic keys are missing or invalid")
	}

	// Verify AEAD creation
	aead, err := CreateAEAD(client1RTTKeys.Key, client1RTTKeys.Suite)
	if err != nil {
		t.Fatalf("failed to create 1-RTT AEAD: %v", err)
	}
	if aead.NonceSize() != 12 {
		t.Errorf("unexpected AEAD nonce size: %d", aead.NonceSize())
	}
}

func TestLoopbackHandshakeALPNMismatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cert := generateTestCert(t)
	destCID := NewConnectionID([]byte{0x01, 0x02, 0x03, 0x04})

	serverCfg := &Config{
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"server-proto"},
			MinVersion:   tls.VersionTLS13,
		},
		LocalParams: DefaultTransportParameters(),
		DestConnID:  destCID,
		Version:     Version1,
	}
	server, err := NewServerHandshake(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server handshake driver: %v", err)
	}
	defer server.Close()

	clientCfg := &Config{
		TLSConfig: &tls.Config{
			ServerName:         "localhost",
			InsecureSkipVerify: true,
			NextProtos:         []string{"client-proto"}, // mismatching ALPN
			MinVersion:         tls.VersionTLS13,
		},
		LocalParams: DefaultTransportParameters(),
		DestConnID:  destCID,
		Version:     Version1,
	}
	client, err := NewClientHandshake(clientCfg)
	if err != nil {
		t.Fatalf("failed to create client handshake driver: %v", err)
	}
	defer client.Close()

	_ = client.Start(ctx)
	_ = server.Start(ctx)

	clientHello := client.DrainCryptoData(EncryptionLevelInitial)
	err = server.HandleCryptoData(EncryptionLevelInitial, clientHello)
	// Handshake should fail due to no application protocol
	if err == nil && !server.IsHandshakeDone() {
		// Server might emit an alert at Initial level
		if server.HasCryptoData(EncryptionLevelInitial) {
			alertData := server.DrainCryptoData(EncryptionLevelInitial)
			clientErr := client.HandleCryptoData(EncryptionLevelInitial, alertData)
			if clientErr == nil {
				t.Fatalf("expected client error on ALPN alert, got nil")
			}
		}
	}
}

func TestLoopbackHandshakeVersion2(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cert := generateTestCert(t)
	alpn := []string{"quic-v2"}
	destCID := NewConnectionID([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})

	serverCfg := &Config{
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   alpn,
			MinVersion:   tls.VersionTLS13,
		},
		LocalParams: DefaultTransportParameters(),
		DestConnID:  destCID,
		Version:     Version2,
	}
	server, err := NewServerHandshake(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server handshake: %v", err)
	}
	defer server.Close()

	clientCfg := &Config{
		TLSConfig: &tls.Config{
			ServerName:         "localhost",
			InsecureSkipVerify: true,
			NextProtos:         alpn,
			MinVersion:         tls.VersionTLS13,
		},
		LocalParams: DefaultTransportParameters(),
		DestConnID:  destCID,
		Version:     Version2,
	}
	client, err := NewClientHandshake(clientCfg)
	if err != nil {
		t.Fatalf("failed to create client handshake: %v", err)
	}
	defer client.Close()

	_ = client.Start(ctx)
	_ = server.Start(ctx)

	levels := []EncryptionLevel{EncryptionLevelInitial, EncryptionLevelHandshake, EncryptionLevel1RTT}
	for i := 0; i < 20; i++ {
		if client.IsHandshakeDone() && server.IsHandshakeDone() {
			break
		}
		for _, lvl := range levels {
			if client.HasCryptoData(lvl) {
				_ = server.HandleCryptoData(lvl, client.DrainCryptoData(lvl))
			}
			if server.HasCryptoData(lvl) {
				_ = client.HandleCryptoData(lvl, server.DrainCryptoData(lvl))
			}
		}
	}

	if !client.IsHandshakeDone() || !server.IsHandshakeDone() {
		t.Fatalf("QUIC v2 handshake did not complete")
	}
}

func TestLoopbackHandshakeEventHooks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cert := generateTestCert(t)
	alpn := []string{"quic-hooks"}
	destCID := NewConnectionID([]byte{0x12, 0x34, 0x56, 0x78})

	var (
		clientDone       bool
		serverDone       bool
		clientParamsGot  bool
		serverParamsGot  bool
		clientHSSecret   bool
		client1RTTSecret bool
	)

	serverCfg := &Config{
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   alpn,
			MinVersion:   tls.VersionTLS13,
		},
		LocalParams: DefaultTransportParameters(),
		DestConnID:  destCID,
		Version:     Version1,
		OnHandshakeDone: func() {
			serverDone = true
		},
		OnPeerTransportParams: func(tp *TransportParameters) {
			serverParamsGot = true
		},
	}
	server, err := NewServerHandshake(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer server.Close()

	clientCfg := &Config{
		TLSConfig: &tls.Config{
			ServerName:         "localhost",
			InsecureSkipVerify: true,
			NextProtos:         alpn,
			MinVersion:         tls.VersionTLS13,
		},
		LocalParams: DefaultTransportParameters(),
		DestConnID:  destCID,
		Version:     Version1,
		OnHandshakeDone: func() {
			clientDone = true
		},
		OnPeerTransportParams: func(tp *TransportParameters) {
			clientParamsGot = true
		},
		OnHandshakeWriteSecret: func(level EncryptionLevel, secret []byte, suite uint16) {
			if level == EncryptionLevelHandshake {
				clientHSSecret = true
			} else if level == EncryptionLevel1RTT {
				client1RTTSecret = true
			}
		},
	}
	client, err := NewClientHandshake(clientCfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_ = client.Start(ctx)
	_ = server.Start(ctx)

	levels := []EncryptionLevel{EncryptionLevelInitial, EncryptionLevelHandshake, EncryptionLevel1RTT}
	for i := 0; i < 20; i++ {
		if client.IsHandshakeDone() && server.IsHandshakeDone() {
			break
		}
		for _, lvl := range levels {
			if client.HasCryptoData(lvl) {
				_ = server.HandleCryptoData(lvl, client.DrainCryptoData(lvl))
			}
			if server.HasCryptoData(lvl) {
				_ = client.HandleCryptoData(lvl, server.DrainCryptoData(lvl))
			}
		}
	}

	if !clientDone || !serverDone {
		t.Errorf("OnHandshakeDone hook did not fire: client=%v, server=%v", clientDone, serverDone)
	}
	if !clientParamsGot || !serverParamsGot {
		t.Errorf("OnPeerTransportParams hook did not fire: client=%v, server=%v", clientParamsGot, serverParamsGot)
	}
	if !clientHSSecret || !client1RTTSecret {
		t.Errorf("OnHandshakeWriteSecret hook did not fire: HS=%v, 1RTT=%v", clientHSSecret, client1RTTSecret)
	}
}
