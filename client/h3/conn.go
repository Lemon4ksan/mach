// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/silicon/pool"

	"github.com/lemon4ksan/foundation/net/quic"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

const errCodeH3RequestCancelled = quic.StreamErrorCode(coreh3.ErrCodeH3RequestCancelled)

var (
	dataBufPool = generic.NewPool(func() *[]byte {
		b := make([]byte, 32768)
		return &b
	})

	h3RequestStorage = pool.NewPerPStorage(func() *[]byte {
		b := make([]byte, 0, 65536)
		return &b
	})

	h3HeaderBlockStorage = pool.NewPerPStorage(func() *[]byte {
		b := make([]byte, 0, 16384)
		return &b
	})
)

// ClientConn manages HTTP/3 frame exchanges over a quic.Conn session (RFC 9114 §3, §4, §6 & §7).
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

	if err := cc.setupControlStream(); err != nil {
		_ = conn.CloseWithError(quic.ApplicationErrorCode(coreh3.ErrCodeH3NoError), "failed control stream setup")
		return nil, err
	}

	go cc.readUnidirectionalStreams()

	return cc, nil
}

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

func (cc *ClientConn) setupControlStream() error {
	str, err := cc.conn.OpenUniStream()
	if err != nil {
		return err
	}

	var buf []byte

	buf = varint.Append(buf, coreh3.StreamTypeControl)
	buf = append(buf, cc.settings.Encode()...)

	_, err = str.Write(buf)

	return err
}

func (cc *ClientConn) readUnidirectionalStreams() {
	for {
		str, err := cc.conn.AcceptUniStream(context.Background())
		if err != nil {
			return
		}

		go cc.handleUnidirectionalStream(str)
	}
}

func (cc *ClientConn) handleUnidirectionalStream(str *quic.ReceiveStream) {
	r := varint.NewReader(str)

	streamType, err := varint.Read(r)
	if err != nil {
		return
	}

	switch streamType {
	case coreh3.StreamTypeControl:
		if cc.hasControlIn.Swap(true) {
			_ = cc.conn.CloseWithError(quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError), "duplicate control stream")
			return
		}
		cc.handleControlStream(r)
	case coreh3.StreamTypeQPACKEncoder:
		if cc.hasQPACKEncoder.Swap(true) {
			_ = cc.conn.CloseWithError(quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError), "duplicate QPACK encoder stream")
			return
		}
		_, _ = io.Copy(io.Discard, str)
	case coreh3.StreamTypeQPACKDecoder:
		if cc.hasQPACKDecoder.Swap(true) {
			_ = cc.conn.CloseWithError(quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError), "duplicate QPACK decoder stream")
			return
		}
		_, _ = io.Copy(io.Discard, str)
	default:
		// Unknown unidirectional stream: RFC 9114 §6.2
	}
}

func (cc *ClientConn) handleControlStream(r varint.Reader) {
	firstFrame := true

	for {
		frameType, payloadLen, err := coreh3.ReadFrameHeader(r)
		if err != nil {
			return
		}

		if firstFrame {
			if frameType != coreh3.FrameTypeSettings {
				_ = cc.conn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3MissingSettings),
					"H3_MISSING_SETTINGS: first frame must be SETTINGS (RFC 9114 §7.2.4)",
				)

				return
			}

			firstFrame = false
		}

		switch frameType {
		case coreh3.FrameTypeSettings:
			st, err := coreh3.DecodeSettings(r, payloadLen)
			if err != nil {
				if errors.Is(err, coreh3.ErrH3SettingsError) {
					_ = cc.conn.CloseWithError(
						quic.ApplicationErrorCode(coreh3.ErrCodeH3SettingsError),
						"H3_SETTINGS_ERROR: reserved H2 setting ID (RFC 9114 §7.2.4.1)",
					)
				} else {
					_ = cc.conn.CloseWithError(
						quic.ApplicationErrorCode(coreh3.ErrCodeH3SettingsError),
						"H3_SETTINGS_ERROR: invalid settings payload (RFC 9114 §7.2.4)",
					)
				}

				return
			}

			cc.settings = *st

		case coreh3.FrameTypeGoAway:
			cc.handleGoAway(r, payloadLen)
			return

		default:
			if _, err := io.CopyN(io.Discard, r, int64(payloadLen)); err != nil { //nolint:gosec
				return
			}
		}
	}
}

func (cc *ClientConn) handleGoAway(r varint.Reader, payloadLen uint64) {
	if payloadLen > 0 {
		_, _ = varint.Read(r)
	}

	_ = cc.Close()
}

// Do executes a h1.Request over a QUIC stream and populates h1.Response and captured trailers.
func (cc *ClientConn) Do(
	ctx context.Context,
	req *h1.Request,
	resp *h1.Response,
	headerOrder []string,
) (map[string][]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	str, err := cc.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			str.CancelWrite(errCodeH3RequestCancelled)
			str.CancelRead(errCodeH3RequestCancelled)
		case <-done:
		}
	}()

	defer str.Close() //nolint:errcheck

	if err := cc.sendRequest(str, req, headerOrder); err != nil {
		return nil, err
	}

	return cc.readResponse(str, resp)
}

// DoScoped executes a h1.Request over a QUIC stream and scopes response body allocations to s.
func (cc *ClientConn) DoScoped(
	ctx context.Context,
	req *h1.Request,
	resp *h1.Response,
	headerOrder []string,
	s *borrow.Scope,
) (map[string][]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	str, err := cc.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			str.CancelWrite(errCodeH3RequestCancelled)
			str.CancelRead(errCodeH3RequestCancelled)
		case <-done:
		}
	}()

	defer str.Close() //nolint:errcheck

	if err := cc.sendRequest(str, req, headerOrder); err != nil {
		return nil, err
	}

	return cc.readResponseScoped(str, resp, s)
}

func (cc *ClientConn) readResponseScoped(
	reader io.Reader,
	resp *h1.Response,
	_ *borrow.Scope,
) (trailers map[string][]string, err error) {
	return cc.readResponseFrom(reader, resp, 0)
}

func (cc *ClientConn) sendRequest(str *quic.Stream, req *h1.Request, headerOrder []string) error {
	return cc.sendRequestTo(str, req, headerOrder, uint64(str.StreamID()))
}

func (cc *ClientConn) sendRequestTo(w io.Writer, req *h1.Request, headerOrder []string, streamID uint64) error {
	// Encode headers into a block so we know the size. Wait, we don't have it natively anymore unless we use a buffer.
	// We'll write to a buffer first.
	buf := bytes.Buffer{}
	if err := cc.qpack.EncodeRequestHeaders(streamID, &buf, req, headerOrder); err != nil {
		return err
	}
	headerBlock := buf.Bytes()
	
	body := req.Body()

	headLen := varint.Len(coreh3.FrameTypeHeaders) + varint.Len(uint64(len(headerBlock)))
	totalLen := headLen + len(headerBlock)
	if len(body) > 0 {
		totalLen += varint.Len(coreh3.FrameTypeData) + varint.Len(uint64(len(body))) + len(body)
	}

	var (
		stackOut [8192]byte
		out      []byte
	)

	if totalLen <= len(stackOut) {
		out = stackOut[:0]
	} else {
		out = make([]byte, 0, totalLen)
	}

	out = varint.Append(out, coreh3.FrameTypeHeaders)
	out = varint.Append(out, uint64(len(headerBlock)))
	out = append(out, headerBlock...)

	if len(body) > 0 {
		out = varint.Append(out, coreh3.FrameTypeData)
		out = varint.Append(out, uint64(len(body)))
		out = append(out, body...)
	}

	_, err := w.Write(out)

	return err
}

func (cc *ClientConn) readResponse(
	str *quic.Stream,
	resp *h1.Response,
) (trailers map[string][]string, err error) {
	return cc.readResponseFrom(str, resp, uint64((*str).StreamID()))
}

func (cc *ClientConn) readResponseFrom(
	reader io.Reader,
	resp *h1.Response,
	streamID uint64,
) (trailers map[string][]string, err error) {
	r := varint.NewReader(reader)
	headersParsed := false

	var stackHeaderBuf [4096]byte

	for {
		frameType, payloadLen, err := coreh3.ReadFrameHeader(r)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, err
		}

		switch frameType {
		case coreh3.FrameTypeHeaders:
			if payloadLen > 16*1024*1024 {
				return nil, fmt.Errorf("aoni/h3engine: headers payload too large: %d", payloadLen)
			}

			var (
				headerBlock   []byte
				heapHeaderBuf *[]byte
			)

			if payloadLen <= uint64(len(stackHeaderBuf)) {
				headerBlock = stackHeaderBuf[:payloadLen]
			} else {
				heapHeaderBuf = h3HeaderBlockStorage.Get()

				b := (*heapHeaderBuf)[:0]
				if uint64(cap(b)) < payloadLen {
					b = make([]byte, payloadLen)
				} else {
					b = b[:payloadLen]
				}

				headerBlock = b
			}

			if _, err := io.ReadFull(r, headerBlock); err != nil {
				if heapHeaderBuf != nil {
					*heapHeaderBuf = (*heapHeaderBuf)[:0]
					h3HeaderBlockStorage.Put(heapHeaderBuf)
				}

				return nil, err
			}

			if headersParsed {
				trailers, err = cc.qpack.DecodeResponseTrailers(streamID, headerBlock)

				if heapHeaderBuf != nil {
					*heapHeaderBuf = (*heapHeaderBuf)[:0]
					h3HeaderBlockStorage.Put(heapHeaderBuf)
				}

				if err != nil {
					return nil, err
				}
			} else {
				statusCode, err := cc.qpack.DecodeResponseHeaders(streamID, headerBlock, &resp.Header)

				if heapHeaderBuf != nil {
					*heapHeaderBuf = (*heapHeaderBuf)[:0]
					h3HeaderBlockStorage.Put(heapHeaderBuf)
				}

				if err != nil {
					return nil, err
				}

				if statusCode < 100 || statusCode >= 200 || statusCode == 101 {
					headersParsed = true
				}
			}

		case coreh3.FrameTypeData:
			if !headersParsed {
				return nil, coreh3.ErrFrameUnexpected
			}

			lr := io.LimitReader(r, int64(payloadLen)) //nolint:gosec
			bufPtr := dataBufPool.Get()
			buf := *bufPtr

			for {
				n, rErr := lr.Read(buf)
				if n > 0 {
					resp.AppendBody(buf[:n])
				}

				if rErr == io.EOF {
					break
				}

				if rErr != nil {
					dataBufPool.Put(bufPtr)
					return nil, rErr
				}
			}

			dataBufPool.Put(bufPtr)

		default:
			if _, err := io.CopyN(io.Discard, r, int64(payloadLen)); err != nil { //nolint:gosec
				return nil, err
			}
		}
	}

	return trailers, nil
}

// Close gracefully terminates the HTTP/3 client connection and releases transport sockets.
func (cc *ClientConn) Close() error {
	cc.closeOnce.Do(func() {
		close(cc.closed)

		if cc.conn != nil {
			_ = cc.conn.CloseWithError(0x100, "connection closed")
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
