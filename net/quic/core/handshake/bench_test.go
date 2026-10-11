// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"context"
	"crypto/tls"
	"testing"
	"time"
)

func BenchmarkTransportParametersAppend(b *testing.B) {
	tp := DefaultTransportParameters()
	tp.InitialSourceConnectionID = NewConnectionID([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	tp.HasInitialSourceConnectionID = true
	tp.MaxIdleTimeout = 30 * time.Second
	tp.HasMaxIdleTimeout = true
	tp.InitialMaxData = 1000000
	tp.InitialMaxStreamDataBidiLocal = 500000
	tp.InitialMaxStreamsBidi = 100

	buf := make([]byte, 256)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = tp.Append(buf[:0], PerspectiveClient)
	}
}

func BenchmarkTransportParametersUnmarshal(b *testing.B) {
	tp := DefaultTransportParameters()
	tp.InitialSourceConnectionID = NewConnectionID([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	tp.HasInitialSourceConnectionID = true
	tp.MaxIdleTimeout = 30 * time.Second
	tp.HasMaxIdleTimeout = true
	tp.InitialMaxData = 1000000
	tp.InitialMaxStreamDataBidiLocal = 500000
	tp.InitialMaxStreamsBidi = 100

	data := tp.Marshal(PerspectiveClient)
	var parsed TransportParameters
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = parsed.Unmarshal(data, PerspectiveClient)
	}
}

func BenchmarkInitialKeyDerivation(b *testing.B) {
	cid := []byte{0x83, 0x94, 0xc8, 0xf0, 0x3e, 0x51, 0x57, 0x08}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewInitialKeys(cid, Version1)
	}
}

func BenchmarkLoopbackHandshake(b *testing.B) {
	cert := generateTestCert(&testing.T{})
	alpn := []string{"quic-bench"}
	destCID := NewConnectionID([]byte{1, 2, 3, 4, 5, 6, 7, 8})

	levels := []EncryptionLevel{
		EncryptionLevelInitial,
		EncryptionLevelHandshake,
		EncryptionLevel1RTT,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		serverCfg := &Config{
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				NextProtos:   alpn,
				MinVersion:   tls.VersionTLS13,
			},
			LocalParams: DefaultTransportParameters(),
			DestConnID:  destCID,
			Version:     Version1,
		}
		server, _ := NewServerHandshake(serverCfg)

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
		}
		client, _ := NewClientHandshake(clientCfg)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = client.Start(ctx)
		_ = server.Start(ctx)

		for r := 0; r < 20; r++ {
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
		cancel()
		_ = client.Close()
		_ = server.Close()
	}
}
