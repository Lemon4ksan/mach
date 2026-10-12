package ws

import (
	"bytes"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func tcpPair(t *testing.T) (*Conn, *Conn) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	defer ln.Close()

	serverCh := make(chan net.Conn, 1)
	go func() {
		sc, err := ln.Accept()
		if err != nil {
			return
		}
		serverCh <- sc
	}()

	cc, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	sc := <-serverCh

	client := NewClientConn(cc)
	server := NewServerConn(sc)
	return client, server
}

func TestConn_BasicTextAndBinary(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	// Client -> Server Text
	go func() {
		if err := client.WriteMessage(OpText, []byte("Hello Server")); err != nil {
			t.Errorf("client write error: %v", err)
		}
	}()

	op, msg, err := server.ReadMessage()
	if err != nil {
		t.Fatalf("server read error: %v", err)
	}
	if op != OpText || string(msg) != "Hello Server" {
		t.Fatalf("unexpected message: op=%v, msg=%s", op, string(msg))
	}

	// Server -> Client Binary
	go func() {
		if err := server.WriteMessage(OpBinary, []byte{0xDE, 0xAD, 0xBE, 0xEF}); err != nil {
			t.Errorf("server write error: %v", err)
		}
	}()

	op, msg, err = client.ReadMessage()
	if err != nil {
		t.Fatalf("client read error: %v", err)
	}
	if op != OpBinary || !bytes.Equal(msg, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Fatalf("unexpected message: op=%v, msg=%v", op, msg)
	}
}

func TestConn_PingPongAutomaticHeartbeat(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	pongReceived := make(chan []byte, 1)
	client.SetPongHandler(func(appData []byte) error {
		pongReceived <- appData
		return nil
	})

	// Server is reading messages in background
	go func() {
		_, _, _ = server.ReadMessage()
	}()

	// Client sends Ping to Server
	pingPayload := []byte("heartbeat-data-123")
	if err := client.WritePing(pingPayload); err != nil {
		t.Fatalf("client write ping error: %v", err)
	}

	// Client reads in background to receive Pong
	go func() {
		_, _, _ = client.ReadMessage()
	}()

	select {
	case pongData := <-pongReceived:
		if !bytes.Equal(pongData, pingPayload) {
			t.Fatalf("pong payload mismatch: got %s, want %s", string(pongData), string(pingPayload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for automatic pong reply")
	}
}

func TestConn_InterleavedPingDuringFragmentedMessage(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	pongReceived := make(chan []byte, 1)
	client.SetPongHandler(func(appData []byte) error {
		pongReceived <- appData
		return nil
	})

	serverResult := make(chan struct {
		op      Opcode
		payload []byte
		err     error
	}, 1)

	// Server starts reading the fragmented message
	go func() {
		op, payload, err := server.ReadMessage()
		serverResult <- struct {
			op      Opcode
			payload []byte
			err     error
		}{op, payload, err}
	}()

	// Client reads in background to catch the automatic Pong reply
	go func() {
		_, _, _ = client.ReadMessage()
	}()

	// 1. Send first fragment (OpText, FIN=false)
	if err := client.writeDataFrame(false, false, OpText, []byte("Part1-")); err != nil {
		t.Fatalf("write fragment 1 error: %v", err)
	}

	// 2. Interleave a Ping control frame
	pingPayload := []byte("interleaved-ping-payload")
	if err := client.WritePing(pingPayload); err != nil {
		t.Fatalf("write ping error: %v", err)
	}

	// Verify Pong is received promptly while fragmented message is still pending!
	select {
	case pongData := <-pongReceived:
		if !bytes.Equal(pongData, pingPayload) {
			t.Fatalf("pong mismatch: got %s, want %s", string(pongData), string(pingPayload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for interleaved pong reply")
	}

	// 3. Send second fragment (OpContinuation, FIN=false)
	if err := client.writeDataFrame(false, false, OpContinuation, []byte("Part2-")); err != nil {
		t.Fatalf("write fragment 2 error: %v", err)
	}

	// 4. Send final fragment (OpContinuation, FIN=true)
	if err := client.writeDataFrame(true, false, OpContinuation, []byte("Part3")); err != nil {
		t.Fatalf("write fragment 3 error: %v", err)
	}

	// Verify server reassembles the complete message seamlessly!
	select {
	case res := <-serverResult:
		if res.err != nil {
			t.Fatalf("server read message error: %v", res.err)
		}
		if res.op != OpText {
			t.Fatalf("expected OpText, got %v", res.op)
		}
		if string(res.payload) != "Part1-Part2-Part3" {
			t.Fatalf("payload mismatch: got %q, want %q", string(res.payload), "Part1-Part2-Part3")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for complete fragmented message reassembly")
	}
}

func TestConn_CloseHandshake_ClientInitiated(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	serverCloseErr := make(chan error, 1)
	go func() {
		_, _, err := server.ReadMessage()
		serverCloseErr <- err
	}()

	clientCloseErr := make(chan error, 1)
	go func() {
		_, _, err := client.ReadMessage()
		clientCloseErr <- err
	}()

	// Client initiates close
	if err := client.WriteClose(StatusNormalClosure, "Client normal close"); err != nil {
		t.Fatalf("client write close error: %v", err)
	}

	// Server should receive Close, echo it back, and return CloseError
	select {
	case err := <-serverCloseErr:
		var ce *CloseError
		if !errors.As(err, &ce) {
			t.Fatalf("server expected CloseError, got: %v", err)
		}
		if ce.Code != StatusNormalClosure || ce.Reason != "Client normal close" {
			t.Fatalf("server close code/reason mismatch: %v, %s", ce.Code, ce.Reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server timed out waiting for close handshake")
	}

	// Client should receive the echoed Close and return CloseError
	select {
	case err := <-clientCloseErr:
		var ce *CloseError
		if !errors.As(err, &ce) {
			t.Fatalf("client expected CloseError, got: %v", err)
		}
		if ce.Code != StatusNormalClosure {
			t.Fatalf("client close code mismatch: %v", ce.Code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client timed out waiting for echoed close")
	}

	if client.State() != StateClosed {
		t.Fatalf("client state expected Closed, got %v", client.State())
	}
	if server.State() != StateClosed {
		t.Fatalf("server state expected Closed, got %v", server.State())
	}
}

func TestConn_CloseHandshake_SimultaneousClose(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	var clientErr, serverErr error

	go func() {
		defer wg.Done()
		_ = client.WriteClose(StatusNormalClosure, "client closing")
		_, _, clientErr = client.ReadMessage()
	}()

	go func() {
		defer wg.Done()
		_ = server.WriteClose(StatusGoingAway, "server going away")
		_, _, serverErr = server.ReadMessage()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Both should receive CloseError and neither should hang or deadlock
		var ceClient, ceServer *CloseError
		if !errors.As(clientErr, &ceClient) {
			t.Fatalf("client expected CloseError, got: %v", clientErr)
		}
		if !errors.As(serverErr, &ceServer) {
			t.Fatalf("server expected CloseError, got: %v", serverErr)
		}
		if client.State() != StateClosed || server.State() != StateClosed {
			t.Fatalf("states must be closed, got client=%v, server=%v", client.State(), server.State())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("simultaneous close handshake timed out")
	}
}

func TestConn_ReadLimitExceeded(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	server.SetReadLimit(100) // 100 bytes limit

	go func() {
		bigPayload := make([]byte, 105)
		_ = client.WriteMessage(OpBinary, bigPayload)
	}()

	_, _, err := server.ReadMessage()
	if !errors.Is(err, ErrMessageTooBig) {
		t.Fatalf("expected ErrMessageTooBig, got: %v", err)
	}
}

func TestConn_PermessageDeflate(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	client.EnableCompression(true)
	server.EnableCompression(true)

	// Compressible repetitive payload
	input := bytes.Repeat([]byte("Mach-WebSocket-Zero-Alloc-Deflate-Testing-"), 100)

	go func() {
		if err := client.WriteMessage(OpText, input); err != nil {
			t.Errorf("client write compressed error: %v", err)
		}
	}()

	op, msg, err := server.ReadMessage()
	if err != nil {
		t.Fatalf("server read compressed error: %v", err)
	}
	if op != OpText {
		t.Fatalf("expected OpText, got %v", op)
	}
	if !bytes.Equal(msg, input) {
		t.Fatalf("decompressed message mismatch: got len %d, want %d", len(msg), len(input))
	}
}

func TestConn_ConcurrentPingAndData(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		count := 0
		for count < 50 {
			op, msg, err := server.ReadMessage()
			if err != nil {
				return
			}
			if op == OpBinary && len(msg) == 256 {
				count++
			}
		}
	}()

	// Concurrently write data messages and pings from client
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		payload := make([]byte, 256)
		for i := 0; i < 50; i++ {
			if err := client.WriteMessage(OpBinary, payload); err != nil {
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if err := client.WritePing([]byte("ping")); err != nil {
				return
			}
		}
	}()

	wg.Wait()
	select {
	case <-serverDone:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for concurrent messages to complete")
	}
}
