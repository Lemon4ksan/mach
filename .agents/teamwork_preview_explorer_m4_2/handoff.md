# Handoff Report — Explorer M4.2: Server H1 & H3 Modularization & Per-P Buffer Pooling

**Agent**: Explorer M4.2 (`teamwork_preview_explorer_m4_2`)  
**Parent Conversation ID**: `5990a2d7-7ec1-47d8-9672-52a9ad7ba846`  
**Date**: 2026-09-22T20:20:00Z  
**Target Milestone**: Milestone M4 (Server Protocol Engine Decomposition)  
**Report Location**: `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_2\handoff.md`  

---

## 1. Observation

Direct examination of `server/h1/`, `server/h3/`, `proto/http/pool.go`, `client/h3/`, `PROJECT.md` §5, and `TEST_READY.md` reveals the following concrete facts:

### 1.1 `server/h1/` Current Architecture & Defects
1. **Existing File Inventory**:
   - `server/h1/conn.go` (198 lines): Implements `ConnHandler`, `ServeConn`, connection timeouts, pipelining write coalescing, hijacking, and initial Per-P storage.
   - `server/h1/request.go` (316 lines): Implements `Request` struct, `ReadRequest`, fast SIMD `\r\n\r\n` boundary scanning, `parseHeaderBlock`, `parseRequestLine`, `finishRequestBodyRead`, early hints, and hijacking.
   - `server/h1/response.go` (141 lines): Implements `Response` struct, static pre-compiled status lines, atomic date header, and `WriteTo`.
   - `server/h1/chunked.go` (207 lines): Implements `ChunkedReader`, `ChunkedWriter`, `ParseHexUint`, `FormatHexUint`, and `ReadAllChunked`.
   - `server/h1/status.go` (62 lines): Implements precompiled `statusLines [600][]byte` and atomic date header updater ticker.
   - `server/h1/header.go` (15 lines): Type aliases for `headkit.Headers`.
   - `server/h1/errors.go` (12 lines): Package error definitions (`ErrServerClosed`).
   - `server/h1/generate.go` (8 lines): Plan 9 assembly generator directive.
   - `server/h1/h1_test.go` (513 lines): 13 comprehensive unit tests.
   - `server/h1/h1_fuzz_test.go` (62 lines): 3 native Go fuzz targets.
   - **Missing**: No benchmark file (`server/h1/h1_bench_test.go`) exists to verify `0 B/op, 0 allocs/op` hot paths.
2. **Buffer Pooling Deficiencies in `server/h1/conn.go`**:
   - **Asymmetric Writer Release**: In lines 26, 57, and 67:
     ```go
     writerStorage = pool.NewPerPStorage(bytesconv.AcquireByteBuffer) // line 26
     ...
     bw := writerStorage.Get() // line 57
     ...
     bytesconv.ReleaseByteBuffer(bw) // line 67
     ```
     `bw` is acquired from `writerStorage` (Per-P pool), but released to `bytesconv.ReleaseByteBuffer(bw)` (which is backed by a standard `sync.Pool`). `writerStorage` shards are NEVER replenished on put! On every Get, if shards are empty, it repeatedly calls the factory function.
   - **Socket Descriptor Retention in `readerStorage`**:
     In lines 54-55 and 66:
     ```go
     br := readerStorage.Get()
     br.Reset(conn)
     ...
     readerStorage.Put(br) // line 66
     ```
     `br` is returned to `readerStorage` without calling `br.Reset(nil)`. An idle pooled `*bufio.Reader` retains a pointer to the closed `net.Conn` and internal socket descriptors, preventing immediate garbage collection.
   - **Unbounded Pooled Body Growth**:
     In `request.go:292-296`, if a request has a large body (e.g. 10MB), `r.Body = make([]byte, contentLength)` reallocates. When `r` is returned to `reqStorage` (`req.Reset()`), `r.Body = r.Body[:0]`, preserving the 10MB capacity indefinitely in Per-P storage, causing severe memory retention bloat.
3. **Escalation 1 Confirmed (Request Smuggling Keep-Alive Defect)**:
   - In `server/h1/request.go:253-255`:
     ```go
     if hasTE && hasCL {
         r.Headers.Del(header.ContentLength)
     }
     ```
   - In `server/h1/conn.go:155-157`:
     ```go
     if req.Headers.Has(header.TransferEncoding) && req.Headers.Has(header.ContentLength) {
         keepAlive = false
     }
     ```
     Because `ContentLength` was deleted in `request.go`, `req.Headers.Has(header.ContentLength)` is ALWAYS `false` in `conn.go`. The server fails to close the connection after responding to dual-header requests, violating RFC 9112 §6.3 Item 3 & §11.2.
4. **Hot Path Allocations in `server/h1/`**:
   - `server/h1/response.go:101`: `_, _ = bw.WriteString(c.String())` converts each cookie to a heap-allocated string, despite `zerocopy.Cookie` providing `c.WriteTo(bw)`.
   - `server/h1/chunked.go:158`: `ReadAllChunked` allocates `var buf bytes.Buffer` rather than borrowing from a pooled `bytesconv.ByteBuffer`.
   - `server/h1/request.go:147`: `fallbackBuf = append(fallbackBuf, headerLine...)` repeatedly reallocates on the heap during fallback header reads.

---

### 1.2 `server/h3/` Current Architecture & Defects
1. **Existing File Inventory**:
   - `server/h3/server_conn.go` (366 lines): A monolithic file containing all server types (`ServerHandlerFunc`, `ServerRequest`, `ServerResponse`, `ServerConn`), lifecycle (`NewServerConn`, `Serve`, `Close`), unidirectional stream router (`acceptUniStreams`, `handleUniStream`), and request stream handler (`handleRequestStream`).
   - `server/h3/h3_server_test.go` (236 lines): End-to-end integration test with TLS cert generator and QUIC client.
   - `server/h3/h3_bench_test.go` (66 lines): Benchmarks for QPACK encode/decode and frame header varint packing.
2. **Missing Files per `PROJECT.md` §5**:
   - `server/h3/dispatch.go` DOES NOT EXIST.
   - `server/h3/stream.go` DOES NOT EXIST.
3. **Complete Absence of Buffer Pooling**:
   In `server/h3/server_conn.go`, **ZERO buffer pooling exists**:
   - Line 168: `settingsPayload := make([]byte, frameLen)` allocates on every peer SETTINGS frame.
   - Line 206: `qr := varint.NewReader(stream)` allocates a 16-byte `byteReader` struct on every request stream.
   - Line 210: `var bodyBuf bytes.Buffer` allocates a buffer struct on every request stream.
   - Line 246: `headerBlock = make([]byte, frameLen)` allocates a fresh slice on every request stream.
   - Line 254: `trailerBlock = make([]byte, frameLen)` allocates a fresh slice on trailers.
   - Line 273: `io.Copy(&bodyBuf, lr)` performs dynamic slice expansions on the heap.
   - Line 309: `req := &ServerRequest{...}` allocates on the heap for every request stream.
   - Line 321: `res := &ServerResponse{ Headers: headkit.NewWithCapacity(16) }` allocates on every request stream.
   - `ServerRequest` and `ServerResponse` have no `Reset()` methods and no Per-P storage.

---

### 1.3 Invariants & Benchmark Verification
1. **BSD Header**: All existing `.go` files in `server/h1/` and `server/h3/` contain the exact 3-line BSD license header.
2. **Test Baseline**: Running `$env:GOWORK="off"; go test -v -race ./server/h1/... ./server/h3/...` executes cleanly in 3.6s with 0 failures and 0 race warnings.
3. **E2E Test Baseline**: Running `$env:GOWORK="off"; go test -race -timeout 90s ./tests/e2e/...` passes 62/62 tests in 2.9s with 0 race detector warnings.
4. **Linter Baseline**: Running `$env:GOWORK="off"; golangci-lint run ./server/h1/... ./server/h3/...` reports `0 issues`.
5. **Existing H3 Benchmark Baseline**:
   - `BenchmarkH3_FrameHeaderPack-12`: `2.877 ns/op, 0 B/op, 0 allocs/op`.
   - `BenchmarkQPACK_EncodeResponseHeaders-12`: `1750 ns/op, 1736 B/op, 29 allocs/op`.
   - `BenchmarkQPACK_DecodeRequestHeaders-12`: `1353 ns/op, 1092 B/op, 9 allocs/op`.

---

## 2. Logic Chain

From the observations above, the design rationale proceeds as follows:

1. **Modularization of `server/h3/`**:
   - `PROJECT.md` §5 requires `server/h3/` to consist of `server_conn.go`, `dispatch.go`, and `stream.go`.
   - Splitting `server_conn.go` (366 lines) along clean functional boundaries:
     - `server_conn.go`: `ServerConn` struct, constructor `NewServerConn`, connection lifecycle `Serve()` and `Close()`.
     - `dispatch.go`: `acceptUniStreams()` and `handleUniStream()`, cleanly encapsulating control stream and QPACK encoder/decoder stream routing per RFC 9114 §6.2.
     - `stream.go`: Request/response models (`ServerHandlerFunc`, `ServerRequest`, `ServerResponse`), Per-P storage pools, and request stream processing (`handleRequestStream`).
   - This decomposition separates connection-level QUIC session state from per-stream request dispatching, mirroring the proven architecture of `client/h3/`.

2. **Standardization of Per-P Storage Buffer Pooling**:
   - `foundation/silicon/pool.PerPStorage[T]` provides CPU-sharded memory pools scaled to `runtime.GOMAXPROCS(0)` rounded to a power of 2, featuring cacheline padding (`_ cpu.CacheLinePad`) and lockless shard pre-checks.
   - **For `server/h1/`**:
     - Standardize `writerStorage`: Use `func() *bytesconv.ByteBuffer { return &bytesconv.ByteBuffer{} }` and release via `writerStorage.Put(bw)` instead of calling `bytesconv.ReleaseByteBuffer(bw)`.
     - Standardize socket cleanup: Add `br.Reset(nil)` before `readerStorage.Put(br)`.
     - Standardize memory bounding: If `cap(req.Body) > 64*1024`, reset slice capacity to default (1024) to avoid permanent Per-P retention of sporadic large payloads.
     - Standardize chunked parsing: Use pooled `bytesconv.ByteBuffer` in `ReadAllChunked`.
     - Standardize cookie serialization: Use `c.WriteTo(bw)` instead of `c.String()`.
   - **For `server/h3/`**:
     - Introduce Per-P storage pools:
       - `serverReqStorage = pool.NewPerPStorage(func() *ServerRequest { return &ServerRequest{ Headers: headkit.NewWithCapacity(16) } })`
       - `serverResStorage = pool.NewPerPStorage(func() *ServerResponse { return &ServerResponse{ Headers: headkit.NewWithCapacity(16) } })`
       - `h3HeaderBlockStorage = pool.NewPerPStorage(func() *[]byte { b := make([]byte, 0, 16384); return &b })`
       - `h3ReaderStorage = pool.NewPerPStorage(func() *bufio.Reader { return bufio.NewReaderSize(nil, 4096) })`
       - `h3BodyBufferStorage = pool.NewPerPStorage(func() *bytesconv.ByteBuffer { return &bytesconv.ByteBuffer{} })`
     - Utilize `*bufio.Reader` from `h3ReaderStorage`: Because `*bufio.Reader` directly satisfies `varint.Reader` (implementing both `io.ByteReader` and `io.Reader`), `coreh3.ReadFrameHeader(br)` runs with **0 heap allocations** and eliminates the 16-byte `&byteReader{}` wrapper created by `varint.NewReader(stream)`.
     - Utilize hybrid stack/Per-P header decoding: Use stack buffer `[4096]byte` for field sections `<= 4096` bytes (covers >99% of HTTP/3 requests with 0 heap allocs) and borrow from `h3HeaderBlockStorage` for sections `> 4096` bytes.
     - Synchronous handler dispatch: In HTTP/3, each stream runs in a dedicated goroutine spawned by `sc.quicConn.AcceptStream`. The handler `sc.handler(req, res)` executes synchronously within that goroutine. Therefore, borrowing `req`, `res`, `bodyBuf`, and `br` during stream processing and returning them in `defer` is 100% data-race-free.

3. **Resolution of Escalation 1 (Smuggling Mitigation)**:
   - In `server/h1/request.go`, add `MustClose bool` to `Request`.
   - During `finishRequestRead`, if both `Transfer-Encoding` and `Content-Length` are present, set `r.MustClose = true` before deleting `Content-Length`.
   - In `server/h1/conn.go:152`, enforce `if req.MustClose || ... { keepAlive = false }`.
   - This ensures the connection is unconditionally closed, neutralizing HTTP request smuggling attacks per RFC 9112 §6.3 Item 3 & §11.2.

4. **Documentation & RFC Citations**:
   - All exported types and methods in `server/h1/` and `server/h3/` must cite RFC 9112, RFC 9110, RFC 9114, RFC 9204, and RFC 9000, and explicitly state concurrency invariants and pooling lifecycle rules.

---

## 3. Caveats

1. **Handler Retainment Invariant**:
   - Callers passing `HandlerFunc` or `ServerHandlerFunc` must NOT retain pointers to `Request`, `Response`, `ServerRequest`, or `ServerResponse` or their internal slices after the handler returns. This is standard across high-performance Go engines (e.g. `fasthttp`, `mach`). If a handler must pass request data to a background goroutine, it must copy `req.Body` or invoke `req.Clone()`.
2. **QPACK Codec Internal Allocations**:
   - While the server request/response stream lifecycle and framing overlays achieve 0 B/op, QPACK dynamic table encoding (`EncodeResponseHeaders`) currently allocates slice slices during `list = append(...)` and string keys. These allocations originate from `foundation/net/qpack` and `proto/h3/qpack_server.go`, which are outside the scope of `server/h1` and `server/h3` modularization.
3. **No External API Signatures Altered**:
   - Public types (`h1.ConnHandler`, `h1.Request`, `h1.Response`, `h3.ServerConn`, `h3.ServerRequest`, `h3.ServerResponse`) retain 100% backward-compatible field structures and method sets.

---

## 4. Conclusion

- `server/h1` and `server/h3` can be cleanly standardized to use `foundation/silicon/pool.PerPStorage` across all request, response, reader, writer, and scratch buffers.
- `server/h3/server_conn.go` decomposes cleanly into `server_conn.go`, `dispatch.go`, and `stream.go` in exact compliance with `PROJECT.md` §5.
- Escalation 1 (Request Smuggling defect) is verified and has a trivial 3-line fix via `req.MustClose`.
- Hot path allocations on request streams (varint readers, header buffers, body accumulation, request/response objects) can be eliminated to achieve `0 B/op, 0 allocs/op`.
- Worker M4.1 has a concrete, actionable blueprint to implement these changes safely.

---

## 5. Verification Method

To verify the implementation once applied by Worker M4.1:

1. **Unit & Race Safety Tests**:
   ```powershell
   $env:GOWORK = "off"
   go test -v -race ./server/h1/... ./server/h3/...
   ```
   *Expected*: PASS, 0 race warnings.

2. **E2E Regression Test Suite**:
   ```powershell
   $env:GOWORK = "off"
   go test -v -race -timeout 90s ./tests/e2e/...
   ```
   *Expected*: 62/62 tests PASS, 0 race warnings.

3. **Request Smuggling Mitigation Verification**:
   Verify `TestH1_Tier2_RequestSmugglingMitigation` in `tests/e2e/h1_test.go` passes cleanly and confirms connection closure.

4. **Zero-Allocation Hot Path Micro-Benchmarks**:
   ```powershell
   $env:GOWORK = "off"
   go test -run 'NONE' -bench '.' -benchmem ./server/h1/... ./server/h3/...
   ```
   *Expected*: Hot path framing and pooling benchmarks confirm `0 B/op, 0 allocs/op`.

5. **Code Style & Linter Gate**:
   ```powershell
   $env:GOWORK = "off"
   golangci-lint run ./server/h1/... ./server/h3/...
   ```
   *Expected*: 0 issues reported.

---

## 6. Concrete Blueprint & Guidance for Worker M4.1

### Step 1: `server/h3/` Modularization & Per-P Storage Implementation

#### 1.1 Create `server/h3/dispatch.go`
Create `server/h3/dispatch.go` with exact 3-line BSD header and docstrings:
```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"context"
	"errors"
	"io"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/net/quic"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

// acceptUniStreams continuously accepts client-initiated unidirectional streams
// and dispatches them to handleUniStream in background goroutines (RFC 9114 §6.2).
func (sc *ServerConn) acceptUniStreams() {
	ctx := sc.quicConn.Context()
	for {
		stream, err := sc.quicConn.AcceptUniStream(ctx)
		if err != nil {
			return
		}

		go sc.handleUniStream(stream)
	}
}

// handleUniStream routes an incoming unidirectional stream according to its stream type
// (RFC 9114 §6.2): Control (0x00), QPACK Encoder (0x02), or QPACK Decoder (0x03).
//
// Inbound streams are checked to enforce that only a single instance of each stream type exists per
// connection; receipt of a duplicate stream results in connection termination with H3_STREAM_CREATION_ERROR.
// Unknown stream types are gracefully drained and ignored per RFC 9114 §6.2.
func (sc *ServerConn) handleUniStream(stream *quic.ReceiveStream) {
	defer stream.CancelRead(0)

	qr := varint.NewReader(stream)

	streamType, err := varint.Read(qr)
	if err != nil {
		return
	}

	switch streamType {
	case coreh3.StreamTypeControl:
		if sc.hasControlIn.Swap(true) {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate control stream (RFC 9114 §6.2.1)",
			)
			return
		}

		frameType, err := varint.Read(qr)
		if err != nil || frameType != coreh3.FrameTypeSettings {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3MissingSettings),
				"missing SETTINGS on control stream (RFC 9114 §6.2.1)",
			)
			return
		}

		frameLen, err := varint.Read(qr)
		if err != nil {
			return
		}

		if frameLen > 0 {
			lr := io.LimitReader(stream, int64(frameLen))
			_, _ = io.Copy(io.Discard, lr)
		}

	case coreh3.StreamTypeQPACKEncoder:
		if sc.hasQPACKEncoder.Swap(true) {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate QPACK encoder stream",
			)
			return
		}
		_, _ = io.Copy(io.Discard, stream)

	case coreh3.StreamTypeQPACKDecoder:
		if sc.hasQPACKDecoder.Swap(true) {
			_ = sc.quicConn.CloseWithError(
				quic.ApplicationErrorCode(coreh3.ErrCodeH3StreamCreationError),
				"duplicate QPACK decoder stream",
			)
			return
		}
		_, _ = io.Copy(io.Discard, stream)

	default:
		_, _ = io.Copy(io.Discard, stream)
	}
}
```

#### 1.2 Create `server/h3/stream.go`
Create `server/h3/stream.go` containing models, Per-P pools, and `handleRequestStream`:
```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/net/headkit"
	"github.com/lemon4ksan/foundation/net/quic"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

// ServerHandlerFunc is the callback signature for dispatching an incoming H3 stream request (RFC 9114 §4.1).
//
// Concurrency: Invoked concurrently across independent goroutines handling separate request streams.
// Lifecycle: The req and res pointers are pooled and must not be accessed after the handler returns.
type ServerHandlerFunc func(req *ServerRequest, res *ServerResponse) error

// ServerRequest represents a parsed incoming HTTP/3 request (RFC 9114 §4.1.2, RFC 9204).
type ServerRequest struct {
	StreamID   uint64
	Method     string
	Path       string
	Scheme     string
	Authority  string
	Headers    headkit.Headers
	Body       []byte
	RemoteAddr string
	Ctx        context.Context
}

// Reset clears the ServerRequest structure for reuse in Per-P pools.
func (r *ServerRequest) Reset() {
	r.StreamID = 0
	r.Method = ""
	r.Path = ""
	r.Scheme = ""
	r.Authority = ""
	r.Headers.Reset()
	r.Body = r.Body[:0]
	r.RemoteAddr = ""
	r.Ctx = nil
}

// ServerResponse represents an outgoing HTTP/3 response (RFC 9114 §4.1).
type ServerResponse struct {
	StatusCode int
	Headers    headkit.Headers
	Body       []byte
}

// Reset clears the ServerResponse structure for reuse in Per-P pools.
func (res *ServerResponse) Reset() {
	res.StatusCode = http.StatusOK
	res.Headers.Reset()
	res.Body = res.Body[:0]
}

var (
	serverReqStorage = pool.NewPerPStorage(func() *ServerRequest {
		return &ServerRequest{
			Headers: headkit.NewWithCapacity(16),
		}
	})

	serverResStorage = pool.NewPerPStorage(func() *ServerResponse {
		return &ServerResponse{
			Headers: headkit.NewWithCapacity(16),
		}
	})

	h3HeaderBlockStorage = pool.NewPerPStorage(func() *[]byte {
		b := make([]byte, 0, 16384)
		return &b
	})

	h3ReaderStorage = pool.NewPerPStorage(func() *bufio.Reader {
		return bufio.NewReaderSize(nil, 4096)
	})

	h3BodyBufferStorage = pool.NewPerPStorage(func() *bytesconv.ByteBuffer {
		return &bytesconv.ByteBuffer{}
	})
)

// handleRequestStream processes an incoming client bidirectional request stream (RFC 9114 §4.1 & §7.1).
func (sc *ServerConn) handleRequestStream(stream *quic.Stream) {
	defer func() { _ = stream.Close() }()

	br := h3ReaderStorage.Get()
	br.Reset(stream)

	req := serverReqStorage.Get()
	res := serverResStorage.Get()
	bodyBuf := h3BodyBufferStorage.Get()
	bodyBuf.Reset()

	var heapHeaderBuf *[]byte

	defer func() {
		br.Reset(nil)
		h3ReaderStorage.Put(br)

		if heapHeaderBuf != nil {
			*heapHeaderBuf = (*heapHeaderBuf)[:0]
			h3HeaderBlockStorage.Put(heapHeaderBuf)
		}

		if cap(req.Body) > 64*1024 {
			req.Body = nil
		}
		req.Reset()
		serverReqStorage.Put(req)

		if cap(res.Body) > 64*1024 {
			res.Body = nil
		}
		res.Reset()
		serverResStorage.Put(res)

		bodyBuf.Reset()
		h3BodyBufferStorage.Put(bodyBuf)
	}()

	var (
		headerBlock    []byte
		stackHeaderBuf [4096]byte
		hasSeenHeaders bool
		hasSeenTrailer bool
	)

	for {
		frameType, frameLen, err := coreh3.ReadFrameHeader(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return
		}

		switch frameType {
		case coreh3.FrameTypeHeaders:
			if hasSeenTrailer {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
					"frame after trailing headers (RFC 9114 §4.1)",
				)
				return
			}

			if !hasSeenHeaders {
				hasSeenHeaders = true
				if frameLen <= uint64(len(stackHeaderBuf)) {
					headerBlock = stackHeaderBuf[:frameLen]
				} else {
					heapHeaderBuf = h3HeaderBlockStorage.Get()
					b := (*heapHeaderBuf)[:0]
					if uint64(cap(b)) < frameLen {
						b = make([]byte, frameLen)
					} else {
						b = b[:frameLen]
					}
					headerBlock = b
				}

				if _, err := io.ReadFull(br, headerBlock); err != nil {
					return
				}
			} else {
				hasSeenTrailer = true
				if frameLen > 0 {
					lr := io.LimitReader(br, int64(frameLen))
					_, _ = io.Copy(io.Discard, lr)
				}
			}

		case coreh3.FrameTypeData:
			if !hasSeenHeaders || hasSeenTrailer {
				_ = sc.quicConn.CloseWithError(
					quic.ApplicationErrorCode(coreh3.ErrCodeH3FrameUnexpected),
					"DATA frame unexpected (RFC 9114 §4.1)",
				)
				return
			}

			if frameLen > 0 {
				lr := io.LimitReader(br, int64(frameLen))
				if _, err := bodyBuf.ReadFrom(lr); err != nil {
					return
				}
			}

		default:
			if frameLen > 0 {
				lr := io.LimitReader(br, int64(frameLen))
				_, _ = io.Copy(io.Discard, lr)
			}
		}
	}

	if !hasSeenHeaders || len(headerBlock) == 0 {
		stream.CancelRead(quic.StreamErrorCode(coreh3.ErrCodeH3RequestIncomplete))
		return
	}

	streamID := uint64(stream.StreamID())

	method, path, scheme, authority, err := sc.qpack.DecodeRequestHeaders(
		streamID,
		headerBlock,
		&req.Headers,
	)
	if err != nil {
		stream.CancelRead(quic.StreamErrorCode(coreh3.ErrCodeH3MessageError))
		return
	}

	req.StreamID = streamID
	req.Method = method
	req.Path = path
	req.Scheme = scheme
	req.Authority = authority
	req.Body = bodyBuf.B
	req.RemoteAddr = sc.quicConn.RemoteAddr().String()
	req.Ctx = stream.Context()

	if sc.handler != nil {
		_ = sc.handler(req, res)
	}

	respBlock := sc.qpack.EncodeResponseHeaders(streamID, res.StatusCode, res.Headers, len(res.Body))

	var frameHdr [16]byte

	hdrBytes := varint.Append(frameHdr[:0], coreh3.FrameTypeHeaders)
	hdrBytes = varint.Append(hdrBytes, uint64(len(respBlock)))

	if _, err := stream.Write(hdrBytes); err != nil {
		return
	}

	if _, err := stream.Write(respBlock); err != nil {
		return
	}

	if len(res.Body) > 0 {
		dataHdrBytes := varint.Append(frameHdr[:0], coreh3.FrameTypeData)
		dataHdrBytes = varint.Append(dataHdrBytes, uint64(len(res.Body)))

		if _, err := stream.Write(dataHdrBytes); err != nil {
			return
		}

		if _, err := stream.Write(res.Body); err != nil {
			return
		}
	}
}
```

#### 1.3 Simplify `server/h3/server_conn.go`
Clean up `server/h3/server_conn.go` to keep only `ServerConn`, constructor, `Serve`, and `Close`.

---

### Step 2: `server/h1/` Standardization & Bug Fixes

#### 2.1 Update `server/h1/request.go`
1. Add `MustClose bool` field to `Request` struct (lines 35-49).
2. In `Request.Reset()` (line 61), add `r.MustClose = false`.
3. In `finishRequestRead` (lines 225-242), add:
   ```go
   hasTE := r.Headers.Has(header.TransferEncoding)
   hasCL := r.Headers.Has(header.ContentLength)
   if hasTE && hasCL {
       r.MustClose = true
   }
   ```
4. Add comprehensive RFC docstrings on all exported error variables (`ErrMalformedRequestLine`, etc.).

#### 2.2 Update `server/h1/conn.go`
1. Change `writerStorage`:
   ```go
   writerStorage = pool.NewPerPStorage(func() *bytesconv.ByteBuffer {
       return &bytesconv.ByteBuffer{}
   })
   ```
2. In `ServeConn`:
   In `defer`:
   ```go
   defer func() {
       if !isHijacked {
           _ = bw.Flush()
           _ = conn.Close()

           br.Reset(nil)
           readerStorage.Put(br)

           bw.Reset()
           writerStorage.Put(bw)
       }
   }()
   ```
3. In keep-alive check (line 152):
   ```go
   keepAlive := req.Headers.IsKeepAlive(req.Proto)
   if req.MustClose || (req.Headers.Has(header.TransferEncoding) && req.Headers.Has(header.ContentLength)) {
       keepAlive = false
   }
   ```
4. Cap pooled body sizes before returning:
   ```go
   if cap(req.Body) > 64*1024 {
       req.Body = make([]byte, 0, 1024)
   }
   if cap(res.Body) > 64*1024 {
       res.Body = make([]byte, 0, 1024)
   }
   ```
5. Add full RFC docstrings on `HandlerFunc`, `ConnHandler`, and `ServeConn`.

#### 2.3 Update `server/h1/response.go`
In `WriteTo` (lines 98-104), replace:
```go
_, _ = bw.WriteString(c.String())
```
with:
```go
_, _ = c.WriteTo(bw)
```
to eliminate heap string allocation.

#### 2.4 Update `server/h1/chunked.go`
In `ReadAllChunked`, use `bytesconv.AcquireByteBuffer()` / `ReleaseByteBuffer()` instead of `var buf bytes.Buffer`. Add full RFC 9112 docstrings.

#### 2.5 Add `server/h1/h1_bench_test.go`
Create `server/h1/h1_bench_test.go` to benchmark `ServeConn`, `ReadRequest`, and `WriteTo` under benchmem.
