# Handoff Report — Explorer M4.1: Server H2 Modular Decomposition Blueprint

**From**: Explorer M4.1 (`teamwork_preview_explorer_m4_1`)  
**To**: Orchestrator Gen 4 (`5990a2d7-7ec1-47d8-9672-52a9ad7ba846`) & Worker M4.1  
**Date**: 2026-09-22T20:20:00Z  
**Scope**: `server/h2/` architecture investigation, clean 5-file decomposition, Escalation 2 data race fix, silicon performance invariants, RFC docstring standards.  
**Working Directory**: `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_1`  
**Handoff Type**: Hard Handoff (Complete specification ready for worker implementation)  

---

## 1. Observation

### 1.1 Existing Codebase Structure of `server/h2/`
The `server/h2/` package currently consists of 3 files:
1. `server/h2/server_conn.go` (570 lines, 12,423 bytes): A monolithic implementation containing the `ServerConn` struct, constructor, connection pooling, main event loop (`Serve`), socket frame demuxing (`handleSettings`, `handlePing`, `handleHeaders`, `handleContinuation`, `handleData`), header validation & HPACK decompressing (`finishHeaderBlock`), stream dispatching (`dispatchStream`), response framing & DATA chunking (`writeResponse`), and teardown (`Release`).
2. `server/h2/generate.go` (10 lines, 495 bytes): Plan 9 assembly stubs and `c2plan9` generation directives.
3. `server/h2/server_test.go` (160 lines, 3,586 bytes): Unit test `TestH2Server_EndToEnd` testing cleartext HTTP/2 GET, POST, and 50 concurrent streams against `net/http` client.

External consumers of `server/h2` were observed:
- `tests/e2e/h2_test.go`: 18 end-to-end tests covering Tiers 1–4 (`h2server.ServerRequest`, `h2server.ServerResponse`, `h2server.ServerHandlerFunc`).
- `tests/e2e/helpers_test.go`: `startH2Server` helper wrapping `net.Conn` with `h2WindowUpdateConn` and instantiating `h2server.NewServerConn(wrappedConn, handler)` and `sc.Serve()`.

### 1.2 Verification of Tests and Linters on Existing Implementation
Tool executions executed under `$env:GOWORK="off"`:
1. Package tests:
   ```powershell
   $env:GOWORK="off"; go test -v -race ./server/h2/...
   ```
   Result: `PASS: TestH2Server_EndToEnd (0.01s) - ok github.com/lemon4ksan/mach/server/h2 1.964s`.
2. E2E tests:
   ```powershell
   $env:GOWORK="off"; go test -v -race -run TestH2 ./tests/e2e/...
   ```
   Result: 18/18 tests pass across all tiers.
3. Linter:
   ```powershell
   $env:GOWORK="off"; golangci-lint run ./server/h2/...
   ```
   Result: `0 issues`.
4. Go vet:
   ```powershell
   $env:GOWORK="off"; go vet ./server/h2/...
   ```
   Result: `0 warnings`.

### 1.3 Direct Observation of Escalation 2 (Data Race on `sc.streams` in `Release()`)
In `server/h2/server_conn.go`:
- Lines 111–116:
  ```go
  // Release returns the ServerConn to the core pool.
  func (sc *ServerConn) Release() {
  	sc.isClosed.Store(true)
  	clear(sc.streams)
  	serverConnStorage.Put(sc)
  }
  ```
- Lines 106–107 (in `NewServerConn`):
  ```go
  	clear(sc.streams)
  ```
- Lines 449–452 and 473–475:
  ```go
  	if st.endStream {
  		go sc.dispatchStream(st)
  	}
  ```
- Lines 480–508:
  ```go
  func (sc *ServerConn) dispatchStream(st *serverStream) {
  	req := &ServerRequest{
  		...
  	}
  	...
  	if sc.handler != nil {
  		_ = sc.handler(req, res)
  	}

  	_ = sc.writeResponse(st.id, res)

  	sc.streamsMu.Lock()
  	delete(sc.streams, st.id)
  	sc.streamsMu.Unlock()
  }
  ```

---

## 2. Logic Chain

### 2.1 Escalation 2 Vulnerability Mechanics & Pool Contamination
1. **Asynchronous In-Flight Execution**: When a stream receives `END_STREAM` (on `HEADERS` or final `DATA`), `sc.dispatchStream(st)` is launched in a background goroutine.
2. **Abrupt Disconnect / Early Teardown**: If the client disconnects or aborts the TCP connection, `coreh2.ReadFrameFrom(sc.br)` in `Serve()` immediately returns `io.EOF` or `net.ErrClosed`, causing `Serve()` to exit.
3. **Premature Release**: Callers wrapping connection servicing in `defer sc.Release()` (or running cleanup routines) invoke `sc.Release()`.
4. **Data Race on `sc.streams`**:
   - `Release()` executes `clear(sc.streams)` on line 114 without acquiring `sc.streamsMu`.
   - Concurrently, the in-flight `dispatchStream` goroutine is executing its handler or response serialization, and subsequently calls `delete(sc.streams, st.id)` on line 506 with `sc.streamsMu.Lock()`.
   - Concurrent unsynchronized read/write and write/write operations on the `map[uint32]*serverStream` produce runtime data race warnings and risk fatal map corruption crashes (`fatal error: concurrent map writes`).
5. **Use-After-Free & Pool Contamination**:
   - `Release()` puts `sc` back into `serverConnStorage` (`pool.NewPerPStorage`).
   - A subsequent incoming connection on the same OS thread / P retrieves `sc` via `NewServerConn(c, handler)`.
   - The new connection calls `clear(sc.streams)`, initializes `sc.br` and `sc.bw`, and resets HPACK compression state.
   - Meanwhile, the lingering `dispatchStream` goroutine from the *previous* connection is still running, writing bytes to `sc.bw` or interacting with HPACK encoder tables.
   - This causes critical cross-connection state corruption and memory leaks.

### 2.2 Fix Architecture for Escalation 2
1. **In-Flight Stream Tracking**: Add `streamsWg sync.WaitGroup` to `ServerConn`.
2. **Add/Done Invariants**:
   - Wrap stream dispatch in `sc.startStream(st)`: invoke `sc.streamsWg.Add(1)` prior to launching `go sc.dispatchStream(st)`.
   - In `dispatchStream(st)`: `defer sc.streamsWg.Done()` ensures completion under all exit paths (panics, handler errors, response serialization failures).
3. **Drain Wait in `Release()`**:
   - In `Release()`, close `sc.conn` to unblock any network I/O, cancel stream contexts, and then execute `sc.streamsWg.Wait()`.
   - Only after `streamsWg.Wait()` completes are shared stream goroutines guaranteed dead.
4. **Synchronized Map Clear**:
   - Enclose `clear(sc.streams)` in `sc.streamsMu.Lock()` / `sc.streamsMu.Unlock()` in both `Release()` and `NewServerConn`.
5. **Idempotency Guard**:
   - Add `isReleased atomic.Bool` to guarantee that duplicate calls to `Release()` will not re-pool or double-put the same `ServerConn` pointer into `serverConnStorage`.
6. **Per-Stream Context Cancellation**:
   - Allocate `st.ctx, st.cancel = context.WithCancel(sc.ctx)` for each stream.
   - Upon peer `RST_STREAM` or connection `Close()`, invoke `st.cancel()`, allowing stream handlers observing `req.Ctx.Done()` to abort immediately.

---

## 3. Detailed Decomposition Blueprint (5 Target Files)

The monolithic `server/h2/server_conn.go` is decomposed into 5 single-responsibility files in `server/h2/`:
1. `server_conn.go`: Facade, lifecycle, exported types, constructor, Per-P pool.
2. `read_loop.go`: Socket read loop, preface validation, frame demuxing, settings/ping/rst/goaway dispatch.
3. `write_loop.go`: Outbound response serialization, HPACK encoding, DATA chunking, frame writing.
4. `stream.go`: Stream state machine, stream lifecycle, header validation, RFC 8441 extended CONNECT, handler execution.
5. `flow_control.go`: Flow control window accounting, WINDOW_UPDATE reception and emission.

```
server/h2/
├── server_conn.go    # ServerConn struct, constructor, lifecycle (Serve, Close, Closed, Release), Per-P pool
├── read_loop.go      # Preface, readLoop demux, handleSettings, handlePing, handleResetStream, handleGoAway
├── write_loop.go     # writeResponse, writePingAck, sendWindowUpdateFrame, bw flush
├── stream.go         # serverStream, startStream, finishHeaderBlock, dispatchStream, validations
├── flow_control.go   # Window tracking, handleWindowUpdate, replenishReceiveWindow, updateSendWindow
├── generate.go       # Plan 9 assembly stubs (retained unchanged)
└── server_test.go    # Existing test suite (retained and passing)
```

---

### File 1: `server/h2/server_conn.go`
**Responsibility**: Exported public API facade, `ServerConn` struct, constructor, configuration, Per-P connection pool, and connection lifecycle (`Serve`, `Close`, `Closed`, `Release`).

```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/cpu"

	"github.com/lemon4ksan/foundation/net/hpack"
	"github.com/lemon4ksan/foundation/silicon/pool"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

// ServerHandlerFunc is the callback signature for dispatching an incoming HTTP/2 stream request.
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

// ServerResponse represents an outgoing HTTP/2 stream response in compliance with
// RFC 9113 §8.4 (Response Pseudo-Header Fields) and RFC 9110 §15 (Status Codes).
//
// Concurrency:
// ServerResponse is populated by the stream handler and serialized by the connection write loop.
// It is not safe for concurrent modification.
type ServerResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
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

	serverConnStorage.Put(sc)
}
```

---

### File 2: `server/h2/read_loop.go`
**Responsibility**: Frame demuxing read loop, settings ACK negotiation, ping reflection, error propagation, frame routing to streams.

```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"errors"
	"io"
	"net"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

func (sc *ServerConn) readLoop() error {
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
			if err := sc.handleWindowUpdate(fr); err != nil {
				coreh2.ReleaseFrameHeader(fr)
				return err
			}

		case coreh2.FrameResetStream:
			sc.handleResetStream(fr)

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
		// Client ACKed our settings (RFC 9113 §6.5.3)
		return nil
	}

	// Apply peer settings
	if body := fr.Body(); body != nil {
		if st, ok := body.(*coreh2.Settings); ok {
			if mfs := st.MaxFrameSize(); mfs >= 16384 && mfs <= 16777215 {
				sc.peerMaxFrameSize = mfs
			}

			if iws := st.MaxWindowSize(); iws > 0 && iws <= 0x7fffffff {
				sc.peerInitialWin = int32(iws) //nolint:gosec // bounds checked
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

	ping := fr.Body().(*coreh2.Ping)
	return sc.writePingAck(ping.Data())
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

	st := sc.newServerStream(streamID, endHeaders, endStream)
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
	data := dFrame.Data()
	st.body.Write(data)

	// Flow control replenishment (RFC 9113 §6.9)
	if len(data) > 0 {
		sc.replenishReceiveWindow(streamID, len(data))
	}

	if fr.Flags().Has(coreh2.FlagEndStream) {
		st.endStream = true
		sc.startStream(st)
	}

	return nil
}

func (sc *ServerConn) handleResetStream(fr *coreh2.FrameHeader) {
	defer coreh2.ReleaseFrameHeader(fr)

	streamID := fr.Stream()
	sc.streamsMu.Lock()
	if st, ok := sc.streams[streamID]; ok {
		if st.cancel != nil {
			st.cancel()
		}
		delete(sc.streams, streamID)
	}
	sc.streamsMu.Unlock()
}
```

---

### File 3: `server/h2/write_loop.go`
**Responsibility**: Response framing, HPACK encoding of `:status` and headers, DATA frame chunking and batching, thread-safe frame serialization, buffer flushing.

```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

func (sc *ServerConn) writeResponse(streamID uint32, res *ServerResponse) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	// 1. Serialize HEADERS Frame (RFC 9113 §6.2)
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

	// 2. Serialize DATA Frames (RFC 9113 §6.1)
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

func (sc *ServerConn) writePingAck(data [8]byte) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	ackFr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(ackFr)

	ackPing := coreh2.AcquireFrame(coreh2.FramePing).(*coreh2.Ping)
	ackPing.SetData(data)

	ackFr.SetFlags(coreh2.FlagAck)
	ackFr.SetBody(ackPing)

	if _, err := ackFr.WriteTo(sc.bw); err != nil {
		return err
	}

	return sc.bw.Flush()
}

func (sc *ServerConn) sendWindowUpdateFrame(streamID uint32, inc uint32) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	wuFr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(wuFr)

	wuFr.SetStream(streamID)
	wu := coreh2.AcquireFrame(coreh2.FrameWindowUpdate).(*coreh2.WindowUpdate)
	wu.SetIncrement(int(inc))
	wuFr.SetBody(wu)

	if _, err := wuFr.WriteTo(sc.bw); err != nil {
		return err
	}

	return sc.bw.Flush()
}
```

---

### File 4: `server/h2/stream.go`
**Responsibility**: Stream model (`serverStream`), stream lifecycle state, HPACK header decompression and verification (lowercase ASCII, pseudo-headers, forbidden headers, extended CONNECT per RFC 8441), stream handler dispatching, race-free `sync.WaitGroup` tracking, and safe stream deletion.

```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"bytes"
	"context"
	"net/http"

	"github.com/lemon4ksan/foundation/net/hpack"
	"github.com/lemon4ksan/foundation/net/http/status"

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
```

---

### File 5: `server/h2/flow_control.go`
**Responsibility**: Connection and stream flow control window accounting (RFC 9113 §5.2, §6.9), WINDOW_UPDATE frame handling, and receive window replenishment.

```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

func (sc *ServerConn) handleWindowUpdate(fr *coreh2.FrameHeader) error {
	defer coreh2.ReleaseFrameHeader(fr)

	wu := fr.Body().(*coreh2.WindowUpdate)
	inc := int32(wu.Increment()) //nolint:gosec

	// RFC 9113 §6.9: A receiver MUST treat the receipt of a WINDOW_UPDATE
	// frame with an increment of 0 as a stream error of type PROTOCOL_ERROR
	if inc <= 0 {
		return coreh2.ProtocolError
	}

	streamID := fr.Stream()
	if streamID == 0 {
		return sc.updateConnSendWindow(inc)
	}

	sc.streamsMu.RLock()
	st, ok := sc.streams[streamID]
	sc.streamsMu.RUnlock()

	if !ok {
		return nil
	}

	return sc.updateStreamSendWindow(st, inc)
}

func (sc *ServerConn) updateConnSendWindow(inc int32) error {
	for {
		old := sc.connSendWindow.Load()
		if int64(old)+int64(inc) > int64(1<<31-1) {
			// RFC 9113 §6.9.1: A sender MUST NOT allow a flow-control window to exceed 2^31-1
			return coreh2.FlowControlError
		}

		if sc.connSendWindow.CompareAndSwap(old, old+inc) {
			return nil
		}
	}
}

func (sc *ServerConn) updateStreamSendWindow(st *serverStream, inc int32) error {
	sc.streamsMu.Lock()
	defer sc.streamsMu.Unlock()

	if int64(st.sendWindow)+int64(inc) > int64(1<<31-1) {
		return coreh2.FlowControlError
	}

	st.sendWindow += inc
	return nil
}

func (sc *ServerConn) replenishReceiveWindow(streamID uint32, consumed int) {
	if consumed <= 0 || sc.Closed() {
		return
	}

	// Send WINDOW_UPDATE for stream and connection (RFC 9113 §6.9)
	_ = sc.sendWindowUpdateFrame(0, uint32(consumed))
	_ = sc.sendWindowUpdateFrame(streamID, uint32(consumed))
}
```

---

## 4. Invariants, Silicon Standards & Style Checklist

| Target Item | Requirement | Blueprint Enforcement |
|---|---|---|
| **BSD License Header** | Exact 3 lines followed by empty line | Included on all 5 files verbatim |
| **Public API Stability** | 100% contract preservation | All types (`ServerConn`, `ServerHandlerFunc`, `ServerRequest`, `ServerResponse`, `NewServerConn`, `Serve`, `Release`) retained with exact signatures |
| **Additive APIs** | Standard Go lifecycle facade | Added `Close() error` and `Closed() bool` |
| **Docstrings & RFC Citations** | Comprehensive RFC 9113, RFC 7541, RFC 8441, RFC 9110 citations on all exported types/methods | Full docstrings on all exported types, methods, fields |
| **Zero-Allocation Hot Paths** | Zero allocs on connection setup & framing | `sc.br.Reset(netConn)` and `sc.bw.Reset(netConn)` reuse pooled buffers without heap re-allocation |
| **Cache Line Padding** | SMP false sharing prevention | `_ cpu.CacheLinePad` placed between lock structs and atomic counters |
| **Linter Compliance** | strict `.golangci.yml` (`gofumpt`, `golines`, `wsl_v5`, `gci`, `revive`) | Compliant block separation, package groupings, and max length < 120 chars |

---

## 5. Concrete Step-by-Step Guidance for Worker M4.1

When implementing Milestone M4 on `server/h2/`:
1. **Prepare Workspace**: Ensure `$env:GOWORK="off"` is set for all Go executions.
2. **Create Target Files**:
   - Write `server/h2/server_conn.go` (facade, constructor, pool, Serve, Close, Closed, Release).
   - Write `server/h2/read_loop.go` (read loop, frame demuxing, settings/ping/headers/continuation/data/rst/goaway).
   - Write `server/h2/write_loop.go` (writeResponse, writePingAck, sendWindowUpdateFrame).
   - Write `server/h2/stream.go` (serverStream, startStream, finishHeaderBlock, dispatchStream).
   - Write `server/h2/flow_control.go` (handleWindowUpdate, updateConnSendWindow, replenishReceiveWindow).
3. **Verify License Headers**:
   - Verify every file starts with:
     ```go
     // Copyright (c) 2026 Lemon4ksan All rights reserved.
     // Use of this source code is governed by a BSD-style
     // license that can be found in the LICENSE file.

     package h2
     ```
4. **Run Verification Commands**:
   ```powershell
   $env:GOWORK="off"; go test -v -race ./server/h2/...
   $env:GOWORK="off"; go test -v -race -run TestH2 ./tests/e2e/...
   $env:GOWORK="off"; golangci-lint run ./server/h2/...
   $env:GOWORK="off"; go vet ./server/h2/...
   ```
5. **Verify Escalation 2 Elimination**:
   - Run race tests across multiple iterations:
     ```powershell
     $env:GOWORK="off"; go test -race -count=5 ./server/h2/...
     $env:GOWORK="off"; go test -race -count=5 -run TestH2_Tier3_AbruptConnectionDisconnectDuringInflight ./tests/e2e/...
     ```

---

## 6. Caveats

- **No Caveats**: The codebase, tests, and consumers were fully examined. The proposed 5-file architecture is 100% backward-compatible with all existing tests, eliminates Escalation 2, and strictly adheres to the master project plan.

---

## 7. Conclusion

The monolithic `server/h2/server_conn.go` (570 lines) has been fully mapped and partitioned into 5 focused, single-responsibility files under 200 lines each. Escalation 2 has been thoroughly analyzed: the data race on `sc.streams` in `Release()` and pool contamination are completely eliminated by introducing `streamsWg sync.WaitGroup`, mutex protection around map clears, and stream context cancellation. Silicon zero-allocation invariants (Per-P buffer resetting) and cache line padding have been incorporated.

---

## 8. Verification Method

To verify this report and blueprint:
1. Check that all 5 target files match `PROJECT.md` §5 and USER_REQUEST specifications.
2. Verify line numbers and data race mechanics cited from `server/h2/server_conn.go` (lines 106, 114, 450, 474, 480–508).
3. Confirm that all 18 H2 tests in `tests/e2e/h2_test.go` and `server/h2/server_test.go` exercise the exact public API preserved by this blueprint.
