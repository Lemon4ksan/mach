// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	coreh2 "github.com/lemon4ksan/mach/core/h2"
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/lemon4ksan/foundation/silicon/pool"
)

// ServerHandlerFunc is the callback signature for dispatching an incoming H2 stream request.
type ServerHandlerFunc func(req *ServerRequest, res *ServerResponse) error

// ServerRequest represents a parsed incoming HTTP/2 stream request.
type ServerRequest struct {
	StreamID   uint32
	Method     string
	Path       string
	Scheme     string
	Authority  string
	Protocol   string
	Headers    http.Header
	Body       []byte
	RemoteAddr string
	Ctx        context.Context
}

// ServerResponse represents an outgoing HTTP/2 stream response.
type ServerResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

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
}

// ServerConn manages a single server-side HTTP/2 connection.
type ServerConn struct {
	conn      net.Conn
	br        *bufio.Reader
	bw        *bufio.Writer
	handler   ServerHandlerFunc
	hpackDec  *coreh2.HPACK
	hpackEnc  *coreh2.HPACK
	encMu     sync.Mutex
	writeMu   sync.Mutex
	streamsMu sync.RWMutex
	streams   map[uint32]*serverStream
	isClosed  atomic.Bool
	closeErr  error

	peerMaxFrameSize uint32
	peerInitialWin   int32
}

var serverConnStorage = pool.NewPerPStorage(func() *ServerConn {
	return &ServerConn{
		hpackDec:         coreh2.AcquireHPACK(),
		hpackEnc:         coreh2.AcquireHPACK(),
		streams:          make(map[uint32]*serverStream, 64),
		peerMaxFrameSize: coreh2.DefaultMaxLen,
		peerInitialWin:   65535,
	}
})

// NewServerConn creates a new HTTP/2 server connection handler wrapping netConn.
func NewServerConn(netConn net.Conn, handler ServerHandlerFunc) *ServerConn {
	sc := serverConnStorage.Get()
	sc.conn = netConn
	sc.br = bufio.NewReaderSize(netConn, 4096)
	sc.bw = bufio.NewWriterSize(netConn, 4096)
	sc.handler = handler
	sc.isClosed.Store(false)
	sc.closeErr = nil
	sc.peerMaxFrameSize = coreh2.DefaultMaxLen
	sc.peerInitialWin = 65535
	sc.hpackDec.Reset()
	sc.hpackEnc.Reset()
	sc.hpackEnc.DisableDynamicTable = true
	clear(sc.streams)

	return sc
}

// Release returns the ServerConn to the core pool.
func (sc *ServerConn) Release() {
	sc.isClosed.Store(true)
	clear(sc.streams)
	serverConnStorage.Put(sc)
}

// Serve runs the main HTTP/2 server connection loop.
func (sc *ServerConn) Serve() error {
	defer func() {
		_ = sc.conn.Close()
	}()

	// 1. Read and verify 24-byte client connection preface (RFC 9113 §3.4)
	if !coreh2.ReadPreface(sc.br) {
		return errors.New("h2: invalid connection preface")
	}

	// 2. Send initial server SETTINGS frame
	st := &coreh2.Settings{}
	st.SetMaxConcurrentStreams(1000)
	st.SetMaxFrameSize(coreh2.DefaultMaxLen)
	st.SetMaxWindowSize(65535)

	if err := sc.sendSettings(st, false); err != nil {
		return err
	}

	// 3. Main frame reading loop
	for {
		if sc.isClosed.Load() {
			return sc.closeErr
		}

		fr, err := coreh2.ReadFrameFrom(sc.br)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil
			}

			return err
		}

		switch fr.Type() {
		case coreh2.FrameSettings:
			if err := sc.handleSettings(fr); err != nil {
				coreh2.ReleaseFrameHeader(fr)
				return err
			}

		case coreh2.FramePing:
			if err := sc.handlePing(fr); err != nil {
				coreh2.ReleaseFrameHeader(fr)
				return err
			}

		case coreh2.FrameHeaders:
			if err := sc.handleHeaders(fr); err != nil {
				coreh2.ReleaseFrameHeader(fr)
				return err
			}

		case coreh2.FrameContinuation:
			if err := sc.handleContinuation(fr); err != nil {
				coreh2.ReleaseFrameHeader(fr)
				return err
			}

		case coreh2.FrameData:
			if err := sc.handleData(fr); err != nil {
				coreh2.ReleaseFrameHeader(fr)
				return err
			}

		case coreh2.FrameWindowUpdate:
			// Flow control window updates
			coreh2.ReleaseFrameHeader(fr)

		case coreh2.FrameResetStream:
			sc.streamsMu.Lock()
			delete(sc.streams, fr.Stream())
			sc.streamsMu.Unlock()
			coreh2.ReleaseFrameHeader(fr)

		case coreh2.FrameGoAway:
			coreh2.ReleaseFrameHeader(fr)
			return nil

		default:
			coreh2.ReleaseFrameHeader(fr)
		}
	}
}

func (sc *ServerConn) sendSettings(st *coreh2.Settings, ack bool) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	fr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(fr)

	stFrame := coreh2.AcquireFrame(coreh2.FrameSettings).(*coreh2.Settings)
	if ack {
		stFrame.SetAck(true)
		fr.SetFlags(coreh2.FlagAck)
	} else {
		st.CopyTo(stFrame)
	}

	fr.SetBody(stFrame)

	if _, err := fr.WriteTo(sc.bw); err != nil {
		return err
	}

	return sc.bw.Flush()
}

func (sc *ServerConn) handleSettings(fr *coreh2.FrameHeader) error {
	defer coreh2.ReleaseFrameHeader(fr)

	if fr.Flags().Has(coreh2.FlagAck) {
		// Client ACKed our settings
		return nil
	}

	// Apply peer settings
	if body := fr.Body(); body != nil {
		if st, ok := body.(*coreh2.Settings); ok {
			if mfs := st.MaxFrameSize(); mfs >= 16384 && mfs <= 16777215 {
				sc.peerMaxFrameSize = mfs
			}

			if iws := st.MaxWindowSize(); iws > 0 {
				sc.peerInitialWin = int32(iws)
			}
		}
	}

	// Send Settings ACK
	return sc.sendSettings(nil, true)
}

func (sc *ServerConn) handlePing(fr *coreh2.FrameHeader) error {
	defer coreh2.ReleaseFrameHeader(fr)

	if fr.Flags().Has(coreh2.FlagAck) {
		return nil
	}

	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	ackFr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(ackFr)

	ping := fr.Body().(*coreh2.Ping)
	ackPing := coreh2.AcquireFrame(coreh2.FramePing).(*coreh2.Ping)
	ackPing.SetData(ping.Data())

	ackFr.SetFlags(coreh2.FlagAck)
	ackFr.SetBody(ackPing)

	if _, err := ackFr.WriteTo(sc.bw); err != nil {
		return err
	}

	return sc.bw.Flush()
}

func (sc *ServerConn) handleHeaders(fr *coreh2.FrameHeader) error {
	defer coreh2.ReleaseFrameHeader(fr)

	streamID := fr.Stream()
	// RFC 9113 §5.1.1: Client-initiated streams MUST use non-zero, odd-numbered stream identifiers
	if streamID == 0 || (streamID%2) == 0 {
		return coreh2.ProtocolError
	}

	hFrame := fr.Body().(*coreh2.Headers)
	endHeaders := fr.Flags().Has(coreh2.FlagEndHeaders)
	endStream := fr.Flags().Has(coreh2.FlagEndStream)

	st := &serverStream{
		id:         streamID,
		headers:    make(http.Header),
		endHeaders: endHeaders,
		endStream:  endStream,
	}

	// Write raw header fragment
	st.headerBlock.Write(hFrame.Headers())

	sc.streamsMu.Lock()
	sc.streams[streamID] = st
	sc.streamsMu.Unlock()

	if endHeaders {
		return sc.finishHeaderBlock(st)
	}

	return nil
}

func (sc *ServerConn) handleContinuation(fr *coreh2.FrameHeader) error {
	defer coreh2.ReleaseFrameHeader(fr)

	streamID := fr.Stream()

	sc.streamsMu.RLock()
	st, ok := sc.streams[streamID]
	sc.streamsMu.RUnlock()

	if !ok {
		return errors.New("h2: CONTINUATION on unknown stream (RFC 9113 §6.10)")
	}

	cFrame := fr.Body().(*coreh2.Continuation)
	st.headerBlock.Write(cFrame.Headers())

	if fr.Flags().Has(coreh2.FlagEndHeaders) {
		st.endHeaders = true
		return sc.finishHeaderBlock(st)
	}

	return nil
}

func (sc *ServerConn) finishHeaderBlock(st *serverStream) error {
	rawBlock := st.headerBlock.Bytes()

	hf := coreh2.AcquireHeaderField()
	defer coreh2.ReleaseHeaderField(hf)

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
		go sc.dispatchStream(st)
	}

	return nil
}

func (sc *ServerConn) handleData(fr *coreh2.FrameHeader) error {
	defer coreh2.ReleaseFrameHeader(fr)

	streamID := fr.Stream()

	sc.streamsMu.RLock()
	st, ok := sc.streams[streamID]
	sc.streamsMu.RUnlock()

	if !ok {
		return nil
	}

	dFrame := fr.Body().(*coreh2.Data)
	st.body.Write(dFrame.Data())

	if fr.Flags().Has(coreh2.FlagEndStream) {
		st.endStream = true
		go sc.dispatchStream(st)
	}

	return nil
}

func (sc *ServerConn) dispatchStream(st *serverStream) {
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
		Ctx:        context.Background(),
	}

	res := &ServerResponse{
		StatusCode: http.StatusOK,
		Headers:    make(http.Header),
	}

	if sc.handler != nil {
		_ = sc.handler(req, res)
	}

	_ = sc.writeResponse(st.id, res)

	sc.streamsMu.Lock()
	delete(sc.streams, st.id)
	sc.streamsMu.Unlock()
}

func (sc *ServerConn) writeResponse(streamID uint32, res *ServerResponse) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	hdrFr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(hdrFr)

	sc.encMu.Lock()
	hFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	coreh2.SerializeResponseHeaders(hFrame, sc.hpackEnc, res.StatusCode, res.Headers, len(res.Body))
	sc.encMu.Unlock()

	hdrFr.SetStream(streamID)
	hdrFr.SetFlags(coreh2.FlagEndHeaders)

	if len(res.Body) == 0 {
		hdrFr.SetFlags(coreh2.FlagEndHeaders | coreh2.FlagEndStream)
	}

	hdrFr.SetBody(hFrame)

	if _, err := hdrFr.WriteTo(sc.bw); err != nil {
		return err
	}

	// 2. Serialize DATA Frames
	body := res.Body

	maxChunk := int(sc.peerMaxFrameSize)
	if maxChunk <= 0 {
		maxChunk = coreh2.DefaultMaxLen
	}

	for len(body) > 0 {
		chunkSize := min(len(body), maxChunk)
		chunk := body[:chunkSize]
		body = body[chunkSize:]

		dataFr := coreh2.AcquireFrameHeader()
		dFrame := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
		dFrame.SetData(chunk)

		dataFr.SetStream(streamID)

		if len(body) == 0 {
			dataFr.SetFlags(coreh2.FlagEndStream)
		}

		dataFr.SetBody(dFrame)

		_, err := dataFr.WriteTo(sc.bw)
		coreh2.ReleaseFrameHeader(dataFr)

		if err != nil {
			return err
		}
	}

	return sc.bw.Flush()
}

