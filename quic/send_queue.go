// Copyright (c) 2016 the quic-go authors. All rights reserved.
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quic

import (
	"net"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
)

type sender interface {
	Send(p *packetBuffer, gsoSize uint16, ecn protocol.ECN)
	SendProbe(*packetBuffer, net.Addr, packetInfo)
	Run() error
	WouldBlock() bool
	Available() <-chan struct{}
	Close()
}

type syncSender struct {
	conn    sendConn
	onError func(error)
}

func newSendQueue(conn sendConn, onError func(error)) sender {
	return &syncSender{conn: conn, onError: onError}
}

func (h *syncSender) Send(p *packetBuffer, gsoSize uint16, ecn protocol.ECN) {
	if err := h.conn.Write(p.Data, gsoSize, ecn); err != nil {
		if !isSendMsgSizeErr(err) && h.onError != nil {
			h.onError(err)
		}
	}

	p.Release()
}

func (h *syncSender) SendProbe(p *packetBuffer, addr net.Addr, info packetInfo) {
	_ = h.conn.WriteTo(p.Data, addr, info)
}

func (h *syncSender) WouldBlock() bool {
	return false
}

func (h *syncSender) Available() <-chan struct{} {
	return nil
}

func (h *syncSender) Run() error {
	return nil
}

func (h *syncSender) Close() {}
