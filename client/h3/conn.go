// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/mach"
	"github.com/lemon4ksan/mach/net/quic"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

var _ mach.Conn = (*ClientConn)(nil)

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
			QpackMaxTableCap:    4096,
			QpackBlockedStreams: 100,
		}
	}

	cc := &ClientConn{
		conn:     conn,
		qpack:    coreh3.NewQPACKCodecWithOptions(settings.QpackMaxTableCap, settings.QpackBlockedStreams, nil),
		settings: *settings,
		closed:   make(chan struct{}),
	}

	cc.qpack.SetErrorHandler(func(err error) {
		if qErr, ok := errors.AsType[*coreh3.QPACKStreamError](err); ok {
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
	go cc.readBidirectionalStreams()

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

// QUICConn yields the underlying QUIC connection session.
func (cc *ClientConn) QUICConn() *quic.Conn {
	return cc.conn
}

// OpenStream creates a new bidirectional HTTP/3 stream satisfying mach.Conn.
func (cc *ClientConn) OpenStream(ctx context.Context) (mach.Stream, error) {
	if cc.conn == nil {
		return nil, errors.New("h3: connection is nil")
	}

	str, err := cc.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	return NewStream(str), nil
}

// readBidirectionalStreams rejects server-initiated bidirectional streams with H3_STREAM_CREATION_ERROR (RFC 9114 §6.1).
func (cc *ClientConn) readBidirectionalStreams() {
	str, err := cc.conn.AcceptStream(context.Background())
	if err != nil {
		return
	}

	_ = cc.conn.CloseWithError(
		quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
		"server-initiated bidirectional stream (RFC 9114 §6.1)",
	)
	_ = str.Close()
}

// EnableConnect reports whether the peer advertised SETTINGS_ENABLE_CONNECT_PROTOCOL (RFC 9220 §3).
func (cc *ClientConn) EnableConnect() bool {
	return cc.settings.EnableConnect
}

// Settings returns the active settings received from the peer.
func (cc *ClientConn) Settings() coreh3.Settings {
	return cc.settings
}

// ExtendedConnectStream adapts an HTTP/3 Extended CONNECT stream to io.ReadWriteCloser.
type ExtendedConnectStream struct {
	stream     *quic.Stream
	br         *bufio.Reader
	readRemain int64
	closed     atomic.Bool
}

func (s *ExtendedConnectStream) Read(p []byte) (int, error) {
	for s.readRemain == 0 {
		frameType, frameLen, err := coreh3.ReadFrameHeader(s.br)
		if err != nil {
			return 0, err
		}
		if frameType == coreh3.FrameTypeData {
			s.readRemain = int64(frameLen)
			if s.readRemain == 0 {
				continue
			}
			break
		}
		if frameLen > 0 {
			lr := io.LimitReader(s.br, int64(frameLen))
			_, _ = io.Copy(io.Discard, lr)
		}
	}

	toRead := int64(len(p))
	if toRead > s.readRemain {
		toRead = s.readRemain
	}
	n, err := s.br.Read(p[:toRead])
	if n > 0 {
		s.readRemain -= int64(n)
	}
	return n, err
}

func (s *ExtendedConnectStream) Write(p []byte) (int, error) {
	if s.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 {
		return 0, nil
	}
	var hdr [16]byte
	b := varint.Append(hdr[:0], coreh3.FrameTypeData)
	b = varint.Append(b, uint64(len(p)))
	if _, err := s.stream.Write(b); err != nil {
		return 0, err
	}
	return s.stream.Write(p)
}

func (s *ExtendedConnectStream) Close() error {
	if s.closed.CompareAndSwap(false, true) {
		return s.stream.Close()
	}
	return nil
}

// StartExtendedConnect initiates an Extended CONNECT stream (RFC 9220 §4).
func (cc *ClientConn) StartExtendedConnect(ctx context.Context, req *h1.Request) (*ExtendedConnectStream, *h1.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	str, err := cc.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, nil, err
	}

	streamID := uint64(str.StreamID()) //nolint:gosec
	if err := cc.sendRequestTo(str, req, nil, streamID); err != nil {
		_ = str.Close()
		return nil, nil, err
	}

	br := bufio.NewReaderSize(str, 4096)
	frameType, payloadLen, err := coreh3.ReadFrameHeader(br)
	if err != nil {
		_ = str.Close()
		return nil, nil, err
	}

	if frameType != coreh3.FrameTypeHeaders {
		_ = str.Close()
		return nil, nil, fmt.Errorf("h3: expected HEADERS frame (0x01), got 0x%x", frameType)
	}

	headerBlock := make([]byte, payloadLen)
	if _, err := io.ReadFull(br, headerBlock); err != nil {
		_ = str.Close()
		return nil, nil, err
	}

	resp := h1.AcquireResponse()
	statusCode, err := cc.qpack.DecodeResponseHeaders(streamID, headerBlock, &resp.Header)
	if err != nil {
		h1.ReleaseResponse(resp)
		_ = str.Close()
		return nil, nil, err
	}

	if statusCode != 200 {
		_ = str.Close()
		return nil, resp, fmt.Errorf("h3: unexpected Extended CONNECT response status: %d", statusCode)
	}

	stream := &ExtendedConnectStream{
		stream: str,
		br:     br,
	}
	return stream, resp, nil
}
