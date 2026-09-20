<div align="center">

# mach

### Silicon-Speed Protocol Engines & Transport Layers for Go 1.27+

_«Aerodynamic protocol mechanics at Mach speed — where zero heap allocation meets wire throughput»_

[![Go Version](https://img.shields.io/badge/go-1.27%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/mach)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)
[![Zero-Alloc](https://img.shields.io/badge/memory-0%20B%2Fop%20%7C%200%20allocs-brightgreen?style=flat-square)](#honest-benchmarks)
[![In-Situ Framing](https://img.shields.io/badge/h2%20overlay-0.49%20ns%2Fop-blueviolet?style=flat-square)](#1-http2-in-situ-binary-overlay-vs-legacy-decode)
[![SIMD Wire Scanning](https://img.shields.io/badge/simd-2.04%20GB%2Fs-orange?style=flat-square)](#2-http11-simd-wire-scanning--header-parsing)
[![Fuzz Verified](https://img.shields.io/badge/fuzzing-Continuous%208%2F8%20Targets-success?style=flat-square)](scripts/fuzz_all.go)

**mach** is the low-level, zero-allocation protocol engine powering the [`aoni`](https://github.com/lemon4ksan/aoni) client ecosystem and [`sein`](https://github.com/lemon4ksan/sein) server frameworks. It provides raw, silicon-optimized bytes-to-frames mechanics: SIMD header delimiter scanning, sub-nanosecond binary frame overlays, stream multiplexing, HPACK/QPACK table compression, and RFC-conforming protocol state machines.

</div>

---

## Architecture: The Zero-Drag Manifesto

Named after the Mach number and aerodynamic concept of **Zero Drag**, `mach` eliminates computational friction from network pipelines:

* **Agnostic Wire Core**: The engine accepts raw bytes from a socket, enforces RFC constraints, and emits protocol frames. It contains zero application routing, middleware, or business logic.
* **Forever-Frozen Standard**: Mechanics strictly implement IETF RFCs (HTTP/1.1 RFC 9112, HTTP/2 RFC 9113, HPACK RFC 7541, HTTP/3 RFC 9114, QPACK RFC 9204).
* **Symmetric Isolation**: Client and server architectures require fundamentally distinct memory layouts, buffer reuse patterns, and concurrency lifecycles. `mach` isolates them into distinct packages (`mach/client` vs `mach/server`).
* **Zero Heap Allocation on Hot Path**: Every frame, header slice, and URI component is read and recycled using multi-tiered arenas, Per-P pools, and scoped borrow semantics.

---

## Protocol Index

```text
mach/
├── proto/                    # RFC-conforming wire mechanics (client/server agnostic)
│   ├── http/                 # HTTP/1.1 wire parser, SIMD scanner, zero-alloc Request/Response
│   │   └── stackless/        # Goroutine-free async I/O dispatch
│   ├── h2/                   # HTTP/2 framing (RFC 9113), priority trees, flow control
│   │   └── overlay/          # Sub-nanosecond In-Situ binary frame overlay
│   ├── h3/                   # HTTP/3 framing (RFC 9114), QPACK (RFC 9204), datagrams, capsules
│   └── compress/             # Streaming Brotli, Gzip, Deflate, Zstd decompressors
├── client/                   # Client protocol engines (optimized for `aoni`)
│   ├── h1/                   # HTTP/1.1 Client Engine. Connection reuse, chunked transfers
│   ├── h2/                   # HTTP/2 Client Engine. Concurrent stream multiplexing
│   └── h3/                   # HTTP/3 Client Engine. QUIC stream lifecycle, 0-RTT transactions
├── server/                   # Server protocol engines (optimized for `sein`)
│   ├── h1/                   # HTTP/1.1 Server Engine. High-throughput edge listener
│   ├── h2/                   # HTTP/2 Server Engine. Prioritized stream dispatch
│   └── h3/                   # HTTP/3 Server Engine. Connection ID routing
├── fsm/                      # Finite State Machines
│   └── h2/                   # Strict RFC 9113 §5.1 HTTP/2 stream transition verifier
├── scripts/                  # CI & Fuzzing suites
│   └── fuzz_all.go           # Heavy automated fuzz harness (8/8 protocol targets)
└── x/                        # Experimental extensions
    └── raptor/               # Ultra-low-latency layer-7 proxy and tunneling core
```

---

## Honest Benchmarks

All benchmarks measured on bare-metal hardware (`12th Gen Intel Core i5-12400F @ 4.40 GHz`, Go 1.27, Windows x86_64).

### 1. HTTP/2 In-Situ Binary Overlay vs Legacy Decode

Instead of allocating and copying wire bytes into Go structs, `proto/h2/overlay` casts raw frame buffers into zero-copy in-situ memory views:

| Implementation | Speed | Throughput / Overhead | Allocations |
| :--- | :--- | :--- | :--- |
| **Legacy Struct Heap Decode** | `17.17 ns/op` | 48 B/op | 1 allocs/op |
| **`mach/proto/h2/overlay`** | **`0.49 ns/op`** | **0 B/op** | **0 allocs/op** |
| **Improvement** | **35.0x faster** | **100% memory eliminated** | **Zero garbage created** |

---

### 2. HTTP/1.1 SIMD Wire Scanning & Header Parsing

Accelerated vector scanners scan `\r\n` and colon delimiters using hardware SIMD registers:

| Operation | Latency | Throughput | Allocations |
| :--- | :--- | :--- | :--- |
| **SIMD Header Wire Scan** | **`323.6 ns/op`** | **`2,042.61 MB/s` (~2.04 GB/s)** | 3 allocs/op |
| **Response Header Full SIMD Parse** | **`1,481.0 ns/op`** | **`446.28 MB/s`** | 25 allocs/op |
| **Scoped Borrow Pipeline** | **`106.0 ns/op`** | Wire decoding + Header mapping | **0 allocs/op** |
| Legacy Copy Pipeline | `121.0 ns/op` | 80 B/op | 1 allocs/op |

---

### 3. Multi-Tiered Memory Pool Latency

Comparison of object recycling strategies across parallel execution threads:

| Pool Strategy | Latency | Memory Overhead | Allocations |
| :--- | :--- | :--- | :--- |
| **Per-Goroutine Ring Pool** | **`1.889 ns/op`** | **0 B/op** | **0 allocs/op** |
| **Per-P Storage (`foundation/pool`)** | **`4.390 ns/op`** | **0 B/op** | **0 allocs/op** |
| Standard `sync.Pool` | `4.574 ns/op` | 0 B/op | 0 allocs/op |
| Connection-Bound Frame Pool (Ping/WU/Rst) | **`10.510 ns/op`** | **0 B/op** | **0 allocs/op** |

---

### 4. HTTP/3 & QPACK Wire Performance

| Component | Operation | Latency | Memory | Allocations |
| :--- | :--- | :--- | :--- | :--- |
| **HTTP/3 Frame Header Pack** | Pack varint header | **`2.812 ns/op`** | **0 B/op** | **0 allocs/op** |
| **QPACK Response Decode** | Decode headers into map | **`943.8 ns/op`** | 693 B/op | 13 allocs/op |
| **QPACK Request Decode** | Decode headers into map | **`1,056.0 ns/op`** | 696 B/op | 9 allocs/op |
| **QPACK Request Encode** | Encode dynamic headers | **`1,962.0 ns/op`** | 2,208 B/op | 34 allocs/op |

---

## Engineering Invariants

1. **Strict Zero-Allocation Hot Path**:
   Connection handshakes, stream processing, frame serialization, and URI parsing recycling must produce zero heap allocations during steady-state traffic.
2. **Cache-Line Alignment (64 Bytes)**:
   Core data-in-motion structs (`Request`, `Response`, `FrameHeader`) are packed and padded to 64-byte boundaries to prevent cross-core false sharing on symmetric multi-processor (SMP) architectures.
3. **No Hidden Background Goroutines**:
   State transitions are driven synchronously by the caller's thread or explicit external reactors. No unmonitored goroutine leaks.
4. **RFC Invariant Integrity**:
   No non-standard extensions or framework-specific compromises in `mach/proto`.

---

## Continuous Fuzzing & Verification

The protocol mechanics in `mach` are continuously verified against malformed frame injections and protocol corruption using Go native fuzzing:

```bash
# Run the complete multi-protocol fuzzing suite across all 8 targets
go run ./scripts/fuzz_all.go -fuzztime=5s
```

Run test suite with the race detector enabled:
```bash
go test -v -race ./...
```

Run micro-benchmarks with memory profiling:
```bash
go test -bench=. -benchmem ./...
```

---

## License

This project is licensed under the **BSD 3-Clause License**. See the [LICENSE](LICENSE) file for details.

Copyright (c) 2026 Lemon4ksan. All rights reserved.
