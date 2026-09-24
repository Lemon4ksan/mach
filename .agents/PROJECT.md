# Project: mach Protocol Engine Refactoring & Optimization

## 1. Architecture

### 1.1 Overview & System Topology
`mach` is a high-performance, zero-allocation protocol engine in Go implementing HTTP/1.1 (RFC 9112), HTTP/2 (RFC 9113), and HTTP/3 (RFC 9114) over QUIC (RFC 9000). The engine is structured into five primary architectural layers:
1. **Core Abstraction Layer (`mach`)**: Defines generic protocol engine interfaces (`Engine`, `Conn`, `Stream`, `Frame`).
2. **Wire Framing & Codec Layer (`proto/`)**: Binary frame serializers/deserializers, zero-allocation in-situ overlays, QPACK (RFC 9204) and HPACK (RFC 7541) codecs, and streaming compression (`proto/compress`).
3. **HTTP Semantic & Message Layer (`proto/http`)**: Zero-allocation request/response message representations, SIMD-accelerated header scanners, scoped memory borrowing (`foundation/borrow`), and chunked transfer encoding.
4. **Client Transport Engines (`client/`)**: Connection pooling, HTTP/1.1 pipelining, HTTP/2 multiplexed socket I/O with lock-free SPSC ring buffers, and HTTP/3 QUIC connection adapters.
5. **Server Transport Engines (`server/`)**: Per-P buffer-shielded connection dispatchers, HTTP/1.1 request handling with early hints and hijacking, HTTP/2 multi-stream multiplexers, and HTTP/3 server engines.

### 1.2 Downstream Consumer Contract
`mach` is the core protocol transport library for `aoni` (high-performance protocol client) and integrates deeply with `foundation` silicon primitives. All refactoring operations must preserve 100% public API stability:
- No exported structs, interfaces, methods, or package constants may be deleted or modified in signature.
- All internal refactorings are performed via package-internal modular decomposition across single-responsibility files.

---

## 2. Feature Inventory

| # | Feature | Description | Milestone | Source |
|---|---|---|---|---|
| F01 | H2 Frame Payload Decomposition | Decompose monolithic `proto/h2/frames.go` into dedicated single-responsibility frame files | M1 | Survey §1.1 |
| F02 | QPACK Codec Modularization | Decompose `proto/h3/qpack.go` into client, server, and validation components | M1 | Survey §1.1 |
| F03 | Compression Subsystem Decomposition | Decompose `proto/compress/compress.go` into format-specific gzip and flate pools | M1 | Survey §1.1 |
| F04 | Protocol Frame & Codec Docstrings | Add comprehensive RFC 9113, 9114, 9204 docstrings and BSD headers across `proto/` | M1 | Survey §1.3 |
| F05 | HTTP Header Model Decomposition | Reorganize `proto/http` headers into base, parse, fields, cookies, trailers, and scoped borrow | M2 | Survey §1.1 |
| F06 | HTTP Request Model Modularization | Partition `proto/http/request.go` into model, body, streaming, wire I/O, and multipart | M2 | Survey §1.1 |
| F07 | HTTP Response Model Modularization | Partition `proto/http/response.go` into model, body, streaming, wire I/O, and compression | M2 | Survey §1.1 |
| F08 | HTTP Transfer Coding & Body Adapters | Decompose body handling into chunked, identity, compression, and multipart files | M2 | Survey §1.1 |
| F09 | Scratch File Purge | Remove obsolete `.tmp/` AST generator scratch scripts | M2 | Survey §1.2 |
| F10 | HTTP Message Layer Docstrings & Headers | Add comprehensive RFC 9112/9110 docstrings and BSD headers across `proto/http` | M2 | Survey §1.3 |
| F11 | Client H2 Monolith Decomposition | Decompose `client/h2/conn.go` (1,667 lines) into 9 single-responsibility components | M3 | Survey §1.1 |
| F12 | Client H2 Silicon Performance Invariants | Retain CPU cacheline padding (`_ cpu.CacheLinePad`) and SPSC ring buffers in H2 client | M3 | Survey §1.4 |
| F13 | Client H3 Modularization | Decompose `client/h3/conn.go` into connection, control stream, request, and response components | M3 | Survey §1.1 |
| F14 | Client Package Standards & Docstrings | Add comprehensive RFC docstrings and BSD headers across all `client/` packages | M3 | Survey §1.3 |
| F15 | Server H2 Monolith Decomposition | Decompose `server/h2/server_conn.go` (570 lines) into 5 focused components | M4 | Survey §1.1 |
| F16 | Server H1 & H3 Cleanliness & Per-P Shield | Standardize Per-P storage buffer pooling and clean up server implementations | M4 | Survey §1.1 |
| F17 | Server Package Standards & Docstrings | Add comprehensive RFC docstrings and BSD headers across all `server/` packages | M4 | Survey §1.3 |
| F18 | E2E Opaque-Box Test Infrastructure | Design and implement 4-tier requirement-driven E2E test harness (`TEST_READY.md`) | MT1 | Dual Track |
| F19 | Target .golangci.yml Deployment | Deploy strict `.golangci.yml` matching `foundation` and `aoni` with `revive: exported` | M5 | Survey §1.3 |
| F20 | Global Code Formatting & Cleanliness | Verify `gofumpt`, `golines`, `gci`, `wsl_v5` and 0 linter issues across entire codebase | M5 | Acceptance |
| F21 | Race Safety & Fuzz Harness Verification | Verify `go test -race` (0 warnings) and 8/8 fuzz targets (`scripts/fuzz_all.go`) | M5 | Acceptance |
| F22 | Micro-Benchmark & Zero-Alloc Invariant Gate | Verify 0 B/op and 0 allocs/op on framing, varint, SIMD, and borrow hot paths | M5 | Acceptance |
| F23 | Adversarial Coverage Hardening (Tier 5) | White-box stress testing and gap mitigation by adversarial challengers | M5 | Phase 2 |

---

## 3. Milestones

### Implementation Track

| # | Name | Scope | Dependencies | Status |
|---|---|---|---|---|
| M1 | Core Protocol Frame & Codec Decomposition | `proto/h2`, `proto/h3`, `proto/compress` | None | DONE |
| M2 | HTTP Message Model & Parser Modularization | `proto/http`, `proto/http/stackless` | None | DONE |
| M3 | Client Protocol Engine Decomposition | `client`, `client/h1`, `client/h2`, `client/h3` | M1, M2 | DONE |
| M4 | Server Protocol Engine Decomposition | `server/h1`, `server/h2`, `server/h3` | M1, M2 | PLANNED |
| M5 | Final Quality Invariants & Acceptance Gate | Codebase-wide (`./...`), `.golangci.yml` | M3, M4, MT1 | PLANNED |

### E2E Testing Track (Parallel)

| # | Name | Scope | Dependencies | Status |
|---|---|---|---|---|
| MT1 | E2E Test Suite & Multi-Tier Test Harness | End-to-end opaque-box test suite across H1/H2/H3 protocols | None | DONE |

---

## 4. Interface Contracts

### 4.1 Client H2 Multiplexer ↔ Protocol Frames (`client/h2` ↔ `proto/h2`)
- `client/h2` consumes `proto/h2` frame constructors and serializers.
- Re-exports in `client/h2/export.go` must maintain exact alias signatures (`HPACK`, `AcquireHPACK`, `FrameType`, `Frame`, `AcquireFrame`, `HeaderField`, `FrameHeaders`, `Settings`, `Data`, `WindowUpdate`).

### 4.2 Client H3 QUIC Adapter ↔ Protocol Codecs (`client/h3` ↔ `proto/h3`)
- `client/h3` consumes `proto/h3.QPACKCodec`, `proto/h3.Settings`, and frame header decoders.
- Re-exports in `client/h3/export.go` must maintain exact type aliases (`Settings`, `QPACKCodec`, `NewQPACKCodec`).

### 4.3 Downstream Consumer Contract (`aoni` ↔ `mach`)
- `client/h1.ClientConn`: `Do(ctx, req, res)`, `Close()`
- `client/h2.Conn`: `Do(ctx, req, res)`, `Write(ctx)`, `CanOpenStream()`, `Close()`, `Closed()`
- `client/h3.ClientConn`: `Do(ctx, req, resp, headerOrder)`, `DoScoped(...)`, `IsClosed()`, `Close()`
- `server/h1.ConnHandler`: `ServeConn(net.Conn)`
- `server/h2.ServerConn`: `NewServerConn(net.Conn, handler)`, `Serve()`, `Release()`
- `server/h3.ServerConn`: `NewServerConn(quicConn, handler)`, `Serve()`

---

## 5. Code Layout

### Target File Organization

```text
d:/CodingProjects/mach/
├── client/
│   ├── pool.go
│   ├── h1/
│   │   └── conn.go
│   ├── h2/
│   │   ├── conn.go               (struct Conn, NewConn, exported API facade)
│   │   ├── stream_table.go       (stream table open-addressing & overflow buckets)
│   │   ├── flow_control.go       (connection & stream window accounting)
│   │   ├── read_loop.go          (socket read loop & frame demuxing)
│   │   ├── write_loop.go         (socket write loop & SPSC ring batching)
│   │   ├── request_writer.go     (request framing, DATA chunking, 100-continue)
│   │   ├── headers.go            (HPACK encoding/decoding & forbidden filters)
│   │   ├── push.go               (server push promise handling)
│   │   ├── dialer.go             (network dialing & connection factory)
│   │   ├── context.go            (stream context & lifecycle state)
│   │   ├── export.go             (downstream type aliases)
│   │   └── conn_test.go
│   └── h3/
│       ├── conn.go               (struct ClientConn, constructor, lifecycle)
│       ├── control.go            (control stream & unidirectional stream demux)
│       ├── request.go            (Do, DoScoped, request sending)
│       ├── response.go           (readResponse, readResponseScoped)
│       ├── export.go             (downstream type aliases)
│       └── conn_test.go
├── proto/
│   ├── compress/
│   │   ├── compress.go           (compression levels, common helpers)
│   │   ├── gzip.go               (gzip pool, stackless writer, helpers)
│   │   ├── flate.go              (deflate pool, stackless writer, helpers)
│   │   ├── brotli.go
│   │   ├── zstd.go
│   │   └── compress_test.go
│   ├── h2/
│   │   ├── frame.go              (Frame interface, FrameType, FrameFlags, pooling)
│   │   ├── header.go             (FrameHeader serializer/deserializer)
│   │   ├── frame_data.go         (Data frame implementation)
│   │   ├── frame_headers.go      (Headers frame implementation)
│   │   ├── frame_control.go      (Ping, GoAway, RstStream, Priority frames)
│   │   ├── frame_window.go       (WindowUpdate frame implementation)
│   │   ├── frame_ext.go          (Continuation, PushPromise frames)
│   │   ├── frame_pool.go         (slab allocators for POD frames)
│   │   ├── settings.go           (Settings frame implementation)
│   │   ├── errors.go             (RFC 9113 error codes)
│   │   ├── utils.go
│   │   ├── overlay/
│   │   │   └── frame.go          (zero-alloc in-situ frame overlays)
│   │   └── *_test.go
│   ├── h3/
│   │   ├── qpack.go              (QPACKCodec struct, constructor, error handling)
│   │   ├── qpack_client.go       (client-side request encode & response decode)
│   │   ├── qpack_server.go       (server-side request decode & response encode)
│   │   ├── qpack_rules.go        (RFC 9114 forbidden header checks)
│   │   ├── frames.go             (frame types, unidirectional streams, settings)
│   │   ├── errors.go             (RFC 9114 & RFC 9204 error codes)
│   │   └── *_test.go
│   └── http/
│       ├── header.go             (base header, RequestHeader, ResponseHeader structs)
│       ├── header_parse.go       (first line, header loop, SIMD boundary scanning)
│       ├── header_fields.go      (field accessors: Peek, Set, Add, Del, All)
│       ├── header_cookies.go     (cookie parse, format, iteration)
│       ├── header_trailers.go    (trailer handling & validation)
│       ├── header_scoped.go      (scoped zero-alloc methods)
│       ├── headerscanner.go      (SIMD header scanner)
│       ├── headers.go            (standard header constants)
│       ├── request.go            (Request struct, URI accessors)
│       ├── request_body.go       (in-memory body methods)
│       ├── request_stream.go     (streaming request body)
│       ├── request_wire.go       (wire I/O & vectored writes)
│       ├── request_forms.go      (form parsing & multipart)
│       ├── response.go           (Response struct, status & addr accessors)
│       ├── response_body.go      (in-memory body methods)
│       ├── response_stream.go    (streaming response body & SendFile)
│       ├── response_wire.go      (wire I/O & write compression)
│       ├── body_chunked.go       (chunked transfer coding reader/writer)
│       ├── body_identity.go      (identity/fixed-size body reading/copying)
│       ├── body_compress.go      (compressed stream adapters)
│       ├── multipart.go          (multipart encoders/decoders)
│       ├── pool.go               (sync.Pool and per-P allocators)
│       ├── tls.go
│       └── *_test.go
├── server/
│   ├── h1/
│   │   ├── conn.go               (ConnHandler, ServeConn, Per-P storage)
│   │   ├── request.go            (request parser, SIMD scanner, hijack, early hints)
│   │   ├── response.go           (WriteResponse serializer)
│   │   ├── chunked.go            (chunked reader/writer)
│   │   ├── status.go             (status line serializer)
│   │   └── *_test.go
│   ├── h2/
│   │   ├── server_conn.go        (ServerConn struct, constructor, lifecycle)
│   │   ├── dispatch.go           (frame demuxing, settings, ping, data)
│   │   ├── headers.go            (pseudo-header validation, extended CONNECT)
│   │   ├── stream.go             (serverStream, handler dispatch)
│   │   ├── response.go           (response serialization, HPACK, DATA chunking)
│   │   └── *_test.go
│   └── h3/
│       ├── server_conn.go        (ServerConn struct, constructor, lifecycle)
│       ├── dispatch.go           (unidirectional stream router)
│       ├── stream.go             (request handling)
│       └── *_test.go
```

---

## 6. Invariants & Quality Standards

### 6.1 BSD License Header Invariant
Every `.go` file in the repository MUST start with the exact 3-line BSD header followed by a blank line:
```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ...
```

### 6.2 Documentation & RFC Citations
All exported types, functions, methods, interfaces, and constants MUST have comprehensive docstrings adhering to Go conventions and citing authoritative RFC specifications:
- HTTP/1.1: RFC 9112 (syntax, chunked encoding), RFC 9110 (semantics, status codes, methods, trailers).
- HTTP/2: RFC 9113 (framing, streams, flow control, error codes), RFC 7541 (HPACK).
- HTTP/3: RFC 9114 (framing, control streams, error codes), RFC 9204 (QPACK), RFC 9000 (QUIC), RFC 9221 (Datagrams).
- Compression: RFC 8878 (Zstandard), RFC 1952 (Gzip), RFC 1951 (Deflate), RFC 7932 (Brotli).
Docstrings must clearly state concurrency expectations (e.g., whether thread-safe, lock-free, or single-goroutine) and lifecycle rules (e.g., pooling, borrowing lifetimes).

### 6.3 Linter Rules & Formatting Invariants
Target `.golangci.yml` must match `foundation` and `aoni`:
- Formatters: `gofumpt` (with `extra: group-params: true`), `golines` (`max-len: 120`), `gci` (standard, default, `prefix(github.com/lemon4ksan/mach)`).
- Linters: `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, `revive` (with `exported: true`), `bodyclose`, `nilerr`, `noctx`, `errorlint`, `protogetter`, `gocritic`, `prealloc`, `perfsprint`, `wsl_v5`.

### 6.4 Zero-Allocation Hot Paths & Silicon Performance
- Zero heap allocations (`0 B/op, 0 allocs/op`) must be preserved on:
  1. `proto/h2/overlay`: In-situ binary frame decoding (`BenchmarkInSituOverlay`).
  2. `server/h3`: Frame header varint packing (`BenchmarkH3_FrameHeaderPack`).
  3. `proto/http`: Scoped borrow request/response pipeline (`BenchmarkFullPipeline_ScopedBorrow`).
  4. `proto/h2` & `proto/http`: Per-goroutine and Per-P frame pool acquisitions (`BenchmarkAcquireRelease_PerGoroutinePool_Parallel`, `BenchmarkPool_PerPStorage_Parallel`).
- Invariant protection: Retain `_ cpu.CacheLinePad` on atomic counters in `client/h2/conn.go` to prevent SMP false sharing. Retain `ringbuf.SPSCRingBuffer` for lock-free frame batching.
