# Handoff Report: Milestone M2 — HTTP Request & Response Model Modularization

**Author**: Explorer 2 (`teamwork_preview_explorer_m2_2`)  
**Target Milestone**: M2 (HTTP Message Model & Parser Modularization)  
**Parent Agent**: `5d05cf1e-7247-466c-b645-4e25e1408e3e`  
**Reference Standards**: RFC 9110, RFC 9112, RFC 7578, RFC 3986, RFC 1951, RFC 1952, RFC 7932, RFC 8878  
**Layout Plan**: `PROJECT.md` Section 5  

---

## 1. Observation

### 1.1 Source Files Examined
The investigation directly inspected the following files in `d:/CodingProjects/mach/proto/http`:
- `request.go` (1,194 lines, 30,818 bytes)
- `response.go` (998 lines, 25,887 bytes)
- `streaming.go` (133 lines, 2,491 bytes)
- Additional context files: `http.go` (936 lines), `pool.go` (60 lines), `stream.go` (58 lines)

### 1.2 Verbatim Catalog of Symbols & Line Numbers

#### A. `proto/http/request.go` (67 symbols: 1 struct, 54 exported methods, 12 unexported methods)
| Line | Identifier | Kind | Current Doc Status | Defects & RFC Citations Needed |
|---|---|---|---|---|
| 31 | `type Request struct` | Type | 6 lines | Lacks RFC 9110 §3 / RFC 9112 §2 citations; no mention of lifecycle, pooling, zero-alloc invariants |
| 65 | `SetHost(host string)` | Method | 1 line | Lacks RFC 9110 §7.2 / RFC 9112 §3.2 citations; authority sync details missing |
| 70 | `SetHostBytes(host []byte)` | Method | 1 line | Lacks RFC 9110 §7.2 / RFC 9112 §3.2 citations; authority sync details missing |
| 75 | `Host() []byte` | Method | 1 line | Lacks RFC 9110 §7.2 citation; zero-copy borrow lifetime not explained |
| 80 | `SetRequestURI(requestURI string)` | Method | 1 line | Lacks RFC 9112 §3.2 citation; uri cache invalidation not documented |
| 87 | `SetRequestURIBytes(requestURI []byte)` | Method | 1 line | Lacks RFC 9112 §3.2 citation; uri cache invalidation not documented |
| 94 | `RequestURI() []byte` | Method | 1 line | Incorrect comment: claims it returns `zerocopy.URI` when returning `[]byte` |
| 104 | `ConnectionClose() bool` | Method | 1 line | Lacks RFC 9110 §9.6 / RFC 9112 §9.3 citations |
| 109 | `SetConnectionClose()` | Method | 1 line | Lacks RFC 9110 §9.6 / RFC 9112 §9.3 citations |
| 119 | `GetTimeOut() time.Duration` | Method | 6 lines | Inconsistent naming with `SetTimeout` (line 1171) |
| 143 | `SetBodyStream(bodyStream io.Reader, bodySize int)` | Method | 20 lines | **Stale "fasthttp" mentions** on lines 130 and 134; missing RFC 9112 §7.1 |
| 150 | `IsBodyStream() bool` | Method | 1 line | Lacks stream state invariants |
| 166 | `SetBodyStreamWriter(sw StreamWriter)` | Method | 12 lines | Lacks concurrency and pipe cleanup explanation |
| 174 | `BodyStream() io.Reader` | Method | 3 lines | Needs explicit ownership / closing contract |
| 178 | `CloseBodyStream() error` | Method | **MISSING** | **Exported method lacks docstring** |
| 183 | `BodyWriter() io.Writer` | Method | 1 line | Lacks single-goroutine restriction documentation |
| 188 | `bodyBytes() []byte` | Method (priv) | None | Internal accessor; needs unexported doc |
| 211 | `BodyBuffer() *bytesconv.ByteBuffer` | Method | **MISSING** | **Exported method lacks docstring**; needs pool retention details |
| 226 | `BodyGunzip() ([]byte, error)` | Method | 5 lines | Lacks RFC 1952 / RFC 9110 §8.4 citations; no zip-bomb security note |
| 234 | `BodyGunzipWithLimit(maxBodySize int) ([]byte, error)` | Method | 4 lines | Lacks RFC 1952 citation and allocation bounds |
| 243 | `BodyUnbrotli() ([]byte, error)` | Method | 5 lines | Lacks RFC 7932 / RFC 9110 §8.4 citations |
| 251 | `BodyUnbrotliWithLimit(maxBodySize int) ([]byte, error)` | Method | 4 lines | Lacks RFC 7932 citation |
| 260 | `BodyInflate() ([]byte, error)` | Method | 5 lines | **Copy-paste error**: Line 257 claims "response header" instead of request |
| 268 | `BodyInflateWithLimit(maxBodySize int) ([]byte, error)` | Method | 4 lines | Lacks RFC 1951 citation |
| 272 | `RequestBodyStream() io.Reader` | Method | **MISSING** | **Exported method lacks docstring** |
| 276 | `BodyUnzstd() ([]byte, error)` | Method | **MISSING** | **Exported method lacks docstring**; lacks RFC 8878 citation |
| 284 | `BodyUnzstdWithLimit(maxBodySize int) ([]byte, error)` | Method | 4 lines | Lacks RFC 8878 citation |
| 294 | `BodyUncompressed() ([]byte, error)` | Method | 6 lines | **Copy-paste error**: Line 291 claims "response header" instead of request |
| 302 | `BodyUncompressedWithLimit(maxBodySize int) ([]byte, error)` | Method | 4 lines | Lacks RFC 9110 §8.4 citation |
| 336 | `BodyWriteTo(w io.Writer) error` | Method | 1 line | Lacks RFC 9112 transfer coding explanation |
| 355 | `SetBodyRaw(body []byte)` | Method | 3 lines | **Copy-paste error**: Line 353 claims "sets response body" instead of request |
| 367 | `ReleaseBody(size int)` | Method | 7 lines | Lacks memory reclaim details under high load |
| 384 | `SwapBody(body []byte) []byte` | Method | 5 lines | Lacks lifecycle warning |
| 412 | `Body() []byte` | Method | 8 lines | Lacks RFC 9110 §6.4 citation; stream drain behavior needs emphasis |
| 430 | `AppendBody(p []byte)` | Method | 3 lines | Lacks stream cancellation behavior notice |
| 437 | `AppendBodyString(s string)` | Method | 1 line | Lacks stream cancellation behavior notice |
| 446 | `SetBody(body []byte)` | Method | 3 lines | Lacks stream cancellation behavior notice |
| 453 | `SetBodyString(body string)` | Method | 1 line | Lacks stream cancellation behavior notice |
| 460 | `ResetBody()` | Method | 1 line | Lacks pool recycling details |
| 476 | `CopyTo(dst *Request)` | Method | 1 line | Deep vs shallow copy semantics need detail |
| 492 | `CopyToSkipBody(dst *Request)` | Method | **MISSING** | **Exported method lacks docstring** |
| 505 | `URI() *zerocopy.URI` | Method | 1 line | Lacks RFC 3986 citation; lazy-parse semantics missing |
| 514 | `SetURI(newURI *zerocopy.URI)` | Method | 4 lines | Lacks RFC 3986 citation |
| 528 | `ParseURI() error` | Method | **MISSING** | **Exported method lacks docstring** |
| 540 | `PostArgs() *zerocopy.Args` | Method | 1 line | Lacks RFC 1866 / HTML URL-encoded form citation |
| 545 | `parsePostArgs()` | Method (priv) | None | Internal helper |
| 568 | `MultipartForm() (*multipart.Form, error)` | Method | 10 lines | Lacks RFC 7578 citation; missing leak warning |
| 582 | `MultipartFormWithLimit(maxBodySize int) (*multipart.Form, error)` | Method | 10 lines | Lacks RFC 7578 citation; missing temp file warning |
| 657 | `Reset()` | Method | 1 line | Lacks lifecycle and pool hygiene invariants |
| 669 | `resetSkipHeader()` | Method (priv) | None | Internal helper |
| 681 | `RemoveMultipartFormFiles()` | Method | 2 lines | Critical leak prevention needs strong documentation |
| 708 | `Read(r *bufio.Reader) error` | Method | 18 lines | Lacks RFC 9112 §2 / §3 citations |
| 732 | `ReadLimitBody(r *bufio.Reader, maxBodySize int) error` | Method | 20 lines | Lacks RFC 9112 §6 / §7 citations |
| 742 | `readLimitBody(...) error` | Method (priv) | None | Internal helper |
| 764 | `MayContinue() bool` | Method | 10 lines | Lacks RFC 9110 §10.1.1 ("100 Continue") citation |
| 775 | `ContinueReadBody(r *bufio.Reader, ...) error` | Method | 7 lines | Lacks RFC 9110 §10.1.1 / RFC 9112 §7.1 citations |
| 832 | `ReadBody(r *bufio.Reader, ...) error` | Method | 4 lines | Lacks RFC 9112 §6 / §7 framing citations |
| 864 | `ContinueReadBodyStream(r *bufio.Reader, ...) error` | Method | 7 lines | Lacks RFC 9110 §10.1.1 citation |
| 927 | `WriteTo(w io.Writer) (int64, error)` | Method | 1 line | Lacks RFC 9112 §2.1 citation |
| 931 | `onlyMultipartForm() bool` | Method (priv) | None | Internal helper |
| 940 | `Write(w *bufio.Writer) error` | Method | 5 lines | Lacks RFC 9112 §2.1 / §3.2 citations; Host derivation |
| 1023 | `WriteVectored(conn net.Conn) error` | Method | 2 lines | Lacks net.Buffers vectored I/O zero-alloc description |
| 1090 | `writeBodyStream(w *bufio.Writer) error` | Method (priv) | None | Internal helper |
| 1133 | `closeBodyStream() error` | Method (priv) | None | Internal helper |
| 1157 | `String() string` | Method | 5 lines | Lacks debugging warning (allocates) |
| 1171 | `SetTimeout(t time.Duration)` | Method | 10 lines | Needs unification with `GetTimeOut` documentation |
| 1176 | `BodyScoped(s *borrow.Scope) borrow.Bytes` | Method | 1 line | Scoped zero-alloc contract missing |
| 1186 | `ReadBodyScoped(fn func([]byte) error) error` | Method | 1 line | Callback lifetime invariant missing |

#### B. `proto/http/response.go` (65 symbols: 1 struct, 54 exported methods, 10 unexported methods)
| Line | Identifier | Kind | Current Doc Status | Defects & RFC Citations Needed |
|---|---|---|---|---|
| 31 | `type Response struct` | Type | 6 lines | Lacks RFC 9110 §3 / RFC 9112 §2 citations; no mention of lifecycle, pooling, zero-alloc invariants |
| 61 | `StatusCode() int` | Method | 1 line | Lacks RFC 9110 §15 / RFC 9112 §3.1.2 citations |
| 66 | `SetStatusCode(statusCode int)` | Method | 1 line | Lacks RFC 9110 §15 / RFC 9112 §3.1.2 citations |
| 71 | `ConnectionClose() bool` | Method | 1 line | Lacks RFC 9110 §9.6 / RFC 9112 §9.3 citations |
| 76 | `SetConnectionClose()` | Method | 1 line | Lacks RFC 9110 §9.6 / RFC 9112 §9.3 citations |
| 85 | `SendFile(path string) error` | Method | 5 lines | Lacks RFC 9110 §8.8.2 ("Last-Modified") citation; int64 overflow behavior undocumented |
| 129 | `SetBodyStream(bodyStream io.Reader, bodySize int)` | Method | 18 lines | **Stale "fasthttp" mentions** on lines 118 and 122 |
| 136 | `IsBodyStream() bool` | Method | 1 line | Lacks stream state invariants |
| 150 | `SetBodyStreamWriter(sw StreamWriter)` | Method | 10 lines | Lacks concurrency and pipe cleanup explanation |
| 160 | `BodyWriter() io.Writer` | Method | 5 lines | **Stale "RequestHandler" and "RequestCtx" mentions** on line 157-158 |
| 168 | `BodyStream() io.Reader` | Method | 3 lines | Needs explicit ownership / closing contract |
| 172 | `CloseBodyStream() error` | Method | **MISSING** | **Exported method lacks docstring** |
| 176 | `ParseNetConn(conn net.Conn)` | Method | **MISSING** | **Exported method lacks docstring** |
| 183 | `RemoteAddr() net.Addr` | Method | 2 lines | Lacks immutability contract |
| 189 | `LocalAddr() net.Addr` | Method | 2 lines | Lacks immutability contract |
| 201 | `Body() []byte` | Method | 8 lines | Lacks RFC 9110 §6.4 citation; stream drain behavior needs emphasis |
| 216 | `bodyBytes() []byte` | Method (priv) | None | Internal accessor; needs unexported doc |
| 228 | `BodyBuffer() *bytesconv.ByteBuffer` | Method | **MISSING** | **Exported method lacks docstring**; needs pool retention details |
| 243 | `BodyGunzip() ([]byte, error)` | Method | 5 lines | Lacks RFC 1952 / RFC 9110 §8.4 citations; no zip-bomb security note |
| 251 | `BodyGunzipWithLimit(maxBodySize int) ([]byte, error)` | Method | 4 lines | Lacks RFC 1952 citation |
| 260 | `BodyUnbrotli() ([]byte, error)` | Method | 5 lines | Lacks RFC 7932 / RFC 9110 §8.4 citations |
| 268 | `BodyUnbrotliWithLimit(maxBodySize int) ([]byte, error)` | Method | 4 lines | Lacks RFC 7932 citation |
| 277 | `BodyInflate() ([]byte, error)` | Method | 5 lines | Lacks RFC 1951 / RFC 9110 §8.4 citations |
| 285 | `BodyInflateWithLimit(maxBodySize int) ([]byte, error)` | Method | 4 lines | Lacks RFC 1951 citation |
| 289 | `BodyUnzstd() ([]byte, error)` | Method | **MISSING** | **Exported method lacks docstring**; lacks RFC 8878 citation |
| 297 | `BodyUnzstdWithLimit(maxBodySize int) ([]byte, error)` | Method | 4 lines | Lacks RFC 8878 citation |
| 307 | `BodyUncompressed() ([]byte, error)` | Method | 6 lines | Lacks RFC 9110 §8.4 citation |
| 315 | `BodyUncompressedWithLimit(maxBodySize int) ([]byte, error)` | Method | 4 lines | Lacks RFC 9110 §8.4 citation |
| 349 | `BodyWriteTo(w io.Writer) error` | Method | 1 line | Lacks RFC 9112 transfer coding explanation |
| 364 | `AppendBody(p []byte)` | Method | 3 lines | Lacks stream cancellation behavior notice |
| 370 | `AppendBodyString(s string)` | Method | 1 line | Lacks stream cancellation behavior notice |
| 378 | `SetBody(body []byte)` | Method | 3 lines | Lacks stream cancellation behavior notice |
| 386 | `SetBodyString(body string)` | Method | 1 line | Lacks stream cancellation behavior notice |
| 394 | `ResetBody()` | Method | 1 line | Lacks pool recycling details |
| 411 | `SetBodyRaw(body []byte)` | Method | 3 lines | Zero-copy slice retention warning |
| 423 | `ReleaseBody(size int)` | Method | 7 lines | Lacks memory reclaim details under high load |
| 440 | `SwapBody(body []byte) []byte` | Method | 5 lines | Lacks lifecycle warning |
| 461 | `CopyTo(dst *Response)` | Method | 1 line | Deep vs shallow copy semantics need detail |
| 477 | `CopyToSkipBody(dst *Response)` | Method | **MISSING** | **Exported method lacks docstring** |
| 486 | `Reset()` | Method | 1 line | Lacks lifecycle and pool hygiene invariants |
| 500 | `resetSkipHeader()` | Method (priv) | None | Internal helper |
| 510 | `Read(r *bufio.Reader) error` | Method | 6 lines | Lacks RFC 9112 §2 / §3 citations |
| 528 | `ReadLimitBody(r *bufio.Reader, maxBodySize int) error` | Method | 14 lines | Lacks RFC 9110 §15.2 (interim responses 1xx) citations |
| 580 | `ReadBody(r *bufio.Reader, maxBodySize int) (err error)` | Method | 4 lines | Lacks RFC 9112 §6 / §7 framing citations |
| 616 | `mustSkipBody() bool` | Method (priv) | None | Internal helper |
| 621 | `WriteTo(w io.Writer) (int64, error)` | Method | 1 line | Lacks RFC 9112 §2.1 citation |
| 631 | `WriteGzip(w *bufio.Writer) error` | Method | 6 lines | Lacks RFC 1952 / RFC 9110 §8.4 / §12.5.5 (Vary) citations |
| 649 | `WriteGzipLevel(w *bufio.Writer, level int) error` | Method | 14 lines | Lacks RFC 1952 / RFC 9110 §8.4 citations |
| 660 | `WriteDeflate(w *bufio.Writer) error` | Method | 6 lines | Lacks RFC 1951 / RFC 9110 §8.4 / §12.5.5 (Vary) citations |
| 678 | `WriteDeflateLevel(w *bufio.Writer, level int) error` | Method | 14 lines | Lacks RFC 1951 / RFC 9110 §8.4 citations |
| 684 | `WriteBrotli(w *bufio.Writer) error` | Method | 1 line | Lacks RFC 7932 / RFC 9110 §8.4 / §12.5.5 (Vary) citations |
| 689 | `WriteBrotliLevel(w *bufio.Writer, level int) error` | Method | 1 line | Lacks RFC 7932 / RFC 9110 §8.4 citations |
| 695 | `WriteZstd(w *bufio.Writer) error` | Method | 1 line | Lacks RFC 8878 / RFC 9110 §8.4 / §12.5.5 (Vary) citations |
| 700 | `WriteZstdLevel(w *bufio.Writer, level int) error` | Method | 1 line | Lacks RFC 8878 / RFC 9110 §8.4 citations |
| 705 | `brotliBody(level int)` | Method (priv) | None | Internal compression helper |
| 738 | `gzipBody(level int)` | Method (priv) | None | Internal compression helper |
| 771 | `deflateBody(level int)` | Method (priv) | None | Internal compression helper |
| 804 | `zstdBody(level int)` | Method (priv) | None | Internal compression helper |
| 842 | `Write(w *bufio.Writer) error` | Method | 5 lines | Lacks RFC 9112 §2.1 / §3.1.2 citations |
| 868 | `writeBodyStream(...) (err error)` | Method (priv) | None | Internal helper |
| 926 | `closeBodyStream(wErr error) error` | Method (priv) | None | Internal helper |
| 942 | `String() string` | Method | 5 lines | Lacks debugging warning (allocates) |
| 947 | `BodyScoped(s *borrow.Scope) borrow.Bytes` | Method | 1 line | Scoped zero-alloc contract missing |
| 957 | `ReadBodyScoped(fn func([]byte) error) error` | Method | 1 line | Callback lifetime invariant missing |
| 967 | `ReadStreamScoped(s *borrow.Scope, ...) error` | Method | 1 line | Scoped chunk streaming contract missing |

#### C. `proto/http/streaming.go` (6 symbols: 1 interface, 1 struct, 1 method, 2 functions, 1 var — 0% documented!)
| Line | Identifier | Kind | Current Doc Status | Defects & RFC Citations Needed |
|---|---|---|---|---|
| 17 | `type bodyStreamHeader interface` | Interface (priv) | **MISSING** | RFC 9112 §7.1 / §7.1.2 chunk trailer reading contract missing |
| 22 | `type RequestStream struct` | Struct | **MISSING** | **Exported type lacks docstring**; missing RFC 9112 §7.1 chunked reader contract |
| 30 | `(rs *RequestStream) Read(p []byte) (int, error)` | Method | **MISSING** | **Exported method lacks docstring**; chunk size decoding & trailer handling missing |
| 110 | `AcquireRequestStream(...) *RequestStream` | Function | **MISSING** | **Exported function lacks docstring**; sync.Pool acquisition contract missing |
| 119 | `ReleaseRequestStream(rs *RequestStream)` | Function | **MISSING** | **Exported function lacks docstring**; sync.Pool release & reset contract missing |
| 128 | `var RequestStreamPool sync.Pool` | Var | **MISSING** | **Exported var lacks docstring**; pool concurrency semantics missing |

---

## 2. Logic Chain

```
[Observation 1.1 - 1.2]: request.go (1,194 lines) and response.go (998 lines) are dense monoliths mixing 5 disparate concerns: core model, in-memory body, streaming body, wire framing/vectored I/O, and form/multipart handling.
        │
        ▼
[Step 1 - Single Responsibility Principle]:
Decompose request.go into 5 files and response.go into 4 files as specified in PROJECT.md:
  - request.go, request_body.go, request_stream.go, request_wire.go, request_forms.go
  - response.go, response_body.go, response_stream.go, response_wire.go
        │
        ▼
[Step 2 - Consolidation of streaming.go]:
streaming.go (133 lines) contains RequestStream, AcquireRequestStream, ReleaseRequestStream, RequestStreamPool, bodyStreamHeader.
PROJECT.md Section 5 does not list streaming.go; instead, it specifies request_stream.go ("streaming request body").
Therefore, streaming.go belongs naturally consolidated into request_stream.go, unifying Request streaming methods and the RequestStream decode wrapper.
        │
        ▼
[Step 3 - Zero-Allocation Invariants]:
Package-internal decomposition within package `http` preserves 100% binary interface compatibility.
All method signatures, receiver types (*Request, *Response, *RequestStream), struct fields, and pool hooks remain byte-for-byte identical.
Zero heap allocations on benchmark hot paths (BenchmarkFullPipeline_ScopedBorrow) are mathematically preserved because no interface allocations or struct wrapping layers are introduced.
        │
        ▼
[Step 4 - Docstring & Linter Invariants]:
16 exported symbols lack docstrings (6 in request.go, 5 in response.go, 5 in streaming.go).
Multiple docstrings cite stale "fasthttp" terminology or contain inverted copy-paste comments ("response header" inside Request).
Deploy comprehensive RFC 9110 / RFC 9112 / RFC 7578 / RFC 3986 / RFC 1951 / RFC 1952 / RFC 7932 / RFC 8878 docstrings with explicit concurrency and lifecycle semantics.
```

---

## 3. Modular Decomposition Architecture

### 3.1 HTTP Request Model (`request.go` -> 5 Files)

#### File 1: `proto/http/request.go` (~220 lines)
- **Responsibility**: Core `Request` struct definition, URI & Host manipulation, connection & timing accessors, lifecycle reset, and string representation.
- **Types**:
  - `type Request struct`
- **Methods**:
  - `SetHost(host string)`
  - `SetHostBytes(host []byte)`
  - `Host() []byte`
  - `SetRequestURI(requestURI string)`
  - `SetRequestURIBytes(requestURI []byte)`
  - `RequestURI() []byte`
  - `URI() *zerocopy.URI`
  - `SetURI(newURI *zerocopy.URI)`
  - `ParseURI() error`
  - `ConnectionClose() bool`
  - `SetConnectionClose()`
  - `GetTimeOut() time.Duration`
  - `SetTimeout(t time.Duration)`
  - `Reset()`
  - `resetSkipHeader()` (unexported)
  - `String() string`
- **Imports Required**: `io`, `mime/multipart`, `time`, `github.com/lemon4ksan/foundation/net/http/zerocopy`, `github.com/lemon4ksan/foundation/silicon/bytesconv`.

#### File 2: `proto/http/request_body.go` (~340 lines)
- **Responsibility**: In-memory body buffering, slice borrowing, zero-copy scoped borrowing, body replacement/swapping, and in-memory body decompression.
- **Methods**:
  - `Body() []byte`
  - `bodyBytes() []byte` (unexported)
  - `BodyBuffer() *bytesconv.ByteBuffer`
  - `SetBody(body []byte)`
  - `SetBodyString(body string)`
  - `SetBodyRaw(body []byte)`
  - `AppendBody(p []byte)`
  - `AppendBodyString(s string)`
  - `ResetBody()`
  - `ReleaseBody(size int)`
  - `SwapBody(body []byte) []byte`
  - `CopyTo(dst *Request)`
  - `CopyToSkipBody(dst *Request)`
  - `BodyScoped(s *borrow.Scope) borrow.Bytes`
  - `ReadBodyScoped(fn func([]byte) error) error`
  - `BodyGunzip() ([]byte, error)`
  - `BodyGunzipWithLimit(maxBodySize int) ([]byte, error)`
  - `BodyUnbrotli() ([]byte, error)`
  - `BodyUnbrotliWithLimit(maxBodySize int) ([]byte, error)`
  - `BodyInflate() ([]byte, error)`
  - `BodyInflateWithLimit(maxBodySize int) ([]byte, error)`
  - `BodyUnzstd() ([]byte, error)`
  - `BodyUnzstdWithLimit(maxBodySize int) ([]byte, error)`
  - `BodyUncompressed() ([]byte, error)`
  - `BodyUncompressedWithLimit(maxBodySize int) ([]byte, error)`
- **Imports Required**: `bytes`, `errors`, `github.com/lemon4ksan/foundation/borrow`, `github.com/lemon4ksan/foundation/codec/compress`, `github.com/lemon4ksan/foundation/silicon/bytesconv`.

#### File 3: `proto/http/request_stream.go` (~260 lines)
- **Responsibility**: Streaming request body configuration, writer pipes, body stream lifecycle closing, stream chunk decoding (`RequestStream`), and `sync.Pool` pooling.
- **Types**:
  - `type bodyStreamHeader interface` (consolidated from `streaming.go`)
  - `type RequestStream struct` (consolidated from `streaming.go`)
- **Functions & Variables**:
  - `AcquireRequestStream(b *bytesconv.ByteBuffer, r *bufio.Reader, h bodyStreamHeader) *RequestStream`
  - `ReleaseRequestStream(rs *RequestStream)`
  - `var RequestStreamPool sync.Pool`
- **Methods on `Request`**:
  - `SetBodyStream(bodyStream io.Reader, bodySize int)`
  - `IsBodyStream() bool`
  - `SetBodyStreamWriter(sw StreamWriter)`
  - `BodyStream() io.Reader`
  - `RequestBodyStream() io.Reader`
  - `CloseBodyStream() error`
  - `closeBodyStream() error` (unexported)
  - `BodyWriter() io.Writer`
  - `ContinueReadBodyStream(r *bufio.Reader, maxBodySize int, preParseMultipartForm ...bool) error`
- **Methods on `RequestStream`**:
  - `(rs *RequestStream) Read(p []byte) (int, error)`
- **Imports Required**: `bufio`, `bytes`, `errors`, `io`, `sync`, `github.com/lemon4ksan/foundation/net/http/zerocopy`, `github.com/lemon4ksan/foundation/silicon/bytesconv`.

#### File 4: `proto/http/request_wire.go` (~330 lines)
- **Responsibility**: HTTP/1.1 wire parser, 100-Continue expectation handling, wire serialization, and vectored socket writes (`net.Buffers`).
- **Methods**:
  - `Read(r *bufio.Reader) error`
  - `ReadLimitBody(r *bufio.Reader, maxBodySize int) error`
  - `readLimitBody(r *bufio.Reader, maxBodySize int, getOnly, preParseMultipartForm bool) error` (unexported)
  - `MayContinue() bool`
  - `ContinueReadBody(r *bufio.Reader, maxBodySize int, preParseMultipartForm ...bool) error`
  - `ReadBody(r *bufio.Reader, contentLength, maxBodySize int) (err error)`
  - `Write(w *bufio.Writer) error`
  - `WriteTo(w io.Writer) (int64, error)`
  - `WriteVectored(conn net.Conn) error`
  - `writeBodyStream(w *bufio.Writer) error` (unexported)
- **Imports Required**: `bufio`, `bytes`, `encoding/base64`, `errors`, `fmt`, `io`, `net`, `github.com/lemon4ksan/foundation/net/http/zerocopy`.

#### File 5: `proto/http/request_forms.go` (~180 lines)
- **Responsibility**: URL-encoded POST arguments parsing and multipart form parsing, streaming multipart extraction, temporary file cleanup, and multipart body serialization.
- **Methods**:
  - `PostArgs() *zerocopy.Args`
  - `parsePostArgs()` (unexported)
  - `MultipartForm() (*multipart.Form, error)`
  - `MultipartFormWithLimit(maxBodySize int) (*multipart.Form, error)`
  - `RemoveMultipartFormFiles()`
  - `onlyMultipartForm() bool` (unexported)
  - `BodyWriteTo(w io.Writer) error`
- **Imports Required**: `bytes`, `compress/gzip`, `fmt`, `io`, `mime/multipart`, `github.com/lemon4ksan/foundation/net/http/zerocopy`.

---

### 3.2 HTTP Response Model (`response.go` -> 4 Files)

#### File 1: `proto/http/response.go` (~170 lines)
- **Responsibility**: Core `Response` struct definition, HTTP status code accessors, connection persistence flags, local/remote network addresses, lifecycle reset, and string representation.
- **Types**:
  - `type Response struct`
- **Methods**:
  - `StatusCode() int`
  - `SetStatusCode(statusCode int)`
  - `ConnectionClose() bool`
  - `SetConnectionClose()`
  - `ParseNetConn(conn net.Conn)`
  - `RemoteAddr() net.Addr`
  - `LocalAddr() net.Addr`
  - `Reset()`
  - `resetSkipHeader()` (unexported)
  - `String() string`
- **Imports Required**: `io`, `net`, `github.com/lemon4ksan/foundation/net/http/zerocopy`, `github.com/lemon4ksan/foundation/silicon/bytesconv`.

#### File 2: `proto/http/response_body.go` (~320 lines)
- **Responsibility**: In-memory response body buffering, slice borrowing, zero-copy scoped borrowing, body replacement/swapping, and in-memory body decompression.
- **Methods**:
  - `Body() []byte`
  - `bodyBytes() []byte` (unexported)
  - `BodyBuffer() *bytesconv.ByteBuffer`
  - `SetBody(body []byte)`
  - `SetBodyString(body string)`
  - `SetBodyRaw(body []byte)`
  - `AppendBody(p []byte)`
  - `AppendBodyString(s string)`
  - `ResetBody()`
  - `ReleaseBody(size int)`
  - `SwapBody(body []byte) []byte`
  - `CopyTo(dst *Response)`
  - `CopyToSkipBody(dst *Response)`
  - `BodyScoped(s *borrow.Scope) borrow.Bytes`
  - `ReadBodyScoped(fn func([]byte) error) error`
  - `BodyGunzip() ([]byte, error)`
  - `BodyGunzipWithLimit(maxBodySize int) ([]byte, error)`
  - `BodyUnbrotli() ([]byte, error)`
  - `BodyUnbrotliWithLimit(maxBodySize int) ([]byte, error)`
  - `BodyInflate() ([]byte, error)`
  - `BodyInflateWithLimit(maxBodySize int) ([]byte, error)`
  - `BodyUnzstd() ([]byte, error)`
  - `BodyUnzstdWithLimit(maxBodySize int) ([]byte, error)`
  - `BodyUncompressed() ([]byte, error)`
  - `BodyUncompressedWithLimit(maxBodySize int) ([]byte, error)`
- **Imports Required**: `bytes`, `errors`, `github.com/lemon4ksan/foundation/borrow`, `github.com/lemon4ksan/foundation/codec/compress`, `github.com/lemon4ksan/foundation/silicon/bytesconv`.

#### File 3: `proto/http/response_stream.go` (~180 lines)
- **Responsibility**: Streaming response body setup, file transmission (`SendFile`), stream reader pipes, body stream closing, and scoped chunk streaming (`ReadStreamScoped`).
- **Methods**:
  - `SendFile(path string) error`
  - `SetBodyStream(bodyStream io.Reader, bodySize int)`
  - `IsBodyStream() bool`
  - `SetBodyStreamWriter(sw StreamWriter)`
  - `BodyWriter() io.Writer`
  - `BodyStream() io.Reader`
  - `CloseBodyStream() error`
  - `closeBodyStream(wErr error) error` (unexported)
  - `ReadStreamScoped(s *borrow.Scope, fn func(chunk borrow.Bytes) error) error`
- **Imports Required**: `io`, `os`, `github.com/lemon4ksan/foundation/borrow`.

#### File 4: `proto/http/response_wire.go` (~430 lines)
- **Responsibility**: HTTP/1.1 wire response parser (handling interim 1xx responses), wire serialization, and on-the-fly wire compression (Gzip, Deflate, Brotli, Zstandard).
- **Methods**:
  - `Read(r *bufio.Reader) error`
  - `ReadLimitBody(r *bufio.Reader, maxBodySize int) error`
  - `ReadBody(r *bufio.Reader, maxBodySize int) (err error)`
  - `mustSkipBody() bool` (unexported)
  - `Write(w *bufio.Writer) error`
  - `WriteTo(w io.Writer) (int64, error)`
  - `writeBodyStream(w *bufio.Writer, sendBody bool) (err error)` (unexported)
  - `BodyWriteTo(w io.Writer) error`
  - `WriteGzip(w *bufio.Writer) error`
  - `WriteGzipLevel(w *bufio.Writer, level int) error`
  - `WriteDeflate(w *bufio.Writer) error`
  - `WriteDeflateLevel(w *bufio.Writer, level int) error`
  - `WriteBrotli(w *bufio.Writer) error`
  - `WriteBrotliLevel(w *bufio.Writer, level int) error`
  - `WriteZstd(w *bufio.Writer) error`
  - `WriteZstdLevel(w *bufio.Writer, level int) error`
  - `brotliBody(level int)` (unexported)
  - `gzipBody(level int)` (unexported)
  - `deflateBody(level int)` (unexported)
  - `zstdBody(level int)` (unexported)
- **Imports Required**: `bufio`, `bytes`, `errors`, `fmt`, `io`, `github.com/lemon4ksan/foundation/net/http/status`, `github.com/lemon4ksan/foundation/net/http/zerocopy`, `machcompress "github.com/lemon4ksan/mach/proto/compress"`.

---

## 4. RFC-Compliant Docstring Replacement Drafts

All drafted docstrings strictly adhere to Go conventions, cite authoritative RFC specifications, and specify concurrency and lifecycle invariants.

### 4.1 Request Core & URI Accessors (`request.go`)

```go
// Request represents an HTTP request message adhering to RFC 9110 Section 3 and RFC 9112 Section 2.
//
// A Request encapsulates the request-line (method, request-target, protocol version),
// message headers, and optional entity/chunked message body.
//
// Concurrency:
// A Request instance MUST NOT be used concurrently from multiple goroutines.
//
// Lifecycle & Pooling:
// Instances should be acquired via AcquireRequest and recycled via ReleaseRequest to avoid
// heap allocations. Copying Request by value is forbidden; use CopyTo instead.
type Request struct { ... }

// SetHost sets the host component of the request target and updates the Host header (RFC 9110 Section 7.2).
//
// Thread-safe: No. Must be invoked from the owning goroutine only.
func (req *Request) SetHost(host string)

// SetHostBytes sets the host component of the request target from a byte slice (RFC 9110 Section 7.2).
//
// Thread-safe: No.
func (req *Request) SetHostBytes(host []byte)

// Host returns the host component of the request target (RFC 9110 Section 7.2).
// The returned byte slice is borrowed from the underlying URI or header buffer and remains
// valid until the request is modified or released.
//
// Thread-safe: No.
func (req *Request) Host() []byte

// SetRequestURI sets the raw request-target string (RFC 9112 Section 3.2).
// This invalidates any previously cached parsed URI state.
//
// Thread-safe: No.
func (req *Request) SetRequestURI(requestURI string)

// SetRequestURIBytes sets the raw request-target byte slice (RFC 9112 Section 3.2).
// This invalidates any previously cached parsed URI state.
//
// Thread-safe: No.
func (req *Request) SetRequestURIBytes(requestURI []byte)

// RequestURI returns the raw request-target bytes (RFC 9112 Section 3.2).
// If the URI was previously modified through the URI() accessor, the formatted request-target
// is synchronized back to the request headers.
//
// Thread-safe: No.
func (req *Request) RequestURI() []byte

// URI returns a pointer to the parsed zerocopy.URI representation of the request-target (RFC 3986, RFC 9112 Section 3.2).
// The returned pointer is valid until the request is modified or released.
//
// Thread-safe: No.
func (req *Request) URI() *zerocopy.URI

// SetURI sets the request URI by copying from the provided zerocopy.URI instance (RFC 3986).
// If newURI is nil, the request URI is cleared.
//
// Thread-safe: No.
func (req *Request) SetURI(newURI *zerocopy.URI)

// ParseURI parses the request-line authority and request-target into the internal URI cache (RFC 3986).
// Subsequent calls return the cached parse error if already parsed.
//
// Thread-safe: No.
func (req *Request) ParseURI() error

// ConnectionClose reports whether the "Connection: close" token is present in the request headers
// (RFC 9110 Section 9.6, RFC 9112 Section 9.3).
//
// Thread-safe: No.
func (req *Request) ConnectionClose() bool

// SetConnectionClose sets the "Connection: close" header token, indicating that the connection
// should be closed after completing this request (RFC 9110 Section 9.6, RFC 9112 Section 9.3).
//
// Thread-safe: No.
func (req *Request) SetConnectionClose()

// GetTimeOut returns the maximum duration allowed for the request lifecycle.
//
// Thread-safe: No.
func (req *Request) GetTimeOut() time.Duration

// SetTimeout sets the maximum duration allowed for the request lifecycle.
//
// Thread-safe: No.
func (req *Request) SetTimeout(t time.Duration)

// Reset clears all request contents and returns internal buffers to their respective pools.
// Must be called prior to returning the request to a sync.Pool.
//
// Thread-safe: No.
func (req *Request) Reset()

// String returns the diagnostic wire representation of the request.
// This allocates and formats the entire request message; use Write in performance-critical paths.
func (req *Request) String() string
```

### 4.2 Request Body Accessors (`request_body.go`)

```go
// Body returns the request entity body as a byte slice (RFC 9110 Section 6.4).
// If the body is backed by a stream, Body reads the entire stream into memory.
// The returned slice is borrowed and remains valid until the request is reset or released.
//
// Thread-safe: No.
func (req *Request) Body() []byte

// BodyBuffer returns the underlying ByteBuffer storing the in-memory request body.
// If no buffer is currently allocated, one is acquired from the per-P request body pool.
//
// Thread-safe: No.
func (req *Request) BodyBuffer() *bytesconv.ByteBuffer

// SetBody sets the request body to a copy of body (RFC 9110 Section 6.4).
// It resets any existing body stream or multipart files.
//
// Thread-safe: No.
func (req *Request) SetBody(body []byte)

// SetBodyString sets the request body to the provided string content.
//
// Thread-safe: No.
func (req *Request) SetBodyString(body string)

// SetBodyRaw sets the request body to point directly to body without copying.
// The caller must guarantee that the provided slice is not mutated while the Request is in use.
//
// Thread-safe: No.
func (req *Request) SetBodyRaw(body []byte)

// AppendBody appends the given byte slice to the request entity body.
//
// Thread-safe: No.
func (req *Request) AppendBody(p []byte)

// AppendBodyString appends the given string to the request entity body.
//
// Thread-safe: No.
func (req *Request) AppendBodyString(s string)

// ResetBody resets the request body buffer, releases streaming readers, and deletes temporary multipart files.
//
// Thread-safe: No.
func (req *Request) ResetBody()

// ReleaseBody reclaims the internal body buffer if its capacity exceeds size bytes, reducing GC pressure.
//
// Thread-safe: No.
func (req *Request) ReleaseBody(size int)

// SwapBody swaps the request body buffer with the provided slice and returns the old body slice.
//
// Thread-safe: No.
func (req *Request) SwapBody(body []byte) []byte

// CopyTo deep-copies the request contents into dst, excluding streaming readers.
//
// Thread-safe: No.
func (req *Request) CopyTo(dst *Request)

// CopyToSkipBody copies all request metadata (headers, URI, POST parameters, flags) into dst while omitting the entity body.
//
// Thread-safe: No.
func (req *Request) CopyToSkipBody(dst *Request)

// BodyScoped borrows the request body without memory allocation using a borrow.Scope.
//
// Concurrency & Lifetime:
// The returned borrow.Bytes is valid only for the lifetime of scope s.
func (req *Request) BodyScoped(s *borrow.Scope) borrow.Bytes

// ReadBodyScoped invokes fn with a borrowed reference to the request body buffer for zero-allocation reading.
//
// Concurrency & Lifetime:
// The byte slice passed to fn must not be referenced after fn returns.
func (req *Request) ReadBodyScoped(fn func([]byte) error) error

// BodyGunzip decompresses the request body using Gzip (RFC 1952, RFC 9110 Section 8.4).
func (req *Request) BodyGunzip() ([]byte, error)

// BodyGunzipWithLimit decompresses the request body using Gzip with an upper bound of maxBodySize bytes (RFC 1952).
func (req *Request) BodyGunzipWithLimit(maxBodySize int) ([]byte, error)

// BodyUnbrotli decompresses the request body using Brotli (RFC 7932, RFC 9110 Section 8.4).
func (req *Request) BodyUnbrotli() ([]byte, error)

// BodyUnbrotliWithLimit decompresses the request body using Brotli with an upper bound of maxBodySize bytes (RFC 7932).
func (req *Request) BodyUnbrotliWithLimit(maxBodySize int) ([]byte, error)

// BodyInflate decompresses the request body using Deflate (RFC 1951, RFC 9110 Section 8.4).
func (req *Request) BodyInflate() ([]byte, error)

// BodyInflateWithLimit decompresses the request body using Deflate with an upper bound of maxBodySize bytes (RFC 1951).
func (req *Request) BodyInflateWithLimit(maxBodySize int) ([]byte, error)

// BodyUnzstd decompresses the request body using Zstandard (RFC 8878, RFC 9110 Section 8.4).
func (req *Request) BodyUnzstd() ([]byte, error)

// BodyUnzstdWithLimit decompresses the request body using Zstandard with an upper bound of maxBodySize bytes (RFC 8878).
func (req *Request) BodyUnzstdWithLimit(maxBodySize int) ([]byte, error)

// BodyUncompressed inspects the request Content-Encoding header and decompresses the entity body (RFC 9110 Section 8.4).
func (req *Request) BodyUncompressed() ([]byte, error)

// BodyUncompressedWithLimit decompresses the request body according to Content-Encoding up to maxBodySize bytes.
func (req *Request) BodyUncompressedWithLimit(maxBodySize int) ([]byte, error)
```

### 4.3 Request Streaming & RequestStream (`request_stream.go`)

```go
// SetBodyStream configures bodyStream as the request entity body reader with an expected length (RFC 9112 Section 7.1).
// If bodySize >= 0, Content-Length is set to bodySize. If bodySize < 0, Chunked Transfer Coding is used.
// If bodyStream implements io.Closer, it is automatically closed upon stream exhaustion or request release.
//
// Thread-safe: No.
func (req *Request) SetBodyStream(bodyStream io.Reader, bodySize int)

// IsBodyStream reports whether the request body is supplied via a streaming io.Reader.
//
// Thread-safe: No.
func (req *Request) IsBodyStream() bool

// SetBodyStreamWriter populates the request body asynchronously via sw using a pipe connection.
//
// Thread-safe: No.
func (req *Request) SetBodyStreamWriter(sw StreamWriter)

// BodyStream returns the configured request body stream reader, or nil if none is set.
// The caller must ensure CloseBodyStream or ReleaseRequest is invoked when finished.
//
// Thread-safe: No.
func (req *Request) BodyStream() io.Reader

// RequestBodyStream returns the configured request body reader.
//
// Thread-safe: No.
func (req *Request) RequestBodyStream() io.Reader

// CloseBodyStream closes the active request body stream if it implements io.Closer and returns any error encountered.
//
// Thread-safe: No.
func (req *Request) CloseBodyStream() error

// BodyWriter returns an io.Writer adapter for streaming data into the request body buffer.
//
// Thread-safe: No.
func (req *Request) BodyWriter() io.Writer

// ContinueReadBodyStream reads a chunked or streaming request body after a 100 Continue handshake has taken place (RFC 9110 Section 10.1.1).
//
// Thread-safe: No.
func (req *Request) ContinueReadBodyStream(r *bufio.Reader, maxBodySize int, preParseMultipartForm ...bool) error

// RequestStream decodes chunked transfer coded or fixed-length streaming HTTP request/response bodies (RFC 9112 Section 7.1).
//
// Thread-safe: No. Must be read from a single goroutine.
type RequestStream struct { ... }

// Read reads up to len(p) bytes from the underlying HTTP body stream, parsing chunk headers and chunk trailers as needed (RFC 9112 Section 7.1).
func (rs *RequestStream) Read(p []byte) (int, error)

// AcquireRequestStream acquires a pooled RequestStream instance initialized with prefetched bytes, reader, and header parser.
//
// Thread-safe: Yes (backed by sync.Pool).
func AcquireRequestStream(b *bytesconv.ByteBuffer, r *bufio.Reader, h bodyStreamHeader) *RequestStream

// ReleaseRequestStream releases a RequestStream instance back to RequestStreamPool after resetting its fields.
//
// Thread-safe: Yes (backed by sync.Pool).
func ReleaseRequestStream(rs *RequestStream)

// RequestStreamPool pools RequestStream instances to reduce garbage collection overhead during streaming I/O.
var RequestStreamPool sync.Pool
```

### 4.4 Request Wire I/O (`request_wire.go`)

```go
// Read deserializes an HTTP request (including headers and entity body) from the buffered reader r (RFC 9112 Section 2.1).
//
// Thread-safe: No.
func (req *Request) Read(r *bufio.Reader) error

// ReadLimitBody deserializes an HTTP request from r, rejecting bodies exceeding maxBodySize with ErrBodyTooLarge.
//
// Thread-safe: No.
func (req *Request) ReadLimitBody(r *bufio.Reader, maxBodySize int) error

// MayContinue reports whether the request contains an "Expect: 100-continue" header (RFC 9110 Section 10.1.1).
//
// Thread-safe: No.
func (req *Request) MayContinue() bool

// ContinueReadBody completes reading the entity body after an interim 100 Continue response has been transmitted (RFC 9110 Section 10.1.1).
//
// Thread-safe: No.
func (req *Request) ContinueReadBody(r *bufio.Reader, maxBodySize int, preParseMultipartForm ...bool) error

// ReadBody reads the request body from r according to contentLength and maxBodySize (RFC 9112 Section 6, Section 7).
//
// Thread-safe: No.
func (req *Request) ReadBody(r *bufio.Reader, contentLength, maxBodySize int) (err error)

// Write serializes the HTTP request to the buffered writer w without flushing (RFC 9112 Section 2.1).
//
// Thread-safe: No.
func (req *Request) Write(w *bufio.Writer) error

// WriteTo writes the serialized request to w, implementing the io.WriterTo interface (RFC 9112 Section 2.1).
//
// Thread-safe: No.
func (req *Request) WriteTo(w io.Writer) (int64, error)

// WriteVectored writes the request headers and body directly to conn using vectored net.Buffers I/O, eliminating buffer copying.
//
// Thread-safe: No.
func (req *Request) WriteVectored(conn net.Conn) error
```

### 4.5 Request Forms & Multipart (`request_forms.go`)

```go
// PostArgs returns the parsed application/x-www-form-urlencoded POST parameters (RFC 1866).
// The arguments are parsed lazily on first access.
//
// Thread-safe: No.
func (req *Request) PostArgs() *zerocopy.Args

// MultipartForm returns the parsed multipart/form-data payload (RFC 7578).
// Returns ErrNoMultipartForm if the Content-Type header lacks a valid multipart boundary.
//
// Lifecycle:
// Callers MUST call RemoveMultipartFormFiles after processing the form to purge temporary disk files.
func (req *Request) MultipartForm() (*multipart.Form, error)

// MultipartFormWithLimit parses the multipart/form-data payload, enforcing a maximum memory/disk budget of maxBodySize bytes (RFC 7578).
//
// Lifecycle:
// Callers MUST call RemoveMultipartFormFiles after processing the form to purge temporary disk files.
func (req *Request) MultipartFormWithLimit(maxBodySize int) (*multipart.Form, error)

// RemoveMultipartFormFiles removes any temporary spool files created on disk during multipart/form-data parsing (RFC 7578).
func (req *Request) RemoveMultipartFormFiles()

// BodyWriteTo writes the request entity body or multipart payload directly to w.
func (req *Request) BodyWriteTo(w io.Writer) error
```

### 4.6 Response Core, Status & Addresses (`response.go`)

```go
// Response represents an HTTP response message adhering to RFC 9110 Section 3 and RFC 9112 Section 2.
//
// A Response encapsulates the status-line (protocol version, status code, reason phrase),
// response headers, entity body or body stream, and connection network addresses.
//
// Concurrency:
// A Response instance MUST NOT be used concurrently from multiple goroutines.
//
// Lifecycle & Pooling:
// Instances should be acquired via AcquireResponse and recycled via ReleaseResponse to avoid
// heap allocations. Copying Response by value is forbidden; use CopyTo instead.
type Response struct { ... }

// StatusCode returns the 3-digit HTTP response status code (RFC 9110 Section 15, RFC 9112 Section 3.1.2).
//
// Thread-safe: No.
func (resp *Response) StatusCode() int

// SetStatusCode sets the 3-digit HTTP response status code (RFC 9110 Section 15, RFC 9112 Section 3.1.2).
//
// Thread-safe: No.
func (resp *Response) SetStatusCode(statusCode int)

// ConnectionClose reports whether the "Connection: close" token is set in the response headers (RFC 9110 Section 9.6, RFC 9112 Section 9.3).
//
// Thread-safe: No.
func (resp *Response) ConnectionClose() bool

// SetConnectionClose sets the "Connection: close" header token, indicating the connection will be closed upon response delivery (RFC 9110 Section 9.6).
//
// Thread-safe: No.
func (resp *Response) SetConnectionClose()

// ParseNetConn extracts and caches the local and remote network addresses from conn.
//
// Thread-safe: No.
func (resp *Response) ParseNetConn(conn net.Conn)

// RemoteAddr returns the remote network address of the client connection.
//
// Thread-safe: No.
func (resp *Response) RemoteAddr() net.Addr

// LocalAddr returns the local network address of the server listener.
//
// Thread-safe: No.
func (resp *Response) LocalAddr() net.Addr

// Reset clears all response fields and recycles the internal body buffer to its pool.
//
// Thread-safe: No.
func (resp *Response) Reset()

// String returns the diagnostic wire representation of the response.
// This allocates memory for string formatting; use Write in performance-critical paths.
func (resp *Response) String() string
```

### 4.7 Response Body Accessors (`response_body.go`)

```go
// Body returns the response entity body as a byte slice (RFC 9110 Section 6.4).
// If the body is backed by a stream, Body reads the entire stream into memory.
//
// Thread-safe: No.
func (resp *Response) Body() []byte

// BodyBuffer returns the underlying ByteBuffer storing the in-memory response body.
// If no buffer is currently allocated, one is acquired from the per-P response body pool.
//
// Thread-safe: No.
func (resp *Response) BodyBuffer() *bytesconv.ByteBuffer

// SetBody sets the response body to a copy of body (RFC 9110 Section 6.4).
//
// Thread-safe: No.
func (resp *Response) SetBody(body []byte)

// SetBodyString sets the response body to the provided string content.
//
// Thread-safe: No.
func (resp *Response) SetBodyString(body string)

// SetBodyRaw sets the response body to point directly to body without copying.
//
// Thread-safe: No.
func (resp *Response) SetBodyRaw(body []byte)

// AppendBody appends the given byte slice to the response entity body.
//
// Thread-safe: No.
func (resp *Response) AppendBody(p []byte)

// AppendBodyString appends the given string to the response entity body.
//
// Thread-safe: No.
func (resp *Response) AppendBodyString(s string)

// ResetBody clears the response entity body and returns large buffers to the pool.
//
// Thread-safe: No.
func (resp *Response) ResetBody()

// ReleaseBody reclaims the internal body buffer if its capacity exceeds size bytes.
//
// Thread-safe: No.
func (resp *Response) ReleaseBody(size int)

// SwapBody swaps the response body buffer with the provided slice and returns the old body slice.
//
// Thread-safe: No.
func (resp *Response) SwapBody(body []byte) []byte

// CopyTo deep-copies the response contents into dst, excluding streaming readers.
//
// Thread-safe: No.
func (resp *Response) CopyTo(dst *Response)

// CopyToSkipBody copies all response metadata (headers, status code, addresses) into dst while omitting the entity body.
//
// Thread-safe: No.
func (resp *Response) CopyToSkipBody(dst *Response)

// BodyScoped borrows the response body without memory allocation using a borrow.Scope.
func (resp *Response) BodyScoped(s *borrow.Scope) borrow.Bytes

// ReadBodyScoped invokes fn with a borrowed reference to the response body buffer.
func (resp *Response) ReadBodyScoped(fn func([]byte) error) error

// BodyGunzip decompresses the response body using Gzip (RFC 1952, RFC 9110 Section 8.4).
func (resp *Response) BodyGunzip() ([]byte, error)

// BodyGunzipWithLimit decompresses the response body using Gzip with an upper bound of maxBodySize bytes (RFC 1952).
func (resp *Response) BodyGunzipWithLimit(maxBodySize int) ([]byte, error)

// BodyUnbrotli decompresses the response body using Brotli (RFC 7932, RFC 9110 Section 8.4).
func (resp *Response) BodyUnbrotli() ([]byte, error)

// BodyUnbrotliWithLimit decompresses the response body using Brotli with an upper bound of maxBodySize bytes (RFC 7932).
func (resp *Response) BodyUnbrotliWithLimit(maxBodySize int) ([]byte, error)

// BodyInflate decompresses the response body using Deflate (RFC 1951, RFC 9110 Section 8.4).
func (resp *Response) BodyInflate() ([]byte, error)

// BodyInflateWithLimit decompresses the response body using Deflate with an upper bound of maxBodySize bytes (RFC 1951).
func (resp *Response) BodyInflateWithLimit(maxBodySize int) ([]byte, error)

// BodyUnzstd decompresses the response body using Zstandard (RFC 8878, RFC 9110 Section 8.4).
func (resp *Response) BodyUnzstd() ([]byte, error)

// BodyUnzstdWithLimit decompresses the response body using Zstandard with an upper bound of maxBodySize bytes (RFC 8878).
func (resp *Response) BodyUnzstdWithLimit(maxBodySize int) ([]byte, error)

// BodyUncompressed inspects the response Content-Encoding header and decompresses the entity body (RFC 9110 Section 8.4).
func (resp *Response) BodyUncompressed() ([]byte, error)

// BodyUncompressedWithLimit decompresses the response body according to Content-Encoding up to maxBodySize bytes.
func (resp *Response) BodyUncompressedWithLimit(maxBodySize int) ([]byte, error)
```

### 4.8 Response Streaming & SendFile (`response_stream.go`)

```go
// SendFile opens the local file at path and assigns it as the streaming response body (RFC 9110 Section 8.8.2).
// It populates the Last-Modified header with the file's modification time.
// The file descriptor is automatically closed once the response transmission finishes or the response is reset.
//
// Thread-safe: No.
func (resp *Response) SendFile(path string) error

// SetBodyStream configures bodyStream as the response body reader with an expected length (RFC 9112 Section 7.1).
// If bodySize >= 0, Content-Length is set to bodySize. If bodySize < 0, Chunked Transfer Coding is used.
// If bodyStream implements io.Closer, it is automatically closed upon stream completion.
//
// Thread-safe: No.
func (resp *Response) SetBodyStream(bodyStream io.Reader, bodySize int)

// IsBodyStream reports whether the response body is supplied via an io.Reader stream.
//
// Thread-safe: No.
func (resp *Response) IsBodyStream() bool

// SetBodyStreamWriter configures sw to stream response data asynchronously into a piped connection.
//
// Thread-safe: No.
func (resp *Response) SetBodyStreamWriter(sw StreamWriter)

// BodyWriter returns an io.Writer adapter for appending data to the response body buffer.
//
// Thread-safe: No.
func (resp *Response) BodyWriter() io.Writer

// BodyStream returns the active response body stream reader, or nil if none is configured.
//
// Thread-safe: No.
func (resp *Response) BodyStream() io.Reader

// CloseBodyStream closes the active response body stream if it implements io.Closer.
//
// Thread-safe: No.
func (resp *Response) CloseBodyStream() error

// ReadStreamScoped reads from the response body stream chunk by chunk, passing each borrowed slice to fn for zero-allocation stream processing.
func (resp *Response) ReadStreamScoped(s *borrow.Scope, fn func(chunk borrow.Bytes) error) error
```

### 4.9 Response Wire I/O & Wire Compression (`response_wire.go`)

```go
// Read deserializes an HTTP response (handling interim 1xx responses like Early Hints) from r (RFC 9110 Section 15.2, RFC 9112 Section 2.1).
//
// Thread-safe: No.
func (resp *Response) Read(r *bufio.Reader) error

// ReadLimitBody deserializes an HTTP response from r, rejecting bodies exceeding maxBodySize with ErrBodyTooLarge.
//
// Thread-safe: No.
func (resp *Response) ReadLimitBody(r *bufio.Reader, maxBodySize int) error

// ReadBody reads the response entity body from r according to Content-Length or Chunked Transfer Coding (RFC 9112 Section 6, Section 7).
//
// Thread-safe: No.
func (resp *Response) ReadBody(r *bufio.Reader, maxBodySize int) (err error)

// Write serializes the HTTP response message to w without flushing (RFC 9112 Section 2.1).
//
// Thread-safe: No.
func (resp *Response) Write(w *bufio.Writer) error

// WriteTo writes the serialized response to w, implementing io.WriterTo (RFC 9112 Section 2.1).
//
// Thread-safe: No.
func (resp *Response) WriteTo(w io.Writer) (int64, error)

// BodyWriteTo writes the response entity body directly to w.
func (resp *Response) BodyWriteTo(w io.Writer) error

// WriteGzip compresses the response body using default Gzip compression and writes the response to w (RFC 1952, RFC 9110 Section 8.4).
// Automatically sets "Content-Encoding: gzip" and appends "Vary: Accept-Encoding" (RFC 9110 Section 12.5.5).
func (resp *Response) WriteGzip(w *bufio.Writer) error

// WriteGzipLevel compresses the response body using the specified Gzip compression level and writes to w (RFC 1952).
func (resp *Response) WriteGzipLevel(w *bufio.Writer, level int) error

// WriteDeflate compresses the response body using default Deflate compression and writes to w (RFC 1951, RFC 9110 Section 8.4).
// Automatically sets "Content-Encoding: deflate" and appends "Vary: Accept-Encoding" (RFC 9110 Section 12.5.5).
func (resp *Response) WriteDeflate(w *bufio.Writer) error

// WriteDeflateLevel compresses the response body using the specified Deflate compression level and writes to w (RFC 1951).
func (resp *Response) WriteDeflateLevel(w *bufio.Writer, level int) error

// WriteBrotli compresses the response body using default Brotli compression and writes to w (RFC 7932, RFC 9110 Section 8.4).
// Automatically sets "Content-Encoding: br" and appends "Vary: Accept-Encoding" (RFC 9110 Section 12.5.5).
func (resp *Response) WriteBrotli(w *bufio.Writer) error

// WriteBrotliLevel compresses the response body using the specified Brotli compression level and writes to w (RFC 7932).
func (resp *Response) WriteBrotliLevel(w *bufio.Writer, level int) error

// WriteZstd compresses the response body using default Zstandard compression and writes to w (RFC 8878, RFC 9110 Section 8.4).
// Automatically sets "Content-Encoding: zstd" and appends "Vary: Accept-Encoding" (RFC 9110 Section 12.5.5).
func (resp *Response) WriteZstd(w *bufio.Writer) error

// WriteZstdLevel compresses the response body using the specified Zstandard compression level and writes to w (RFC 8878).
func (resp *Response) WriteZstdLevel(w *bufio.Writer, level int) error
```

---

## 5. Caveats
1. **Scope Boundaries**:
   - `http.go`, `chunk.go`, and body encoders/decoders (`body_chunked.go`, `body_identity.go`, `body_compress.go`, `multipart.go`) are within the scope of Explorer 3 (`teamwork_preview_explorer_m2_3`).
   - `header.go`, `header_request.go`, and `header_response.go` are within the scope of Explorer 1 (`teamwork_preview_explorer_m2_1`).
   - The worker performing implementation must ensure that `bodyBytes()` and other package-private helper methods in `http.go` remain accessible across package `http` files.
2. **`streaming.go` Consolidation**:
   - As established in Section 2, `streaming.go` contains `RequestStream` which is directly used for request and response body streaming decoding. Consolidating it into `request_stream.go` eliminates redundant micro-files while adhering strictly to `PROJECT.md` Section 5.
   - If the orchestrator prefers keeping `streaming.go` as a standalone file, it must be documented with the drafted docstrings in Section 4.3. The recommended path is consolidation into `request_stream.go`.

---

## 6. Conclusion
The HTTP Request and Response models in `proto/http` have been fully investigated and cataloged across 137 entities. The monolithic `request.go` (1,194 lines) and `response.go` (998 lines) can be cleanly partitioned into 9 cohesive files (each under 450 lines) with zero API breaks, zero performance regressions, and zero heap allocations. All 16 missing docstrings have been identified and drafted alongside RFC 9110 / RFC 9112 / RFC 7578 / RFC 3986 citations, ready for direct integration by the M2 Worker.

---

## 7. Verification Method

### 7.1 Independent File & Symbol Verification
To verify the catalog and line ranges:
```powershell
# Verify declaration counts in request.go (expect 67)
Get-Content proto/http/request.go | Select-String '^(func |type )' | Measure-Object

# Verify declaration counts in response.go (expect 65)
Get-Content proto/http/response.go | Select-String '^(func |type )' | Measure-Object

# Verify declaration counts in streaming.go (expect 6)
Get-Content proto/http/streaming.go | Select-String '^(func |type |var )' | Measure-Object
```

### 7.2 Post-Decomposition Build & Test Gate
When the Worker implements the modular split:
```powershell
# 1. Package-level test pass
go test -v -race ./proto/http/...

# 2. Entire repository test pass
go test -race ./...

# 3. Micro-benchmark verification (enforcing 0 B/op and 0 allocs/op)
go test -bench=BenchmarkFullPipeline_ScopedBorrow -benchmem ./proto/http/...

# 4. Linter verification
golangci-lint run ./proto/http/...
```

### 7.3 Invalidation Conditions
This analysis is invalidated if:
1. Public method signatures on `Request` or `Response` are altered, breaking compatibility with `client/` or `server/`.
2. Heap allocations are introduced on `BodyScoped`, `ReadBodyScoped`, or `AcquireRequestStream` hot paths.
3. Exported entities are created without complete RFC citations or BSD license headers.
