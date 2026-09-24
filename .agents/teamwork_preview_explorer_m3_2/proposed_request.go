// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"bytes"
	"context"
	"io"

	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/net/quic"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

var requestHeaderBufferPool = generic.NewPool(func() *bytes.Buffer {
	return bytes.NewBuffer(make([]byte, 0, 4096))
})

// Do executes an HTTP request over a new bidirectional QUIC stream and populates resp
// with the decoded status line, headers, and body, returning any response trailers (RFC 9114 §4.1).
//
// Lifecycle:
// 1. Opens a synchronous bidirectional stream on the QUIC connection.
// 2. Starts a cancellation watcher that issues CancelWrite/CancelRead with H3_REQUEST_CANCELLED if ctx expires.
// 3. Encodes and transmits the request HEADERS frame and optional DATA frames.
// 4. Closes the write half of the stream (sending FIN).
// 5. Reads and decodes response HEADERS (including 1xx informational responses), DATA frames, and trailing headers.
//
// Concurrency: Safe for concurrent invocation across multiple goroutines.
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
	cancelDone := make(chan struct{})

	go func() {
		defer close(cancelDone)

		select {
		case <-ctx.Done():
			str.CancelWrite(errCodeH3RequestCancelled)
			str.CancelRead(errCodeH3RequestCancelled)
		case <-done:
		}
	}()

	defer str.Close() //nolint:errcheck

	if err := cc.sendRequest(str, req, headerOrder); err != nil {
		close(done)
		<-cancelDone

		return nil, err
	}

	if err := str.Close(); err != nil {
		close(done)
		<-cancelDone

		return nil, err
	}

	trailers, rErr := cc.readResponse(str, resp)

	close(done)
	<-cancelDone

	return trailers, rErr
}

// DoScoped executes an HTTP request over a new bidirectional QUIC stream and scopes memory allocations
// to borrow.Scope s for zero-allocation request/response pipelines (RFC 9114 §4.1).
//
// Concurrency: Safe for concurrent invocation across multiple goroutines.
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
	cancelDone := make(chan struct{})

	go func() {
		defer close(cancelDone)

		select {
		case <-ctx.Done():
			str.CancelWrite(errCodeH3RequestCancelled)
			str.CancelRead(errCodeH3RequestCancelled)
		case <-done:
		}
	}()

	defer str.Close() //nolint:errcheck

	if err := cc.sendRequest(str, req, headerOrder); err != nil {
		close(done)
		<-cancelDone

		return nil, err
	}

	if err := str.Close(); err != nil {
		close(done)
		<-cancelDone

		return nil, err
	}

	trailers, rErr := cc.readResponseScoped(str, resp, s)

	close(done)
	<-cancelDone

	return trailers, rErr
}

func (cc *ClientConn) sendRequest(str *quic.Stream, req *h1.Request, headerOrder []string) error {
	streamID := uint64(str.StreamID()) //nolint:gosec // StreamID is positive

	return cc.sendRequestTo(str, req, headerOrder, streamID)
}

func (cc *ClientConn) sendRequestTo(w io.Writer, req *h1.Request, headerOrder []string, streamID uint64) error {
	buf := requestHeaderBufferPool.Get()
	buf.Reset()
	defer requestHeaderBufferPool.Put(buf)

	if err := cc.qpack.EncodeRequestHeaders(streamID, buf, req, headerOrder); err != nil {
		return err
	}

	headerBlock := buf.Bytes()
	body := req.Body()

	headLen := varint.Len(coreh3.FrameTypeHeaders) + varint.Len(uint64(len(headerBlock)))
	totalLen := headLen + len(headerBlock)
	if len(body) > 0 {
		totalLen += varint.Len(coreh3.FrameTypeData) + varint.Len(uint64(len(body))) + len(body)
	}

	var stackOut [8192]byte

	if totalLen <= len(stackOut) {
		out := stackOut[:0]
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

	// For payloads exceeding the 8KB stack buffer, write framed headers followed by DATA frame
	// without allocating the entire payload slice on the heap.
	out := stackOut[:0]
	out = varint.Append(out, coreh3.FrameTypeHeaders)
	out = varint.Append(out, uint64(len(headerBlock)))
	out = append(out, headerBlock...)

	if len(body) > 0 {
		out = varint.Append(out, coreh3.FrameTypeData)
		out = varint.Append(out, uint64(len(body)))
	}

	if _, err := w.Write(out); err != nil {
		return err
	}

	if len(body) > 0 {
		_, err := w.Write(body)

		return err
	}

	return nil
}
