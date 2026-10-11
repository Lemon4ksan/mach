// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/lemon4ksan/foundation/silicon/pool"
	"golang.org/x/sys/cpu"

	"github.com/lemon4ksan/mach/proto/codec/hpack"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	"github.com/lemon4ksan/mach/proto/h2/fsm"
)

// ServerHandlerFunc is the callback signature for dispatching an incoming HTTP/2 stream request (RFC 9113 §8.1).
//
// In compliance with RFC 9113 §5 (Streams and Multiplexing), handlers are executed concurrently
// in dedicated per-stream goroutines. The handler must not retain references to req or res after
// returning.
type ServerHandlerFunc func(req *ServerRequest, res *ServerResponse) error

// ServerRequest represents a parsed incoming HTTP/2 stream request (RFC 9113 §8.3, RFC 8441 §4).
type ServerRequest struct {
	StreamID   uint32          // StreamID is the odd-numbered stream identifier (RFC 9113 §5.1.1).
	Method     string          // Method is the :method pseudo-header value (RFC 9113 §8.3.1).
	Path       string          // Path is the :path pseudo-header value (RFC 9113 §8.3.1).
	Scheme     string          // Scheme is the :scheme pseudo-header value (RFC 9113 §8.3.1).
	Authority  string          // Authority is the :authority pseudo-header value (RFC 9113 §8.3.1).
	Protocol   string          // Protocol is the :protocol pseudo-header for extended CONNECT (RFC 8441 §4).
	Headers    http.Header     // Headers contains regular, lowercase header fields (RFC 9113 §8.2).
	Body       []byte          // Body contains the reassembled payload from DATA frames (RFC 9113 §6.1).
	RemoteAddr string          // RemoteAddr is the peer network address.
	Ctx        context.Context // Ctx is the stream context cancelled upon stream termination or reset.
}

// ServerResponse represents an outgoing HTTP/2 stream response (RFC 9113 §8.4, RFC 9110 §15).
type ServerResponse struct {
	StatusCode int         // StatusCode is the HTTP response status code (RFC 9113 §8.4 :status).
	Headers    http.Header // Headers contains outgoing response headers.
	Body       []byte      // Body contains response payload framed into DATA frames (RFC 9113 §6.1).
}

// ServerConn manages a single server-side HTTP/2 connection session (RFC 9113 §3).
type ServerConn struct {
	conn      net.Conn
	bw        *bufio.Writer
	sm        fsm.StateMachine
	readBuf   []byte
	muBuf     []byte
	handler   ServerHandlerFunc
	hpackDec  *hpack.HPACK
	hpackEnc  *hpack.HPACK
	encMu     sync.Mutex
	writeMu   sync.Mutex
	streamsMu sync.RWMutex
	streams   map[uint32]*serverStream
	streamsWg sync.WaitGroup

	peerMaxFrameSize uint32
	peerInitialWin   int32

	_ cpu.CacheLinePad

	isClosed       atomic.Bool
	isReleased     atomic.Bool
	connSendWindow atomic.Int32
	closeErr       error

	ctx      context.Context
	cancelFn context.CancelFunc
}

var serverConnStorage = pool.NewPerCorePool(func() *ServerConn {
	return &ServerConn{
		bw:               bufio.NewWriterSize(nil, 4096),
		readBuf:          make([]byte, 32768),
		muBuf:            make([]byte, 0, 32768),
		hpackDec:         hpack.AcquireHPACK(),
		hpackEnc:         hpack.AcquireHPACK(),
		streams:          make(map[uint32]*serverStream, 64),
		peerMaxFrameSize: coreh2.DefaultMaxLen,
		peerInitialWin:   65535,
	}
})

// NewServerConn creates a new HTTP/2 server connection handler wrapping netConn (RFC 9113 §3).
// When Serve completes, Release must be called to return the ServerConn to the pool.
func NewServerConn(netConn net.Conn, handler ServerHandlerFunc) *ServerConn {
	sc := serverConnStorage.Get()

	sc.conn = netConn

	if sc.bw == nil {
		sc.bw = bufio.NewWriterSize(netConn, 4096)
	}
	sc.bw.Reset(netConn)

	sc.muBuf = sc.muBuf[:0]

	sc.handler = handler
	sc.isClosed.Store(false)
	sc.isReleased.Store(false)
	sc.closeErr = nil
	sc.peerMaxFrameSize = coreh2.DefaultMaxLen
	sc.peerInitialWin = 65535
	sc.connSendWindow.Store(65535)

	sc.ctx, sc.cancelFn = context.WithCancel(context.Background())
	sc.sm.Handler = sc

	sc.hpackDec.Reset()
	sc.hpackEnc.Reset()
	sc.hpackEnc.DisableDynamicTable = true

	sc.streamsMu.Lock()
	clear(sc.streams)
	sc.streamsMu.Unlock()

	return sc
}

// Serve runs the main HTTP/2 server connection loop (RFC 9113 §3.4, §3.5).
//
// Serve reads and validates the 24-byte client connection preface, exchanges initial
// SETTINGS frames, and demuxes incoming frames until the client terminates or an error occurs.
// Serve closes the underlying network connection upon return.
func (sc *ServerConn) Serve() error {
	defer func() {
		sc.isClosed.Store(true)

		if sc.cancelFn != nil {
			sc.cancelFn()
		}

		if sc.conn != nil {
			_ = sc.conn.Close()
		}
	}()

	// 1. Read and verify 24-byte client connection preface (RFC 9113 §3.4)
	if !coreh2.ReadPreface(sc.conn) {
		return errors.New("h2: invalid connection preface")
	}

	// 2. Send initial server SETTINGS frame (RFC 9113 §6.5)
	st := &coreh2.Settings{}
	st.SetMaxConcurrentStreams(1000)
	st.SetMaxFrameSize(coreh2.DefaultMaxLen)
	st.SetMaxWindowSize(65535)

	if err := sc.sendSettings(st, false); err != nil {
		return err
	}

	// 3. Main frame reading and demuxing loop
	return sc.readLoop()
}

// Close terminates the HTTP/2 server connection and closes the underlying socket (RFC 9113 §6.8).
func (sc *ServerConn) Close() error {
	if sc.isClosed.Swap(true) {
		return nil
	}

	if sc.cancelFn != nil {
		sc.cancelFn()
	}

	if sc.conn != nil {
		return sc.conn.Close()
	}

	return nil
}

// Closed reports whether the server connection has been marked as closed.
func (sc *ServerConn) Closed() bool {
	return sc.isClosed.Load()
}

// Release waits for active stream dispatch goroutines to complete, tears down
// connection resources, and returns sc to the pool.
func (sc *ServerConn) Release() {
	if sc.isReleased.Swap(true) {
		return
	}

	sc.isClosed.Store(true)

	if sc.cancelFn != nil {
		sc.cancelFn()
	}

	if sc.conn != nil {
		_ = sc.conn.Close()
	}

	// Wait for all in-flight stream dispatch goroutines to exit (Resolves Escalation 2)
	sc.streamsWg.Wait()

	// Mutex-protected map cleanup
	sc.streamsMu.Lock()
	clear(sc.streams)
	sc.streamsMu.Unlock()

	if sc.bw != nil {
		sc.bw.Reset(nil)
	}

	serverConnStorage.Put(sc)
}
