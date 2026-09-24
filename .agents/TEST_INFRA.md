# mach Protocol Engine — E2E Test Infrastructure Specification

## 1. Architectural Overview & Test Philosophy

The `mach` E2E test harness is an **opaque-box, requirement-driven test suite** designed to validate the protocol engines across **HTTP/1.1 (RFC 9112)**, **HTTP/2 (RFC 9113)**, and **HTTP/3 over QUIC (RFC 9114 / RFC 9000)**. 

The test harness exercises the public API surface exclusively from the perspective of an external downstream consumer (such as `github.com/lemon4ksan/aoni`), ensuring zero leakage of internal implementation details and verifying protocol invariants across live network sockets and loopback pipes.

The test architecture strictly adheres to a **4-tier methodology**:

```
+-------------------------------------------------------------------------+
| Tier 4: Real-World Application Scenarios (High Concurrency, Burst, Pool)|
+-------------------------------------------------------------------------+
| Tier 3: Cross-Feature Combinations (Pairwise protocol interactions)      |
+-------------------------------------------------------------------------+
| Tier 2: Boundary & Corner Cases (Limits, empty frames, zero window, DoS)|
+-------------------------------------------------------------------------+
| Tier 1: Feature Coverage (Primary methods, status codes, framing, RFCs) |
+-------------------------------------------------------------------------+
```

---

## 2. Multi-Tier Test Methodology

### Tier 1: Feature Coverage (>=5 test cases per protocol)
Validates standard protocol behaviors against RFC specifications:
- **HTTP/1.1**: GET request parsing & query decoding, POST/PUT with Content-Length body, keep-alive connection reuse, chunked transfer encoding, 100-continue expectation negotiation, connection hijacking (raw protocol upgrade), and 103 Early Hints.
- **HTTP/2**: HPACK header compression, DATA frame streaming, multi-stream multiplexing on a single connection, stream cancellation via RST_STREAM, flow control WINDOW_UPDATE handling, and custom header ordering preservation.
- **HTTP/3**: QPACK header compression over QUIC bidirectional streams, DATA frames, multi-stream multiplexing over QUIC, trailing headers (trailers), scoped memory borrowing (`DoScoped`), and SETTINGS exchange on unidirectional control streams.

### Tier 2: Boundary & Corner Cases (>=5 test cases per protocol)
Stresses protocol edges, limit conditions, and malicious or degenerate inputs:
- **HTTP/1.1**: Max body size enforcement, missing Host header (RFC 9112 §3.2 violation), leading CRLF tolerance (RFC 9112 §2.2), request smuggling mitigation (forcing close on dual Transfer-Encoding & Content-Length), zero-length bodies, and large header blocks.
- **HTTP/2**: Zero-length DATA frames with END_STREAM, stream ID saturation boundary, flow control zero-window stalling, consecutive control frame limits (RFC 9113 §10.5 DoS mitigation), PING/ACK exchange cycles, and graceful GOAWAY shutdown.
- **HTTP/3**: Zero-length DATA frames, unknown frame types ignored (RFC 9114 §7.2.8), reserved H2 setting IDs rejected with H3_SETTINGS_ERROR (RFC 9114 §7.2.4.1), control stream missing initial SETTINGS (H3_MISSING_SETTINGS), oversized headers (>4KB) exercising pooled buffers, and oversized payloads (>32KB) exercising buffer pools.

### Tier 3: Cross-Feature Combinations (Pairwise Interactions)
Validates interactions between coupled protocol mechanisms:
- **HTTP/1.1**: Chunked transfer encoding + Keep-Alive pipelining; Expect 100-Continue + Large body + Keep-Alive reuse; Early Hints (103) + Connection: close; Bidirectional chunked streaming.
- **HTTP/2**: Multiplexed stream execution combined with concurrent stream resets; Large multi-chunk DATA streaming coupled with dynamic WINDOW_UPDATE flow control; Multiplexed streams with trailing headers; Abrupt connection disconnect during in-flight multiplexed execution.
- **HTTP/3**: Concurrent bidirectional streams each returning independent trailing headers; Scoped borrow memory recycling (`DoScoped`) with multi-chunk large payload streaming; 100 Continue informational response followed by final response and DATA frames; Context cancellation during active stream reads.

### Tier 4: Real-World Application Scenarios
Simulates realistic, high-throughput consumer workloads:
- **HTTP/1.1**: High-throughput keep-alive pipeline with hundreds of sequential requests exercising per-P buffer reuse; Multi-megabyte payload transfer under load.
- **HTTP/2**: High-concurrency burst with 100 parallel streams on a single HTTP/2 connection; Parallel large-payload uploads/downloads across concurrent streams.
- **HTTP/3**: High-concurrency burst with 50 parallel QUIC streams completing requests simultaneously; Multi-chunk large payload transfers (128KB+) over QUIC bidirectional streams.
- **Client Pool**: Global connection pool manager (`client.PoolManager[T]`) testing connection reuse, host isolation, max conns per host capping, and thread-safe concurrent Get/Put.

---

## 3. Directory Layout

All E2E test files reside in the dedicated test package `tests/e2e`:

```text
d:/CodingProjects/mach/
└── tests/
    └── e2e/
        ├── helpers_test.go      # Common test harness, TLS certs, loopback listeners, mock servers
        ├── h1_test.go           # HTTP/1.1 test suite (Tiers 1 - 4)
        ├── h2_test.go           # HTTP/2 test suite (Tiers 1 - 4)
        ├── h3_test.go           # HTTP/3 test suite (Tiers 1 - 4)
        └── pool_test.go         # Client connection pool manager suite
```

Every test file belongs to package `e2e_test` and includes the mandatory 3-line BSD license header:
```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
```

---

## 4. Test Runner Command & Verification

The test harness is executed via the standard Go test toolchain with race detection enabled:

```powershell
go test -v -race -timeout 120s ./tests/e2e/...
```

### Coverage & Pass Invariants
- **100% Pass Rate**: Zero test failures across all tiers.
- **Race Safety**: Zero race detector warnings (`-race`).
- **Opaque API Discipline**: Zero access to package-internal variables; all interactions use exported packages (`client`, `client/h1`, `client/h2`, `client/h3`, `server/h1`, `server/h2`, `server/h3`, `proto/http`).
