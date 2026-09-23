// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"github.com/lemon4ksan/foundation/net/quic"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

// errCodeH3RequestCancelled represents the HTTP/3 request cancellation error code (0x010c)
// transmitted over QUIC streams when a request context is aborted (RFC 9114 §8.1).
const errCodeH3RequestCancelled = quic.StreamErrorCode(coreh3.ErrCodeH3RequestCancelled)

// ClientConn manages HTTP/3 frame exchanges over an underlying QUIC connection session
// (RFC 9114 §3, §4, §6, §7, and RFC 9000).
//
// It provides multiplexed request/response handling over bidirectional QUIC streams,
// manages outbound and inbound unidirectional control streams, and orchestrates QPACK
// header compression (RFC 9204).
//
// Concurrency:
// ClientConn is safe for concurrent use by multiple goroutines. Request methods
// (Do, DoScoped) open independent bidirectional QUIC streams and execute concurrently.
// Lifecycle methods (Close, IsClosed) are coordinated via sync.Once and atomic operations.
type ClientConn struct {
	conn             *quic.Conn
	Transport        *quic.Transport
	UnderlyingCloser io.Closer
	qpack            *coreh3.QPACKCodec
	settings         coreh3.Settings

	closeOnce       sync.Once
	closed          chan struct{}
	hasControlIn    atomic.Bool
	hasQPACKEncoder atomic.Bool
	hasQPACKDecoder atomic.Bool
}

// NewClientConn initializes an HTTP/3 client connection and opens control streams (RFC 9114 §3.2 & §6.2.1).
//
// It configures default HTTP/3 settings if nil (setting MaxFieldSectionSize to 256KB and enabling datagrams),
// attaches a protocol error handler to the QPACK codec, creates the client control stream sending initial
// SETTINGS, and starts a background goroutine to demultiplex inbound peer unidirectional streams.
//
// Concurrency: Safe for immediate concurrent use upon return.
func NewClientConn(conn *quic.Conn, settings *coreh3.Settings) (*ClientConn, error) {
	if settings == nil {
		settings = &coreh3.Settings{
			MaxFieldSectionSize: 262144,
			EnableDatagrams:     true,
		}
	}

	cc := &ClientConn{
		conn:     conn,
		qpack:    coreh3.NewQPACKCodec(),
		settings: *settings,
		closed:   make(chan struct{}),
	}

	cc.qpack.SetErrorHandler(func(err error) {
		var qErr *coreh3.QPACKStreamError
		if errors.As(err, &qErr) {
			_ = cc.conn.CloseWithError(quic.ApplicationErrorCode(qErr.Code), qErr.Message)
		} else {
			_ = cc.conn.CloseWithError(quic.ApplicationErrorCode(coreh3.ErrCodeH3GeneralProtocolError), err.Error())
		}
	})

	if err := cc.setupControlStream(); err != nil {
		_ = conn.CloseWithError(quic.ApplicationErrorCode(coreh3.ErrCodeH3NoError), "failed control stream setup")

		return nil, err
	}

	go cc.readUnidirectionalStreams()

	return cc, nil
}

// IsClosed reports whether the HTTP/3 client connection has been closed or its underlying
// QUIC connection context has expired (RFC 9114 §5.2).
//
// Concurrency: Thread-safe; lock-free select and atomic context query.
func (cc *ClientConn) IsClosed() bool {
	if cc == nil {
		return true
	}

	select {
	case <-cc.closed:
		return true
	default:
		if cc.conn == nil {
			return true
		}

		return cc.conn.Context().Err() != nil
	}
}

// Close gracefully terminates the HTTP/3 client connection and releases transport sockets
// with error code H3_NO_ERROR (0x0100) (RFC 9114 §5.2, RFC 9000 §10.2).
//
// Concurrency: Thread-safe and idempotent; executed at most once via sync.Once.
func (cc *ClientConn) Close() error {
	cc.closeOnce.Do(func() {
		close(cc.closed)

		if cc.conn != nil {
			_ = cc.conn.CloseWithError(quic.ApplicationErrorCode(coreh3.ErrCodeH3NoError), "connection closed")
		}

		if cc.Transport != nil {
			_ = cc.Transport.Close()
		}

		if cc.UnderlyingCloser != nil {
			_ = cc.UnderlyingCloser.Close()
		}
	})

	return nil
}
