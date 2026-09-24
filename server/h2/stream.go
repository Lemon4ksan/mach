// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"bytes"
	"context"
	"net/http"

	"github.com/lemon4ksan/mach/hpack"
	"github.com/lemon4ksan/mach/proto/http/status"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

type streamState uint8

const (
	streamOpen streamState = iota
	streamHalfClosedRemote
	streamClosed
)

type serverStream struct {
	id          uint32
	method      string
	path        string
	scheme      string
	authority   string
	protocol    string
	headers     http.Header
	headerBlock bytes.Buffer
	body        bytes.Buffer
	endHeaders  bool
	endStream   bool
	state       streamState
	sendWindow  int32
	ctx         context.Context
	cancel      context.CancelFunc
}

func (sc *ServerConn) newServerStream(streamID uint32, endHeaders, endStream bool) *serverStream {
	ctx, cancel := context.WithCancel(sc.ctx)

	return &serverStream{
		id:         streamID,
		headers:    make(http.Header),
		endHeaders: endHeaders,
		endStream:  endStream,
		state:      streamOpen,
		sendWindow: sc.peerInitialWin,
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (sc *ServerConn) startStream(st *serverStream) {
	st.state = streamHalfClosedRemote

	sc.streamsWg.Add(1) // Escalation 2 fix: Track active stream goroutine
	go sc.dispatchStream(st)
}

func (sc *ServerConn) finishHeaderBlock(st *serverStream) error {
	rawBlock := st.headerBlock.Bytes()

	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	var hasSeenRegularHeader bool
	for len(rawBlock) > 0 {
		hf.Reset()

		var err error

		rawBlock, err = sc.hpackDec.Next(hf, rawBlock)
		if err != nil {
			// RFC 7541 & RFC 9113 §4.3: HPACK decoding errors MUST be treated as COMPRESSION_ERROR
			return coreh2.CompressionError
		}

		if hf.Empty() {
			continue
		}

		k := string(hf.KeyBytes())
		v := string(hf.ValueBytes())

		// RFC 9113 §8.2: All field names MUST be lowercase ASCII
		for i := 0; i < len(k); i++ {
			if k[i] >= 'A' && k[i] <= 'Z' {
				return coreh2.ProtocolError
			}
		}

		if hf.IsPseudo() {
			// RFC 9113 §8.3: Pseudo-headers MUST appear before regular headers
			if hasSeenRegularHeader {
				return coreh2.ProtocolError
			}

			switch k {
			case ":method":
				if st.method != "" {
					return coreh2.ProtocolError
				}

				st.method = v

			case ":path":
				if st.path != "" {
					return coreh2.ProtocolError
				}

				st.path = v

			case ":scheme":
				if st.scheme != "" {
					return coreh2.ProtocolError
				}

				st.scheme = v

			case ":authority":
				if st.authority != "" {
					return coreh2.ProtocolError
				}

				st.authority = v

			case ":protocol":
				// RFC 8441 §4: Extended CONNECT pseudo-header
				if st.protocol != "" {
					return coreh2.ProtocolError
				}

				st.protocol = v

			default:
				// RFC 9113 §8.3: Unknown or invalid pseudo-header
				return coreh2.ProtocolError
			}
		} else {
			hasSeenRegularHeader = true

			// RFC 9113 §8.2.2: Connection-specific headers are prohibited in HTTP/2
			switch k {
			case "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade":
				return coreh2.ProtocolError
			case "te":
				if v != "trailers" {
					return coreh2.ProtocolError
				}
			}

			st.headers.Add(k, v)
		}
	}

	// RFC 9113 §8.3.1 & RFC 8441 §4: Mandatory request pseudo-headers
	if st.method == "" {
		return coreh2.ProtocolError
	}

	if st.protocol != "" {
		// RFC 8441 §4: :protocol pseudo-header is only valid on CONNECT requests with :scheme and :path
		if st.method != "CONNECT" || st.scheme == "" || st.path == "" {
			return coreh2.ProtocolError
		}
	} else if st.method != "CONNECT" && (st.scheme == "" || st.path == "") {
		return coreh2.ProtocolError
	}

	if st.endStream {
		sc.startStream(st)
	}

	return nil
}

func (sc *ServerConn) dispatchStream(st *serverStream) {
	defer sc.streamsWg.Done() // Escalation 2 fix: Decrement wait group on exit
	defer st.cancel()

	req := &ServerRequest{
		StreamID:   st.id,
		Method:     st.method,
		Path:       st.path,
		Scheme:     st.scheme,
		Authority:  st.authority,
		Protocol:   st.protocol,
		Headers:    st.headers,
		Body:       st.body.Bytes(),
		RemoteAddr: sc.conn.RemoteAddr().String(),
		Ctx:        st.ctx,
	}

	res := &ServerResponse{
		StatusCode: status.OK,
		Headers:    make(http.Header),
	}

	if sc.handler != nil {
		_ = sc.handler(req, res)
	}

	_ = sc.writeResponse(st.id, res)

	st.state = streamClosed

	sc.streamsMu.Lock()
	delete(sc.streams, st.id)
	sc.streamsMu.Unlock()
}
