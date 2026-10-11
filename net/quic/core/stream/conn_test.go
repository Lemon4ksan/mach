// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestConn_OpenAndAcceptBidiStream(t *testing.T) {
	clientConn := NewConn(ConnConfig{
		Perspective: PerspectiveClient,
	})
	serverConn := NewConn(ConnConfig{
		Perspective: PerspectiveServer,
	})

	// Client opens bidirectional stream
	cStr, err := clientConn.OpenStream()
	if err != nil {
		t.Fatalf("OpenStream error: %v", err)
	}
	if cStr.StreamID() != 0 {
		t.Fatalf("expected client-bidi stream ID 0, got %d", cStr.StreamID())
	}

	// Client writes "ping"
	_, err = cStr.Write([]byte("ping"))
	if err != nil {
		t.Fatalf("client write error: %v", err)
	}
	_ = cStr.Close()

	// Pop client frame and deliver to server
	frameData, offset, fin, ok := cStr.SendStream().PopStreamFrame(1024)
	if !ok {
		t.Fatal("expected frame to pop")
	}

	err = serverConn.HandleStreamFrame(cStr.StreamID(), offset, frameData, fin)
	if err != nil {
		t.Fatalf("server HandleStreamFrame error: %v", err)
	}

	// Server accepts the stream
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	sStr, err := serverConn.AcceptStream(ctx)
	if err != nil {
		t.Fatalf("server AcceptStream error: %v", err)
	}
	if sStr.StreamID() != 0 {
		t.Fatalf("expected stream ID 0, got %d", sStr.StreamID())
	}

	// Server reads "ping"
	buf := make([]byte, 16)
	n, err := sStr.Read(buf)
	if err != io.EOF && err != nil {
		t.Fatalf("server read error: %v", err)
	}
	if string(buf[:n]) != "ping" {
		t.Fatalf("expected 'ping', got %q", string(buf[:n]))
	}

	// Server writes "pong" back
	_, err = sStr.Write([]byte("pong"))
	if err != nil {
		t.Fatalf("server write error: %v", err)
	}
	_ = sStr.Close()

	// Deliver server frame to client
	sFrameData, sOffset, sFin, sOk := sStr.SendStream().PopStreamFrame(1024)
	if !sOk {
		t.Fatal("expected server frame to pop")
	}

	err = clientConn.HandleStreamFrame(sStr.StreamID(), sOffset, sFrameData, sFin)
	if err != nil {
		t.Fatalf("client HandleStreamFrame error: %v", err)
	}

	// Client reads "pong"
	cBuf := make([]byte, 16)
	cN, cErr := cStr.Read(cBuf)
	if cErr != io.EOF && cErr != nil {
		t.Fatalf("client read error: %v", cErr)
	}
	if string(cBuf[:cN]) != "pong" {
		t.Fatalf("expected 'pong', got %q", string(cBuf[:cN]))
	}
}

func TestConn_OpenAndAcceptUniStream(t *testing.T) {
	clientConn := NewConn(ConnConfig{Perspective: PerspectiveClient})
	serverConn := NewConn(ConnConfig{Perspective: PerspectiveServer})

	cUni, err := clientConn.OpenUniStream()
	if err != nil {
		t.Fatalf("OpenUniStream error: %v", err)
	}
	if cUni.StreamID() != 2 {
		t.Fatalf("expected client-uni stream ID 2, got %d", cUni.StreamID())
	}

	_, err = cUni.Write([]byte("uni-payload"))
	if err != nil {
		t.Fatalf("write error: %v", err)
	}
	_ = cUni.Close()

	data, offset, fin, ok := cUni.PopStreamFrame(1024)
	if !ok {
		t.Fatal("expected uni frame to pop")
	}

	err = serverConn.HandleStreamFrame(cUni.StreamID(), offset, data, fin)
	if err != nil {
		t.Fatalf("server HandleStreamFrame error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	sUni, err := serverConn.AcceptUniStream(ctx)
	if err != nil {
		t.Fatalf("server AcceptUniStream error: %v", err)
	}
	if sUni.StreamID() != 2 {
		t.Fatalf("expected stream ID 2, got %d", sUni.StreamID())
	}

	buf := make([]byte, 32)
	n, err := sUni.Read(buf)
	if err != io.EOF && err != nil {
		t.Fatalf("read error: %v", err)
	}
	if string(buf[:n]) != "uni-payload" {
		t.Fatalf("expected 'uni-payload', got %q", string(buf[:n]))
	}
}

func TestConn_StreamLimits(t *testing.T) {
	conn := NewConn(ConnConfig{
		Perspective:               PerspectiveClient,
		InitialPeerMaxStreamsBidi: 2,
	})

	// Open 2 allowed streams
	s1, err := conn.OpenStream()
	if err != nil || s1.StreamID() != 0 {
		t.Fatalf("stream 1 failed: %v", err)
	}

	s2, err := conn.OpenStream()
	if err != nil || s2.StreamID() != 4 {
		t.Fatalf("stream 2 failed: %v", err)
	}

	// 3rd stream non-sync fails
	_, err = conn.OpenStream()
	if err == nil {
		t.Fatal("expected StreamLimitError for 3rd stream")
	}
	var tErr *TransportError
	if !errors.As(err, &tErr) || tErr.ErrorCode != StreamLimitError {
		t.Fatalf("expected StreamLimitError, got %v", err)
	}

	// 3rd stream with OpenStreamSync blocks then unblocks on MAX_STREAMS
	streamCh := make(chan *Stream, 1)
	errCh := make(chan error, 1)

	go func() {
		str, sErr := conn.OpenStreamSync(context.Background())
		if sErr != nil {
			errCh <- sErr
		} else {
			streamCh <- str
		}
	}()

	time.Sleep(20 * time.Millisecond)
	// Peer sends MAX_STREAMS increasing limit to 5
	conn.HandleMaxStreamsFrame(StreamTypeClientBidi, 5)

	select {
	case str := <-streamCh:
		if str.StreamID() != 8 {
			t.Fatalf("expected stream ID 8, got %d", str.StreamID())
		}
	case err := <-errCh:
		t.Fatalf("unexpected sync open error: %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("OpenStreamSync did not unblock after MAX_STREAMS frame")
	}
}

func TestConn_CloseWithError(t *testing.T) {
	conn := NewConn(ConnConfig{Perspective: PerspectiveClient})

	str, err := conn.OpenStream()
	if err != nil {
		t.Fatalf("OpenStream error: %v", err)
	}

	err = conn.CloseWithError(0x0100, "H3_NO_ERROR")
	if err != nil {
		t.Fatalf("CloseWithError error: %v", err)
	}

	if !conn.IsClosed() {
		t.Fatal("expected conn to be closed")
	}

	// Stream operations should return error
	_, err = str.Write([]byte("after-close"))
	if err == nil {
		t.Fatal("expected error writing after connection close")
	}

	// New streams rejected
	_, err = conn.OpenStream()
	if !errors.Is(err, ErrConnClosed) {
		t.Fatalf("expected ErrConnClosed, got %v", err)
	}
}
