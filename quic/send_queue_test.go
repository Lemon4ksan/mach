// Copyright (c) 2016 the quic-go authors. All rights reserved.
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quic

import (
	"net"
	"net/netip"
	"testing"
	"testing/synctest"

	"github.com/lemon4ksan/foundation/testkit/gomock"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
)

func getPacketWithContents(b []byte) *packetBuffer {
	buf := getPacketBuffer()
	buf.Data = buf.Data[:len(b)]
	copy(buf.Data, b)

	return buf
}

func TestSendQueueSendOnePacket(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		c := NewMockSendConn(mockCtrl)
		q := newSendQueue(c, nil)

		written := make(chan struct{})
		c.EXPECT().Write([]byte("foobar"), uint16(10), protocol.ECT1).Do(
			func([]byte, uint16, protocol.ECN) error { close(written); return nil },
		)

		q.Send(getPacketWithContents([]byte("foobar")), 10, protocol.ECT1)
		synctest.Wait()

		select {
		case <-written:
		default:
			t.Fatal("write should have returned")
		}
	})
}

func TestSendQueueSendProbe(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	c := NewMockSendConn(mockCtrl)
	q := newSendQueue(c, nil)

	addr := &net.UDPAddr{IP: net.IPv4(42, 42, 42, 42), Port: 42}
	localAddr := netip.MustParseAddr("43.43.43.43")
	c.EXPECT().WriteTo([]byte("foobar"), addr, packetInfo{
		addr: localAddr,
	})
	q.SendProbe(getPacketWithContents([]byte("foobar")), addr, packetInfo{
		addr: localAddr,
	})
}
