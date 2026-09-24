# Milestone M2: HTTP Header Domain Decomposition - Handoff Report

## 1. Observation

### 1.1 Source Files Examined
The HTTP header subsystem in `d:/CodingProjects/mach/proto/http` currently comprises 6 Go source files totalling 4,172 lines:

| File | Size (Bytes) | Lines | Current Content Summary |
|---|---|---|---|
| `proto/http/header.go` | 24,174 | 911 | Base `header` struct, trailer functions, URI validation, date ticker, error sentinels (`ErrBadTrailer` ... `ErrSmallReadBuffer`), error formatting helpers |
| `proto/http/header_request.go` | 43,108 | 1,554 | `RequestHeader` struct, request line parsing, header parsing loop, field peek/set/add/del, cookies, wire I/O (`Read`, `Write`, `Header`), scoped borrowing |
| `proto/http/header_response.go` | 36,851 | 1,286 | `ResponseHeader` struct, status line formatting, response header loop, status/reason parsing, field peek/set/add/del, cookies, wire I/O, scoped borrowing |
| `proto/http/headerscanner.go` | 4,539 | 206 | `headerScanner` struct, SIMD CRLF matching, line trimming, continued line handling |
| `proto/http/header_helpers.go` | 1,710 | 82 | Internal helper functions: `peekArgBytesHeaders`, `setArgBytesHeaders`, `appendArgBytesHeaders`, `copyHeaders`, `peekAllArgBytesToDstHeaders`, `parseTrailerHeaders` |
| `proto/http/headers.go` | 7,539 | 133 | 72 exported standard HTTP header name string constants |

### 1.2 Build and Test Baseline
Executing `go test -v ./proto/http/...` on `d:/CodingProjects/mach` yields:
```text
=== RUN   TestH1Engine_URIAndArgs
--- PASS: TestH1Engine_URIAndArgs (0.00s)
=== RUN   TestLLHTTP_Chunked_OfficialVectors
--- PASS: TestLLHTTP_Chunked_OfficialVectors (0.00s)
=== RUN   FuzzH1Request
--- PASS: FuzzH1Request (0.00s)
=== RUN   FuzzH1Response
--- PASS: FuzzH1Response (0.00s)
PASS
ok  	github.com/lemon4ksan/mach/proto/http	0.621s
```
All existing unit tests and fuzz harnesses compile and pass cleanly.

### 1.3 Target Architecture from PROJECT.md
Section 5 ("Target File Organization") in `d:/CodingProjects/mach/.agents/PROJECT.md` dictates the target layout for `proto/http` headers:
```text
│   └── http/
│       ├── header.go             (base header, RequestHeader, ResponseHeader structs)
│       ├── header_parse.go       (first line, header loop, SIMD boundary scanning)
│       ├── header_fields.go      (field accessors: Peek, Set, Add, Del, All)
│       ├── header_cookies.go     (cookie parse, format, iteration)
│       ├── header_trailers.go    (trailer handling & validation)
│       ├── header_scoped.go      (scoped zero-alloc methods)
│       ├── headerscanner.go      (SIMD header scanner)
│       ├── headers.go            (standard header constants)
```
Currently, `header_request.go` (1,554 lines) and `header_response.go` (1,286 lines) form two monolithic files that duplicate structural concerns (wire parse, wire write, cookie management, trailer handling, field accessors, scoped borrow).

### 1.4 Docstring Violations Observed
Inspection of exported symbols against R2 (`ORIGINAL_REQUEST.md`) and Section 6.2 (`PROJECT.md`) revealed the following violations:
1. **Missing docstrings on exported error sentinels** in `proto/http/header.go` lines 142–156:
   `ErrBadTrailer`, `ErrReadingResponseHeaders`, `ErrReadingResponseTrailer`, `ErrResponseFirstLineMissingSpace`, `ErrUnexpectedStatusCodeChar`, `ErrMissingRequestMethod`, `ErrUnsupportedRequestMethod`, `ErrExtraWhitespaceInRequestLine`, `ErrEmptyRequestURI`, `ErrDuplicateContentLength`, `ErrUnsupportedTransferEncoding`, `ErrNonNumericChars`, `ErrNeedMore`, `ErrSmallReadBuffer`.
2. **Missing docstring on exported method**:
   `func (h *ResponseHeader) SetDisableNormalizing(disable bool)` (`proto/http/header_response.go:1283`).
3. **Missing docstring on exported struct field**:
   `SecureErrorLogMessage bool` on `header` (`proto/http/header.go:43`).
4. **Missing docstrings on 72 exported constants** in `proto/http/headers.go` lines 12–132:
   `HeaderAccept`, `HeaderHost`, `HeaderContentType`, etc., lack any commentary.
5. **Obsolete RFC citations**:
   `SetTrailer`, `SetTrailerBytes`, `AddTrailer`, `AddTrailerBytes` in `proto/http/header.go:88–91` cite `[RFC7231]` and `[RFC7235]`. RFC 7230–7235 were formally obsoleted in June 2022 by RFC 9110 and RFC 9112.
6. **Leaky upstream / tool artifact strings**:
   - `DisableSpecialHeader()` and `EnableSpecialHeader()` (`header_request.go:409, 422`) state: *"fasthttp will not set any special headers for you"*. This references the upstream legacy library name rather than `mach`.
   - Docstrings on `Set`, `AddBytesKV`, `Cookies` (`header_request.go:744, 765`, `header_response.go:311, 438, 504`) reference `zerocopy.Cookie` and `Set-zerocopy.Cookie` in text due to automated substitution artifacts.
   - `PeekCookie` in `header_response.go:97` states *"PeekCookie is able to returns cookie by a given key from response"*.
7. **Missing RFC citations, concurrency expectations, and memory lifecycle invariants** on all exported header methods (`RequestHeader`, `ResponseHeader`, `ErrNothingRead`, `ErrSmallBuffer`, `AppendNormalizedHeaderKey`).

---

## 2. Logic Chain

1. **Downstream API Stability Constraint**:
   According to `PROJECT.md` Section 1.2 and Section 4.3, `mach` is consumed by `aoni` and internal transports (`client/h1`, `client/h2`, `client/h3`, `proto/h2`, `proto/h3`). All exported structs (`RequestHeader`, `ResponseHeader`, `ErrNothingRead`, `ErrSmallBuffer`), method signatures, and constants (`Header*`) must maintain 100% public signature equivalence.
2. **Cohesive Single-Responsibility Decomposition**:
   - Grouping by struct type (`header_request.go`, `header_response.go`) resulted in file sizes exceeding 1,200–1,500 lines and duplicated concerns across request and response implementations.
   - Decomposing by domain concern aligns with `PROJECT.md`:
     - `header.go`: Core data models (`header`, `RequestHeader`, `ResponseHeader`), lifecycle (`Reset`, `CopyTo`), error definitions.
     - `header_parse.go`: Wire input/output and serialization/deserialization logic (`Read`, `Write`, `parse`, `parseFirstLine`, `parseHeaders`, `AppendBytes`).
     - `header_fields.go`: Header value lookup and manipulation (`Peek*`, `Set*`, `Add*`, `Del*`, `All*`, normalization).
     - `header_cookies.go`: State management mechanism (RFC 6265 cookie access, mutation, client deletion).
     - `header_trailers.go`: Chunked transfer trailing headers (RFC 9112 Section 7.1.2 validation, read, write, iteration).
     - `header_scoped.go`: Scoped zero-allocation memory borrowing (`foundation/borrow`).
   - The helper file `header_helpers.go` (82 lines) contains 6 package-private functions. Placing `parseTrailerHeaders` into `header_trailers.go` and the remaining map/slice helpers into `header_fields.go` eliminates this extraneous file and ensures complete cohesion.
3. **Documentation Conformance**:
   - Aligning with RFC 9110 (HTTP Semantics) and RFC 9112 (HTTP/1.1 Syntax) brings all docstrings up to the reference standard set by `foundation` and `aoni`.
   - Explicitly documenting concurrency (single-goroutine, not thread-safe) and memory lifecycles (borrowed slice lifetime tied to message release) prevents downstream bugs in caller pipelines.

---

## 3. Caveats

1. **Package Scope**:
   All 6 target files reside within `package http`. In Go, symbols with package scope remain visible across files without package-level import cycles.
2. **Method Receiver Promotion**:
   `RequestHeader` and `ResponseHeader` both anonymously embed `header`. Methods defined on `*header` (such as `ConnectionClose()`, `SetTrailer()`, `Trailers()`, `IsHTTP11()`) are promoted onto both receivers. Moving these methods between files does not alter method sets or ABI.
3. **No Code Modification Undertaken**:
   In strict compliance with Explorer read-only mode, no files in `proto/http` were modified during this investigation. Implementation is delegated to Milestone M2 implementers.

---

## 4. Conclusion

### 4.1 Full Symbol Inventory

#### A. Types (6 total)
1. `type header struct`: Base header structure with internal field storage, normalization flags, and trailer arrays.
2. `type RequestHeader struct`: Exported HTTP request header container (embeds `header` and `zerocopy.NoCopy`).
3. `type ResponseHeader struct`: Exported HTTP response header container (embeds `header` and `zerocopy.NoCopy`).
4. `type headerScanner struct`: Internal SIMD-accelerated linear byte scanner for CRLF-delimited MIME headers.
5. `type headerValueScanner struct`: Internal comma-separated header value token scanner.
6. `type ErrNothingRead struct`: Exported error wrapper for idle connection closes or read timeouts.
7. `type ErrSmallBuffer struct`: Exported error wrapper indicating buffer exhaustion during header reading.

#### B. Constants (74 total)
- Unexported delimiters: `rChar = byte('\r')`, `nChar = byte('\n')`.
- 72 exported standard header constants in `proto/http/headers.go`:
  `HeaderAccept`, `HeaderAcceptCH`, `HeaderAcceptCharset`, `HeaderAcceptCHLifetime`, `HeaderAcceptEncoding`, `HeaderAcceptLanguage`, `HeaderAcceptPatch`, `HeaderAcceptPushPolicy`, `HeaderAcceptRanges`, `HeaderAcceptSignature`, `HeaderAccessControlAllowCredentials`, `HeaderAccessControlAllowHeaders`, `HeaderAccessControlAllowMethods`, `HeaderAccessControlAllowOrigin`, `HeaderAccessControlExposeHeaders`, `HeaderAccessControlMaxAge`, `HeaderAccessControlRequestHeaders`, `HeaderAccessControlRequestMethod`, `HeaderAge`, `HeaderAllow`, `HeaderAltSvc`, `HeaderAuthorization`, `HeaderCacheControl`, `HeaderClearSiteData`, `HeaderConnection`, `HeaderContentDisposition`, `HeaderContentDPR`, `HeaderContentEncoding`, `HeaderContentLanguage`, `HeaderContentLength`, `HeaderContentLocation`, `HeaderContentRange`, `HeaderContentSecurityPolicy`, `HeaderContentSecurityPolicyReportOnly`, `HeaderContentType`, `HeaderCookie`, `HeaderCookie2`, `HeaderCrossOriginResourcePolicy`, `HeaderDate`, `HeaderDNT`, `HeaderDPR`, `HeaderEarlyData`, `HeaderETag`, `HeaderExpect`, `HeaderExpectCT`, `HeaderExpires`, `HeaderFeaturePolicy`, `HeaderForwarded`, `HeaderFrom`, `HeaderHost`, `HeaderIfMatch`, `HeaderIfModifiedSince`, `HeaderIfNoneMatch`, `HeaderIfRange`, `HeaderIfUnmodifiedSince`, `HeaderIndex`, `HeaderKeepAlive`, `HeaderLargeAllocation`, `HeaderLastEventID`, `HeaderLastModified`, `HeaderLink`, `HeaderLocation`, `HeaderMaxForwards`, `HeaderNEL`, `HeaderOrigin`, `HeaderPingFrom`, `HeaderPingTo`, `HeaderPragma`, `HeaderProxyAuthenticate`, `HeaderProxyAuthorization`, `HeaderProxyConnection`, `HeaderPublicKeyPins`, `HeaderPublicKeyPinsReportOnly`, `HeaderPushPolicy`, `HeaderRange`, `HeaderReferer`, `HeaderReferrerPolicy`, `HeaderReportTo`, `HeaderRetryAfter`, `HeaderSaveData`, `HeaderSecWebSocketAccept`, `HeaderSecWebSocketExtensions`, `HeaderSecWebSocketKey`, `HeaderSecWebSocketProtocol`, `HeaderSecWebSocketVersion`, `HeaderServer`, `HeaderServerTiming`, `HeaderSetCookie`, `HeaderSignature`, `HeaderSignedHeaders`, `HeaderSourceMap`, `HeaderStrictTransportSecurity`, `HeaderTE`, `HeaderTimingAllowOrigin`, `HeaderTk`, `HeaderTrailer`, `HeaderTransferEncoding`, `HeaderUpgrade`, `HeaderUpgradeInsecureRequests`, `HeaderUserAgent`, `HeaderVary`, `HeaderVia`, `HeaderViewportWidth`, `HeaderWarning`, `HeaderWidth`, `HeaderWWWAuthenticate`, `HeaderXContentTypeOptions`, `HeaderXDNSPrefetchControl`, `HeaderXDownloadOptions`, `HeaderXForwardedFor`, `HeaderXForwardedHost`, `HeaderXForwardedProto`, `HeaderXFrameOptions`, `HeaderXPermittedCrossDomainPolicies`, `HeaderXPingback`, `HeaderXPoweredBy`, `HeaderXRequestedWith`, `HeaderXRobotsTag`, `HeaderXUACompatible`, `HeaderXXSSProtection`.

#### C. Variables & Sentinels (16 total)
- `var ErrBadTrailer`: Forbidden trailer field detected.
- `var ErrReadingResponseHeaders`: I/O error during response header read.
- `var ErrReadingResponseTrailer`: I/O error during response trailer read.
- `var ErrResponseFirstLineMissingSpace`: Malformed status line missing SP delimiter.
- `var ErrUnexpectedStatusCodeChar`: Non-digit character in status code.
- `var ErrMissingRequestMethod`: Malformed request line missing method.
- `var ErrUnsupportedRequestMethod`: Method token contains invalid chars.
- `var ErrExtraWhitespaceInRequestLine`: Extra SP in request line.
- `var ErrEmptyRequestURI`: Empty URI target in request line.
- `var ErrDuplicateContentLength`: Multiple conflicting Content-Length headers.
- `var ErrUnsupportedTransferEncoding`: Non-chunked transfer coding in H1.
- `var ErrNonNumericChars`: Non-numeric byte in Content-Length value.
- `var ErrNeedMore`: Incomplete line buffer requiring more I/O bytes.
- `var ErrSmallReadBuffer`: Read buffer smaller than header line.
- `var serverDate atomic.Pointer[[]byte]`: Cached RFC 9110 HTTP-date buffer.
- `var serverDateOnce sync.Once`: Singleton date refresher ticker initializer.

---

### 4.2 Target Modular Decomposition Architecture

The 6 decomposed files in `proto/http` are structured as follows:

```
d:/CodingProjects/mach/proto/http/
├── header.go            (Models, lifecycle, errors, constants: ~260 LOC)
├── header_parse.go      (Wire I/O, first-line & header parsing/writing: ~600 LOC)
├── header_fields.go     (Field accessors, mutators, normalization, iteration: ~880 LOC)
├── header_cookies.go    (RFC 6265 cookies for RequestHeader and ResponseHeader: ~180 LOC)
├── header_trailers.go   (RFC 9112 Section 7.1.2 trailer parsing, validation, I/O: ~270 LOC)
├── header_scoped.go     (foundation/borrow scoped zero-alloc methods: ~100 LOC)
├── headerscanner.go     (SIMD header scanner unchanged: 206 LOC)
└── headers.go           (Standard header constants with docstrings: ~210 LOC)
```

#### Detailed Symbol Placement Matrix

| Target File | Structs & Types | Methods | Functions & Variables |
|---|---|---|---|
| **`header.go`** | `header`<br>`RequestHeader`<br>`ResponseHeader`<br>`ErrNothingRead`<br>`ErrSmallBuffer` | `(*header).copyTo`<br>`(*RequestHeader).Reset`<br>`(*RequestHeader).resetSkipNormalize`<br>`(*RequestHeader).CopyTo`<br>`(*ResponseHeader).Reset`<br>`(*ResponseHeader).resetSkipNormalize`<br>`(*ResponseHeader).CopyTo` | `rChar`, `nChar`<br>`ErrBadTrailer` ... `ErrSmallReadBuffer` sentinels |
| **`header_parse.go`** | *(none)* | `(*RequestHeader).Read`<br>`(*RequestHeader).readLoop`<br>`(*RequestHeader).tryRead`<br>`(*RequestHeader).parse`<br>`(*RequestHeader).parseFirstLine`<br>`(*RequestHeader).parseHeaders`<br>`(*RequestHeader).Write`<br>`(*RequestHeader).WriteTo`<br>`(*RequestHeader).Header`<br>`(*RequestHeader).String`<br>`(*RequestHeader).AppendBytes`<br>`(*RequestHeader).RawHeaders`<br>`(*ResponseHeader).Read`<br>`(*ResponseHeader).tryRead`<br>`(*ResponseHeader).parse`<br>`(*ResponseHeader).parseFirstLine`<br>`(*ResponseHeader).parseHeaders`<br>`(*ResponseHeader).Write`<br>`(*ResponseHeader).WriteTo`<br>`(*ResponseHeader).Header`<br>`(*ResponseHeader).String`<br>`(*ResponseHeader).appendStatusLine`<br>`(*ResponseHeader).AppendBytes` | `readRawHeaders`<br>`nextLine`<br>`isHTTPVersion`<br>`isValidMethod`<br>`validateRequestURI`<br>`parseContentLength`<br>`headerError`<br>`headerErrorMsg`<br>`bufferSnippet`<br>`isOnlyCRLF`<br>`mustPeekBuffered`<br>`mustDiscard` |
| **`header_fields.go`** | `headerValueScanner` | `(*header).ConnectionClose`<br>`(*header).SetConnectionClose`<br>`(*header).ResetConnectionClose`<br>`(*header).SetContentType`<br>`(*header).SetContentTypeBytes`<br>`(*header).Protocol`<br>`(*header).IsHTTP11`<br>`(*header).DisableNormalizing`<br>`(*header).EnableNormalizing`<br>`(*header).SetNoDefaultContentType`<br>`(*header).setNonSpecial`<br>`(*RequestHeader).Peek*` (Peek, PeekBytes, PeekCanonical, peek, PeekAll, peekAll, PeekKeys)<br>`(*RequestHeader).Set*` (Set, SetBytesK, SetBytesV, SetBytesKV, SetCanonical)<br>`(*RequestHeader).Add*` (Add, AddBytesK, AddBytesV, AddBytesKV)<br>`(*RequestHeader).Del*` (Del, DelBytes, del)<br>`(*RequestHeader).setSpecialHeader`<br>`(*RequestHeader).All`, `AllInOrder`, `Len`<br>`(*RequestHeader).SetByteRange`<br>`(*RequestHeader).ConnectionUpgrade`<br>`(*RequestHeader).ContentLength`<br>`(*RequestHeader).SetContentLength`<br>`(*RequestHeader).ContentType`<br>`(*RequestHeader).ContentEncoding`<br>`(*RequestHeader).SetContentEncoding*`<br>`(*RequestHeader).SetMultipartFormBoundary*`<br>`(*RequestHeader).MultipartFormBoundary`<br>`(*RequestHeader).Host`, `SetHost*`<br>`(*RequestHeader).UserAgent`, `SetUserAgent*`<br>`(*RequestHeader).Referer`, `SetReferer*`<br>`(*RequestHeader).Method`, `SetMethod*`<br>`(*RequestHeader).SetProtocol*`<br>`(*RequestHeader).RequestURI`, `SetRequestURI*`<br>`(*RequestHeader).IsGet` ... `IsPatch`<br>`(*RequestHeader).HasAcceptEncoding*`<br>`(*RequestHeader).DisableSpecialHeader`<br>`(*RequestHeader).EnableSpecialHeader`<br>`(*RequestHeader).validate`<br>`(*RequestHeader).ignoreBody`<br>`(*ResponseHeader).Peek*`<br>`(*ResponseHeader).Set*`<br>`(*ResponseHeader).Add*`<br>`(*ResponseHeader).Del*`<br>`(*ResponseHeader).del`<br>`(*ResponseHeader).setSpecialHeader`<br>`(*ResponseHeader).All`, `Len`<br>`(*ResponseHeader).SetContentRange`<br>`(*ResponseHeader).StatusCode`, `SetStatusCode`<br>`(*ResponseHeader).StatusMessage`, `SetStatusMessage`<br>`(*ResponseHeader).SetProtocol`<br>`(*ResponseHeader).SetLastModified`<br>`(*ResponseHeader).ConnectionUpgrade`<br>`(*ResponseHeader).ContentLength`, `SetContentLength`<br>`(*ResponseHeader).mustSkipContentLength`<br>`(*ResponseHeader).isCompressibleContentType`<br>`(*ResponseHeader).ContentType`<br>`(*ResponseHeader).ContentEncoding`, `SetContentEncoding*`<br>`(*ResponseHeader).addVaryBytes`<br>`(*ResponseHeader).Server`, `SetServer*`<br>`(*ResponseHeader).AltSvc`<br>`(*ResponseHeader).SetDisableNormalizing`<br>`(*headerValueScanner).next` | `AppendNormalizedHeaderKey*`<br>`appendHeaderLine`<br>`stripSpace`<br>`hasHeaderValue`<br>`VisitHeaderParams`<br>`peekArgBytesHeaders`<br>`setArgBytesHeaders`<br>`appendArgBytesHeaders`<br>`copyHeaders`<br>`peekAllArgBytesToDstHeaders`<br>`serverDate`<br>`serverDateOnce`<br>`updateServerDate`<br>`refreshServerDate` |
| **`header_cookies.go`** | *(none)* | `(*RequestHeader).Cookie`<br>`(*RequestHeader).CookieBytes`<br>`(*RequestHeader).Cookies`<br>`(*RequestHeader).SetCookie*` (SetCookie, SetCookieBytesK, SetCookieBytesKV)<br>`(*RequestHeader).DelCookie*` (DelCookie, DelCookieBytes, DelAllCookies)<br>`(*RequestHeader).collectCookies`<br>`(*ResponseHeader).Cookie`<br>`(*ResponseHeader).PeekCookie`<br>`(*ResponseHeader).Cookies`<br>`(*ResponseHeader).SetCookie`<br>`(*ResponseHeader).DelCookie*`<br>`(*ResponseHeader).DelAllCookies`<br>`(*ResponseHeader).DelClientCookie*` (DelClientCookie, DelClientCookieBytes) | *(none)* |
| **`header_trailers.go`** | *(none)* | `(*header).SetTrailer`<br>`(*header).SetTrailerBytes`<br>`(*header).AddTrailer`<br>`(*header).AddTrailerBytes`<br>`(*header).Trailers`<br>`(*header).PeekTrailerKeys`<br>`(*header).ReadTrailer`<br>`(*header).tryReadTrailer`<br>`(*RequestHeader).TrailerHeader`<br>`(*RequestHeader).writeTrailer`<br>`(*ResponseHeader).TrailerHeader`<br>`(*ResponseHeader).writeTrailer` | `isBadTrailer`<br>`isValidTrailerKey`<br>`isValidHeaderKey`<br>`validHeaderValueByte`<br>`appendTrailerBytes`<br>`copyTrailer`<br>`parseTrailerHeaders`<br>`parseTrailer` |
| **`header_scoped.go`** | *(none)* | `(*RequestHeader).PeekScoped`<br>`(*RequestHeader).CookieScoped`<br>`(*RequestHeader).PeekAllScoped`<br>`(*RequestHeader).TrailerScoped`<br>`(*ResponseHeader).PeekScoped`<br>`(*ResponseHeader).CookieScoped`<br>`(*ResponseHeader).PeekAllScoped`<br>`(*ResponseHeader).TrailerScoped` | *(none)* |

---

### 4.3 Catalog of Docstring Violations & Proposed RFC 9110 / 9112 Docstrings

#### A. Core Types & Errors (`header.go`)

```go
// ErrBadTrailer indicates that a forbidden trailer field name was specified
// in violation of RFC 9112 Section 7.1.2 and RFC 9110 Section 6.5.1.
var ErrBadTrailer = errors.New("mach: contain forbidden trailer")

// ErrReadingResponseHeaders indicates an I/O error occurred while reading HTTP
// response headers from the wire (RFC 9112 Section 2.1).
var ErrReadingResponseHeaders = errors.New("mach: error when reading response headers")

// ErrReadingResponseTrailer indicates an I/O error occurred while reading trailing
// response header fields after a chunked transfer payload (RFC 9112 Section 7.1.2).
var ErrReadingResponseTrailer = errors.New("mach: error when reading response trailer")

// ErrResponseFirstLineMissingSpace indicates the HTTP response status line is malformed
// because it lacks required whitespace delimiters between version, status code, and reason phrase (RFC 9112 Section 3.1.2).
var ErrResponseFirstLineMissingSpace = errors.New("mach: cannot find whitespace in the first line of response")

// ErrUnexpectedStatusCodeChar indicates a non-numeric byte was encountered in the 3-digit
// HTTP response status code (RFC 9110 Section 15).
var ErrUnexpectedStatusCodeChar = errors.New("mach: unexpected char at the end of status code")

// ErrMissingRequestMethod indicates the HTTP request start-line lacks a valid method token (RFC 9112 Section 3.1.1).
var ErrMissingRequestMethod = errors.New("mach: cannot find http request method")

// ErrUnsupportedRequestMethod indicates the request method contains disallowed characters (RFC 9110 Section 9.1).
var ErrUnsupportedRequestMethod = errors.New("mach: unsupported http request method")

// ErrExtraWhitespaceInRequestLine indicates invalid extra whitespace was found in the request line (RFC 9112 Section 3).
var ErrExtraWhitespaceInRequestLine = errors.New("mach: extra whitespace in request line")

// ErrEmptyRequestURI indicates the request target URI is empty in the request line (RFC 9112 Section 3.2).
var ErrEmptyRequestURI = errors.New("mach: requesturi cannot be empty")

// ErrDuplicateContentLength indicates multiple conflicting Content-Length header field values
// were received, violating RFC 9112 Section 6.1 and posing an HTTP request smuggling risk.
var ErrDuplicateContentLength = errors.New("mach: duplicate content-length header")

// ErrUnsupportedTransferEncoding indicates an unsupported transfer coding was specified (RFC 9112 Section 6.1).
var ErrUnsupportedTransferEncoding = errors.New("mach: unsupported transfer-encoding")

// ErrNonNumericChars indicates invalid non-digit characters were encountered while parsing a Content-Length integer value (RFC 9112 Section 6.2).
var ErrNonNumericChars = errors.New("mach: non-numeric chars found")

// ErrNeedMore indicates the parser requires additional byte buffer chunks to locate line terminators (CRLF).
var ErrNeedMore = errors.New("mach: need more data: cannot find trailing lf")

// ErrSmallReadBuffer indicates the configured read buffer size is insufficient to hold the current header line.
var ErrSmallReadBuffer = errors.New("mach: small read buffer. increase readbuffersize")

// ErrNothingRead is returned when an active connection is closed before any header bytes are read,
// typical of keep-alive connection teardown by a peer (RFC 9112 Section 9.6).
type ErrNothingRead struct{ error }

// ErrSmallBuffer is returned when the configured read buffer size is too small to contain
// the incoming HTTP header block (RFC 9112 Section 2.1).
type ErrSmallBuffer struct{ error }

// RequestHeader represents an HTTP/1.1 request message header block (RFC 9112 Section 2.1).
//
// Concurrency:
// RequestHeader is NOT safe for concurrent access by multiple goroutines.
// If a RequestHeader instance is shared across goroutines, external synchronization is required.
//
// Lifecycle & Pooling:
// RequestHeader instances are typically acquired via object pools or embedded directly within
// Request. Call Reset() before returning instances to pools. Direct struct copying is forbidden;
// use CopyTo instead.
type RequestHeader struct { ... }

// ResponseHeader represents an HTTP/1.1 response message header block (RFC 9112 Section 2.1).
//
// Concurrency:
// ResponseHeader is NOT safe for concurrent access by multiple goroutines.
//
// Lifecycle & Pooling:
// ResponseHeader instances are managed by connection pools or embedded within Response.
// Call Reset() before recycling. Direct struct copying is forbidden; use CopyTo instead.
type ResponseHeader struct { ... }
```

#### B. Parsing & Serialization (`header_parse.go`)

```go
// Read reads an HTTP request header block from r up to and including the CRLFCRLF terminator (RFC 9112 Section 2.1).
// It returns io.EOF if the stream is closed prior to reading the first byte.
//
// Concurrency: Not goroutine-safe; single-threaded execution required.
func (h *RequestHeader) Read(r *bufio.Reader) error

// Write writes the serialized wire representation of the HTTP request header to w (RFC 9112 Section 2.1).
//
// Concurrency: Not goroutine-safe.
func (h *RequestHeader) Write(w *bufio.Writer) error

// WriteTo writes the serialized wire representation of the request header to w, implementing io.WriterTo (RFC 9112 Section 2.1).
func (h *RequestHeader) WriteTo(w io.Writer) (int64, error)

// Header returns the byte slice representation of the serialized request header (RFC 9112 Section 2.1).
// The returned slice is valid until the request is reset or released. Do not retain references.
func (h *RequestHeader) Header() []byte

// RawHeaders returns the unmodified wire byte representation of headers as received from the transport (RFC 9112 Section 2.1).
// The slice is valid only during handler invocation and must not be retained.
func (h *RequestHeader) RawHeaders() []byte

// Read reads an HTTP response header block from r up to and including the CRLFCRLF terminator (RFC 9112 Section 2.1).
func (h *ResponseHeader) Read(r *bufio.Reader) error

// Write writes the serialized wire representation of the HTTP response header to w (RFC 9112 Section 2.1).
func (h *ResponseHeader) Write(w *bufio.Writer) error

// WriteTo writes the serialized wire representation of the response header to w, implementing io.WriterTo (RFC 9112 Section 2.1).
func (h *ResponseHeader) WriteTo(w io.Writer) (int64, error)

// Header returns the byte slice representation of the serialized response header (RFC 9112 Section 2.1).
func (h *ResponseHeader) Header() []byte
```

#### C. Field Accessors & Modifiers (`header_fields.go`)

```go
// Peek returns the header value for key (RFC 9110 Section 5.1).
// The returned slice points to internal memory and remains valid until the header is mutated or released.
// For persistent usage, copy the byte slice or use PeekScoped.
func (h *RequestHeader) Peek(key string) []byte

// Set sets the header field for key to value, replacing any previous entries (RFC 9110 Section 5.1).
func (h *RequestHeader) Set(key, value string)

// Add appends a header field entry for key with value (RFC 9110 Section 5.2).
// Multiple fields with the same key are serialized in order of insertion.
func (h *RequestHeader) Add(key, value string)

// Del removes all header field entries matching key (RFC 9110 Section 5.1).
func (h *RequestHeader) Del(key string)

// All returns an iterator over key-value byte pairs present in the request header (RFC 9110 Section 5).
// Iteration elements point to volatile memory and must not be retained past loop iterations.
func (h *RequestHeader) All() iter.Seq2[[]byte, []byte]

// AllInOrder returns an iterator over header fields in the exact order received from the wire (RFC 9112 Section 2.1).
func (h *RequestHeader) AllInOrder() iter.Seq2[[]byte, []byte]

// Host returns the Host header field value (RFC 9112 Section 3.2, RFC 9110 Section 7.2).
func (h *RequestHeader) Host() []byte

// SetHost sets the Host header field value (RFC 9112 Section 3.2, RFC 9110 Section 7.2).
func (h *RequestHeader) SetHost(host string)

// ContentLength returns the payload body length in bytes (RFC 9112 Section 6.2).
// Returns -1 if Transfer-Encoding is chunked, or -2 if identity framing is absent.
func (h *RequestHeader) ContentLength() int

// SetContentLength sets the Content-Length header field (RFC 9112 Section 6.2).
// A negative value configures chunked transfer coding (RFC 9112 Section 6.1).
func (h *RequestHeader) SetContentLength(contentLength int)

// Method returns the HTTP request method token (RFC 9110 Section 9). Defaults to GET if unset.
func (h *RequestHeader) Method() []byte

// StatusCode returns the 3-digit HTTP response status code (RFC 9110 Section 15). Defaults to 200 OK.
func (h *ResponseHeader) StatusCode() int

// SetStatusCode sets the 3-digit HTTP response status code (RFC 9110 Section 15).
func (h *ResponseHeader) SetStatusCode(statusCode int)

// StatusMessage returns the status line reason phrase (RFC 9112 Section 3.1.2).
func (h *ResponseHeader) StatusMessage() []byte

// AltSvc parses and returns advertised alternative service records from the Alt-Svc header (RFC 7838).
func (h *ResponseHeader) AltSvc() []altsvc.Service

// SetDisableNormalizing controls header key case canonicalization for response headers.
func (h *ResponseHeader) SetDisableNormalizing(disable bool)
```

#### D. State Management & Cookies (`header_cookies.go`)

```go
// Cookie returns the value of the request cookie identified by key (RFC 6265 Section 4.2.1, Section 5.4).
// Returns nil if no cookie matches. The returned slice is volatile and tied to request lifetime.
func (h *RequestHeader) Cookie(key string) []byte

// Cookies returns an iterator yielding cookie name and value byte pairs (RFC 6265 Section 5.4).
// Modifying RequestHeader during iteration causes undefined behavior.
func (h *RequestHeader) Cookies() iter.Seq2[[]byte, []byte]

// SetCookie sets a request cookie key-value pair into the Cookie header block (RFC 6265 Section 4.2.1).
func (h *RequestHeader) SetCookie(key, value string)

// DelCookie removes the cookie matching key from request headers (RFC 6265 Section 5.4).
func (h *RequestHeader) DelCookie(key string)

// DelAllCookies removes all cookies from the request header (RFC 6265 Section 5.4).
func (h *RequestHeader) DelAllCookies()

// SetCookie sets a Set-Cookie response header field (RFC 6265 Section 4.1).
// The passed cookie is copied and may be safely re-used by the caller immediately.
func (h *ResponseHeader) SetCookie(cookie *zerocopy.Cookie)

// DelClientCookie appends a Set-Cookie directive commanding the user agent to expire and evict the specified cookie (RFC 6265 Section 5.3).
func (h *ResponseHeader) DelClientCookie(key string)
```

#### E. Chunked Trailers (`header_trailers.go`)

```go
// SetTrailer specifies a trailer header field to be sent after a chunked message body (RFC 9112 Section 7.1.2).
//
// In accordance with RFC 9112 Section 7.1.2 and RFC 9110 Section 6.5.1, the following headers are forbidden as trailers:
// 1. Message framing fields: Transfer-Encoding, Content-Length.
// 2. Routing fields: Host.
// 3. Request modifiers and controls: Cache-Control, Max-Forwards, TE, Expect.
// 4. Authentication fields: Authorization, Proxy-Authorization.
// 5. Response control data: Location, Set-Cookie.
// 6. Payload representation metadata: Content-Encoding, Content-Type, Content-Range, Trailer.
//
// Returns ErrBadTrailer if trailer contains forbidden fields.
func (h *header) SetTrailer(trailer string) error

// AddTrailer appends trailer field names to the Trailer header for chunked transfer (RFC 9112 Section 7.1.2).
// Returns ErrBadTrailer if any forbidden trailer field is encountered.
func (h *header) AddTrailer(trailer string) error

// Trailers returns an iterator over registered trailer field names (RFC 9112 Section 7.1.2).
func (h *header) Trailers() iter.Seq[[]byte]

// ReadTrailer reads trailing header fields from r following the final zero-length chunk of a chunked body (RFC 9112 Section 7.1.2).
func (h *header) ReadTrailer(r *bufio.Reader) error
```

#### F. Scoped Memory Borrowing (`header_scoped.go`)

```go
// PeekScoped borrows the header value associated with key into scope s (RFC 9110 Section 5.1).
//
// Lifecycle:
// The returned borrow.Bytes reference is valid strictly within the lifetime of borrow.Scope s.
// It avoids heap allocation by borrowing directly from the underlying header buffer without retain copying.
func (h *RequestHeader) PeekScoped(s *borrow.Scope, key string) borrow.Bytes

// CookieScoped borrows the cookie value associated with key into scope s (RFC 6265 Section 5.4).
func (h *RequestHeader) CookieScoped(s *borrow.Scope, key string) borrow.Bytes

// PeekAllScoped borrows all values matching key as a slice of borrow.Bytes in scope s (RFC 9110 Section 5.2).
func (h *RequestHeader) PeekAllScoped(s *borrow.Scope, key string) []borrow.Bytes

// TrailerScoped borrows the trailing header value associated with key into scope s (RFC 9112 Section 7.1.2).
func (h *RequestHeader) TrailerScoped(s *borrow.Scope, key string) borrow.Bytes

// PeekScoped borrows the response header value associated with key into scope s (RFC 9110 Section 5.1).
func (h *ResponseHeader) PeekScoped(s *borrow.Scope, key string) borrow.Bytes

// CookieScoped borrows the response cookie value associated with key into scope s (RFC 6265 Section 4.1).
func (h *ResponseHeader) CookieScoped(s *borrow.Scope, key string) borrow.Bytes

// PeekAllScoped borrows all response values matching key into scope s (RFC 9110 Section 5.2).
func (h *ResponseHeader) PeekAllScoped(s *borrow.Scope, key string) []borrow.Bytes

// TrailerScoped borrows the response trailer value associated with key into scope s (RFC 9112 Section 7.1.2).
func (h *ResponseHeader) TrailerScoped(s *borrow.Scope, key string) borrow.Bytes
```

---

## 5. Verification Method

To independently verify the structural decomposition and docstring conformance during Milestone M2 implementation:

1. **Compilation and Build Integrity**:
   Verify that all packages compile with zero errors:
   ```pwsh
   go build ./proto/http/...
   go build ./...
   ```
2. **Unit Test Suite & Race Detector**:
   Verify that existing tests and fuzz regression targets pass cleanly with no data races:
   ```pwsh
   go test -v -race -timeout 90s ./proto/http/...
   ```
3. **Downstream Package Compatibility**:
   Ensure client and server packages that import `proto/http` continue to compile without signature changes:
   ```pwsh
   go test -v ./client/h1/... ./client/h2/... ./client/h3/... ./proto/h2/... ./proto/h3/... ./tests/e2e/...
   ```
4. **Static Analysis & Linters**:
   Execute linter verification to confirm BSD license headers, revived docstrings, and formatting compliance:
   ```pwsh
   golangci-lint run ./proto/http/...
   ```
5. **Zero-Allocation Benchmark Invariant**:
   Execute scoped borrow benchmarks to confirm zero heap allocations:
   ```pwsh
   go test -bench=BenchmarkFullPipeline_ScopedBorrow -benchmem ./proto/http/...
   ```

### Invalidation Conditions
- Any removal or renaming of exported symbols or methods on `RequestHeader` or `ResponseHeader`.
- Any heap allocation introduced into `PeekScoped`, `CookieScoped`, or `TrailerScoped`.
- Failing fuzz targets or race detector alerts in `h1_fuzz_test.go` or `llhttp_vectors_test.go`.
- Retaining obsolete `RFC7231` or `RFC7235` references in comments.
