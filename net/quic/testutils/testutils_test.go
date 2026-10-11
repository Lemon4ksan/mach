// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutils

import (
	"testing"

	"github.com/lemon4ksan/mach/net/quic/internal/protocol"
	"github.com/lemon4ksan/mach/net/quic/internal/wire"
)

func TestFrameAliases(t *testing.T) {
	frames := []Frame{
		&AckFrame{},
		&ConnectionCloseFrame{},
		&CryptoFrame{},
		&DataBlockedFrame{},
		&HandshakeDoneFrame{},
		&MaxDataFrame{},
		&MaxStreamDataFrame{},
		&MaxStreamsFrame{},
		&NewConnectionIDFrame{},
		&NewTokenFrame{},
		&PathChallengeFrame{},
		&PathResponseFrame{},
		&PingFrame{},
		&ResetStreamFrame{},
		&RetireConnectionIDFrame{},
		&StopSendingFrame{},
		&StreamDataBlockedFrame{},
		&StreamFrame{},
		&StreamsBlockedFrame{},
	}

	if len(frames) != 19 {
		t.Fatalf("expected 19 frame types, got %d", len(frames))
	}
}

func TestComposeInitialPacket(t *testing.T) {
	srcConnID := protocol.ParseConnectionID([]byte{1, 2, 3, 4})
	destConnID := protocol.ParseConnectionID([]byte{5, 6, 7, 8})
	key := protocol.ParseConnectionID([]byte{9, 10, 11, 12})
	token := []byte{0xaa, 0xbb}
	frames := []wire.Frame{&wire.PingFrame{}}

	pkt := ComposeInitialPacket(
		srcConnID,
		destConnID,
		key,
		token,
		frames,
		protocol.PerspectiveClient,
		protocol.Version1,
	)
	if len(pkt) == 0 {
		t.Fatal("expected non-empty initial packet")
	}

	emptyPkt := ComposeInitialPacket(
		srcConnID,
		destConnID,
		key,
		nil,
		nil,
		protocol.PerspectiveClient,
		protocol.Version1,
	)
	if len(emptyPkt) == 0 {
		t.Fatal("expected non-empty initial packet with nil frames")
	}
}

func TestComposeRetryPacket(t *testing.T) {
	srcConnID := protocol.ParseConnectionID([]byte{1, 2, 3, 4})
	destConnID := protocol.ParseConnectionID([]byte{5, 6, 7, 8})
	origDestConnID := protocol.ParseConnectionID([]byte{9, 10, 11, 12})
	token := []byte("retry-token-bytes")

	retryPkt := ComposeRetryPacket(
		srcConnID,
		destConnID,
		origDestConnID,
		token,
		protocol.Version1,
	)
	if len(retryPkt) == 0 {
		t.Fatal("expected non-empty retry packet")
	}
}
