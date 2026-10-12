// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"errors"
	"fmt"
	"io"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/silicon/pool"

	"github.com/lemon4ksan/mach/net/quic"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

var (
	dataBufPool = generic.NewPool(func() *[]byte {
		b := make([]byte, 32768)
		return &b
	})

	h3HeaderBlockStorage = pool.NewPerCorePool(func() *[]byte {
		b := make([]byte, 0, 16384)
		return &b
	})
)

func (cc *ClientConn) readResponse(
	str *quic.Stream,
	resp *h1.Response,
) (trailers map[string][]string, err error) {
	streamID := uint64(str.StreamID()) //nolint:gosec // StreamID is positive

	trailers, err = cc.readResponseFrom(str, resp, streamID)
	if err != nil {
		if errors.Is(err, coreh3.ErrMalformedHeader) || errors.Is(err, coreh3.ErrMissingStatusHeader) {
			str.CancelRead(quic.StreamErrorCode(coreh3.ErrCodeH3MessageError))
			str.CancelWrite(quic.StreamErrorCode(coreh3.ErrCodeH3MessageError))
		}
	}

	return trailers, err
}

func (cc *ClientConn) readResponseScoped(
	reader io.Reader,
	resp *h1.Response,
) (trailers map[string][]string, err error) {
	var streamID uint64
	switch s := reader.(type) {
	case *quic.Stream:
		streamID = uint64(s.StreamID()) //nolint:gosec
	case interface{ StreamID() int64 }:
		streamID = uint64(s.StreamID()) //nolint:gosec
	}

	return cc.readResponseFrom(reader, resp, streamID)
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
				return nil, fmt.Errorf("mach/h3: headers payload too large: %d", payloadLen)
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
			} else {
				var statusCode int

				statusCode, err = cc.qpack.DecodeResponseHeaders(streamID, headerBlock, &resp.Header)
				if err == nil && (statusCode < 100 || statusCode >= 200 || statusCode == 101) {
					headersParsed = true
				}
			}

			if heapHeaderBuf != nil {
				*heapHeaderBuf = (*heapHeaderBuf)[:0]
				h3HeaderBlockStorage.Put(heapHeaderBuf)
			}

			if err != nil {
				return nil, err
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

			if lim, ok := lr.(*io.LimitedReader); ok && lim.N > 0 {
				return nil, io.ErrUnexpectedEOF
			}

		default:
			if coreh3.IsReservedH2FrameType(frameType) {
				// RFC 9114 §7.2.8 & §11.2.1: Reserved HTTP/2 frame types MUST cause H3_FRAME_UNEXPECTED
				return nil, coreh3.ErrFrameUnexpected
			}

			// Control-only frames received on request/response stream (RFC 9114 Table 1, §7.2.3, §7.2.4, §7.2.6, §7.2.7)
			if frameType == coreh3.FrameTypeSettings ||
				frameType == coreh3.FrameTypeGoAway ||
				frameType == coreh3.FrameTypeCancelPush ||
				frameType == coreh3.FrameTypeMaxPushID {
				return nil, coreh3.ErrFrameUnexpected
			}

			if _, err := io.CopyN(io.Discard, r, int64(payloadLen)); err != nil { //nolint:gosec
				return nil, err
			}
		}
	}

	return trailers, nil
}
