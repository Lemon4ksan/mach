// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"errors"
	"fmt"
	"io"

	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/net/quic"
	"github.com/lemon4ksan/foundation/silicon/pool"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

var (
	dataBufPool = generic.NewPool(func() *[]byte {
		b := make([]byte, 32768)
		return &b
	})

	h3HeaderBlockStorage = pool.NewPerPStorage(func() *[]byte {
		b := make([]byte, 0, 16384)
		return &b
	})
)

func (cc *ClientConn) readResponse(
	str *quic.Stream,
	resp *h1.Response,
) (trailers map[string][]string, err error) {
	streamID := uint64(str.StreamID()) //nolint:gosec // StreamID is positive

	return cc.readResponseFrom(str, resp, streamID)
}

func (cc *ClientConn) readResponseScoped(
	reader io.Reader,
	resp *h1.Response,
	_ *borrow.Scope,
) (trailers map[string][]string, err error) {
	var streamID uint64
	if s, ok := reader.(*quic.Stream); ok {
		streamID = uint64(s.StreamID()) //nolint:gosec
	} else if s, ok := reader.(interface{ StreamID() int64 }); ok {
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
