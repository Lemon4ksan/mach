# M1 Explorer 2 Handoff: QPACK & Compression Decomposition Blueprint

## 1. Observation

### 1.1 Investigated Files and Codebase Scope
Direct examination was performed on the following target files and consumers:
- Primary Refactoring Targets:
  - `proto/h3/qpack.go`: 690 lines, 15,861 bytes. Monolithic file containing QPACKCodec definition, client encoding/decoding, server decoding/encoding, error handling, and forbidden header filtering.
  - `proto/compress/compress.go`: 536 lines, 12,573 bytes. Monolithic file combining generic compression levels, shared reader/writer buffers, gzip pools, stackless gzip writers, deflate pools, stackless deflate writers, and limit-protected decompression functions.
  - `proto/compress/brotli.go`: 89 lines. Uses `&byteSliceReader` from `compress.go`.
  - `proto/compress/zstd.go`: 86 lines. Uses `&byteSliceReader` from `compress.go`.
- Downstream Internal Consumers:
  - `client/h3/export.go`: Re-exports `coreh3.QPACKCodec` and `coreh3.NewQPACKCodec`.
  - `client/h3/conn.go`: Instantiates `cc.qpack = coreh3.NewQPACKCodec()`; invokes `cc.qpack.EncodeRequestHeaders`, `cc.qpack.DecodeResponseHeaders`, `cc.qpack.DecodeResponseTrailers`.
  - `server/h3/server_conn.go`: Instantiates `sc.qpack = coreh3.NewQPACKCodec()`; invokes `sc.qpack.DecodeRequestHeaders`, `sc.qpack.EncodeResponseHeaders`.
  - `proto/http/http.go`: Invokes `machcompress.WriteGunzipLimit`, `machcompress.WriteInflateLimit`, `machcompress.WriteUnbrotliLimit`, `machcompress.WriteUnzstdLimit`, `machcompress.AcquireStacklessGzipWriter`, `machcompress.ReleaseStacklessGzipWriter`, `machcompress.AcquireStacklessDeflateWriter`, `machcompress.ReleaseStacklessDeflateWriter`.
  - `proto/http/response.go`: Invokes `machcompress.CompressDefaultCompression`, `machcompress.AppendGzipBytesLevel`, `machcompress.AppendDeflateBytesLevel`, `machcompress.AppendBrotliBytesLevel`, `machcompress.AppendZstdBytesLevel`.
- Downstream External Workspace Consumer (`aoni`):
  - `d:/CodingProjects/aoni/tests/stress/qpack_test.go`: Consumes `coreh3.NewQPACKCodecWithOptions(maxDynamicTableCapacity, maxBlockedStreams, onError)`, `codec.Encoder()`, `codec.Decoder()`, `codec.EncodeRequestHeaders`, `codec.EncodeResponseHeaders`, `codec.DecodeResponseHeaders`.
  - `d:/CodingProjects/aoni/config.go`, `fast/options.go`, `h3_engine.go`, `internal/transport/pool.go`: Consumes `coreh3.Settings`.

### 1.2 Baseline Test Execution Results
All test suites passed cleanly with `-race` enabled:
1. `proto/h3` and `proto/compress`:
   - Command: `go test -v -race ./proto/h3/... ./proto/compress/...`
   - Result: `ok github.com/lemon4ksan/mach/proto/h3 3.434s`, `ok github.com/lemon4ksan/mach/proto/compress 3.454s` (0 failures, 0 race warnings).
2. `client/h3` and `server/h3`:
   - Command: `go test -v -race ./client/h3/... ./server/h3/...`
   - Result: `ok github.com/lemon4ksan/mach/client/h3 2.341s`, `ok github.com/lemon4ksan/mach/server/h3 2.989s` (0 failures, 0 race warnings).
3. `aoni` stress test suite (`d:/CodingProjects/aoni/tests/stress`):
   - Command: `go test -v -run TestQPACK ./tests/stress/...`
   - Result: `ok github.com/lemon4ksan/aoni/tests/stress 3.255s` (all 20+ concurrent QPACK soak tests passed).

### 1.3 Baseline Micro-Benchmark Performance Numbers
- Command: `go test -bench . -benchmem -run NONE github.com/lemon4ksan/mach/proto/h3`
  - `BenchmarkQPACKEncodeRequestHeaders-12`: 198,658 ops, 6,141 ns/op, 2,208 B/op, 34 allocs/op
  - `BenchmarkQPACKDecodeResponseHeaders-12`: 600,542 ops, 2,151 ns/op, 709 B/op, 13 allocs/op
- Command: `go test -bench . -benchmem -run NONE github.com/lemon4ksan/mach/server/h3`
  - `BenchmarkQPACK_EncodeResponseHeaders-12`: 520,676 ops, 2,776 ns/op, 1,736 B/op, 29 allocs/op
  - `BenchmarkQPACK_DecodeRequestHeaders-12`: 695,518 ops, 2,153 ns/op, 712 B/op, 9 allocs/op
  - `BenchmarkH3_FrameHeaderPack-12`: 219,870,663 ops, 5.254 ns/op, 0 B/op, 0 allocs/op (Zero-Allocation Hot Path verified)

### 1.4 Verbatim Revive Violations Catalog
From `teamwork_preview_explorer_survey_standards/revive_violations.txt`:
- `proto/h3/qpack.go` (7 violations):
  1. Line 39: `exported: exported method QPACKStreamError.Is should have comment or be unexported (revive)`
  2. Line 141: `exported: exported method QPACKCodec.Decoder should have comment or be unexported (revive)`
  3. Line 145: `exported: exported method QPACKCodec.Encoder should have comment or be unexported (revive)`
  4. Line 407: `exported: exported method QPACKCodec.DecodeResponseHeaders should have comment or be unexported (revive)`
  5. Line 462: `exported: exported method QPACKCodec.DecodeResponseTrailers should have comment or be unexported (revive)`
  6. Line 598: `exported: exported method QPACKCodec.DecodeRequestHeaders should have comment or be unexported (revive)`
  7. Line 656: `exported: exported method QPACKCodec.EncodeResponseHeaders should have comment or be unexported (revive)`
- `proto/compress/compress.go` (6 violations):
  8. Line 76: `exported: exported function AcquireStacklessGzipWriter should have comment or be unexported (revive)`
  9. Line 93: `exported: exported function ReleaseStacklessGzipWriter should have comment or be unexported (revive)`
  10. Line 231: `exported: exported function WriteGunzipLimit should have comment or be unexported (revive)`
  11. Line 350: `exported: exported function WriteInflateLimit should have comment or be unexported (revive)`
  12. Line 414: `exported: exported function AcquireStacklessDeflateWriter should have comment or be unexported (revive)`
  13. Line 431: `exported: exported function ReleaseStacklessDeflateWriter should have comment or be unexported (revive)`
- `proto/compress/brotli.go` and `zstd.go` (3 violations):
  14. `proto/compress/brotli.go:66`: `exported: exported function WriteUnbrotliLimit should have comment or be unexported (revive)`
  15. `proto/compress/zstd.go:17`: `exported: exported const CompressZstdSpeedNotSet should have comment (or a comment on this block) or be unexported (revive)`
  16. `proto/compress/zstd.go:63`: `exported: exported function WriteUnzstdLimit should have comment or be unexported (revive)`

---

## 2. Logic Chain

### Step 1: QPACK Modular Decomposition Rationale
Observation 1.1 reveals that `proto/h3/qpack.go` mixes four distinct logical concerns:
1. Core QPACK codec lifecycle, error channels, and struct definitions (`QPACKCodec`, `QPACKStreamError`, `NewQPACKCodec`, `NewQPACKCodecWithOptions`, `Err`, `ErrChan`, `SetErrorHandler`, `Decoder`, `Encoder`).
2. Client-side framing operations (`EncodeRequestHeaders`, `getOrderedHeaders`, `DecodeResponseHeaders`, `DecodeResponseTrailers`, `responseHeaderHandler`, `trailersHandler`).
3. Server-side framing operations (`DecodeRequestHeaders`, `EncodeResponseHeaders`, `requestHeaderHandler`).
4. HTTP/3 forbidden header validation rules (`isForbiddenH3Header`, `isForbiddenH3HeaderStr`).

Because Go packages share package-private visibility across files within the same directory, decomposing `proto/h3/qpack.go` into:
- `proto/h3/qpack.go`
- `proto/h3/qpack_client.go`
- `proto/h3/qpack_server.go`
- `proto/h3/qpack_rules.go`
requires zero structural or visibility changes to private fields (`encMu`, `decMu`, `decoder`, `encoder`, `recordError`). All methods remain members of `*QPACKCodec`, preserving 100% ABI and API compatibility with external consumers.

### Step 2: Compression Subsystem Decomposition Rationale
Observation 1.1 reveals that `proto/compress/compress.go` merges the generic compression level constants, common reader/writer buffering, and two independent compression algorithm pools (gzip and flate/deflate).
Furthermore, `brotli.go` and `zstd.go` rely on `byteSliceReader` defined in `compress.go`.
Therefore:
1. `proto/compress/compress.go` retains common elements:
   - Compression level constants (`CompressNoCompression`, `CompressBestSpeed`, `CompressBestCompression`, `CompressDefaultCompression`, `CompressHuffmanOnly`).
   - Shared buffering structs: `byteSliceWriter`, `byteSliceReader`.
   - Pool map infrastructure: `newCompressWriterPoolMap()`, `normalizeCompressLevel(level int) int`.
   - Compression invocation context: `compressCtx`.
   - File compressibility check: `isFileCompressible(f fs.File, minCompressRatio float64) bool`.
2. `proto/compress/gzip.go` receives all Gzip-specific pools and functions:
   - Readers: `acquireGzipReader`, `releaseGzipReader`.
   - Real writers & pool: `realGzipWriterPoolMap`, `acquireRealGzipWriter`, `releaseRealGzipWriter`.
   - Stackless writers & pool: `stacklessGzipWriterPoolMap`, `AcquireStacklessGzipWriter`, `ReleaseStacklessGzipWriter`.
   - Asynchronous stackless runner: `stacklessWriteGzipOnce`, `stacklessWriteGzipFunc`, `stacklessWriteGzip`, `nonblockingWriteGzip`.
   - Public APIs: `AppendGzipBytes`, `AppendGzipBytesLevel`, `WriteGzip`, `WriteGzipLevel`, `AppendGunzipBytes`, `WriteGunzip`, `WriteGunzipLimit`.
3. `proto/compress/flate.go` receives all Deflate/Flate-specific pools and functions:
   - Readers & pool: `flateReaderPool`, `acquireFlateReader`, `releaseFlateReader`, `resetFlateReader`.
   - Real writers & pool: `realDeflateWriterPoolMap`, `acquireRealDeflateWriter`, `releaseRealDeflateWriter`.
   - Stackless writers & pool: `stacklessDeflateWriterPoolMap`, `AcquireStacklessDeflateWriter`, `ReleaseStacklessDeflateWriter`.
   - Asynchronous stackless runner: `stacklessWriteDeflateOnce`, `stacklessWriteDeflateFunc`, `stacklessWriteDeflate`, `nonblockingWriteDeflate`.
   - Public APIs: `AppendDeflateBytes`, `AppendDeflateBytesLevel`, `WriteDeflate`, `WriteDeflateLevel`, `AppendInflateBytes`, `WriteInflate`, `WriteInflateLimit`.

### Step 3: Zero-Allocation and Thread-Safety Invariant Preservation
1. `QPACKCodec` retains its dual-mutex architecture (`encMu` for encoding, `decMu` for decoding). Neither mutex interacts with the other, ensuring zero lock contention between concurrent request writes and response reads.
2. `stackless.Writer` and `sync.Pool` allocations in `gzip.go` and `flate.go` use identical pool indexing `normalizeCompressLevel(level) -> [0..11]`, preserving pool reuse without cacheline churn.
3. In-memory writers (`*byteSliceWriter`, `*bytes.Buffer`, `*bytesconv.ByteBuffer`) bypass stackless goroutine handoff and run directly via `compressCtx`, eliminating heap allocations on hot in-memory compression paths.

### Step 4: Downstream Consumer Verification
Observations 1.1 and 1.2 demonstrate that:
- `client/h3/conn.go` calls `cc.qpack.EncodeRequestHeaders`, `cc.qpack.DecodeResponseHeaders`, `cc.qpack.DecodeResponseTrailers`.
- `server/h3/server_conn.go` calls `sc.qpack.DecodeRequestHeaders`, `sc.qpack.EncodeResponseHeaders`.
- `aoni/tests/stress/qpack_test.go` calls `coreh3.NewQPACKCodecWithOptions`, `codec.Encoder()`, `codec.Decoder()`.
- `proto/http/http.go` calls `machcompress.WriteGunzipLimit`, `machcompress.WriteInflateLimit`, `machcompress.AcquireStacklessGzipWriter`, `machcompress.ReleaseStacklessGzipWriter`, etc.
Because all method receivers (`*QPACKCodec`), method signatures, and exported function names remain identical within their original packages (`h3` and `compress`), downstream consumers experience 100% source and binary compatibility.

---

## 3. Detailed File Blueprint & Entity Distribution

### 3.1 `proto/h3/` Decomposition

#### File 1: `proto/h3/qpack.go` (~165 lines)
- **Header**: Standard BSD license header (`// Copyright (c) 2026 Lemon4ksan All rights reserved.`)
- **Package**: `package h3`
- **Imports**:
  ```go
  import (
  	"fmt"
  	"sync"

  	"github.com/lemon4ksan/foundation/generic"
  	"github.com/lemon4ksan/foundation/net/qpack"
  )
  ```
- **Entities**:
  - `type QPACKStreamError struct`
  - `func (e *QPACKStreamError) Error() string`
  - `func (e *QPACKStreamError) Is(target error) bool`
  - `type QPACKCodec struct`
  - `func (q *QPACKCodec) recordError(err error)`
  - `func (q *QPACKCodec) Err() error`
  - `func (q *QPACKCodec) ErrChan() <-chan error`
  - `func (q *QPACKCodec) SetErrorHandler(fn func(err error))`
  - `func NewQPACKCodec() *QPACKCodec`
  - `func NewQPACKCodecWithOptions(maxDynamicTableCapacity, maxBlockedStreams uint64, onError func(error)) *QPACKCodec`
  - `func (q *QPACKCodec) Decoder() *qpack.Decoder`
  - `func (q *QPACKCodec) Encoder() *qpack.Encoder`

#### File 2: `proto/h3/qpack_client.go` (~280 lines)
- **Header**: Standard BSD license header
- **Package**: `package h3`
- **Imports**:
  ```go
  import (
  	"fmt"
  	"io"
  	"strconv"

  	"github.com/lemon4ksan/foundation/net/qpack"
  	"github.com/lemon4ksan/foundation/silicon/bytesconv"

  	"github.com/lemon4ksan/mach/proto/http"
  )
  ```
- **Entities**:
  - `func (q *QPACKCodec) EncodeRequestHeaders(streamID uint64, w io.Writer, req *http.Request, orderedKeys []string) (retErr error)`
  - `func (q *QPACKCodec) getOrderedHeaders(req *http.Request, orderedKeys []string) []qpack.HeaderField`
  - `type responseHeaderHandler struct`
  - `func (h *responseHeaderHandler) OnHeaderDecoded(name, value string)`
  - `func (h *responseHeaderHandler) OnDecodingCompleted()`
  - `func (h *responseHeaderHandler) OnDecodingErrorDetected(errorCode uint64, errorMessage string)`
  - `func (q *QPACKCodec) DecodeResponseHeaders(streamID uint64, headerBlock []byte, res *http.ResponseHeader) (statusCode int, retErr error)`
  - `type trailersHandler struct`
  - `func (h *trailersHandler) OnHeaderDecoded(name, value string)`
  - `func (h *trailersHandler) OnDecodingCompleted()`
  - `func (h *trailersHandler) OnDecodingErrorDetected(errorCode uint64, errorMessage string)`
  - `func (q *QPACKCodec) DecodeResponseTrailers(streamID uint64, headerBlock []byte) (trailers map[string][]string, retErr error)`

#### File 3: `proto/h3/qpack_server.go` (~195 lines)
- **Header**: Standard BSD license header
- **Package**: `package h3`
- **Imports**:
  ```go
  import (
  	"fmt"
  	"strconv"
  	"strings"

  	"github.com/lemon4ksan/foundation/net/headkit"
  	"github.com/lemon4ksan/foundation/net/qpack"
  	"github.com/lemon4ksan/foundation/silicon/bytesconv"
  )
  ```
- **Entities**:
  - `type requestHeaderHandler struct`
  - `func (h *requestHeaderHandler) OnHeaderDecoded(k, v string)`
  - `func (h *requestHeaderHandler) OnDecodingCompleted()`
  - `func (h *requestHeaderHandler) OnDecodingErrorDetected(errorCode uint64, errorMessage string)`
  - `func (q *QPACKCodec) DecodeRequestHeaders(streamID uint64, headerBlock []byte, reqHeaders *headkit.Headers) (method, path, scheme, authority string, retErr error)`
  - `func (q *QPACKCodec) EncodeResponseHeaders(streamID uint64, statusCode int, headers headkit.Headers, bodyLen int) (block []byte)`

#### File 4: `proto/h3/qpack_rules.go` (~60 lines)
- **Header**: Standard BSD license header
- **Package**: `package h3`
- **Imports**:
  ```go
  import (
  	"github.com/lemon4ksan/foundation/silicon/bytesconv"
  )
  ```
- **Entities**:
  - `func isForbiddenH3Header(key, val []byte) bool`
  - `func isForbiddenH3HeaderStr(key string, val []byte) bool`

---

### 3.2 `proto/compress/` Decomposition

#### File 1: `proto/compress/compress.go` (~135 lines)
- **Header**: Standard BSD license header
- **Package**: `package compress`
- **Imports**:
  ```go
  import (
  	"io"
  	"io/fs"
  	"sync"

  	"github.com/lemon4ksan/foundation/codec/compress/flate"
  	"github.com/lemon4ksan/foundation/silicon/bytesconv"
  )
  ```
- **Entities**:
  - `const ( CompressNoCompression = ... )` (with RFC 1951 docstring)
  - `type byteSliceWriter struct`
  - `func (w *byteSliceWriter) Write(p []byte) (int, error)`
  - `func (w *byteSliceWriter) WriteString(s string) (int, error)`
  - `type byteSliceReader struct`
  - `func (r *byteSliceReader) Read(p []byte) (int, error)`
  - `func (r *byteSliceReader) ReadByte() (byte, error)`
  - `type compressCtx struct`
  - `func newCompressWriterPoolMap() []*sync.Pool`
  - `func normalizeCompressLevel(level int) int`
  - `func isFileCompressible(f fs.File, minCompressRatio float64) bool`

#### File 2: `proto/compress/gzip.go` (~230 lines)
- **Header**: Standard BSD license header
- **Package**: `package compress`
- **Imports**:
  ```go
  import (
  	"bytes"
  	"fmt"
  	"io"
  	"sync"

  	"github.com/lemon4ksan/foundation/codec/compress"
  	"github.com/lemon4ksan/foundation/codec/compress/gzip"
  	"github.com/lemon4ksan/foundation/silicon/bytesconv"

  	"github.com/lemon4ksan/mach/proto/http/stackless"
  )
  ```
- **Entities**:
  - Readers: `acquireGzipReader(r io.Reader)`, `releaseGzipReader(zr *gzip.Reader)`
  - Real writer pool & methods: `realGzipWriterPoolMap`, `acquireRealGzipWriter(w io.Writer, level int)`, `releaseRealGzipWriter(zw *gzip.Writer, level int)`
  - Stackless writer pool & methods: `stacklessGzipWriterPoolMap`, `AcquireStacklessGzipWriter(w io.Writer, level int)`, `ReleaseStacklessGzipWriter(sw stackless.Writer, level int)`
  - Stackless execution: `stacklessWriteGzipOnce`, `stacklessWriteGzipFunc`, `stacklessWriteGzip(ctx any)`, `nonblockingWriteGzip(ctxv any)`
  - Public functions:
    - `AppendGzipBytesLevel(dst, src []byte, level int) []byte`
    - `WriteGzipLevel(w io.Writer, p []byte, level int) (int, error)`
    - `WriteGzip(w io.Writer, p []byte) (int, error)`
    - `AppendGzipBytes(dst, src []byte) []byte`
    - `WriteGunzip(w io.Writer, p []byte) (int, error)`
    - `WriteGunzipLimit(w io.Writer, p []byte, maxBodySize int) (int, error)`
    - `AppendGunzipBytes(dst, src []byte) ([]byte, error)`

#### File 3: `proto/compress/flate.go` (~210 lines)
- **Header**: Standard BSD license header
- **Package**: `package compress`
- **Imports**:
  ```go
  import (
  	"bytes"
  	"compress/zlib"
  	"fmt"
  	"io"
  	"sync"

  	"github.com/lemon4ksan/foundation/codec/compress"
  	"github.com/lemon4ksan/foundation/silicon/bytesconv"

  	"github.com/lemon4ksan/mach/proto/http/stackless"
  )
  ```
- **Entities**:
  - Readers: `flateReaderPool`, `acquireFlateReader(r io.Reader)`, `releaseFlateReader(zr io.ReadCloser)`, `resetFlateReader(zr io.ReadCloser, r io.Reader)`
  - Real writer pool & methods: `realDeflateWriterPoolMap`, `acquireRealDeflateWriter(w io.Writer, level int)`, `releaseRealDeflateWriter(zw *zlib.Writer, level int)`
  - Stackless writer pool & methods: `stacklessDeflateWriterPoolMap`, `AcquireStacklessDeflateWriter(w io.Writer, level int)`, `ReleaseStacklessDeflateWriter(sw stackless.Writer, level int)`
  - Stackless execution: `stacklessWriteDeflateOnce`, `stacklessWriteDeflateFunc`, `stacklessWriteDeflate(ctx any)`, `nonblockingWriteDeflate(ctxv any)`
  - Public functions:
    - `AppendDeflateBytesLevel(dst, src []byte, level int) []byte`
    - `WriteDeflateLevel(w io.Writer, p []byte, level int) (int, error)`
    - `WriteDeflate(w io.Writer, p []byte) (int, error)`
    - `AppendDeflateBytes(dst, src []byte) []byte`
    - `WriteInflate(w io.Writer, p []byte) (int, error)`
    - `WriteInflateLimit(w io.Writer, p []byte, maxBodySize int) (int, error)`
    - `AppendInflateBytes(dst, src []byte) ([]byte, error)`

---

## 4. Draft RFC Docstrings (16 Entities)

### 4.1 `proto/h3/` Docstrings (7 entities)

```go
// Is reports whether the receiver matches target under errors.Is semantics (RFC 9204 §6).
//
// Specifically, it matches ErrQPACKDecompressFailed when e.Code equals
// ErrCodeQpackDecompressionFailed (RFC 9114 §4.2, RFC 9204 §8.3).
// Concurrency: Safe for concurrent invocation across goroutines.
func (e *QPACKStreamError) Is(target error) bool

// Decoder returns the underlying QPACK progressive decoder instance (RFC 9204 §3).
//
// Callers may use the decoder to inspect dynamic table state or create progressive decoders.
// Concurrency: State inspection is thread-safe; progressive decoders created from this instance
// must be synchronized or managed per-stream.
func (q *QPACKCodec) Decoder() *qpack.Decoder

// Encoder returns the underlying QPACK encoder instance (RFC 9204 §4).
//
// Callers may use the encoder to inspect header table capacity or encode raw header lists.
// Concurrency: Concurrent encoding operations must be synchronized via the codec's encoder mutex.
func (q *QPACKCodec) Encoder() *qpack.Encoder

// DecodeResponseHeaders decodes a QPACK-encoded response header block into res (RFC 9204 §4.5, RFC 9114 §4.3).
//
// It decodes the mandatory :status pseudo-header and populates response header fields,
// returning the parsed HTTP status code. If :status is missing or malformed (such as status 101,
// prohibited in HTTP/3 by RFC 9114 §4.3.1), ErrMissingStatusHeader or ErrMalformedHeader is returned.
// Concurrency: Thread-safe; protected by the internal decoder mutex (decMu).
func (q *QPACKCodec) DecodeResponseHeaders(
	streamID uint64,
	headerBlock []byte,
	res *http.ResponseHeader,
) (statusCode int, retErr error)

// DecodeResponseTrailers decodes a QPACK-encoded trailing field section (RFC 9204 §4.5, RFC 9114 §4.3.4).
//
// Trailing pseudo-headers (beginning with ':') are discarded per RFC 9114 §4.3.4.
// Concurrency: Thread-safe; protected by the internal decoder mutex (decMu).
func (q *QPACKCodec) DecodeResponseTrailers(
	streamID uint64,
	headerBlock []byte,
) (trailers map[string][]string, retErr error)

// DecodeRequestHeaders decodes a QPACK-encoded request header block from a client stream (RFC 9204 §4.5, RFC 9114 §4.1.2).
//
// It validates mandatory pseudo-headers (:method, :scheme, :authority, :path) according to HTTP/3
// semantics, handles extended CONNECT tunnels (RFC 9114 §4.4), and populates reqHeaders with regular
// headers while discarding or rejecting prohibited hop-by-hop headers.
// Concurrency: Thread-safe; protected by the internal decoder mutex (decMu).
func (q *QPACKCodec) DecodeRequestHeaders(
	streamID uint64,
	headerBlock []byte,
	reqHeaders *headkit.Headers,
) (method, path, scheme, authority string, retErr error)

// EncodeResponseHeaders encodes server response headers into a QPACK-encoded field section (RFC 9204 §4.5, RFC 9114 §4.3).
//
// It serializes the :status pseudo-header, optional content-length header if bodyLen >= 0,
// and all supplied response headers, filtering out prohibited HTTP/3 connection-specific headers (RFC 9114 §4.1).
// Concurrency: Thread-safe; protected by the internal encoder mutex (encMu).
func (q *QPACKCodec) EncodeResponseHeaders(
	streamID uint64,
	statusCode int,
	headers headkit.Headers,
	bodyLen int,
) (block []byte)
```

### 4.2 `proto/compress/` Docstrings (9 entities)

```go
// AcquireStacklessGzipWriter acquires an asynchronous stackless Gzip writer targeting w at the given compression level (RFC 1952).
//
// It shields calling goroutines from large stack allocations by offloading compression work
// to a worker pool managed by stackless.Writer. Callers must release the returned writer
// using ReleaseStacklessGzipWriter when finished.
// Concurrency: Acquired writer is single-goroutine; pool acquisition is thread-safe.
func AcquireStacklessGzipWriter(w io.Writer, level int) stackless.Writer

// ReleaseStacklessGzipWriter closes sw and returns it to the corresponding compression level pool (RFC 1952).
//
// Callers must not use sw after calling ReleaseStacklessGzipWriter.
// Concurrency: Thread-safe.
func ReleaseStacklessGzipWriter(sw stackless.Writer, level int)

// WriteGunzipLimit decompresses gzipped payload p and writes up to maxBodySize uncompressed bytes to w (RFC 1952).
//
// If maxBodySize is 0 or negative, uncompressed size is unlimited. If decompression produces
// more than maxBodySize bytes, decompression halts and an error is returned to prevent decompression bombs.
// Concurrency: Thread-safe; utilizes pooled gzip readers.
func WriteGunzipLimit(w io.Writer, p []byte, maxBodySize int) (int, error)

// WriteInflateLimit decompresses zlib/deflate payload p and writes up to maxBodySize uncompressed bytes to w (RFC 1950, RFC 1951).
//
// If maxBodySize is 0 or negative, uncompressed size is unlimited. If decompression produces
// more than maxBodySize bytes, decompression halts and an error is returned to prevent decompression bombs.
// Concurrency: Thread-safe; utilizes pooled zlib readers.
func WriteInflateLimit(w io.Writer, p []byte, maxBodySize int) (int, error)

// AcquireStacklessDeflateWriter acquires an asynchronous stackless Deflate writer targeting w at the given compression level (RFC 1951).
//
// It offloads deflate compression work to a dedicated worker pool, avoiding stack expansion on the caller goroutine.
// Callers must return the writer to the pool with ReleaseStacklessDeflateWriter when completed.
// Concurrency: Acquired writer is single-goroutine; pool acquisition is thread-safe.
func AcquireStacklessDeflateWriter(w io.Writer, level int) stackless.Writer

// ReleaseStacklessDeflateWriter closes sw and returns it to the corresponding deflate pool (RFC 1951).
//
// Callers must not use sw after calling ReleaseStacklessDeflateWriter.
// Concurrency: Thread-safe.
func ReleaseStacklessDeflateWriter(sw stackless.Writer, level int)

// WriteUnbrotliLimit decompresses Brotli payload p and writes up to maxBodySize uncompressed bytes to w (RFC 7932).
//
// If maxBodySize is 0 or negative, uncompressed size is unlimited. If decompression produces
// more than maxBodySize bytes, decompression halts and an error is returned to prevent decompression bombs.
// Concurrency: Thread-safe; utilizes pooled brotli readers.
func WriteUnbrotliLimit(w io.Writer, p []byte, maxBodySize int) (int, error)

// Supported Zstandard compression speed levels (RFC 8878).
const (
	// CompressZstdSpeedNotSet indicates default compression speed.
	CompressZstdSpeedNotSet = iota
	// CompressZstdBestSpeed indicates fastest compression speed.
	CompressZstdBestSpeed
	// CompressZstdDefault indicates standard balanced compression speed.
	CompressZstdDefault
	// CompressZstdSpeedBetter indicates improved compression ratio over default.
	CompressZstdSpeedBetter
	// CompressZstdBestCompression indicates maximum compression ratio.
	CompressZstdBestCompression
)

// WriteUnzstdLimit decompresses Zstandard payload p and writes up to maxBodySize uncompressed bytes to w (RFC 8878).
//
// If maxBodySize is 0 or negative, uncompressed size is unlimited. If decompression produces
// more than maxBodySize bytes, decompression halts and an error is returned to prevent decompression bombs.
// Concurrency: Thread-safe; utilizes pooled zstd decoders.
func WriteUnzstdLimit(w io.Writer, p []byte, maxBodySize int) (int, error)
```

---

## 5. Caveats
- No caveats. The decomposition strictly preserves all package boundaries, struct definitions, method signatures, and concurrency invariants. Zero public identifiers are added, renamed, or removed.

---

## 6. Conclusion
1. **Decomposition Strategy**:
   - `proto/h3/qpack.go` (690 lines) splits cleanly into 4 single-responsibility files: `qpack.go` (core codec), `qpack_client.go` (client framing), `qpack_server.go` (server framing), and `qpack_rules.go` (RFC 9114 validation).
   - `proto/compress/compress.go` (536 lines) splits cleanly into 3 focused files: `compress.go` (common levels, buffers, pool infrastructure), `gzip.go` (RFC 1952 gzip pool & stackless writer), and `flate.go` (RFC 1951 deflate pool & stackless writer).
2. **API & Downstream Stability**:
   - 100% public API stability is guaranteed across `client/h3`, `server/h3`, `proto/http`, and the downstream `aoni` workspace repository.
3. **Docstring & Standard Compliance**:
   - All 16 documented revive violations across `proto/h3/qpack.go` (7), `proto/compress/compress.go` (6), and `proto/compress/brotli.go` + `zstd.go` (3) are addressed with full RFC citations (RFC 9204, RFC 9114, RFC 1952, RFC 1951, RFC 7932, RFC 8878) and explicit concurrency contracts.

---

## 7. Verification Method
The Worker agent implementing this decomposition can independently verify full correctness using the following sequential test and benchmark commands:

1. **Unit & Race Test Suite**:
   ```bash
   go test -v -race ./proto/h3/... ./proto/compress/...
   go test -v -race ./client/h3/... ./server/h3/...
   ```
   *Expected*: PASS across all packages, 0 race warnings, 0 failures.

2. **Downstream Workspace Test Suite**:
   ```bash
   go test -v -run TestQPACK ./tests/stress/... # in d:/CodingProjects/aoni
   ```
   *Expected*: PASS across all 20+ concurrent stress and soak tests.

3. **Performance & Zero-Allocation Hot Path Benchmark Verification**:
   ```bash
   go test -bench . -benchmem -run NONE github.com/lemon4ksan/mach/proto/h3
   go test -bench . -benchmem -run NONE github.com/lemon4ksan/mach/server/h3
   ```
   *Expected*:
   - `BenchmarkH3_FrameHeaderPack-12`: `0 B/op`, `0 allocs/op` (zero allocation invariant strictly preserved).
   - `BenchmarkQPACKEncodeRequestHeaders` and `BenchmarkQPACKDecodeResponseHeaders` match or exceed baseline throughput.

4. **Linter & Docstring Verification**:
   ```bash
   golangci-lint run proto/h3/... proto/compress/...
   ```
   *Expected*: 0 revive violations, clean formatting under `gofumpt`, `golines`, `gci`, and `wsl_v5`.
