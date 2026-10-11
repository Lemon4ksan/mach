// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quic

import (
	"context"
	"crypto/tls"
	"io"
	"testing"
	"time"
)

func generateTransportTLS(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	cert := generateTestBridgeCert(t)
	alpn := []string{"h3"}

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   alpn,
		MinVersion:   tls.VersionTLS13,
	}

	clientTLS := &tls.Config{
		ServerName:         "localhost",
		InsecureSkipVerify: true,
		NextProtos:         alpn,
		MinVersion:         tls.VersionTLS13,
	}

	return serverTLS, clientTLS
}

func TestTransport_ListenAddrAndDialAddr(t *testing.T) {
	serverTLS, clientTLS := generateTransportTLS(t)

	ln, err := ListenAddr("127.0.0.1:0", serverTLS, WithDatagrams(true))
	if err != nil {
		t.Fatalf("ListenAddr error: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientErrCh := make(chan error, 1)
	go func() {
		clientConn, err := DialAddr(ctx, addr, clientTLS, WithDatagrams(true))
		if err != nil {
			clientErrCh <- err
			return
		}
		defer clientConn.CloseWithError(0, "")

		// 1. Unidirectional stream
		uniStr, err := clientConn.OpenUniStreamSync(ctx)
		if err != nil {
			clientErrCh <- err
			return
		}
		if _, err := uniStr.Write([]byte("client-uni-payload")); err != nil {
			clientErrCh <- err
			return
		}
		if err := uniStr.Close(); err != nil {
			clientErrCh <- err
			return
		}

		// 2. Bidirectional stream
		bidiStr, err := clientConn.OpenStreamSync(ctx)
		if err != nil {
			clientErrCh <- err
			return
		}
		defer bidiStr.Close()

		if _, err := bidiStr.Write([]byte("ping")); err != nil {
			clientErrCh <- err
			return
		}

		reply := make([]byte, 4)
		if _, err := io.ReadFull(bidiStr, reply); err != nil {
			clientErrCh <- err
			return
		}
		if string(reply) != "pong" {
			t.Errorf("expected pong, got %s", string(reply))
		}

		clientErrCh <- nil
	}()

	serverConn, err := ln.Accept(ctx)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	defer serverConn.CloseWithError(0, "")

	// Accept client uni stream
	srvUni, err := serverConn.AcceptUniStream(ctx)
	if err != nil {
		t.Fatalf("AcceptUniStream error: %v", err)
	}
	uniBuf := make([]byte, len("client-uni-payload"))
	if _, err := io.ReadFull(srvUni, uniBuf); err != nil {
		t.Fatalf("srvUni ReadFull error: %v", err)
	}
	if string(uniBuf) != "client-uni-payload" {
		t.Fatalf("expected client-uni-payload, got %s", string(uniBuf))
	}

	// Accept client bidi stream
	srvBidi, err := serverConn.AcceptStream(ctx)
	if err != nil {
		t.Fatalf("AcceptStream error: %v", err)
	}
	defer srvBidi.Close()

	pingBuf := make([]byte, 4)
	if _, err := io.ReadFull(srvBidi, pingBuf); err != nil {
		t.Fatalf("srvBidi ReadFull error: %v", err)
	}
	if string(pingBuf) != "ping" {
		t.Fatalf("expected ping, got %s", string(pingBuf))
	}

	if _, err := srvBidi.Write([]byte("pong")); err != nil {
		t.Fatalf("srvBidi Write pong error: %v", err)
	}

	if err := <-clientErrCh; err != nil {
		t.Fatalf("client error: %v", err)
	}
}
