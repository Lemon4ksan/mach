// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server_test

import (
	"bytes"
	"net"
	"sync"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/ws"
	"github.com/lemon4ksan/mach/server"
)

func pipePair() (net.Conn, net.Conn) {
	return net.Pipe()
}

func TestServer_WebSocket_EchoSession(t *testing.T) {
	clientRaw, serverRaw := pipePair()
	defer clientRaw.Close()
	defer serverRaw.Close()

	clientConn := ws.NewClientConn(clientRaw)
	serverConn := ws.NewServerConn(serverRaw)

	defer clientConn.Close()
	defer serverConn.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		_ = server.EchoSession(serverConn)
	}()

	testPayload := []byte("Hello Supercharged WebSocket Engine!")
	err := clientConn.WriteMessage(ws.OpcodeText, testPayload)
	require.NoError(t, err)

	var buf [512]byte
	hdr, echoed, err := clientConn.ReadFrame(buf[:])
	require.NoError(t, err)
	assert.Equal(t, ws.OpcodeText, hdr.Opcode)
	assert.True(t, bytes.Equal(testPayload, echoed))

	_ = clientConn.Close()
	wg.Wait()
}

func TestServer_WebSocket_EchoMessageSession(t *testing.T) {
	clientRaw, serverRaw := pipePair()
	defer clientRaw.Close()
	defer serverRaw.Close()

	clientConn := ws.NewClientConn(clientRaw)
	serverConn := ws.NewServerConn(serverRaw)

	defer clientConn.Close()
	defer serverConn.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		_ = server.EchoMessageSession(serverConn)
	}()

	testPayload := []byte("Streaming Message Echo Test")
	err := clientConn.WriteMessage(ws.OpcodeBinary, testPayload)
	require.NoError(t, err)

	op, echoed, err := clientConn.ReadMessageBuf()
	require.NoError(t, err)
	assert.Equal(t, ws.OpcodeBinary, op)
	assert.True(t, bytes.Equal(testPayload, echoed))

	_ = clientConn.Close()
	wg.Wait()
}

func TestServer_WebSocket_ZeroAllocation_EchoLoop(t *testing.T) {
	clientRaw, serverRaw := pipePair()
	defer clientRaw.Close()
	defer serverRaw.Close()

	clientConn := ws.NewClientConn(clientRaw)
	serverConn := ws.NewServerConn(serverRaw)

	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		_ = server.EchoSession(serverConn)
	}()

	payload := []byte("zero-alloc-echo-payload")
	var frameBuf [512]byte

	// Warmup
	_ = clientConn.WriteMessage(ws.OpcodeBinary, payload)
	_, _, _ = clientConn.ReadFrame(frameBuf[:])

	allocs := testing.AllocsPerRun(100, func() {
		if err := clientConn.WriteMessage(ws.OpcodeBinary, payload); err != nil {
			t.Fatal(err)
		}
		_, _, err := clientConn.ReadFrame(frameBuf[:])
		if err != nil {
			t.Fatal(err)
		}
	})

	assert.Equal(t, float64(0), allocs, "Client frame write + read must produce 0 heap allocations")
}

func BenchmarkWS_EchoThroughput(b *testing.B) {
	benchmarkWSEcho(b, 1024)
}

func BenchmarkWS_EchoThroughput_64B(b *testing.B) {
	benchmarkWSEcho(b, 64)
}

func BenchmarkWS_EchoThroughput_1KB(b *testing.B) {
	benchmarkWSEcho(b, 1024)
}

func BenchmarkWS_EchoThroughput_64KB(b *testing.B) {
	benchmarkWSEcho(b, 65536)
}

func benchmarkWSEcho(b *testing.B, size int) {
	clientRaw, serverRaw := net.Pipe()
	defer clientRaw.Close()
	defer serverRaw.Close()

	clientConn := ws.NewClientConn(clientRaw)
	serverConn := ws.NewServerConn(serverRaw)

	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		_ = server.EchoSession(serverConn)
	}()

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}

	buf := make([]byte, size+14)

	// Warmup
	_ = clientConn.WriteMessage(ws.OpcodeBinary, payload)
	_, _, _ = clientConn.ReadFrame(buf)

	b.SetBytes(int64(size))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := clientConn.WriteMessage(ws.OpcodeBinary, payload); err != nil {
			b.Fatal(err)
		}
		_, _, err := clientConn.ReadFrame(buf)
		if err != nil {
			b.Fatal(err)
		}
	}
}
