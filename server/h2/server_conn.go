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

	"github.com/lemon4ksan/foundation/net/hpack"
	"github.com/lemon4ksan/foundation/silicon/pool"
	"golang.org/x/sys/cpu"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

// ServerHandlerFunc is the callback signature for dispatching an incoming HTTP/2 stream request (RFC 9113 §8.1).
//
// In compliance with RFC 9113 §5 (Streams and Multiplexing), handlers are executed concurrently
// in dedicated per-stream goroutines. The handler must not retain references to req or res after
// returning.
type ServerHandlerFunc func(req *ServerRequest, res *ServerResponse) error

// ServerRequest represents a parsed incoming HTTP/2 stream request in compliance with
// RFC 9113 §8.3 (Request Pseudo-Header Fields) and RFC 8441 §4 (Extended CONNECT).
//
// Concurrency:
// ServerRequest is allocated per stream and passed to a single ServerHandlerFunc goroutine.
// It is not safe for concurrent use across multiple goroutines without external synchronization.
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

// ServerResponse represents an outgoing HTTP/2 stream response in compliance with
// RFC 9113 §8.4 (Response Pseudo-Header Fields) and RFC 9110 §15 (Status Codes).
//
// Concurrency:
// ServerResponse is populated by the stream handler and serialized by the connection write loop.
// It is not safe for concurrent modification.
type ServerResponse struct {
	StatusCode int         // StatusCode is the HTTP response status code (RFC 9113 §8.4 :status).
	Headers    http.Header // Headers contains outgoing response headers.
	Body       []byte      // Body contains response payload framed into DATA frames (RFC 9113 §6.1).
}

// ServerConn manages a single server-side HTTP/2 connection session in compliance with
// RFC 9113 §3 (Starting HTTP/2), §4 (HTTP Frames), §5 (Streams and Multiplexing),
// and §6 (Frame Definitions).
//
// Concurrency:
// ServerConn is safe for concurrent use. Inbound frame demuxing is handled by Serve(),
// while outbound frames and responses are serialized via writeMu. In-flight stream
// goroutines are tracked by streamsWg to ensure race-free teardown and pooling.
//
// Silicon Invariants:
// Hot atomic state counters are isolated on a dedicated cache line (_ cpu.CacheLinePad)
// to prevent false sharing during high-concurrency stream multiplexing. Connection instances
// are pooled via Per-P storage (foundation/silicon/pool) with zero allocations on hot paths.
type ServerConn struct {
	conn      net.Conn
	br        *bufio.Reader
	bw        *bufio.Writer
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

var serverConnStorage = pool.NewPerPStorage(func() *ServerConn {
	return &ServerConn{
		br:               bufio.NewReaderSize(nil, 4096),
		bw:               bufio.NewWriterSize(nil, 4096),
		hpackDec:         hpack.AcquireHPACK(),
		hpackEnc:         hpack.AcquireHPACK(),
		streams:          make(map[uint32]*serverStream, 64),
		peerMaxFrameSize: coreh2.DefaultMaxLen,
		peerInitialWin:   65535,
	}
})

// NewServerConn creates a new HTTP/2 server connection handler wrapping netConn (RFC 9113 §3).
//
// Lifecycle:
// Connection instances are acquired from an internal Per-P storage pool. When Serve()
// completes, Release() MUST be called to return the ServerConn to the pool.
func NewServerConn(netConn net.Conn, handler ServerHandlerFunc) *ServerConn {
	sc := serverConnStorage.Get()

	sc.conn = netConn
	if sc.br == nil {
		sc.br = bufio.NewReaderSize(netConn, 4096)
	} else {
		sc.br.Reset(netConn)
	}

	if sc.bw == nil {
		sc.bw = bufio.NewWriterSize(netConn, 4096)
	} else {
		sc.bw.Reset(netConn)
	}

	sc.handler = handler
	sc.isClosed.Store(false)
	sc.isReleased.Store(false)
	sc.closeErr = nil
	sc.peerMaxFrameSize = coreh2.DefaultMaxLen
	sc.peerInitialWin = 65535
	sc.connSendWindow.Store(65535)

	sc.ctx, sc.cancelFn = context.WithCancel(context.Background())

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
	if !coreh2.ReadPreface(sc.br) {
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

// Release waits for all active stream dispatch goroutines to complete, tears down
// connection resources, and returns the ServerConn to the Per-P storage pool.
//
// Concurrency:
// Release is race-safe against asynchronous stream handlers and socket disconnects.
// It guarantees zero pool contamination and eliminates use-after-free conditions.
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

	if sc.br != nil {
		sc.br.Reset(nil)
	}

	if sc.bw != nil {
		sc.bw.Reset(nil)
	}

	serverConnStorage.Put(sc)
}
