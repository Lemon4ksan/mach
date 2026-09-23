// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"context"
	"errors"
	"io"
	"sync/atomic"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/net/quic"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

// ServerConn manages an active HTTP/3 server connection over an underlying QUIC connection (RFC 9114, RFC 9204, RFC 9000).
//
// Concurrency:
// ServerConn is safe for concurrent use. Bidirectional request streams are dispatched
// concurrently across independent goroutines.
type ServerConn struct {
	quicConn        *quic.Conn
	handler         ServerHandlerFunc
	qpack           *coreh3.QPACKCodec
	isClosed        atomic.Bool
	closeErr        error
	controlOut      *quic.SendStream
	hasControlIn    atomic.Bool
	hasQPACKEncoder atomic.Bool
	hasQPACKDecoder atomic.Bool
}

// NewServerConn creates a new HTTP/3 server connection wrapping a QUIC connection (RFC 9114).
func NewServerConn(quicConn *quic.Conn, handler ServerHandlerFunc) *ServerConn {
	return &ServerConn{
		quicConn: quicConn,
		handler:  handler,
		qpack:    coreh3.NewQPACKCodec(),
	}
}

// Serve initializes control streams and handles incoming bidirectional request streams (RFC 9114 §6.2.1, §6.1).
func (sc *ServerConn) Serve() error {
	ctx := sc.quicConn.Context()

	// 1. Open and initialize server unidirectional control stream (RFC 9114 §6.2.1)
	ctrlStream, err := sc.quicConn.OpenUniStreamSync(ctx)
	if err != nil {
		return err
	}

	sc.controlOut = ctrlStream

	// Write Control Stream Type (0x00)
	var typeBuf [8]byte

	n := varint.Append(typeBuf[:0], coreh3.StreamTypeControl)
	if _, err := ctrlStream.Write(n); err != nil {
		return err
	}

	// Write initial server SETTINGS frame (RFC 9114 §7.2.4)
	st := &coreh3.Settings{
		MaxFieldSectionSize: 64 * 1024,
	}
	if _, err := ctrlStream.Write(st.Encode()); err != nil {
		return err
	}

	// 2. Accept client-initiated unidirectional streams (Client control & QPACK streams) in background
	go sc.acceptUniStreams()

	// 3. Main loop: accept client-initiated bidirectional request streams (RFC 9114 §6.1)
	for {
		if sc.isClosed.Load() {
			return sc.closeErr
		}

		stream, err := sc.quicConn.AcceptStream(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return nil
			}

			return err
		}

		go sc.handleRequestStream(stream)
	}
}

// Close gracefully closes the HTTP/3 connection with H3_NO_ERROR (0x0100) (RFC 9114 §8.1).
func (sc *ServerConn) Close() error {
	sc.isClosed.Store(true)
	return sc.quicConn.CloseWithError(0x0100, "h3 normal closure")
}
