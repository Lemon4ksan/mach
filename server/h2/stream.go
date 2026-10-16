// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/lemon4ksan/foundation/generic"

	"github.com/lemon4ksan/mach/proto/codec/hpack"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	"github.com/lemon4ksan/mach/proto/semantic/status"
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
	sendWindow  atomic.Int32
	recvUnacked int32
	ctx         context.Context
	cancel      context.CancelFunc
	isTunnel    bool
	pipeReader  *io.PipeReader
	pipeWriter  *io.PipeWriter
}

var (
	serverStreamPool = generic.NewPool(func() *serverStream {
		return &serverStream{
			headers: make(http.Header, 8),
		}
	})
	serverRequestPool = generic.NewPool(func() *ServerRequest {
		return &ServerRequest{}
	})
	serverResponsePool = generic.NewPool(func() *ServerResponse {
		return &ServerResponse{
			Headers: make(http.Header, 8),
		}
	})
)

func acquireServerStream(
	streamID uint32,
	endHeaders, endStream bool,
	initWin int32,
	ctx context.Context,
	cancel context.CancelFunc,
) *serverStream {
	st := serverStreamPool.Get()
	st.id = streamID
	st.endHeaders = endHeaders
	st.endStream = endStream
	st.state = streamOpen
	st.sendWindow.Store(initWin)
	st.recvUnacked = 0
	st.ctx = ctx
	st.cancel = cancel
	st.method = ""
	st.path = ""
	st.scheme = ""
	st.authority = ""
	st.protocol = ""
	st.isTunnel = false
	st.pipeReader = nil
	st.pipeWriter = nil
	st.headerBlock.Reset()
	st.body.Reset()
	clear(st.headers)

	return st
}

func releaseServerStream(st *serverStream) {
	if st == nil {
		return
	}

	if st.cancel != nil {
		st.cancel()
	}

	st.pipeReader = nil
	st.pipeWriter = nil
	st.headerBlock.Reset()
	st.body.Reset()
	clear(st.headers)
	serverStreamPool.Put(st)
}

func (sc *ServerConn) newServerStream(streamID uint32, endHeaders, endStream bool) *serverStream {
	ctx, cancel := context.WithCancel(sc.ctx)
	return acquireServerStream(streamID, endHeaders, endStream, sc.peerInitialWin, ctx, cancel)
}

func (sc *ServerConn) startStream(st *serverStream) {
	if st.method == "CONNECT" && st.protocol != "" {
		st.state = streamOpen
	} else {
		st.state = streamHalfClosedRemote
	}

	sc.streamsWg.Add(1) // Escalation 2 fix: Track active stream goroutine
	go sc.dispatchStream(st)
}

func (sc *ServerConn) finishHeaderBlock(st *serverStream) error {
	rawBlock := st.headerBlock.Bytes()

	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	var hasSeenRegularHeader bool

	sc.hpackDec.BeginHeaderBlock()

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

		kBytes := hf.KeyBytes()
		vBytes := hf.ValueBytes()

		// RFC 9113 §8.2.1: Field names MUST be lowercase ASCII and conform to field validity requirements.
		if err := coreh2.ValidateHeaderName(kBytes, hf.IsPseudo()); err != nil {
			return err
		}

		// RFC 9113 §8.2.1: Field values MUST NOT contain NUL, CR, or LF at any position.
		if err := coreh2.ValidateHeaderValue(vBytes); err != nil {
			return err
		}

		k := string(kBytes)
		v := string(vBytes)

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

			continue
		}

		hasSeenRegularHeader = true

		// RFC 9113 §8.2.2: Connection-specific headers are prohibited in HTTP/2
		if coreh2.IsConnectionSpecificHeader(kBytes, vBytes) {
			return coreh2.ProtocolError
		}

		st.headers.Add(k, v)
	}

	// RFC 9113 §8.3.1, §8.5 & RFC 8441 §4: Request pseudo-header validation
	if st.method == "" {
		return coreh2.ProtocolError
	}

	if st.method == "CONNECT" {
		if st.protocol != "" {
			if !sc.EnableConnectProtocol {
				return coreh2.ProtocolError
			}

			// RFC 8441 §4: Extended CONNECT requires :scheme, :path, and :authority
			if st.scheme == "" || st.path == "" || st.authority == "" {
				return coreh2.ProtocolError
			}
		} else if st.scheme != "" || st.path != "" || st.authority == "" {
			// RFC 9113 §8.5: Standard CONNECT requires :scheme and :path to be omitted, and :authority present
			return coreh2.ProtocolError
		}
	} else {
		// RFC 8441 §4: :protocol pseudo-header is only valid on CONNECT requests
		if st.protocol != "" {
			return coreh2.ProtocolError
		}

		// RFC 9113 §8.3.1: All other requests MUST include exactly one :method, :scheme, and :path
		if st.scheme == "" || st.path == "" {
			return coreh2.ProtocolError
		}
	}

	if st.endStream || (st.method == "CONNECT" && st.protocol != "") {
		sc.startStream(st)
	}

	return nil
}

type h2ServerStreamAdapter struct {
	sc       *ServerConn
	streamID uint32
	pr       *io.PipeReader
	pw       *io.PipeWriter
	closed   atomic.Bool
}

func (a *h2ServerStreamAdapter) Read(p []byte) (int, error) {
	return a.pr.Read(p)
}

func (a *h2ServerStreamAdapter) Write(p []byte) (int, error) {
	if a.closed.Load() {
		return 0, io.ErrClosedPipe
	}

	return a.sc.writeStreamData(a.streamID, p, false)
}

func (a *h2ServerStreamAdapter) Close() error {
	if a.closed.CompareAndSwap(false, true) {
		_ = a.pr.Close()
		_ = a.pw.Close()
		_, _ = a.sc.writeStreamData(a.streamID, nil, true)
	}

	return nil
}

func (sc *ServerConn) dispatchStream(st *serverStream) {
	defer sc.streamsWg.Done() // Escalation 2 fix: Decrement wait group on exit
	defer func() {
		sc.deleteStream(st.id)
		releaseServerStream(st)
	}()

	isExtConnect := (st.method == "CONNECT" && st.protocol != "")

	var adapter *h2ServerStreamAdapter
	if isExtConnect {
		pr, pw := io.Pipe()
		st.pipeReader = pr
		st.pipeWriter = pw

		adapter = &h2ServerStreamAdapter{
			sc:       sc,
			streamID: st.id,
			pr:       pr,
			pw:       pw,
		}
		defer func() {
			_ = adapter.Close()
		}()
	}

	req := serverRequestPool.Get()
	defer func() {
		*req = ServerRequest{}
		serverRequestPool.Put(req)
	}()

	req.StreamID = st.id
	req.Method = st.method
	req.Path = st.path
	req.Scheme = st.scheme
	req.Authority = st.authority
	req.Protocol = st.protocol
	req.Headers = st.headers
	req.Body = st.body.Bytes()
	req.RemoteAddr = sc.conn.RemoteAddr().String()
	req.Ctx = st.ctx
	req.Stream = adapter
	req.IsExtendedConnect = isExtConnect
	req.UpgradeFn = func(statusCode int, header http.Header) error {
		st.isTunnel = true
		return sc.writeHeaders(st.id, statusCode, header, false)
	}

	res := serverResponsePool.Get()
	defer func() {
		clear(res.Headers)
		res.Body = nil
		serverResponsePool.Put(res)
	}()

	res.StatusCode = status.OK
	clear(res.Headers)
	res.Body = nil

	if sc.handler != nil {
		_ = sc.handler(req, res)
	}

	if st.isTunnel {
		if adapter != nil {
			_ = adapter.Close()
		}
	} else {
		_ = sc.writeResponse(st.id, res)
	}

	st.state = streamClosed
}
