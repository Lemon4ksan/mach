# Architecture & Modularity Exploration Report: Mach Protocol Engine

## 1. Observation

### 1.1 Codebase Structure and File Size Inventory
A complete scan of `d:\CodingProjects\mach` (Go version: 1.27.0, foundation dependency `v0.0.0-20260920191713-7709c688b2d7`) identified 74 Go source and test files across 16 packages. The line counts, file byte sizes, and primary responsibilities are itemized below:

| Package | File | Total Lines | Bytes | Core Responsibilities Observed |
|---|---|---|---|---|
| `client/h2` | `conn.go` | 1667 | 35425 | Client H2 multiplexer, socket I/O, write/read event loops, stream table hash/overflow map, flow control window accounting, frame dispatch, request encoding, response parsing, server push, network dialing |
| `proto/http` | `header_request.go` | 1298 | 43108 | Request header accessors, URI parsing, ASCII validation, cookie collection, SIMD trailers, scoped borrowing |
| `proto/http` | `request.go` | 1194 | 30818 | Request struct, body buffers, streaming body, multipart forms, wire reading/writing, vectored socket writes |
| `proto/http` | `header_response.go` | 1079 | 36851 | Response header accessors, status line formatting, content length / encoding headers, cookie mutations |
| `proto/http` | `http.go` | 936 | 20900 | Body pool limits, chunked coding reader/writer, identity body reading, compressed stream wrapping, stats writers |
| `proto/http` | `header.go` | 911 | 24174 | Shared base `header` struct, trailer validation, header normalization, error definitions |
| `proto/http` | `response.go` | 837 | 25887 | Response struct, body buffers, streaming body, wire reading/writing, compression on write (gzip, brotli, zstd) |
| `proto/h3` | `qpack.go` | 690 | 15861 | QPACK encoder/decoder codec, client/server header block serialization, forbidden header rules, stream error tracking |
| `server/h2` | `server_conn.go` | 570 | 12423 | H2 server connection lifecycle, preface validation, frame dispatch, pseudo-header validation, stream handling, response framing |
| `client/h3` | `conn.go` | 560 | 12103 | H3 QUIC client connection, control stream lifecycle, unidirectional stream demuxing, request sending, response decoding |
| `proto/compress` | `compress.go` | 536 | 12573 | Gzip and Deflate pool allocators, stackless writers, limit readers, stream compressors |
| `proto/h2` | `frames.go` | 492 | 15178 | H2 frame payload structs: `Continuation`, `Data`, `GoAway`, `Headers`, `Ping`, `Priority`, `PushPromise`, `RstStream`, `WindowUpdate` |
| `server/h1` | `h1_test.go` | 397 | 12632 | H1 server test suite (pipelining, chunked, headers, error cases) |
| `client/h3` | `conn_test.go` | 375 | 13371 | H3 client unit and mock tests |
| `server/h3` | `server_conn.go` | 366 | 8866 | H3 server connection lifecycle, control stream setup, unidirectional stream dispatch, request execution |
| `proto/h3` | `qpack_test.go` | 351 | 11704 | QPACK codec unit tests (dynamic table, pseudo headers, error cases) |
| `server/h1` | `request.go` | 316 | 7891 | H1 server request parser with SIMD header boundary scanning, early hints, connection hijacking |
| `proto/h2` | `header.go` | 295 | 7239 | H2 9-byte frame header serializer/deserializer (`FrameHeader`), memory pooling |
| `proto/http` | `pipeconns.go` | 263 | 6384 | In-memory pipe connection implementation for testing and bridging |
| `proto/http` | `h1_test.go` | 251 | 7385 | H1 protocol tests |
| `proto/h2` | `frames_test.go` | 246 | 6741 | H2 frame serialization/deserialization tests |
| `proto/h2` | `utils.go` | 232 | 6083 | H2 connection preface verification, client handshake, 24-bit integer packing |
| `proto/h2` | `settings.go` | 222 | 8062 | H2 `Settings` frame implementation, parameter encoding/decoding |
| `proto/h3` | `frames.go` | 221 | 6665 | H3 frame type constants, unidirectional stream types, `Settings` frame encoder/decoder |
| `proto/http` | `headerscanner.go` | 206 | 4539 | Zero-allocation HTTP header scanner powered by `foundation/silicon/simd` |
| `server/h1` | `conn.go` | 198 | 4606 | H1 server connection lifecycle (`ConnHandler`, `ServeConn`), per-P buffer pooling |
| `proto/h2` | `frame_pool.go` | 190 | 4994 | `ConnectionFramePool` off-heap slab allocators for POD frames (`Ping`, `WindowUpdate`, `RstStream`, `Priority`) |
| `server/h3` | `h3_server_test.go` | 183 | 6322 | H3 server integration tests |
| `proto/h2` | `errors.go` | 168 | 6255 | H2 error codes (RFC 9113 §7) and error types |
| `client/h2` | `conn_test.go` | 162 | 4754 | H2 client connection tests |
| `server/h1` | `chunked.go` | 162 | 4304 | H1 server chunked transfer encoding reader and writer |
| `proto/h2` | `frame.go` | 161 | 4977 | `Frame` interface, `FrameType`, `FrameFlags`, global sync.Pool frame allocators |
| `proto/http/stackless` | `writer.go` | 155 | 3398 | Coroutine-like stackless writer implementation |
| `proto/compress` | `compress_test.go` | 148 | 4715 | Compression test suite |
| `proto/http` | `headers.go` | 133 | 7539 | Standard HTTP header string constants |
| `proto/http` | `streaming.go` | 133 | 2491 | `RequestStream` chunked/fixed streaming reader |
| `client` | `pool.go` | 131 | 2868 | Generic host connection pool manager (`PoolManager[T]`) |
| `server/h2` | `server_test.go` | 129 | 3586 | H2 server integration tests |
| `proto/h2/overlay` | `frame.go` | 119 | 3006 | Zero-alloc in-situ H2 frame overlays (`Frame`, `DataFrame`, `HeadersFrame`) |
| `proto/h2` | `frame_pool_test.go` | 119 | 3383 | Off-heap frame slab allocator tests |
| `server/h1` | `response.go` | 116 | 3456 | H1 server response serializer (`WriteResponse`) |
| `proto/h2` | `frame_stress_test.go`| 106 | 3115 | Concurrent H2 frame parsing stress tests |
| `proto/http` | `llhttp_vectors_test.go` | 99 | 2846 | llhttp compatibility test vector suite |
| `proto/compress` | `brotli.go` | 89 | 2281 | Brotli compression / decompression wrappers |
| `proto/compress` | `zstd.go` | 86 | 2089 | Zstandard compression / decompression wrappers |
| `proto/h2/overlay` | `frame_test.go` | 86 | 1955 | Overlay frame benchmark and sanity tests |
| `proto/h3` | `errors.go` | 77 | 6035 | H3 and QPACK RFC 9114 / RFC 9204 error codes |
| `proto/h2` | `fuzz_test.go` | 75 | 1693 | H2 frame reader and HPACK decoder fuzz harnesses |
| `proto/http/stackless` | `func.go` | 74 | 2029 | Stackless worker pool execution function |
| `fsm/h2` | `fsm.go` | 74 | 1777 | Sans-IO HTTP/2 finite state machine skeleton |
| `scripts` | `fuzz_all.go` | 71 | 2133 | Multi-target protocol fuzzing orchestrator script |
| `proto/h2` | `frame_pool_bench_test.go` | 70 | 1974 | Benchmark suite comparing off-heap slab vs sync.Pool |
| `proto/h3` | `frames_test.go` | 70 | 2284 | H3 frame serialization tests |
| `x/raptor` | `decoder.go` | 70 | 1558 | Experimental RaptorQ fountain code decoder |
| `proto/http` | `header_helpers.go` | 63 | 1710 | Header normalization and date parsing helpers |
| `client/h1` | `conn.go` | 60 | 982 | H1 client connection wrapper |
| `proto/http` | `pool.go` | 60 | 1534 | Sync.Pool request and response allocators |
| `x/raptor` | `encoder.go` | 56 | 1434 | Experimental RaptorQ fountain code encoder |
| `client/h2` | `export.go` | 56 | 1422 | Type aliases and constructor re-exports from `proto/h2` and `hpack` |
| `proto/http` | `tls.go` | 55 | 1603 | TLS client config and connection setup helpers |
| `proto/http` | `h1_fuzz_test.go` | 55 | 1789 | H1 request/response wire fuzz tests |
| `client/h2` | `context.go` | 52 | 1075 | Client stream context and stream state definitions |
| `server/h1` | `status.go` | 51 | 1449 | HTTP status line fast serializer |
| `server/h1` | `h1_fuzz_test.go` | 50 | 1696 | Server H1 wire parser fuzz harnesses |
| `server/h3` | `h3_bench_test.go` | 49 | 1669 | H3 server request benchmark |
| `proto/http` | `chunk.go` | 49 | 1450 | Chunked body trailer writer helper |
| `mach` | `engine.go` | 48 | 1457 | Core engine interfaces (`Engine`, `Conn`, `Stream`, `Frame`) |
| `proto/http` | `stream.go` | 47 | 1302 | StreamWriter and stream adapter functions |
| `proto/http` | `round2_32.go` | 26 | 670 | 32-bit power-of-two rounding helper |
| `proto/h3` | `fuzz_test.go` | 22 | 629 | H3 frame header fuzz test |
| `proto/h2` | `frame_bench_test.go` | 22 | 608 | H2 frame allocation micro-benchmarks |
| `proto/http` | `round2_64.go` | 21 | 547 | 64-bit power-of-two rounding helper |
| `client/h3` | `export.go` | 18 | 620 | Re-exports of `Settings`, `QPACKCodec`, `NewQPACKCodec` |
| `proto/http` | `methods.go` | 16 | 633 | HTTP method byte slice constants |
| `proto/http` | `errors.go` | 14 | 716 | Common HTTP protocol errors |
| `server/h1` | `header.go` | 10 | 379 | Internal server header type alias |
| `server/h1` | `errors.go` | 8 | 250 | Internal server errors |
| `server/h2` | `generate.go` | 7 | 495 | Go generate directive for server H2 |
| `server/h1` | `generate.go` | 5 | 264 | Go generate directive for server H1 |
| `mach` | `doc.go` | 5 | 257 | Package docstring |

### 1.2 Identified Monolithic Hotspots

1. **`client/h2/conn.go` (1,667 lines)**:
   - Contains 10+ orthogonal responsibilities:
     1. Multiplexer stream table (`reqStreams` open-addressing table + `reqShards` overflow buckets)
     2. Stream lifecycle (`getStream`, `storeStream`, `deleteStream`, `broadcastErrorToAllStreams`, `purgeStreamsAfterID`)
     3. Connection-level & stream-level flow control (`waitForWindowUpdate`, `calculateChunkSize`, `handleWindowUpdate`, `updateServerWindow`, `updateStreamWindow`)
     4. Background read loop & frame demuxing (`readLoop`, `readNext`, `handleConnectionFrame`)
     5. Background write loop & vector batching (`writeLoop`, `selectWriteEvent`, `recoverWriteLoop`)
     6. Request execution & DATA chunking (`writeRequest`, `writeData`, `waitExpectContinue`)
     7. HPACK header formatting & forbidden header filtering (`encodeRequestHeaders`, `appendOrderedHeaders`, `isForbiddenH2Header`)
     8. Response & trailer decoding (`readHeader`, `readTrailers`)
     9. Server Push Promise handling (`handlePushPromise`, `decodePushHeaders`, `awaitPushedResponse`)
     10. TLS and raw TCP dialing (`Dialer`, `Dial`, `DialContext`, `tryDial`)
     11. Public client facade (`NewConn`, `Do`, `Write`, `Close`, `Closed`, `CanOpenStream`)

2. **`proto/http` Monolithic Modules (~6,100 lines total across 6 files)**:
   - Evidence of prior mechanical split (`.tmp/split_header.go`, `.tmp/split_http.go`):
     - `header_request.go` (1,298 lines) and `header_response.go` (1,079 lines) mechanically partitioned all `RequestHeader` vs `ResponseHeader` declarations regardless of cohesion.
     - `request.go` (1,194 lines) and `response.go` (837 lines) contain disjoint features: body slice manipulation, streaming body readers, scoped borrow allocations, multipart parsing, wire encoding/decoding, and file transfers.
     - `http.go` (936 lines) combines body pooling limits, chunked coding reading/writing, identity body reading, compression stream pipeline wrappers, and stats writers.
     - `header.go` (911 lines) mixes common header storage, trailer enforcement, and protocol validation.

3. **`server/h2/server_conn.go` (570 lines)**:
   - Mixes socket I/O, 24-byte preface check, frame reading loop, pseudo-header validation (RFC 9113 §8.3), application handler execution, and response serialization (HPACK HEADERS + DATA frame chunking).

4. **`client/h3/conn.go` (560 lines)**:
   - Mixes QUIC transport session management, control stream bootstrapping (type 0x00), unidirectional stream demultiplexing (control, QPACK encoder, QPACK decoder), request encoding, and response decoding.

5. **`proto/h3/qpack.go` (690 lines)**:
   - Mixes `QPACKCodec` lifecycle and error management with client request encoding, client response decoding, server request decoding, server response encoding, and forbidden header filtering.

6. **`proto/compress/compress.go` (536 lines)**:
   - Combines generic compression levels with Gzip readers/writers/stackless pools and Flate readers/writers/stackless pools.

7. **`proto/h2/frames.go` (492 lines)**:
   - Defines 9 different frame types (`Continuation`, `Data`, `GoAway`, `Headers`, `Ping`, `Priority`, `PushPromise`, `RstStream`, `WindowUpdate`) in a single file.

---

## 2. Logic Chain

### 2.1 Preserving Public Contracts while Achieving Modularity
1. **Observation**: All external consumers (`aoni`, `foundation`, end-user applications) interact with `mach` exclusively through exported types, methods, functions, and interfaces across `mach`, `client`, `client/h1`, `client/h2`, `client/h3`, `server/h1`, `server/h2`, `server/h3`, `proto/http`, `proto/h2`, `proto/h3`, `proto/compress`.
2. **Logic Step**: In Go, all files within the same package share the exact same namespace and visibility. Internal struct fields, package-level variables, and unexported helper functions are directly accessible across different files within that package without any syntax changes or wrapper indirections.
3. **Inference**: A dense monolithic file can be partitioned into multiple files within the same package without altering a single exported signature, receiver type, or interface contract.

### 2.2 Preservation of Zero-Allocation & Compiler Invariants
1. **Observation**: `mach` hot paths achieve 0 B/op and 0 allocs/op by leveraging `foundation` primitives:
   - `github.com/lemon4ksan/foundation/silicon/bytesconv`: `B2S`, `S2B`, `EqualFoldASCII`, `CopyZeroAlloc`, `ByteBufferPool`
   - `github.com/lemon4ksan/foundation/silicon/simd`: `MatchCRLF`, `MatchCRLFCRLF`, `IndexCRLFCRLF`
   - `github.com/lemon4ksan/foundation/silicon/ringbuf`: `SPSCRingBuffer[coreh2.FrameHeader]`
   - `github.com/lemon4ksan/foundation/silicon/pool`: `NewPerPStorage`
   - `github.com/lemon4ksan/foundation/silicon/offheap`: `Arena`, `SlabAllocator`, `AllocStruct` for POD frames
   - `github.com/lemon4ksan/foundation/sync/spinlock`: `SpinLock`
   - `github.com/lemon4ksan/foundation/borrow`: `Scope`, `Bytes`
   - `golang.org/x/sys/cpu`: `cpu.CacheLinePad`
2. **Logic Step**: The Go compiler builds all files of a package as a single compilation unit. Function inlining, bounds-check elimination (BCE), and escape analysis are evaluated at the package level, not per-file.
3. **Inference**: Splitting monolithic files into single-responsibility files inside their current packages introduces zero runtime penalty, zero interface boxing, zero heap allocations, and zero cacheline regressions.

### 2.3 Eliminating Redundancy from Defunct Scripts
1. **Observation**: The `.tmp/` directory contains `split_header.go`, `split_http.go`, and `split_http2.go`, which performed the previous AST split that created `header_request.go` and `header_response.go`.
2. **Logic Step**: These scripts were scratch tools and are not part of the build or test targets.
3. **Inference**: A clean, domain-driven decomposition by responsibility (e.g. wire parsing, body management, forms, cookies, trailers) supersedes this naive AST split and allows purging the `.tmp/` artifacts.

---

## 3. Public API Surface & Interface Contract Preservation Map

The following table comprehensively maps all exported contracts across the codebase that MUST remain 100% stable during refactoring:

### 3.1 Root Package (`github.com/lemon4ksan/mach`)
- `Engine` interface:
  - `Dial(ctx context.Context, addr string) (Conn, error)`
- `Conn` interface (`io.Closer`):
  - `OpenStream(ctx context.Context) (Stream, error)`
- `Stream` interface (`io.Reader`, `io.Writer`, `io.Closer`):
  - `ReadFrame() (Frame, error)`
- `Frame` interface:
  - `Type() uint8`
  - `Payload() []byte`

### 3.2 Client Packages
- **`github.com/lemon4ksan/mach/client`**:
  - `PoolManager[T any]` struct
  - `NewPoolManager[T any]() *PoolManager[T]`
  - `(p *PoolManager[T]) Get(ctx context.Context, addr string) (T, error)`
  - `(p *PoolManager[T]) Put(addr string, c T)`
- **`github.com/lemon4ksan/mach/client/h1`**:
  - `ClientConn` struct
  - `NewClientConn(c net.Conn) *ClientConn`
  - `(cc *ClientConn) Do(ctx context.Context, req *http.Request, res *http.Response) error`
  - `(cc *ClientConn) Close() error`
- **`github.com/lemon4ksan/mach/client/h2`**:
  - `Conn` struct
  - `NewConn(c net.Conn, opts ConnOpts) *Conn`
  - `(c *Conn) SetOrderedHeaders(keys []string)`
  - `(c *Conn) CancelStream(ctx *Context)`
  - `(c *Conn) Close() error`
  - `(c *Conn) Handshake() error`
  - `(c *Conn) CanOpenStream() bool`
  - `(c *Conn) Closed() bool`
  - `(c *Conn) Write(r *Context) error`
  - `(c *Conn) Do(ctx context.Context, req *h1.Request, res *h1.Response) error`
  - `Dialer` struct: `Addr`, `TLSConfig`, `PingInterval`, `NetDial`, `RawDial`, `RawDialContext`
  - `(d *Dialer) Dial(opts ConnOpts) (*Conn, error)`
  - `(d *Dialer) DialContext(ctx context.Context, opts ConnOpts) (*Conn, error)`
  - `ConnOpts` struct
  - `ClientOpts` struct
  - `Context` struct & methods `State() streamState`, `SetState(s streamState)`
  - `DefaultPingInterval` const
  - `FrameWithHeaders` interface: `Headers() []byte`
  - Re-exports in `export.go`:
    - `HPACK`, `AcquireHPACK`, `ReleaseHPACK`
    - `FrameType`, `Frame`, `AcquireFrame`, `ReleaseFrame`
    - `HeaderField`, `AcquireHeaderField`, `ReleaseHeaderField`
    - `FrameHeaders`, `Headers`, `FrameHeader`, `AcquireFrameHeader`, `ReleaseFrameHeader`
    - `FrameSettings`, `Settings`, `ReadFrameFrom`
    - `FrameData`, `Data`, `FrameWindowUpdate`, `WindowUpdate`
- **`github.com/lemon4ksan/mach/client/h3`**:
  - `ClientConn` struct: `conn`, `Transport`, `UnderlyingCloser`
  - `NewClientConn(conn *quic.Conn, settings *coreh3.Settings) (*ClientConn, error)`
  - `(cc *ClientConn) IsClosed() bool`
  - `(cc *ClientConn) Close() error`
  - `(cc *ClientConn) Do(ctx context.Context, req *h1.Request, resp *h1.Response, headerOrder []string) (map[string][]string, error)`
  - `(cc *ClientConn) DoScoped(ctx context.Context, req *h1.Request, resp *h1.Response, headerOrder []string, s *borrow.Scope) (map[string][]string, error)`
  - Re-exports in `export.go`:
    - `Settings = coreh3.Settings`
    - `QPACKCodec = coreh3.QPACKCodec`
    - `NewQPACKCodec = coreh3.NewQPACKCodec`

### 3.3 Server Packages
- **`github.com/lemon4ksan/mach/server/h1`**:
  - `Request` struct: `Conn`, `Method`, `URI`, `Path`, `Query`, `Proto`, `Host`, `Headers`, `Body`, `RemoteAddr`, `TLS`, `HijackFn`, `EarlyHintsFn`
  - `(r *Request) WriteEarlyHints(h http.Header) error`
  - `(r *Request) Reset()`
  - `(r *Request) Hijack() (net.Conn, *bufio.ReadWriter, error)`
  - `(r *Request) ReadRequest(br *bufio.Reader, bw *bytesconv.ByteBuffer, maxBodySize int64) error`
  - `Response` struct: `StatusCode`, `Headers`, `Body`
  - `(r *Response) Reset()`
  - `(r *Response) WriteResponse(bw *bytesconv.ByteBuffer, reqMethod, reqProto string) error`
  - `ConnHandler` struct: `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, `MaxBodySize`, `Handler`
  - `(ch *ConnHandler) ServeConn(conn net.Conn) error`
  - `HandlerFunc` type: `func(req *Request, res *Response) error`
  - Standard error variables (`ErrMalformedRequestLine`, `ErrUnsupportedProtocol`, etc.)
- **`github.com/lemon4ksan/mach/server/h2`**:
  - `ServerConn` struct
  - `NewServerConn(netConn net.Conn, handler ServerHandlerFunc) *ServerConn`
  - `(sc *ServerConn) Release()`
  - `(sc *ServerConn) Serve() error`
  - `ServerHandlerFunc` type: `func(req *ServerRequest, res *ServerResponse) error`
  - `ServerRequest` struct: `StreamID`, `Method`, `Path`, `Scheme`, `Authority`, `Protocol`, `Headers`, `Body`, `RemoteAddr`, `Ctx`
  - `ServerResponse` struct: `StatusCode`, `Headers`, `Body`
- **`github.com/lemon4ksan/mach/server/h3`**:
  - `ServerConn` struct
  - `NewServerConn(quicConn *quic.Conn, handler ServerHandlerFunc) *ServerConn`
  - `(sc *ServerConn) Serve() error`
  - `ServerHandlerFunc` type: `func(req *ServerRequest, res *ServerResponse) error`
  - `ServerRequest` struct: `StreamID`, `Method`, `Path`, `Scheme`, `Authority`, `Headers`, `Body`, `RemoteAddr`, `Ctx`
  - `ServerResponse` struct: `StatusCode`, `Headers`, `Body`

### 3.4 Protocol Packages
- **`github.com/lemon4ksan/mach/proto/http`**:
  - `Request` struct & full method suite
  - `Response` struct & full method suite
  - `RequestHeader` & `ResponseHeader` structs & full method suites
  - `AcquireRequest() *Request`, `ReleaseRequest(req *Request)`
  - `AcquireResponse() *Response`, `ReleaseResponse(resp *Response)`
  - `SetBodySizePoolLimit(reqBodyLimit, respBodyLimit int)`
  - `SwapRequestBody(a, b *Request)`, `SwapResponseBody(a, b *Response)`
  - `WriteMultipartForm(w io.Writer, f *multipart.Form, boundary string) error`
  - `CopyZeroAllocWithLimit(w io.Writer, r io.Reader, maxBodySize int) (int64, error)`
  - Interfaces: `ReadCloserWithError`, `BodyWriterTo`, `StreamWriter`
  - All standard error variables (`ErrNoMultipartForm`, `ErrBodyTooLarge`, etc.)
- **`github.com/lemon4ksan/mach/proto/h2`**:
  - Types: `FrameType`, `FrameFlags`, `Frame` interface, `FrameHeader`
  - Concrete frame implementations: `Data`, `Headers`, `Priority`, `RstStream`, `Settings`, `PushPromise`, `Ping`, `GoAway`, `WindowUpdate`, `Continuation`
  - Allocation / Pooling: `AcquireFrameHeader`, `ReleaseFrameHeader`, `AcquireFrame`, `AcquireFrameInArena`, `ReleaseFrame`, `ConnectionFramePool`, `NewConnectionFramePool`
  - Wire functions: `ReadFrameFrom`, `ReadFrameFromWithSize`, `ReadPreface`, `WritePreface`, `PerformHandshake`, `SerializeResponseHeaders`
  - RFC 9113 §7 error codes (`ErrorCode`, `NoError` .. `HTTP11Required`) and sentinel errors
- **`github.com/lemon4ksan/mach/proto/h2/overlay`**:
  - `Frame`, `DataFrame`, `HeadersFrame`
- **`github.com/lemon4ksan/mach/proto/h3`**:
  - `QPACKCodec`, `NewQPACKCodec`, `NewQPACKCodecWithOptions`, `QPACKStreamError`
  - `Settings`, `DecodeSettings`, `ReadFrameHeader`
  - Constants: `FrameType*`, `StreamType*`, `ErrorCode*`
  - Sentinel errors: `ErrH3SettingsError`, `ErrQPACKDecompressFailed`, etc.
- **`github.com/lemon4ksan/mach/proto/compress`**:
  - Compression constants (`CompressNoCompression` .. `CompressHuffmanOnly`, Brotli and Zstd levels)
  - Full suite of functions: `WriteGzip`, `WriteGzipLevel`, `WriteGunzip`, `WriteGunzipLimit`, `AppendGzipBytes`, `AppendGunzipBytes`, `WriteDeflate`, `WriteDeflateLevel`, `WriteInflate`, `WriteInflateLimit`, `AppendDeflateBytes`, `AppendInflateBytes`, `WriteBrotli`, `WriteBrotliLevel`, `WriteUnbrotli`, `WriteUnbrotliLimit`, `AppendBrotliBytes`, `AppendBrotliBytesLevel`, `AppendUnbrotliBytes`, `WriteZstd`, `WriteZstdLevel`, `WriteUnzstd`, `WriteUnzstdLimit`, `AppendZstdBytes`, `AppendZstdBytesLevel`, `AppendUnzstdBytes`, `AcquireStacklessGzipWriter`, `ReleaseStacklessGzipWriter`, `AcquireStacklessDeflateWriter`, `ReleaseStacklessDeflateWriter`
- **`github.com/lemon4ksan/mach/fsm/h2`**:
  - `StateMachine`, `NewStateMachine`, `Feed`, `NextEvent`, event types
- **`github.com/lemon4ksan/mach/x/raptor`**:
  - `Datagram`, `Decoder`, `NewDecoder`, `ProcessAsync`, `Feed`, `Recovered`, `Encoder`, `NewEncoder`, `AddData`, `GenerateSymbols`, `Encoded`

---

## 4. Component Boundaries & Modular Decomposition Proposals

Below is the concrete blueprint for decomposing each oversized module into distinct, single-responsibility files (all targeting lines < 350-400 lines per file):

### 4.1 Decomposition of `client/h2/`
Decompose monolithic `client/h2/conn.go` (1,667 lines) into:

1. `client/h2/conn.go` (~180 lines):
   - Definition of `Conn` struct.
   - `NewConn(c net.Conn, opts ConnOpts) *Conn` constructor.
   - Core exported lifecycle methods: `Do`, `Write`, `Close`, `Closed`, `CanOpenStream`, `SetOrderedHeaders`, `recordRTT`.
2. `client/h2/stream_table.go` (~160 lines):
   - Stream multiplexing structures: `streamShard`, `streamTableSize`, `streamMaxProbes`, `streamNumShards`.
   - Table operations: `getStream`, `storeStream`, `deleteStream`, `broadcastErrorToAllStreams`, `purgeStreamsAfterID`.
3. `client/h2/flow_control.go` (~150 lines):
   - Flow control accounting: `waitForWindowUpdate`, `broadcastWindowUpdate`, `calculateChunkSize`.
   - Window updates: `handleWindowUpdate`, `updateServerWindow`, `updateStreamWindow`, `updateWindow`.
4. `client/h2/read_loop.go` (~180 lines):
   - Socket reader: `readLoop`, `readNext`.
   - Connection-level frame demuxing: `handleConnectionFrame`, `handleSettings`, `handlePing`, `handlePingAck`, `handleGoAway`.
   - Stream frame router: `readStream`, `resetStream`.
5. `client/h2/write_loop.go` (~170 lines):
   - Socket writer event loop: `writeLoop`, `selectWriteEvent`, `recoverWriteLoop`.
   - Outbound batching using `outRing` (SPSC ring buffer) and vectorized flush via `sysnet.WriteVectorBuffers`.
   - Control writes: `writePing`, `sendSettingsAck`.
6. `client/h2/request_writer.go` (~160 lines):
   - Request transmission: `writeRequest`, `writeData`.
   - 100-Continue handling: `waitExpectContinue`, `isExpectContinue`.
   - Stream finalization: `finish`, `CancelStream`.
7. `client/h2/headers.go` (~190 lines):
   - HPACK request encoding: `encodeRequestHeaders`, `appendOrderedHeaders`.
   - Response & trailer decoding: `readHeader`, `readTrailers`.
   - Header validators & filters: `isForbiddenH2Header`, `isForbiddenH2HeaderStr`, `getFastHTTPCookieHeader`, `peekHeaderCaseInsensitive`.
8. `client/h2/push.go` (~120 lines):
   - HTTP/2 Server Push Promise handling: `handlePushPromise`, `decodePushHeaders`, `awaitPushedResponse`.
9. `client/h2/dialer.go` (~120 lines):
   - Connection factory: `Dialer` struct, `Dial`, `DialContext`, `tryDial`.

### 4.2 Decomposition of `proto/http/`
Reorganize the sprawling files (`header_request.go`, `header_response.go`, `request.go`, `response.go`, `header.go`, `http.go`) into cohesive, domain-driven files:

1. **Header Domain**:
   - `proto/http/header.go` (~200 lines): Base `header` struct, `RequestHeader`, `ResponseHeader` struct declarations, common accessors (`Len`, `Reset`, `CopyTo`).
   - `proto/http/header_parse.go` (~280 lines): RFC 9112 first-line parsing (`parseFirstLine`, `parseHeaders`, `parse`, `tryRead`, `readLoop`, `validate`), SIMD-accelerated header scanners.
   - `proto/http/header_fields.go` (~300 lines): Common header operations: `Peek`, `PeekBytes`, `Set`, `SetBytes*`, `Add`, `AddBytes*`, `Del`, `DelBytes`, `All`, `AllInOrder`.
   - `proto/http/header_cookies.go` (~220 lines): Cookie parsing, formatting, deletion, iteration (`SetCookie`, `DelCookie`, `DelAllCookies`, `Cookie`, `collectCookies`).
   - `proto/http/header_trailers.go` (~180 lines): Trailer management: `SetTrailer`, `AddTrailer`, `writeTrailer`, `TrailerHeader`, RFC forbidden trailer validation.
   - `proto/http/header_scoped.go` (~140 lines): Foundation zero-alloc scoped methods: `PeekScoped`, `CookieScoped`, `TrailerScoped`, `PeekAllScoped`.

2. **Request Domain**:
   - `proto/http/request.go` (~180 lines): `Request` struct, `Reset`, `CopyTo`, `CopyToSkipBody`, URI methods (`SetHost`, `Host`, `SetRequestURI`, `RequestURI`, `URI`, `SetURI`, `ParseURI`).
   - `proto/http/request_body.go` (~220 lines): In-memory body methods: `Body`, `AppendBody`, `AppendBodyString`, `SetBody`, `SetBodyString`, `ResetBody`, `SetBodyRaw`, `ReleaseBody`, `SwapBody`, `BodyBuffer`, `BodyScoped`, `ReadBodyScoped`.
   - `proto/http/request_stream.go` (~160 lines): Streaming request body: `SetBodyStream`, `IsBodyStream`, `SetBodyStreamWriter`, `BodyStream`, `CloseBodyStream`, `BodyWriter`, `RequestBodyStream`, `writeBodyStream`.
   - `proto/http/request_wire.go` (~240 lines): Wire I/O: `Read`, `ReadLimitBody`, `ReadBody`, `MayContinue`, `ContinueReadBody`, `Write`, `WriteTo`, `WriteVectored`.
   - `proto/http/request_forms.go` (~160 lines): Form and multipart support: `PostArgs`, `parsePostArgs`, `MultipartForm`, `MultipartFormWithLimit`, `RemoveMultipartFormFiles`.

3. **Response Domain**:
   - `proto/http/response.go` (~180 lines): `Response` struct, `Reset`, `CopyTo`, `CopyToSkipBody`, status code and network info: `StatusCode`, `SetStatusCode`, `RemoteAddr`, `LocalAddr`.
   - `proto/http/response_body.go` (~220 lines): In-memory body methods: `Body`, `AppendBody`, `AppendBodyString`, `SetBody`, `SetBodyString`, `ResetBody`, `SetBodyRaw`, `ReleaseBody`, `SwapBody`, `BodyBuffer`, `BodyScoped`, `ReadBodyScoped`, `ReadStreamScoped`.
   - `proto/http/response_stream.go` (~160 lines): Streaming response body & file transmission: `SetBodyStream`, `IsBodyStream`, `SetBodyStreamWriter`, `BodyStream`, `CloseBodyStream`, `BodyWriter`, `SendFile`, `writeBodyStream`.
   - `proto/http/response_wire.go` (~240 lines): Wire I/O and compression on write: `Read`, `ReadLimitBody`, `ReadBody`, `mustSkipBody`, `Write`, `WriteTo`, `WriteGzip`, `WriteDeflate`, `WriteBrotli`, `WriteZstd`.

4. **Body & Transfer Coding Domain**:
   - `proto/http/body_chunked.go` (~200 lines): Chunked transfer coding: `readBodyChunked`, `parseChunkSize`, `readCrLf`, `writeBodyChunked`, `writeChunk`, `chunkedBodyWriter`, `ErrBrokenChunk`.
   - `proto/http/body_identity.go` (~180 lines): Identity and fixed-size reading: `readBody`, `readBodyIdentity`, `appendBodyFixedSize`, `writeBodyFixedSize`, `copyBodyStream`, `copyZeroAlloc`, `CopyZeroAllocWithLimit`.
   - `proto/http/body_compress.go` (~220 lines): Compressed stream adapters: `compressedBodyStream`, `newCompressedBodyStream`, `compressGzipBodyStream`, `compressDeflateBodyStream`, `gunzipData`, `unBrotliData`, `unzstdData`, `inflateData`.
   - `proto/http/multipart.go` (~150 lines): Multipart form encoders and decoders: `WriteMultipartForm`, `readMultipartForm`, `marshalMultipartForm`.

5. **Housekeeping**:
   - Delete obsolete scratch scripts in `d:\CodingProjects\mach\.tmp/`.

### 4.3 Decomposition of `server/h2/`
Decompose `server/h2/server_conn.go` (570 lines) into:
1. `server/h2/server_conn.go` (~160 lines): `ServerConn` struct, `NewServerConn`, `Release`, `Serve` main loop, per-P storage.
2. `server/h2/dispatch.go` (~150 lines): Frame type switch, `sendSettings`, `handleSettings`, `handlePing`, `handleData`, `handleContinuation`.
3. `server/h2/headers.go` (~150 lines): `handleHeaders`, `finishHeaderBlock`, pseudo-header validation (RFC 9113 §8.2 & §8.3), extended CONNECT check (RFC 8441 §4).
4. `server/h2/stream.go` (~120 lines): `serverStream` struct, `dispatchStream` (populates `ServerRequest`, invokes `ServerHandlerFunc`).
5. `server/h2/response.go` (~140 lines): `writeResponse`, HPACK response header serialization, DATA frame chunking.

### 4.4 Decomposition of `client/h3/`
Decompose `client/h3/conn.go` (560 lines) into:
1. `client/h3/conn.go` (~140 lines): `ClientConn` struct, `NewClientConn`, `Close`, `IsClosed`.
2. `client/h3/control.go` (~170 lines): `setupControlStream`, `readUnidirectionalStreams`, `handleUnidirectionalStream`, `handleControlStream`, `handleGoAway`.
3. `client/h3/request.go` (~150 lines): `Do`, `DoScoped`, `sendRequest`, `sendRequestTo`.
4. `client/h3/response.go` (~160 lines): `readResponse`, `readResponseScoped`, `readResponseFrom`.

### 4.5 Decomposition of `proto/h3/`
Decompose `proto/h3/qpack.go` (690 lines) into:
1. `proto/h3/qpack.go` (~150 lines): `QPACKCodec` struct, `NewQPACKCodec`, `NewQPACKCodecWithOptions`, error tracking (`recordError`, `Err`, `ErrChan`, `SetErrorHandler`, `QPACKStreamError`).
2. `proto/h3/qpack_client.go` (~220 lines): Client-side methods: `EncodeRequestHeaders`, `getOrderedHeaders`, `DecodeResponseHeaders`, `DecodeResponseTrailers`, `responseHeaderHandler`, `trailersHandler`.
3. `proto/h3/qpack_server.go` (~200 lines): Server-side methods: `DecodeRequestHeaders`, `EncodeResponseHeaders`, `requestHeaderHandler`.
4. `proto/h3/qpack_rules.go` (~100 lines): Prohibited header checkers: `isForbiddenH3Header`, `isForbiddenH3HeaderStr`.

### 4.6 Decomposition of `proto/compress/`
Decompose `proto/compress/compress.go` (536 lines) into:
1. `proto/compress/compress.go` (~100 lines): Compression levels (`CompressNoCompression`, etc.), common error limits, file type detection.
2. `proto/compress/gzip.go` (~220 lines): Gzip reader/writer pools, stackless gzip writers, `WriteGzip`, `WriteGunzip`, `AppendGzipBytes`, `AppendGunzipBytes`.
3. `proto/compress/flate.go` (~220 lines): Flate reader/writer pools, stackless deflate writers, `WriteDeflate`, `WriteInflate`, `AppendDeflateBytes`, `AppendInflateBytes`.

### 4.7 Decomposition of `proto/h2/`
Decompose `proto/h2/frames.go` (492 lines) into:
1. `proto/h2/frame_data.go` (~100 lines): `Data` frame struct, serialization, deserialization, padding trimming.
2. `proto/h2/frame_headers.go` (~120 lines): `Headers` frame struct, serialization, deserialization, priority field unpacking.
3. `proto/h2/frame_control.go` (~160 lines): `Ping`, `GoAway`, `RstStream`, `Priority` frames.
4. `proto/h2/frame_window.go` (~90 lines): `WindowUpdate` frame.
5. `proto/h2/frame_ext.go` (~110 lines): `Continuation`, `PushPromise` frames.

---

## 5. Milestone Decomposition for Implementation Track

The refactoring execution track should proceed across 5 sequentially verifiable milestones. Each milestone must compile cleanly, pass all package unit tests, race detector runs, and benchmarks prior to advancing to the next.

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│ Milestone 1: Core Protocol Frame & Codec Decomposition                          │
│ Packages: proto/h2, proto/h3, proto/compress                                    │
│ Targets: Split frames.go, qpack.go, compress.go; add docstrings & BSD headers.  │
└────────────────────────────────────────┬────────────────────────────────────────┘
                                         │
                                         ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│ Milestone 2: HTTP Message Model & Parser Modularization                         │
│ Packages: proto/http, proto/http/stackless                                      │
│ Targets: Replace giant AST-split files with domain-driven components;           │
│          purge .tmp/; enforce SIMD scanner zero-alloc invariants.               │
└────────────────────────────────────────┬────────────────────────────────────────┘
                                         │
                                         ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│ Milestone 3: Client Protocol Engines                                            │
│ Packages: client, client/h1, client/h2, client/h3                               │
│ Targets: Decompose client/h2/conn.go (1,667 lines) into 9 focused components;   │
│          modularize client/h3/conn.go into 4 files; preserve pool and exports.  │
└────────────────────────────────────────┬────────────────────────────────────────┘
                                         │
                                         ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│ Milestone 4: Server Protocol Engines                                            │
│ Packages: server/h1, server/h2, server/h3                                       │
│ Targets: Decompose server/h2/server_conn.go into 5 components; clean server/h1  │
│          and server/h3; verify pipeline and hijacking semantics.                │
└────────────────────────────────────────┬────────────────────────────────────────┘
                                         │
                                         ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│ Milestone 5: Global Formatting, Linting Invariants & Acceptance Gate            │
│ Targets: Format all files (gofumpt, golines, gci, wsl_v5); 100% BSD headers;    │
│          golangci-lint 0 errors; go test -race; fuzz_all.go; benchmark check.   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Detailed Milestone Breakdown:

#### Milestone 1: Core Protocol Frame & Codec Decomposition
- **Scope**:
  - `proto/h2/frames.go` -> `frame_data.go`, `frame_headers.go`, `frame_control.go`, `frame_window.go`, `frame_ext.go`
  - `proto/h3/qpack.go` -> `qpack.go`, `qpack_client.go`, `qpack_server.go`, `qpack_rules.go`
  - `proto/compress/compress.go` -> `compress.go`, `gzip.go`, `flate.go`
- **Quality Requirements**:
  - Add BSD license header on every created file.
  - Add RFC docstrings to all exported entities (`RFC 9113`, `RFC 9204`, `RFC 9114`).
- **Gate**: `go test -race ./proto/h2/... ./proto/h3/... ./proto/compress/...`

#### Milestone 2: HTTP Message Model & Parser Modularization
- **Scope**:
  - Dissolve `proto/http/header_request.go` & `header_response.go` & `header.go` into:
    - `header.go`, `header_parse.go`, `header_fields.go`, `header_cookies.go`, `header_trailers.go`, `header_scoped.go`
  - Dissolve `proto/http/request.go` & `response.go` into:
    - `request.go`, `request_body.go`, `request_stream.go`, `request_wire.go`, `request_forms.go`
    - `response.go`, `response_body.go`, `response_stream.go`, `response_wire.go`
  - Dissolve `proto/http/http.go` into:
    - `body_chunked.go`, `body_identity.go`, `body_compress.go`, `multipart.go`
  - Delete `.tmp/split_header.go`, `.tmp/split_http.go`, `.tmp/split_http2.go`.
- **Quality Requirements**:
  - Zero-allocation hot path verification on headerscanner and body chunks.
  - Add BSD headers and RFC 9112 / RFC 9110 docstrings.
- **Gate**: `go test -race ./proto/http/...`, `go test -bench=. -benchmem ./proto/http/...`

#### Milestone 3: Client Protocol Engines
- **Scope**:
  - Decompose `client/h2/conn.go` (1,667 lines) into:
    - `conn.go`, `stream_table.go`, `flow_control.go`, `read_loop.go`, `write_loop.go`, `request_writer.go`, `headers.go`, `push.go`, `dialer.go`
  - Decompose `client/h3/conn.go` (560 lines) into:
    - `conn.go`, `control.go`, `request.go`, `response.go`
  - Preserve all type re-exports in `client/h2/export.go` and `client/h3/export.go`.
- **Quality Requirements**:
  - Preserve `cpu.CacheLinePad` isolation and atomic synchronization.
  - SPSC ring buffer batching preserved without locks.
- **Gate**: `go test -race ./client/...`

#### Milestone 4: Server Protocol Engines
- **Scope**:
  - Decompose `server/h2/server_conn.go` (570 lines) into:
    - `server_conn.go`, `dispatch.go`, `headers.go`, `stream.go`, `response.go`
  - Clean `server/h1` and `server/h3` ensuring single-responsibility per file.
- **Quality Requirements**:
  - RFC 9113 §8.3 pseudo-header rules and RFC 8441 extended CONNECT validations cleanly separated in `server/h2/headers.go`.
  - Preserve Per-P buffer pooling (`foundation/silicon/pool`).
- **Gate**: `go test -race ./server/...`

#### Milestone 5: Global Formatting, Linting Invariants & Acceptance Gate
- **Scope**:
  - Apply standard BSD license header (`// Copyright (c) 2026 Lemon4ksan All rights reserved.`) to all Go files.
  - Provide docstrings with RFC citations for all exported types, methods, interfaces, and constants.
  - Format with `.golangci.yml` rules: `gofumpt` (with `group-params`), `golines` (max-len 120), `gci`, `wsl_v5`.
- **Quality Requirements**:
  - Zero linter errors under `golangci-lint run ./...`.
  - Zero race warnings: `go test -race -timeout 90s ./...`.
  - Zero fuzz panics: `go run ./scripts/fuzz_all.go -fuzztime=5s`.
  - Zero heap allocation regressions on hot path benchmarks: `go test -bench=. -benchmem ./...`.

---

## 6. Caveats

1. **Test Coverage Scope**:
   - `github.com/lemon4ksan/mach/fsm/h2` and `github.com/lemon4ksan/mach/x/raptor` currently have no unit test files in the repository. They are experimental packages and should not be modified or disrupted during refactoring.
2. **Platform Constraints**:
   - SIMD operations (`MatchCRLF`, `IndexCRLFCRLF`) and vector I/O (`sysnet.WriteVectorBuffers`) rely on underlying CPU instruction sets (AVX2/NEON/SSE4.2). Benchmarks should be validated on hardware supporting these instructions.
3. **Compiler Inlining Invariants**:
   - While moving methods to sibling files within the same package does not break inlining, keeping unexported helpers in the same package (rather than moving them into separate subpackages) is strictly required to avoid interface boxing and extra heap escapes.

---

## 7. Conclusion

1. The `mach` codebase exhibits substantial protocol logic maturity, zero-allocation silicon design, and RFC compliance, but suffers from monolithic file density—specifically `client/h2/conn.go` (1,667 lines), `proto/http` (over 6,100 lines across 6 mechanically generated files), `server/h2/server_conn.go` (570 lines), `client/h3/conn.go` (560 lines), `proto/h3/qpack.go` (690 lines), and `proto/compress/compress.go` (536 lines).
2. A intra-package, domain-driven modular decomposition preserves 100% of the public API surface, interface contracts, zero-allocation micro-benchmarks, and compiler inlining behaviors.
3. The proposed 5-milestone implementation plan provides a predictable, step-by-step refactoring strategy that maintains working tests and race-safety invariants at every commit.

---

## 8. Verification Method

To independently verify all findings and validate future implementation milestones:

1. **Compilation & Basic Test Suite**:
   ```bash
   go test ./...
   ```
2. **Race Safety & Concurrency Invariants**:
   ```bash
   go test -race -timeout 90s ./...
   ```
3. **Protocol Fuzz Harness**:
   ```bash
   go run ./scripts/fuzz_all.go -fuzztime=5s
   ```
4. **Zero-Allocation Hot Path Micro-Benchmarks**:
   ```bash
   go test -bench=. -benchmem ./proto/h2/... ./proto/h3/... ./proto/http/...
   ```
5. **Strict Linter & Formatting Check**:
   ```bash
   golangci-lint run ./...
   ```
