# Clean Code & Standards Survey — Handoff Report

## 1. Observation

### 1.1 Original Request & Requirements
From `d:/CodingProjects/mach/.agents/ORIGINAL_REQUEST.md`:
- **R2. Clean Code, Documentation & Style Invariants**:
  - Include standard BSD license header (`// Copyright (c) 2026 Lemon4ksan All rights reserved.`) on every Go source file.
  - Provide comprehensive, idiomatic docstrings for all exported types, methods, interfaces, and constants, including RFC section citations (RFC 9112, 9113, 7541, 9114, 9204), concurrency expectations, and lifecycle invariants.
  - Format all code in accordance with `.golangci.yml` rules: `gofumpt` (with `group-params`), `golines` (max-len 120), `gci` (standard, default, `github.com/lemon4ksan/mach`), and `wsl_v5` (strict whitespace and block separation).
  - Linter acceptance criteria: `golangci-lint run ./...` completes with 0 errors/warnings under strict configuration matching `aoni` and `foundation`.

### 1.2 Reference Codebases Audit

#### 1.2.1 `d:/CodingProjects/foundation`
- **License Header**: Exactly 3 lines at the top of every file:
  ```go
  // Copyright (c) 2026 Lemon4ksan All rights reserved.
  // Use of this source code is governed by a BSD-style
  // license that can be found in the LICENSE file.
  ```
  Followed by an empty line 4.
- **Linter Configuration (`.golangci.yml`)**:
  - Config version: `"2"`.
  - Formatters enabled: `gci`, `gofmt`, `gofumpt` (with `extra: group-params: true`), `goimports` (`local-prefixes: github.com/lemon4ksan/foundation`), `golines` (`max-len: 120`).
  - Linters enabled: `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, `revive`, `bodyclose`, `nilerr`, `noctx`, `errorlint`, `protogetter`, `gocritic`, `prealloc`, `perfsprint`, `gosec`.
  - Exclusions: `generated: lax`, `presets: [common-false-positives, std-error-handling]`, rules suppressing `gosec` false alarms (`G103`, `G104`, `G404`, `G115`, `G306:|G301:`, `G703:|G706:`) and test files (`_test.go`).
- **Docstrings & RFC References**:
  - Dedicated `doc.go` in every package outlining architectural philosophy, concurrency model, and zero-allocation contracts.
  - Granular RFC section citations, e.g.:
    - `// Extract implements HKDF-Extract (RFC 5869 Section 2.2).`
    - `// UUID represents a 128-bit Universally Unique Identifier ... (RFC 9562 §4).`
    - `// Nil is the special Nil UUID with all 128 bits set to zero (RFC 9562 §5.9).`
    - `// Writer implements a streaming Zstandard (RFC 8878) encoder.`

#### 1.2.2 `d:/CodingProjects/aoni`
- **Consumer Relationship**: Directly imports `mach` packages (`github.com/lemon4ksan/mach/client/h1`, `/h2`, `/h3`, `/proto/h3`, `/proto/http`, `/server/h3`). Public API contracts must be strictly preserved.
- **License Header**: Identical 3-line BSD header as `foundation`.
- **Linter Configuration (`.golangci.yml`)**:
  - Config version: `"2"`.
  - Formatters: identical set (`gci`, `gofmt`, `gofumpt`, `goimports`, `golines`).
  - Enabled linters: identical set plus `wsl_v5` and `depguard`.
  - `wsl_v5` settings:
    ```yaml
    wsl_v5:
      default: default
      allow-first-in-block: true
      allow-whole-block: true
      branch-max-lines: 3
      case-max-lines: 5
      enable:
        - leading-whitespace
        - trailing-whitespace
        - after-block
        - err
    ```
- **Docstrings & RFC References**:
  - Rich `doc.go` documenting package architecture, usage tiers, pipeline stages, and subpackage catalog.
  - RFC references: `(RFC 9113 §3.1 & §3.2)`, `(RFC 9114 §7.2.4)`, `(RFC 6265 §5.2)`, `(RFC 8305 §5)`, `(RFC 9000 §14.1)`.

### 1.3 Current State Audit in `d:/CodingProjects/mach`

#### 1.3.1 BSD License Header Audit
- Command executed:
  ```powershell
  Get-ChildItem -Recurse -Filter *.go | Where-Object { $_.FullName -notmatch '\\.agents' }
  ```
- **Result**: Exactly 83 `.go` source files exist in `mach`.
- **License Header Status**: All 83 files (100%) already contain the exact 3-line BSD header:
  ```go
  // Copyright (c) 2026 Lemon4ksan All rights reserved.
  // Use of this source code is governed by a BSD-style
  // license that can be found in the LICENSE file.
  ```
  followed by an empty line 4.
- **Invariance Rule**: Any new file created during modular decomposition must include this identical header.

#### 1.3.2 Code Formatting Audit
- Command executed:
  ```powershell
  golangci-lint fmt --diff
  ```
- **Result**: Exit code 0, 0 diffs. All 83 files currently comply with `gofumpt` (`group-params: true`), `golines` (`max-len: 120`), and `gci`.

#### 1.3.3 Existing `.golangci.yml` Analysis
- Currently in `mach/.golangci.yml`:
  - `revive.rules: - name: exported, disabled: true` (hiding all missing docstring errors).
  - Missing `exclusions.rules:` section under `linters:` that exists in `foundation` and `aoni`.
  - Running `golangci-lint run ./...` with current configuration yields `0 issues` only because the `exported` rule is disabled and `errcheck` has no test exclusions.

#### 1.3.4 Linter Audit with `revive: exported` Enabled
- When `revive: exported` is enabled with standard exclusions matching `foundation` and `aoni`:
  - `errcheck`: 0 issues (test defer close suppressed).
  - `gosec`: 0 issues (unsafe G103 and G304 in `SendFile` handled).
  - `govet`: 0 issues.
  - `ineffassign`: 0 issues.
  - `staticcheck`: 0 issues.
  - `unused`: 0 issues.
  - `bodyclose`: 0 issues.
  - `nilerr`: 0 issues.
  - `noctx`: 0 issues.
  - `errorlint`: 0 issues.
  - `protogetter`: 0 issues.
  - `gocritic`: 0 issues.
  - `prealloc`: 0 issues.
  - `perfsprint`: 0 issues.
  - `wsl_v5`: 0 issues.
  - **`revive: exported`**: Exactly **212 issues** across **28 files**.

### 1.4 Detailed Breakdown of 212 Docstring Violations

| File | Count | Violations Summary |
|---|---|---|
| `proto/h2/frames.go` | 87 | Methods on `Continuation`, `Data`, `GoAway`, `Headers`, `Ping`, `Priority`, `PushPromise`, `RstStream`, `WindowUpdate` (`Type`, `Reset`, `Headers`, `EndHeaders`, `SetEndHeaders`, `Deserialize`, `Serialize`, `Stream`, `SetStream`, `Code`, `SetCode`, `Weight`, `Len`, etc.) |
| `client/h2/export.go` | 22 | Exported aliases and factory functions: `HPACK`, `AcquireHPACK`, `ReleaseHPACK`, `FrameType`, `Frame`, `AcquireFrame`, `ReleaseFrame`, `HeaderField`, `AcquireHeaderField`, `ReleaseHeaderField`, `FrameHeaders`, `Headers`, `FrameHeader`, `AcquireFrameHeader`, `ReleaseFrameHeader`, `FrameSettings`, `Settings`, `ReadFrameFrom`, `FrameData`, `Data`, `FrameWindowUpdate`, `WindowUpdate` |
| `proto/h2/settings.go` | 22 | Methods on `Settings`: `Type`, `Reset`, `CopyTo`, `SetHeaderTableSize`, `HeaderTableSize`, `SetPush`, `Push`, `SetMaxConcurrentStreams`, `MaxConcurrentStreams`, `SetMaxWindowSize`, `MaxWindowSize`, `SetMaxFrameSize`, `MaxFrameSize`, `SetMaxHeaderListSize`, `MaxHeaderListSize`, `SetEnableConnect`, `EnableConnect`, `IsAck`, `SetAck`, `Encode`, `Deserialize`, `Serialize` |
| `proto/h2/header.go` | 10 | `FrameHeader` methods (`Type`, `Flags`, `SetFlags`, `Stream`, `SetStream`, `Len`, `MaxLen`, `Body`, `SetBody`); `Headers.AppendHeaderField` |
| `client/h3/export.go` | 8 | Exported aliases and constructors: `QPACKCodec`, `NewQPACKCodec`, `FrameTypeHeaders`, `ReadFrameHeader`, `QUICOption`, `QUICTransport`, `QUICConnection`, `QUICWithDatagrams` |
| `proto/h3/qpack.go` | 7 | `QPACKStreamError.Is`, `QPACKCodec.Decoder`, `QPACKCodec.Encoder`, `DecodeResponseHeaders`, `DecodeResponseTrailers`, `DecodeRequestHeaders`, `EncodeResponseHeaders` |
| `proto/http/request.go` | 6 | `Request.CloseBodyStream`, `Request.BodyBuffer`, `Request.RequestBodyStream`, `Request.BodyUnzstd`, `Request.CopyToSkipBody`, `Request.ParseURI` |
| `proto/compress/compress.go` | 6 | `AcquireStacklessGzipWriter`, `ReleaseStacklessGzipWriter`, `WriteGunzipLimit`, `WriteInflateLimit`, `AcquireStacklessDeflateWriter`, `ReleaseStacklessDeflateWriter` |
| `client/h2/context.go` | 5 | `DefaultPingInterval`, `ClientOpts`, `Context`, `Context.State`, `Context.SetState` |
| `proto/http/response.go` | 5 | `Response.CloseBodyStream`, `Response.ParseNetConn`, `Response.BodyBuffer`, `Response.BodyUnzstd`, `Response.CopyToSkipBody` |
| `proto/http/http.go` | 5 | `ReadCloserWithError`, `NewCloseReaderWithError`, `ErrContentEncodingUnsupported`, `SwapRequestBody`, `SwapResponseBody` |
| `client/h1/conn.go` | 4 | `ClientConn`, `NewClientConn`, `ClientConn.Do`, `ClientConn.Close` |
| `proto/http/streaming.go` | 4 | `RequestStream`, `AcquireRequestStream`, `ReleaseRequestStream`, `RequestStreamPool` |
| `server/h1/header.go` | 3 | `Headers`, `HeaderEntry`, `NewHeadersWithCapacity` |
| `proto/h3/frames.go` | 3 | `ErrH3SettingsError`, `AppendHeadersHeader`, `AppendDataHeader` |
| `proto/h2/utils.go` | 2 | `StringPath` (comment prefix format), `StringStatus` |
| `proto/compress/zstd.go` | 2 | `CompressZstdSpeedNotSet`, `WriteUnzstdLimit` |
| `proto/h2/errors.go` | 1 | `ErrServerSupport` |
| `proto/http/header_response.go` | 1 | `ResponseHeader.SetDisableNormalizing` |
| `proto/http/header.go` | 1 | `ErrBadTrailer` |
| `proto/http/headers.go` | 1 | `HeaderAccept` |
| `proto/compress/brotli.go` | 1 | `WriteUnbrotliLimit` |
| `client/h3/conn.go` | 1 | `ClientConn.IsClosed` |
| `client/h2/conn.go` | 1 | `Conn.Do` |
| `server/h1/chunked.go` | 1 | `ErrInvalidChunkSize` |
| `server/h1/errors.go` | 1 | `ErrServerClosed` |
| `proto/h3/errors.go` | 1 | `ErrMissingMethodOrPath` |
| `server/h1/request.go` | 1 | `ErrMalformedRequestLine` |
| **Total** | **212** | Across 28 files |

---

## 2. Logic Chain

1. **License Header Invariant**:
   - *Observation*: All 83 existing Go files in `mach` already have the 3-line BSD header and line 4 empty.
   - *Reasoning*: The baseline codebase is already consistent. However, as the refactoring progresses and modular decomposition splits dense files (e.g. `client/h2/conn.go`, `proto/http/`, `server/`), each new file generated will cause a regression unless the generator explicitly prepends this header.
   - *Actionable Requirement*: Implementers must verify every new file has the exact 3-line header.

2. **Code Formatting & Whitespace**:
   - *Observation*: `golangci-lint fmt --diff` returned 0 diffs, and `wsl_v5` returned 0 violations.
   - *Reasoning*: The current code is formatted to the desired standard (`gofumpt` with `group-params`, `golines` 120, `gci`, `wsl_v5`).
   - *Actionable Requirement*: Maintain this formatting strictly across all decomposed and refactored code.

3. **Linter Exclusions Alignment**:
   - *Observation*: `mach/.golangci.yml` was missing `exclusions.rules` under `linters:`, which caused `errcheck` to flag test file deferred closes and `gosec` to flag `SendFile`.
   - *Reasoning*: Protocol engines legitimately utilize `os.Open` in static file responders (`SendFile`) and test files frequently defer `Close()` without checking error returns. Aligning `mach` with `foundation` and `aoni` by adopting their exclusion rules eliminates false positives while keeping the core production checks strictly enforced.

4. **Revive Docstring Gap**:
   - *Observation*: Currently, `revive.rules: - name: exported, disabled: true` in `mach/.golangci.yml` conceals 212 missing or malformed docstrings on exported symbols.
   - *Reasoning*: The user request (R2) explicitly requires:
     > "Provide comprehensive, idiomatic docstrings for all exported types, methods, interfaces, and constants, including RFC section citations (RFC 9112, 9113, 7541, 9114, 9204), concurrency expectations, and lifecycle invariants."
     Enabling `revive: exported` in the target `.golangci.yml` is required to enforce this requirement systematically.

5. **RFC Section & Invariant Mapping**:
   - *Observation*: Each package implements specific IETF RFC protocols:
     - `proto/http`, `client/h1`, `server/h1`: RFC 9112 (HTTP/1.1 syntax & chunking), RFC 9110 (HTTP semantics).
     - `proto/h2`, `fsm/h2`, `client/h2`, `server/h2`: RFC 9113 (HTTP/2 framing & multiplexing), RFC 7541 (HPACK).
     - `proto/h3`, `client/h3`, `server/h3`: RFC 9114 (HTTP/3 framing & control streams), RFC 9204 (QPACK), RFC 9000 (QUIC transport), RFC 9221 (Datagrams).
     - `proto/compress`: RFC 8878 (Zstandard), RFC 1952 (Gzip), RFC 1951 (Deflate), RFC 7932 (Brotli).
   - *Reasoning*: Standardizing docstrings with explicit RFC section numbers, concurrency model, and lifecycle rules provides exact technical clarity for library consumers (such as `aoni`).

---

## 3. Caveats

1. **Parent `go.work` Interaction**:
   - `D:\CodingProjects\go.work` is active by default and includes both `./foundation` and `./mach`.
   - Local uncommitted edits in `foundation/net/quic/stream.go` currently lack `ReceiveStream.FinalSize()`.
   - Running `golangci-lint` or `go test` with `$env:GOWORK="off"` ensures `mach` compiles cleanly against the stable published foundation module version (`v0.0.0-20260920191713-7709c688b2d7`) in the Go cache. When testing in the global workspace, `GOWORK=off` should be specified if `foundation` has uncommitted local API drifts.
2. **Public API Invariance for Downstream Consumers**:
   - `aoni` imports `client/h1`, `client/h2`, `client/h3`, `proto/h3`, `proto/http`, `server/h3`.
   - When decomposing files, public struct names, exported methods, and interface signatures MUST NOT be renamed, deleted, or altered.
3. **Revive Rule Transition**:
   - If `revive: exported` is enabled immediately before writing docstrings, CI will exit with code 1 due to the 212 catalogued violations. The target config can be checked incrementally, or adopted as the gate for the final milestone.

---

## 4. Conclusion & Target Specifications

### 4.1 Target `.golangci.yml` Configuration
The following exact configuration must be written to `d:/CodingProjects/mach/.golangci.yml`:

```yaml
version: "2"

run:
  timeout: 5m
  modules-download-mode: readonly

formatters:
  enable:
    - gci
    - gofmt
    - gofumpt
    - goimports
    - golines

  settings:
    gci:
      sections:
        - standard
        - default
        - prefix(github.com/lemon4ksan/mach)

    gofumpt:
      module-path: github.com/lemon4ksan/mach
      extra:
        group-params: true

    goimports:
      local-prefixes:
        - github.com/lemon4ksan/mach

    golines:
      max-len: 120

linters:
  enable:
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused
    - revive
    - bodyclose
    - nilerr
    - noctx
    - errorlint
    - protogetter
    - gocritic
    - prealloc
    - perfsprint
    - gosec
    - wsl_v5

  settings:
    revive:
      rules:
        - name: exported
          disabled: false

    wsl_v5:
      default: default
      allow-first-in-block: true
      allow-whole-block: true
      branch-max-lines: 3
      case-max-lines: 5
      enable:
        - leading-whitespace
        - trailing-whitespace
        - after-block
        - err

  exclusions:
    generated: lax
    presets:
      - common-false-positives
      - std-error-handling
    rules:
      - linters: [errcheck, revive, staticcheck, protogetter]
        path: .*(pb\.go|generated\.go)

      - linters: [staticcheck]
        text: "SA1019: .*golang.org/x/net/http2"

      - linters: [gosec]
        text: "G103:"

      - linters: [gosec]
        text: "G104:"

      - linters: [gosec]
        text: "G404:"

      - linters: [gosec]
        text: "G115:"

      - linters: [gosec]
        text: "G306:|G301:"

      - linters: [gosec]
        text: "G304:"
        path: "proto/http/response\\.go"

      - linters: [revive]
        path: "_test\\.go"
        text: "exported: (exported|comment on exported)"

      - linters: [gosec, noctx, bodyclose, errcheck, prealloc, staticcheck, errorlint, gocritic, unused, perfsprint, ineffassign]
        path: "_test\\.go"

      - linters: [gosec]
        text: "G703:|G706:"
        path: "(cmd|scripts)/.*"
```

### 4.2 Standard License Header Convention
Every `.go` source file MUST begin with:
```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package <pkgname>
```

### 4.3 Standard Docstring & RFC Citation Conventions

Every exported identifier must strictly follow these concrete patterns:

#### 1. Package Documentation (`doc.go`)
Every package should contain a `doc.go` formatted with:
```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package h2 implements high-performance, zero-allocation HTTP/2 framing and multiplexing.
//
// # Architectural Standards & Protocol Specs
//
// This package strictly implements:
//   - RFC 9113: HTTP/2 (Hypertext Transfer Protocol Version 2)
//   - RFC 7541: HPACK (Header Compression for HTTP/2)
//
// # Concurrency & Thread-Safety Invariants
//
//   - Framing primitives ([Frame], [FrameHeader]) are NOT safe for concurrent use without external synchronization.
//   - Connection handlers manage internal synchronization for concurrent stream multiplexing.
//
// # Memory & Zero-Allocation Invariants
//
//   - All frames are pooled via [sync.Pool] and recycled using [AcquireFrame] and [ReleaseFrame].
//   - Frame payloads reuse backing slice buffers; callers must not retain payload slices past stream dispatch.
package h2
```

#### 2. Exported Types & Interfaces
```go
// ClientConn manages a persistent HTTP/1.1 transport connection over a stream-oriented socket (RFC 9112 §9).
//
// # Concurrency
// ClientConn is not safe for concurrent use across multiple goroutines without external synchronization.
// For concurrent request dispatch, use a connection pool.
//
// # Lifecycle & Invariants
// Callers must call [ClientConn.Close] to release underlying socket resources when done.
type ClientConn struct { ... }
```

#### 3. Exported Methods & Functions
```go
// Do executes a single HTTP/1.1 request-response transaction over the persistent connection (RFC 9112 §9.3).
//
// Concurrency: Must be called sequentially per [ClientConn].
// Memory: Reuses internal buffers; req and res payload buffers are recycled via zero-allocation memory pools.
func (cc *ClientConn) Do(ctx context.Context, req *http.Request, res *http.Response) error
```

```go
// Type returns the protocol-specific frame type identifier FrameHeaders (0x1) (RFC 9113 §6.2).
func (h *Headers) Type() FrameType { return FrameHeaders }
```

```go
// Reset clears all header frame fields, releasing internal references for object pooling.
func (h *Headers) Reset()
```

#### 4. Exported Constants & Variables
Revive enforces that comments on exported vars/consts begin with the identifier:
```go
// StringPath represents the HTTP/2 ":path" pseudo-header (RFC 9113 §8.3.1).
var StringPath = []byte(":path")

// StringStatus represents the HTTP/2 ":status" pseudo-header (RFC 9113 §8.3.2).
var StringStatus = []byte(":status")

// DefaultPingInterval defines the default duration between proactive HTTP/2 keepalive PING frames (RFC 9113 §6.7).
const DefaultPingInterval = time.Second * 10

// ErrServerClosed indicates operations were attempted on a closed HTTP server.
var ErrServerClosed = errors.New("h1: server is closed")
```

#### 5. Re-exports and Type Aliases (`export.go`)
```go
// HPACK is an alias for [hpack.HPACK], providing header compression and decompression (RFC 7541).
type HPACK = hpack.HPACK

// AcquireHPACK borrows an [HPACK] instance from the internal thread-safe pool.
// Callers must return it via [ReleaseHPACK].
func AcquireHPACK() *HPACK { return hpack.AcquireHPACK() }

// ReleaseHPACK returns an [HPACK] instance to the internal pool for reuse.
func ReleaseHPACK(hp *HPACK) { hpack.ReleaseHPACK(hp) }
```

---

## 5. Verification Method

To independently verify these findings and confirm full compliance at any stage:

1. **Verify License Headers on All Go Files**:
   ```powershell
   Get-ChildItem -Recurse -Filter *.go | Where-Object { $_.FullName -notmatch '\\.agents' } | ForEach-Object {
       $first3 = Get-Content -Head 3 -Path $_.FullName
       $ok = ($first3[0] -eq '// Copyright (c) 2026 Lemon4ksan All rights reserved.') -and `
             ($first3[1] -eq '// Use of this source code is governed by a BSD-style') -and `
             ($first3[2] -eq '// license that can be found in the LICENSE file.')
       if (-not $ok) { Write-Error "Non-compliant header in: $($_.FullName)" }
   }
   ```
   *Expected result*: 0 errors.

2. **Verify Code Formatting (`gofumpt`, `golines`, `gci`)**:
   ```powershell
   golangci-lint fmt --diff
   ```
   *Expected result*: 0 diffs.

3. **Verify Current Linter Baseline**:
   ```powershell
   $env:GOWORK="off"
   golangci-lint run ./...
   ```
   *Expected result*: 0 issues under current `.golangci.yml`.

4. **Verify Target Linter Configuration & 212 Docstring Violations**:
   ```powershell
   $env:GOWORK="off"
   golangci-lint run -c .agents/teamwork_preview_explorer_survey_standards/candidate.golangci.yml --max-issues-per-linter 0 --max-same-issues 0 ./...
   ```
   *Expected result*: Exactly 212 violations, all from `revive: exported`, and 0 issues from `errcheck`, `gosec`, `govet`, `ineffassign`, `staticcheck`, `unused`, `bodyclose`, `nilerr`, `noctx`, `errorlint`, `protogetter`, `gocritic`, `prealloc`, `perfsprint`, and `wsl_v5`.

5. **Post-Refactoring Target Invalidation Condition**:
   Once all 212 docstrings are populated and any new decomposed files are created, running step 4 must produce:
   ```
   0 issues.
   ```
