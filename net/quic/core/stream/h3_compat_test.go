// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/encoding/varint"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

func TestH3_DropInCompatibility_ControlAndRequestStreams(t *testing.T) {
	clientConn := NewConn(ConnConfig{Perspective: PerspectiveClient})
	serverConn := NewConn(ConnConfig{Perspective: PerspectiveServer})

	// Network pipe between clientConn and serverConn
	pipeFrames := func(cStr, sStr *Stream) {
		for {
			data, offset, fin, ok := cStr.SendStream().PopStreamFrame(4096)
			if ok {
				_ = sStr.ReceiveStream().HandleStreamFrame(offset, data, fin)
			}
			sData, sOffset, sFin, sOk := sStr.SendStream().PopStreamFrame(4096)
			if sOk {
				_ = cStr.ReceiveStream().HandleStreamFrame(sOffset, sData, sFin)
			}
			if !ok && !sOk {
				break
			}
		}
	}

	// 1. HTTP/3 Control Stream Setup (RFC 9114 §6.2.1)
	clientControlOut, err := clientConn.OpenUniStream()
	if err != nil {
		t.Fatalf("OpenUniStream control stream error: %v", err)
	}

	settings := &coreh3.Settings{
		MaxFieldSectionSize: 262144,
		EnableDatagrams:     true,
	}

	var ctrlBuf []byte
	ctrlBuf = varint.Append(ctrlBuf, coreh3.StreamTypeControl)
	ctrlBuf = append(ctrlBuf, settings.Encode()...)

	_, err = clientControlOut.Write(ctrlBuf)
	if err != nil {
		t.Fatalf("write control stream error: %v", err)
	}

	// Deliver control stream to server
	cData, cOffset, cFin, cOk := clientControlOut.PopStreamFrame(4096)
	if !cOk {
		t.Fatal("expected control frame to pop")
	}

	err = serverConn.HandleStreamFrame(clientControlOut.StreamID(), cOffset, cData, cFin)
	if err != nil {
		t.Fatalf("server HandleStreamFrame error: %v", err)
	}

	serverControlIn, err := serverConn.AcceptUniStream(context.Background())
	if err != nil {
		t.Fatalf("server AcceptUniStream error: %v", err)
	}

	r := varint.NewReader(serverControlIn)
	streamType, err := varint.Read(r)
	if err != nil {
		t.Fatalf("read stream type error: %v", err)
	}
	if streamType != coreh3.StreamTypeControl {
		t.Fatalf("expected stream type control (0x00), got 0x%x", streamType)
	}

	frameType, payloadLen, err := coreh3.ReadFrameHeader(r)
	if err != nil {
		t.Fatalf("read frame header error: %v", err)
	}
	if frameType != coreh3.FrameTypeSettings {
		t.Fatalf("expected SETTINGS frame (0x04), got 0x%x", frameType)
	}
	settingsPayload := make([]byte, payloadLen)
	_, err = io.ReadFull(r, settingsPayload)
	if err != nil {
		t.Fatalf("read settings payload error: %v", err)
	}

	// 2. HTTP/3 Request / Response Stream (RFC 9114 §4.1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	clientReqStream, err := clientConn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatalf("OpenStreamSync error: %v", err)
	}

	// Client sends request DATA frame
	requestBody := []byte(`{"ping":true}`)
	var reqBuf []byte
	reqBuf = varint.Append(reqBuf, coreh3.FrameTypeData)
	reqBuf = varint.Append(reqBuf, uint64(len(requestBody)))
	reqBuf = append(reqBuf, requestBody...)

	_, err = clientReqStream.Write(reqBuf)
	if err != nil {
		t.Fatalf("write request error: %v", err)
	}
	_ = clientReqStream.Close() // send FIN

	// Deliver to server
	frameData, offset, fin, ok := clientReqStream.SendStream().PopStreamFrame(4096)
	if !ok {
		t.Fatal("expected request frame")
	}
	err = serverConn.HandleStreamFrame(clientReqStream.StreamID(), offset, frameData, fin)
	if err != nil {
		t.Fatalf("server handle request stream frame error: %v", err)
	}

	serverReqStream, err := serverConn.AcceptStream(ctx)
	if err != nil {
		t.Fatalf("server AcceptStream error: %v", err)
	}

	sr := varint.NewReader(serverReqStream)
	sFrameType, sPayloadLen, err := coreh3.ReadFrameHeader(sr)
	if err != nil {
		t.Fatalf("server read request frame header error: %v", err)
	}
	if sFrameType != coreh3.FrameTypeData {
		t.Fatalf("expected DATA frame type, got 0x%x", sFrameType)
	}
	readBody := make([]byte, sPayloadLen)
	_, err = io.ReadFull(sr, readBody)
	if err != nil {
		t.Fatalf("server read request body error: %v", err)
	}
	if !bytes.Equal(readBody, requestBody) {
		t.Fatalf("request body mismatch: %q vs %q", string(readBody), string(requestBody))
	}

	// Server sends response DATA frame
	responseBody := []byte(`{"pong":true}`)
	var respBuf []byte
	respBuf = varint.Append(respBuf, coreh3.FrameTypeData)
	respBuf = varint.Append(respBuf, uint64(len(responseBody)))
	respBuf = append(respBuf, responseBody...)

	_, err = serverReqStream.Write(respBuf)
	if err != nil {
		t.Fatalf("server write response error: %v", err)
	}
	_ = serverReqStream.Close()

	// Deliver to client
	pipeFrames(clientReqStream, serverReqStream)

	cr := varint.NewReader(clientReqStream)
	cFrameType, cPayloadLen, err := coreh3.ReadFrameHeader(cr)
	if err != nil {
		t.Fatalf("client read response frame header error: %v", err)
	}
	if cFrameType != coreh3.FrameTypeData {
		t.Fatalf("expected DATA frame type, got 0x%x", cFrameType)
	}
	cResponseBody := make([]byte, cPayloadLen)
	_, err = io.ReadFull(cr, cResponseBody)
	if err != nil {
		t.Fatalf("client read response body error: %v", err)
	}
	if !bytes.Equal(cResponseBody, responseBody) {
		t.Fatalf("response body mismatch: %q vs %q", string(cResponseBody), string(responseBody))
	}

	// Final EOF on client stream
	extra := make([]byte, 1)
	_, err = clientReqStream.Read(extra)
	if err != io.EOF {
		t.Fatalf("expected io.EOF on completed stream, got %v", err)
	}
}
