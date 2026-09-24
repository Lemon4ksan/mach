# Milestone M1: Verification, Standards & Invariants Gate — Explorer 3 Report

## 1. Observation

### 1.1 Scope and File Inventory Observed
Through filesystem inspection and tree exploration, Milestone M1 comprises the following protocol packages:
- `proto/compress`: 4 Go source files
  - `compress.go` (536 lines, 12,573 bytes): monolithic gzip/flate pools, stackless wrappers, helpers
  - `brotli.go` (89 lines, 2,281 bytes): brotli compression wrappers
  - `zstd.go` (86 lines, 2,089 bytes): zstd compression wrappers
  - `compress_test.go` (191 lines, 4,715 bytes): tests for all 4 compression algorithms and HTTP request/response uncompressed body methods
- `proto/h2`: 13 Go source files + 1 subdirectory
  - `errors.go` (168 lines, 6,255 bytes): RFC 9113 §7 ErrorCode and Err* definitions
  - `frame.go` (161 lines, 4,977 bytes): FrameType, FrameFlags, Frame interface, sync.Pool allocators
  - `frame_bench_test.go` (30 lines, 608 bytes): BenchmarkFrameHeader_ReadFrame
  - `frame_pool.go` (190 lines, 4,994 bytes): ConnectionFramePool off-heap slab allocators for POD frames
  - `frame_pool_bench_test.go` (92 lines, 1,974 bytes): 7 micro-benchmarks comparing sync.Pool vs ConnectionFramePool
  - `frame_pool_test.go` (150 lines, 3,383 bytes): unit tests for ConnectionFramePool (POD, non-POD, nil receiver, concurrency)
  - `frame_stress_test.go` (129 lines, 3,115 bytes): adversarial stress tests (varint, Huffman, frame header unpack)
  - `frames.go` (492 lines, 15,178 bytes): monolithic frame payload structs (Data, Headers, GoAway, Ping, Priority, PushPromise, RstStream, WindowUpdate, Continuation)
  - `frames_test.go` (308 lines, 6,741 bytes): unit tests for frame headers, bounds, roundtrips, and arena allocation
  - `fuzz_test.go` (90 lines, 1,693 bytes): FuzzHPACKDecode, FuzzFrameRead
  - `header.go` (295 lines, 7,239 bytes): FrameHeader struct, PackFrameHeader, UnpackFrameHeader, ReadFrameFrom, WriteTo
  - `settings.go` (222 lines, 8,062 bytes): Settings frame, setting IDs, validation per RFC 9113 §6.5
  - `utils.go` (232 lines, 6,083 bytes): HTTP/2 client preface, PerformHandshake, ToLowerCopy, FasthttpResponseHeaders
- `proto/h2/overlay`: 2 Go source files
  - `frame.go` (119 lines, 3,006 bytes): zero-alloc in-situ frame overlays (Frame, DataFrame, HeadersFrame)
  - `frame_test.go` (105 lines, 1,955 bytes): BenchmarkLegacyStructDecode, BenchmarkInSituOverlay
- `proto/h3`: 6 Go source files
  - `errors.go` (113 lines, 6,035 bytes): RFC 9114 §8.1 ErrorCode, RFC 9204 §6 ErrorCode, Err* definitions
  - `frames.go` (221 lines, 6,665 bytes): HTTP/3 frame types, unidirectional stream types, Settings struct, ReadFrameHeader
  - `frames_test.go` (91 lines, 2,284 bytes): Settings encode/decode, ReadFrameHeader, AppendHeadersHeader, AppendDataHeader
  - `fuzz_test.go` (28 lines, 629 bytes): FuzzH3FrameHeaderRead
  - `qpack.go` (690 lines, 15,861 bytes): monolithic QPACKCodec (client encode/decode, server encode/decode, forbidden headers rules)
  - `qpack_test.go` (458 lines, 11,704 bytes): request/response encode/decode, custom order sequence, forbidden filtering, RFC 9204 Appendix B examples, micro-benchmarks
- Direct Downstream Consumer Modules:
  - `server/h3/h3_bench_test.go`: `BenchmarkH3_FrameHeaderPack`, `BenchmarkQPACK_EncodeResponseHeaders`, `BenchmarkQPACK_DecodeRequestHeaders`
  - `client/h2/export.go`: re-exports `proto/h2` symbols (`FrameType`, `Frame`, `AcquireFrame`, `ReleaseFrame`, `FrameHeaders`, `Headers`, `FrameHeader`, `AcquireFrameHeader`, `ReleaseFrameHeader`, `FrameSettings`, `Settings`, `ReadFrameFrom`, `FrameData`, `Data`, `FrameWindowUpdate`, `WindowUpdate`)
  - `client/h3/export.go`: re-exports `proto/h3` symbols (`QPACKCodec`, `NewQPACKCodec`, `FrameTypeHeaders`, `ReadFrameHeader`)
  - `scripts/fuzz_all.go`: runs 3 M1 fuzz targets (`FuzzHPACKDecode`, `FuzzFrameRead`, `FuzzH3FrameHeaderRead`) among its 8 targets

### 1.2 Baseline Execution Results Observed
Execution on local hardware (`12th Gen Intel(R) Core(TM) i5-12400F @ 4.40 GHz`, Windows x86_64, Go 1.27):

1. Unit Tests (`go test -v ./proto/compress/... ./proto/h2/... ./proto/h3/...`):
   - `github.com/lemon4ksan/mach/proto/compress`: PASS (6 tests, 6 subtests)
   - `github.com/lemon4ksan/mach/proto/h2`: PASS (15 test functions, 14 subtests)
   - `github.com/lemon4ksan/mach/proto/h2/overlay`: `testing: warning: no tests to run` (0 unit tests present!)
   - `github.com/lemon4ksan/mach/proto/h3`: PASS (10 test functions, 1 subtest)
2. Race Detector (`go test -race ./proto/compress/... ./proto/h2/... ./proto/h3/...`):
   - All packages pass with 0 warnings, 0 data races, 0 panics.
3. Fuzz Harness (`go test -fuzz=^FuzzHPACKDecode$ -fuzztime=2s ./proto/h2`):
   - Passed with 0 errors, 0 panics.
4. Linter (`golangci-lint run ./proto/compress/... ./proto/h2/... ./proto/h3/...`):
   - 0 issues reported with current `.golangci.yml`. Note: `.golangci.yml` currently has `revive: exported: disabled: true`.
5. Micro-Benchmarks (`go test "-bench=." "-benchmem"`):
   - `BenchmarkInSituOverlay`: **1.320 ns/op, 0 B/op, 0 allocs/op** (vs legacy struct decode: 117.9 ns/op, 48 B/op, 1 allocs/op)
   - `BenchmarkAcquireRelease_PerGoroutinePool_Parallel`: **2.558 ns/op, 0 B/op, 0 allocs/op**
   - `BenchmarkAcquireRelease_ConnPool_Ping`: **24.70 ns/op, 0 B/op, 0 allocs/op**
   - `BenchmarkAcquireRelease_ConnPool_WindowUpdate`: **22.81 ns/op, 0 B/op, 0 allocs/op**
   - `BenchmarkAcquireRelease_ConnPool_RstStream`: **22.58 ns/op, 0 B/op, 0 allocs/op**
   - `BenchmarkH3_FrameHeaderPack` (`server/h3`): **5.394 ns/op, 0 B/op, 0 allocs/op**
   - `BenchmarkQPACKEncodeRequestHeaders`: **9,470 ns/op, 2,208 B/op, 34 allocs/op**
   - `BenchmarkQPACKDecodeResponseHeaders`: **3,036 ns/op, 709 B/op, 13 allocs/op**
   - `BenchmarkQPACK_EncodeResponseHeaders` (`server/h3`): **3,341 ns/op, 1,736 B/op, 29 allocs/op**
   - `BenchmarkQPACK_DecodeRequestHeaders` (`server/h3`): **2,650 ns/op, 712 B/op, 9 allocs/op**

---

## 2. Logic Chain

### 2.1 Test Suite Mapping & Gap Analysis
1. From Section 1.1 & 1.2, all unit tests across `proto/compress`, `proto/h2`, and `proto/h3` pass cleanly.
2. However, `proto/h2/overlay` currently outputs `testing: warning: no tests to run` because `frame_test.go` defines only benchmarks (`BenchmarkLegacyStructDecode` and `BenchmarkInSituOverlay`) and zero `Test*` functions.
3. Therefore, `proto/h2/overlay` has an unverified test gap. In-situ methods (`Length()`, `Type()`, `Flags()`, `StreamID()`, `Payload()`, `IsValid()`, `DataFrame.IsEndStream()`, `DataFrame.IsPadded()`, `DataFrame.PadLength()`, `DataFrame.Data()`, `HeadersFrame.IsEndStream()`, `HeadersFrame.IsEndHeaders()`, `HeadersFrame.HeaderBlockFragment()`) lack direct assertion tests.
4. Challengers and Worker must add a comprehensive unit test suite `TestInSituOverlay_ValidFrames`, `TestInSituOverlay_TruncatedFrames`, and `TestInSituOverlay_PaddedDataFrame` to close this gap.

### 2.2 Micro-Benchmark and Silicon Performance Invariant Gate
1. The project specification (ORIGINAL_REQUEST.md R3 and PROJECT.md §6.4) dictates zero heap allocations (`0 B/op, 0 allocs/op`) on framing overlays, varint packing, and pool acquisitions.
2. In Section 1.2:
   - `BenchmarkInSituOverlay` produces `0 B/op, 0 allocs/op`.
   - `BenchmarkAcquireRelease_PerGoroutinePool_Parallel` produces `0 B/op, 0 allocs/op`.
   - `BenchmarkAcquireRelease_ConnPool_*` produces `0 B/op, 0 allocs/op`.
   - `BenchmarkH3_FrameHeaderPack` produces `0 B/op, 0 allocs/op`.
3. If any refactoring in M1 causes any of these to allocate even 1 byte (`>0 B/op` or `>0 allocs/op`), it violates the core invariant of `mach`.

### 2.3 Verification Workflow & Criteria Across Roles
1. Each agent role in Milestone M1 must execute specific commands with explicit pass criteria:
   - **Worker**: must run incremental tests, race detection, benchmarks, and linter before submitting handoff.
   - **Reviewers**: must verify 3-line BSD header invariant, single-responsibility modular decomposition, RFC citations in docstrings, and strict signature preservation for downstream consumers.
   - **Challengers**: must run adversarial stress tests, native fuzzing, zero-allocation regressions, and cover the `proto/h2/overlay` test gap.
   - **Auditor**: must execute the end-to-end acceptance gate (`go test -race ./...`, `fuzz_all.go`, `golangci-lint`, and git diff file ownership check).

### 2.4 Worker File Ownership Boundaries
1. M1 is scoped strictly to `proto/h2`, `proto/h3`, and `proto/compress`.
2. Packages `client/`, `server/`, and `proto/http/` are assigned to Milestones M2, M3, and M4 respectively.
3. Therefore, any modification by Worker to files outside `proto/h2`, `proto/h3`, `proto/compress`, and Worker's own `.agents/` folder must be treated as a critical boundary violation.

### 2.5 Exported Symbol Docstring Checklist
1. All exported symbols in `proto/compress`, `proto/h2`, `proto/h2/overlay`, and `proto/h3` must have full idiomatic Go docstrings referencing authoritative RFCs (RFC 9113, RFC 7541, RFC 8441, RFC 9114, RFC 9204, RFC 1952, RFC 1951, RFC 7932, RFC 8878).
2. Currently, while types like `FrameHeader`, `Settings`, and `QPACKCodec` have docstrings, dozens of exported methods (e.g. `Headers.*`, `Data.*`, `Continuation.*`, `Ping.*`, `Priority.*`, `PushPromise.*`, `RstStream.*`, `WindowUpdate.*`, `Settings.*`, `FrameHeader.*`) and struct fields (e.g. `Settings.MaxStreams` in h2, `Settings.Other`, `Settings.MaxFieldSectionSize`, etc. in h3) completely lack docstrings.
3. Every exported entity must document:
   - Summary of purpose with authoritative RFC section citation
   - Concurrency contract (thread-safe, lock-free, or single-goroutine)
   - Lifecycle contract (pooling rules, release requirements, or memory borrowing)

---

## 3. Caveats
- No caveats: All test suites, fuzz harnesses, and micro-benchmarks across `proto/h2`, `proto/h3`, and `proto/compress` were inspected, compiled, and executed directly on the development environment.

---

## 4. Conclusion

Milestone M1 establishes the structural and invariant baseline for the entire repository. The exact test inventory, benchmark criteria, role verification matrices, worker write boundaries, and symbol docstring checklists defined below provide the complete roadmap for Worker, Reviewers, Challengers, and Auditor.

### 4.1 Master Test Suite Inventory

| Package | Test File | Test / Subtest Name | Purpose / Coverage Area |
|---|---|---|---|
| `proto/compress` | `compress_test.go` | `TestH1Engine_Compression_Gzip` | Gzip stream and byte buffer compression/decompression roundtrip |
| `proto/compress` | `compress_test.go` | `TestH1Engine_Compression_Brotli` | Brotli stream and byte buffer roundtrip |
| `proto/compress` | `compress_test.go` | `TestH1Engine_Compression_Zstd` | Zstandard byte buffer and stream decompression |
| `proto/compress` | `compress_test.go` | `TestH1Engine_Compression_Deflate` | Deflate / Inflate byte buffer and stream decompression |
| `proto/compress` | `compress_test.go` | `TestH1Engine_Response_BodyUncompressed` | HTTP response body decompression across gzip, br, zstd, identity, empty, and unsupported encoding |
| `proto/compress` | `compress_test.go` | `TestH1Engine_Request_BodyUncompressed` | HTTP request body decompression with gzip |
| `proto/h2` | `frames_test.go` | `TestFrameHeaderFlags` | Bitwise operations on FrameFlags (Add, Has, Del) |
| `proto/h2` | `frames_test.go` | `TestFrameHeaderBoundsCheck` | Maximum payload length bounds enforcement (`ErrPayloadExceeds`) |
| `proto/h2` | `frames_test.go` | `TestFrameHeaderParseHeaderSymmetry` | Wire header packing and unpacking symmetry |
| `proto/h2` | `frames_test.go` | `TestFramesSerializationRoundtrip` | Full wire serialization roundtrip for all 8 frame types |
| `proto/h2` | `frames_test.go` | `TestPaddingHelpers` | Addition and truncation of security padding (`cutPadding`/`addPadding`) |
| `proto/h2` | `frames_test.go` | `TestPingInvalidPayload` | Rejection of non-8-byte PING payloads per RFC 9113 §6.7 |
| `proto/h2` | `frames_test.go` | `TestRstStreamErrorFormatting` | Error formatting and reset lifecycle for RST_STREAM |
| `proto/h2` | `frames_test.go` | `TestGoAwayErrorFormatting` | Error formatting, stream ID, and error code verification for GOAWAY |
| `proto/h2` | `frames_test.go` | `TestContinuationFrame` | CONTINUATION frame header aggregation and reset |
| `proto/h2` | `frames_test.go` | `TestAcquireFrameInArena` | 0-alloc single-cycle off-heap arena construction for POD frames |
| `proto/h2` | `frame_pool_test.go` | `TestConnectionFramePool_AcquireRelease_POD` | Off-heap slab allocation and release for Ping, WindowUpdate, RstStream, Priority |
| `proto/h2` | `frame_pool_test.go` | `TestConnectionFramePool_AcquireRelease_NonPOD` | Transparent sync.Pool fallback for non-POD frames |
| `proto/h2` | `frame_pool_test.go` | `TestConnectionFramePool_NilReceiver` | Panic safety on nil `*ConnectionFramePool` receivers |
| `proto/h2` | `frame_pool_test.go` | `TestConnectionFramePool_ZeroCapacity_UsesDefault` | Capacity clamping to defaultConnPoolCapacity (256) |
| `proto/h2` | `frame_pool_test.go` | `TestConnectionFramePool_FreeAndReallocate` | Slot reuse verification within slab allocator |
| `proto/h2` | `frame_pool_test.go` | `TestConnectionFramePool_PerGoroutinePool` | 32 concurrent goroutines with independent slab pools |
| `proto/h2` | `frame_pool_test.go` | `TestConnectionFramePool_Release_Idempotent` | Double-release safety on ConnectionFramePool |
| `proto/h2` | `frame_stress_test.go` | `TestH2_FrameHeader_Adversarial` | Boundary packing and 50,000 fuzz unpacks on frame headers |
| `proto/h2` | `frame_stress_test.go` | `TestH2_Varint_Adversarial` | Boundary and fuzz testing of HPACK integer prefix decoding |
| `proto/h2` | `frame_stress_test.go` | `TestH2_Huffman_Adversarial` | Huffman encoding/decoding and corrupted stream fault-tolerance |
| `proto/h2` | `fuzz_test.go` | `FuzzHPACKDecode` | Native fuzzing of HPACK header field decoder |
| `proto/h2` | `fuzz_test.go` | `FuzzFrameRead` | Native fuzzing of HTTP/2 wire frame reader |
| `proto/h2/overlay` | `frame_test.go` | *(GAP: None)* | **VERIFICATION GAP**: Must add `TestInSituOverlay_*` |
| `proto/h3` | `frames_test.go` | `TestSettingsEncodeAndParse` | HTTP/3 SETTINGS frame serialization and parsing |
| `proto/h3` | `frames_test.go` | `TestReadFrameHeader` | Varint frame type and payload length reader |
| `proto/h3` | `frames_test.go` | `TestAppendFrameHeaders` | Zero-alloc varint appending for HEADERS and DATA frames |
| `proto/h3` | `fuzz_test.go` | `FuzzH3FrameHeaderRead` | Native fuzzing of HTTP/3 varint frame header parsing |
| `proto/h3` | `qpack_test.go` | `TestQPACKEncodeRequestHeaders` | Request header encoding and progressive decoding verification |
| `proto/h3` | `qpack_test.go` | `TestQPACKOrderedHeadersSequence` | RFC 9204 deterministic header ordering preservation |
| `proto/h3` | `qpack_test.go` | `TestQPACKDecodeResponseHeaders` | Response header decoding and status extraction |
| `proto/h3` | `qpack_test.go` | `TestQPACKDecodeResponseMissingStatus` | Rejection of responses missing `:status` pseudo-header |
| `proto/h3` | `qpack_test.go` | `TestQPACKEncodeExtendedCONNECTProtocolHeader` | RFC 8441 / RFC 9220 Extended CONNECT with `:protocol` |
| `proto/h3` | `qpack_test.go` | `TestIsForbiddenH3Header` | Validation of prohibited connection-specific headers |
| `proto/h3` | `qpack_test.go` | `TestQPACKForbiddenHeadersFilteringInEncode` | Automated stripping of forbidden headers during encoding |
| `proto/h3` | `qpack_test.go` | `TestRFC9204AppendixBExamples` | Conformance against RFC 9204 Appendix B.1 reference vectors |

---

### 4.2 Micro-Benchmark & Zero-Allocation Invariant Matrix

| Benchmark | Package | Invariant Target | Baseline Latency | Allocations Target | Purpose |
|---|---|---|---|---|---|
| `BenchmarkInSituOverlay` | `proto/h2/overlay` | **CRITICAL ZERO-ALLOC** | `< 2.0 ns/op` (baseline: 0.49 - 1.32 ns) | **0 B/op, 0 allocs/op** | In-situ zero-copy HTTP/2 binary frame parsing |
| `BenchmarkLegacyStructDecode` | `proto/h2/overlay` | Reference comparison | ~117 ns/op | 48 B/op, 1 allocs/op | Benchmark baseline for struct heap decoding |
| `BenchmarkAcquireRelease_PerGoroutinePool_Parallel` | `proto/h2` | **CRITICAL ZERO-ALLOC** | `< 4.0 ns/op` (baseline: 1.89 - 2.56 ns) | **0 B/op, 0 allocs/op** | Multi-goroutine lock-free slab frame allocation |
| `BenchmarkAcquireRelease_ConnPool_Ping` | `proto/h2` | **CRITICAL ZERO-ALLOC** | `< 30.0 ns/op` (baseline: 24.70 ns) | **0 B/op, 0 allocs/op** | Slab recycling of Ping frames |
| `BenchmarkAcquireRelease_ConnPool_WindowUpdate` | `proto/h2` | **CRITICAL ZERO-ALLOC** | `< 30.0 ns/op` (baseline: 22.81 ns) | **0 B/op, 0 allocs/op** | Slab recycling of WindowUpdate frames |
| `BenchmarkAcquireRelease_ConnPool_RstStream` | `proto/h2` | **CRITICAL ZERO-ALLOC** | `< 30.0 ns/op` (baseline: 22.58 ns) | **0 B/op, 0 allocs/op** | Slab recycling of RstStream frames |
| `BenchmarkAcquireRelease_SyncPool_Ping` | `proto/h2` | Fallback tracking | ~32 ns/op | **0 B/op, 0 allocs/op** | Standard sync.Pool frame acquisition |
| `BenchmarkAcquireRelease_SyncPool_WindowUpdate` | `proto/h2` | Fallback tracking | ~51 ns/op | **0 B/op, 0 allocs/op** | Standard sync.Pool frame acquisition |
| `BenchmarkAcquireRelease_SyncPool_RstStream` | `proto/h2` | Fallback tracking | ~30 ns/op | **0 B/op, 0 allocs/op** | Standard sync.Pool frame acquisition |
| `BenchmarkFrameHeader_ReadFrame` | `proto/h2` | Wire reader tracking | ~143 ns/op | 48 B/op, 1 allocs/op | Standard bufio frame header read |
| `BenchmarkH3_FrameHeaderPack` | `server/h3` | **CRITICAL ZERO-ALLOC** | `< 6.0 ns/op` (baseline: 2.81 - 5.39 ns) | **0 B/op, 0 allocs/op** | Varint packing for HTTP/3 frame headers |
| `BenchmarkQPACKEncodeRequestHeaders` | `proto/h3` | Memory ceiling | `< 10 µs/op` | `<= 2,208 B/op, <= 34 allocs` | QPACK dynamic request header compression |
| `BenchmarkQPACKDecodeResponseHeaders` | `proto/h3` | Memory ceiling | `< 3.5 µs/op` | `<= 709 B/op, <= 13 allocs` | QPACK progressive response header decompression |
| `BenchmarkQPACK_EncodeResponseHeaders` | `server/h3` | Memory ceiling | `< 4.0 µs/op` | `<= 1,736 B/op, <= 29 allocs` | Server QPACK response encoding |
| `BenchmarkQPACK_DecodeRequestHeaders` | `server/h3` | Memory ceiling | `< 3.0 µs/op` | `<= 712 B/op, <= 9 allocs` | Server QPACK request decoding |

---

### 4.3 Detailed Verification Commands & Criteria by Role

#### A. Worker (Execution & Self-Verification)
1. **Compilation & Unit Tests**:
   ```bash
   go test -v ./proto/compress/... ./proto/h2/... ./proto/h3/...
   ```
   *Pass Criteria*: Exit code 0, 0 test failures, all subtests pass.
2. **Race Detector**:
   ```bash
   go test -race ./proto/compress/... ./proto/h2/... ./proto/h3/...
   ```
   *Pass Criteria*: Exit code 0, 0 race warnings.
3. **Micro-Benchmarks & Zero-Allocation Gate**:
   ```bash
   go test "-bench=." "-benchmem" ./proto/h2/... ./proto/h3/... ./proto/compress/... ./server/h3
   ```
   *Pass Criteria*:
   - `BenchmarkInSituOverlay`: exactly `0 B/op` and `0 allocs/op`.
   - `BenchmarkAcquireRelease_PerGoroutinePool_Parallel`: exactly `0 B/op` and `0 allocs/op`.
   - `BenchmarkH3_FrameHeaderPack`: exactly `0 B/op` and `0 allocs/op`.
   - Latency within 10% of baseline.
4. **Downstream Integration Build**:
   ```bash
   go test ./client/h2/... ./client/h3/... ./server/h2/... ./server/h3/...
   ```
   *Pass Criteria*: Exit code 0; confirms no broken type aliases or modified exported signatures.
5. **Lint & Formatting**:
   ```bash
   golangci-lint run ./proto/compress/... ./proto/h2/... ./proto/h3/...
   ```
   *Pass Criteria*: 0 linter issues.

#### B. Reviewers (Standards, Signatures & Invariants)
1. **BSD License Header Invariant**:
   Every modified and newly created file MUST begin with:
   ```go
   // Copyright (c) 2026 Lemon4ksan All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.
   ```
   Followed by an empty line and `package ...`.
2. **Modular File Layout Invariant**:
   Confirm decomposition meets target architecture:
   - `proto/h2/frames.go` removed and decomposed into `frame_data.go`, `frame_headers.go`, `frame_control.go`, `frame_window.go`, `frame_ext.go`.
   - `proto/h3/qpack.go` decomposed into `qpack.go`, `qpack_client.go`, `qpack_server.go`, `qpack_rules.go`.
   - `proto/compress/compress.go` decomposed into `compress.go`, `gzip.go`, `flate.go`.
3. **Exported RFC Docstrings**:
   Inspect every symbol against the checklist in Section 4.5. All exported types, methods, fields, and constants must cite relevant RFC sections and declare concurrency and lifecycle models.
4. **API Stability**:
   Verify zero deletions or signature changes in exported functions and structs consumed by `client/h2/export.go` and `client/h3/export.go`.

#### C. Challengers (Adversarial Testing & Boundary Stress)
1. **Adversarial Stress Test**:
   ```bash
   go test -v -run=Adversarial ./proto/h2/...
   ```
   *Pass Criteria*: 50,000 frame header rounds, 20,000 varint rounds, and 10,000 corrupted Huffman rounds pass without panics or memory corruption.
2. **Native Protocol Fuzzing**:
   ```bash
   go test "-fuzz=^FuzzHPACKDecode$" "-fuzztime=5s" ./proto/h2
   go test "-fuzz=^FuzzFrameRead$" "-fuzztime=5s" ./proto/h2
   go test "-fuzz=^FuzzH3FrameHeaderRead$" "-fuzztime=5s" ./proto/h3
   ```
   *Pass Criteria*: Exit code 0, 0 panics, 0 crashes on fuzz worker iterations.
3. **Overlay Test Gap Verification**:
   Verify that a new unit test suite has been added to `proto/h2/overlay` testing all getter methods, bounds check elimination (BCE), and padded data frame handling.
4. **Allocation Regression Attack**:
   Run memory profiling and verify that zero-allocation paths have not silently fallen back to heap allocations.

#### D. Auditor (Final Gatekeeper)
1. **Codebase-Wide Race Safety**:
   ```bash
   go test -race -timeout 90s ./...
   ```
   *Pass Criteria*: 0 failures, 0 race warnings.
2. **Automated Heavy Fuzz Harness**:
   ```bash
   go run ./scripts/fuzz_all.go -fuzztime=5s
   ```
   *Pass Criteria*: All 8 targets pass with 0 panics and 0 errors.
3. **Strict Linter Run**:
   ```bash
   golangci-lint run ./...
   ```
   *Pass Criteria*: 0 issues reported.
4. **File Ownership Audit**:
   `git status` must confirm zero modifications outside allowed M1 packages and Worker workspace.

---

### 4.4 Strict File Write Ownership Boundaries for Worker

```text
┌────────────────────────────────────────────────────────────────────────┐
│                        WORKER PERMITTED WRITE PATHS                    │
├────────────────────────────────────────────────────────────────────────┤
│ 1. proto/compress/                                                     │
│    - compress.go (edit)                                                │
│    - gzip.go (create)                                                  │
│    - flate.go (create)                                                 │
│    - brotli.go (edit)                                                  │
│    - zstd.go (edit)                                                    │
│    - compress_test.go (edit/extend)                                    │
│                                                                        │
│ 2. proto/h2/                                                           │
│    - frame.go (edit)                                                   │
│    - header.go (edit)                                                  │
│    - frame_data.go (create)                                            │
│    - frame_headers.go (create)                                         │
│    - frame_control.go (create)                                         │
│    - frame_window.go (create)                                          │
│    - frame_ext.go (create)                                             │
│    - frame_pool.go (edit)                                              │
│    - settings.go (edit)                                                │
│    - errors.go (edit)                                                  │
│    - utils.go (edit)                                                   │
│    - frames.go (DELETE after decomposition)                            │
│    - *_test.go (edit/extend)                                           │
│                                                                        │
│ 3. proto/h2/overlay/                                                   │
│    - frame.go (edit)                                                   │
│    - frame_test.go (edit/extend with unit tests)                       │
│                                                                        │
│ 4. proto/h3/                                                           │
│    - qpack.go (edit)                                                   │
│    - qpack_client.go (create)                                          │
│    - qpack_server.go (create)                                          │
│    - qpack_rules.go (create)                                           │
│    - frames.go (edit)                                                  │
│    - errors.go (edit)                                                  │
│    - *_test.go (edit/extend)                                           │
│                                                                        │
│ 5. .agents/teamwork_preview_worker_m1_gen2/ (Worker's own workspace)   │
└────────────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────────────┐
│                      STRICTLY FORBIDDEN (OFF-LIMITS)                   │
├────────────────────────────────────────────────────────────────────────┤
│ ❌ client/... (client/h1, client/h2, client/h3, client/pool.go)         │
│ ❌ server/... (server/h1, server/h2, server/h3)                         │
│ ❌ proto/http/... (proto/http/header*, request*, response*, etc.)      │
│ ❌ fsm/..., x/...                                                      │
│ ❌ Repository root: go.mod, go.sum, LICENSE, README.md, scripts/*      │
│ ❌ .golangci.yml (Reserved for Milestone M5!)                          │
│ ❌ Any folder under .agents/ belonging to other agents                 │
└────────────────────────────────────────────────────────────────────────┘
```

---

### 4.5 Complete Checklist of Exported Symbols Requiring RFC Docstrings

Every exported item listed below must be checked and provided with an authoritative RFC citation, concurrency expectations, and lifecycle rules:

#### Package `proto/compress`
- [ ] `const CompressNoCompression` (RFC 1951)
- [ ] `const CompressBestSpeed` (RFC 1951)
- [ ] `const CompressBestCompression` (RFC 1951)
- [ ] `const CompressDefaultCompression` (RFC 1951)
- [ ] `const CompressHuffmanOnly` (RFC 1951)
- [ ] `const CompressBrotliNoCompression` (RFC 7932)
- [ ] `const CompressBrotliBestSpeed` (RFC 7932)
- [ ] `const CompressBrotliBestCompression` (RFC 7932)
- [ ] `const CompressBrotliDefaultCompression` (RFC 7932)
- [ ] `const CompressZstdSpeedNotSet` (RFC 8878)
- [ ] `const CompressZstdBestSpeed` (RFC 8878)
- [ ] `const CompressZstdDefault` (RFC 8878)
- [ ] `const CompressZstdSpeedBetter` (RFC 8878)
- [ ] `const CompressZstdBestCompression` (RFC 8878)
- [ ] `func AcquireStacklessGzipWriter(w io.Writer, level int) stackless.Writer`
- [ ] `func ReleaseStacklessGzipWriter(sw stackless.Writer, level int)`
- [ ] `func AppendGzipBytesLevel(dst, src []byte, level int) []byte` (RFC 1952)
- [ ] `func WriteGzipLevel(w io.Writer, p []byte, level int) (int, error)` (RFC 1952)
- [ ] `func WriteGzip(w io.Writer, p []byte) (int, error)` (RFC 1952)
- [ ] `func AppendGzipBytes(dst, src []byte) []byte` (RFC 1952)
- [ ] `func WriteGunzip(w io.Writer, p []byte) (int, error)` (RFC 1952)
- [ ] `func WriteGunzipLimit(w io.Writer, p []byte, maxBodySize int) (int, error)` (RFC 1952)
- [ ] `func AppendGunzipBytes(dst, src []byte) ([]byte, error)` (RFC 1952)
- [ ] `func AcquireStacklessDeflateWriter(w io.Writer, level int) stackless.Writer` (RFC 1951)
- [ ] `func ReleaseStacklessDeflateWriter(sw stackless.Writer, level int)`
- [ ] `func AppendDeflateBytesLevel(dst, src []byte, level int) []byte` (RFC 1951)
- [ ] `func WriteDeflateLevel(w io.Writer, p []byte, level int) (int, error)` (RFC 1951)
- [ ] `func WriteDeflate(w io.Writer, p []byte) (int, error)` (RFC 1951)
- [ ] `func AppendDeflateBytes(dst, src []byte) []byte` (RFC 1951)
- [ ] `func WriteInflate(w io.Writer, p []byte) (int, error)` (RFC 1951)
- [ ] `func WriteInflateLimit(w io.Writer, p []byte, maxBodySize int) (int, error)` (RFC 1951)
- [ ] `func AppendInflateBytes(dst, src []byte) ([]byte, error)` (RFC 1951)
- [ ] `func AppendBrotliBytesLevel(dst, src []byte, level int) []byte` (RFC 7932)
- [ ] `func WriteBrotliLevel(w io.Writer, p []byte, level int) (int, error)` (RFC 7932)
- [ ] `func WriteBrotli(w io.Writer, p []byte) (int, error)` (RFC 7932)
- [ ] `func AppendBrotliBytes(dst, src []byte) []byte` (RFC 7932)
- [ ] `func WriteUnbrotli(w io.Writer, p []byte) (int, error)` (RFC 7932)
- [ ] `func WriteUnbrotliLimit(w io.Writer, p []byte, maxBodySize int) (int, error)` (RFC 7932)
- [ ] `func AppendUnbrotliBytes(dst, src []byte) ([]byte, error)` (RFC 7932)
- [ ] `func AppendZstdBytesLevel(dst, src []byte, level int) []byte` (RFC 8878)
- [ ] `func WriteZstdLevel(w io.Writer, p []byte, level int) (int, error)` (RFC 8878)
- [ ] `func WriteZstd(w io.Writer, p []byte) (int, error)` (RFC 8878)
- [ ] `func AppendZstdBytes(dst, src []byte) []byte` (RFC 8878)
- [ ] `func WriteUnzstd(w io.Writer, p []byte) (int, error)` (RFC 8878)
- [ ] `func WriteUnzstdLimit(w io.Writer, p []byte, maxBodySize int) (int, error)` (RFC 8878)
- [ ] `func AppendUnzstdBytes(dst, src []byte) ([]byte, error)` (RFC 8878)

#### Package `proto/h2`
- [ ] `type FrameType uint8` (RFC 9113 §6)
- [ ] `func (ft FrameType) String() string`
- [ ] `const FrameData` (RFC 9113 §6.1)
- [ ] `const FrameHeaders` (RFC 9113 §6.2)
- [ ] `const FramePriority` (RFC 9113 §6.3)
- [ ] `const FrameResetStream` (RFC 9113 §6.4)
- [ ] `const FrameSettings` (RFC 9113 §6.5)
- [ ] `const FramePushPromise` (RFC 9113 §6.6)
- [ ] `const FramePing` (RFC 9113 §6.7)
- [ ] `const FrameGoAway` (RFC 9113 §6.8)
- [ ] `const FrameWindowUpdate` (RFC 9113 §6.9)
- [ ] `const FrameContinuation` (RFC 9113 §6.10)
- [ ] `type FrameFlags uint8` (RFC 9113 §4.1)
- [ ] `func (flags FrameFlags) Has(target FrameFlags) bool`
- [ ] `func (flags FrameFlags) Add(target FrameFlags) FrameFlags`
- [ ] `func (flags FrameFlags) Del(target FrameFlags) FrameFlags`
- [ ] `const FlagAck` (RFC 9113 §6.5, §6.7)
- [ ] `const FlagEndStream` (RFC 9113 §6.1, §6.2)
- [ ] `const FlagEndHeaders` (RFC 9113 §6.2, §6.6, §6.10)
- [ ] `const FlagPadded` (RFC 9113 §6.1, §6.2, §6.6)
- [ ] `const FlagPriority` (RFC 9113 §6.2)
- [ ] `type Frame interface` (RFC 9113 §4)
- [ ] `func AcquireFrame(frameType FrameType) Frame`
- [ ] `func AcquireFrameInArena(arena *offheap.Arena, frameType FrameType) Frame`
- [ ] `func ReleaseFrame(fr Frame)`
- [ ] `const DefaultMaxLen` (RFC 9113 §4.2)
- [ ] `type FrameHeader struct` (RFC 9113 §4.1, §4.2)
- [ ] `func AcquireFrameHeader() *FrameHeader`
- [ ] `func ReleaseFrameHeader(fr *FrameHeader)`
- [ ] `func (f *FrameHeader) Reset()`
- [ ] `func (f *FrameHeader) Type() FrameType`
- [ ] `func (f *FrameHeader) Flags() FrameFlags`
- [ ] `func (f *FrameHeader) SetFlags(flags FrameFlags)`
- [ ] `func (f *FrameHeader) Stream() uint32`
- [ ] `func (f *FrameHeader) SetStream(stream uint32)`
- [ ] `func (f *FrameHeader) Len() int`
- [ ] `func (f *FrameHeader) MaxLen() uint32`
- [ ] `func (f *FrameHeader) Body() Frame`
- [ ] `func (f *FrameHeader) SetBody(fr Frame)`
- [ ] `func (f *FrameHeader) WriteTo(w *bufio.Writer) (wb int64, err error)`
- [ ] `func PackFrameHeader(dst []byte, length int, kind FrameType, flags FrameFlags, stream uint32)`
- [ ] `func UnpackFrameHeader(src []byte) (length int, kind FrameType, flags FrameFlags, stream uint32)`
- [ ] `func ReadFrameFrom(br *bufio.Reader) (*FrameHeader, error)`
- [ ] `func ReadFrameFromWithSize(br *bufio.Reader, max uint32) (*FrameHeader, error)`
- [ ] `type Continuation struct` (RFC 9113 §6.10)
- [ ] `func (c *Continuation) Type() FrameType`
- [ ] `func (c *Continuation) Reset()`
- [ ] `func (c *Continuation) Headers() []byte`
- [ ] `func (c *Continuation) SetEndHeaders(v bool)`
- [ ] `func (c *Continuation) EndHeaders() bool`
- [ ] `func (c *Continuation) SetHeader(b []byte)`
- [ ] `func (c *Continuation) AppendHeader(b []byte)`
- [ ] `func (c *Continuation) Write(b []byte) (int, error)`
- [ ] `func (c *Continuation) Deserialize(fr *FrameHeader) error`
- [ ] `func (c *Continuation) Serialize(fr *FrameHeader)`
- [ ] `type Data struct` (RFC 9113 §6.1)
- [ ] `func (d *Data) Type() FrameType`
- [ ] `func (d *Data) Reset()`
- [ ] `func (d *Data) SetEndStream(v bool)`
- [ ] `func (d *Data) EndStream() bool`
- [ ] `func (d *Data) Data() []byte`
- [ ] `func (d *Data) SetData(b []byte)`
- [ ] `func (d *Data) Padding() bool`
- [ ] `func (d *Data) SetPadding(v bool)`
- [ ] `func (d *Data) Append(b []byte)`
- [ ] `func (d *Data) Len() int`
- [ ] `func (d *Data) Write(b []byte) (int, error)`
- [ ] `func (d *Data) Deserialize(fr *FrameHeader) error`
- [ ] `func (d *Data) Serialize(fr *FrameHeader)`
- [ ] `type GoAway struct` (RFC 9113 §6.8)
- [ ] `func (ga *GoAway) Type() FrameType`
- [ ] `func (ga *GoAway) Reset()`
- [ ] `func (ga *GoAway) Code() ErrorCode`
- [ ] `func (ga *GoAway) SetCode(code ErrorCode)`
- [ ] `func (ga *GoAway) Stream() uint32`
- [ ] `func (ga *GoAway) SetStream(stream uint32)`
- [ ] `func (ga *GoAway) Data() []byte`
- [ ] `func (ga *GoAway) SetData(b []byte)`
- [ ] `func (ga *GoAway) Error() string`
- [ ] `func (ga *GoAway) Deserialize(fr *FrameHeader) error`
- [ ] `func (ga *GoAway) Serialize(fr *FrameHeader)`
- [ ] `type Headers struct` (RFC 9113 §6.2)
- [ ] `func (h *Headers) Type() FrameType`
- [ ] `func (h *Headers) Headers() []byte`
- [ ] `func (h *Headers) SetHeaders(b []byte)`
- [ ] `func (h *Headers) AppendRawHeaders(b []byte)`
- [ ] `func (h *Headers) EndStream() bool`
- [ ] `func (h *Headers) SetEndStream(v bool)`
- [ ] `func (h *Headers) EndHeaders() bool`
- [ ] `func (h *Headers) SetEndHeaders(v bool)`
- [ ] `func (h *Headers) Stream() uint32`
- [ ] `func (h *Headers) SetStream(stream uint32)`
- [ ] `func (h *Headers) Weight() byte`
- [ ] `func (h *Headers) SetWeight(w byte)`
- [ ] `func (h *Headers) Exclusive() bool`
- [ ] `func (h *Headers) SetExclusive(v bool)`
- [ ] `func (h *Headers) Padding() bool`
- [ ] `func (h *Headers) SetPadding(v bool)`
- [ ] `func (h *Headers) Reset()`
- [ ] `func (h *Headers) Deserialize(frh *FrameHeader) error`
- [ ] `func (h *Headers) Serialize(frh *FrameHeader)`
- [ ] `func (h *Headers) AppendHeaderField(hp *hpack.HPACK, hf *hpack.HeaderField, store bool)`
- [ ] `type Ping struct` (RFC 9113 §6.7)
- [ ] `func (p *Ping) Type() FrameType`
- [ ] `func (p *Ping) IsAck() bool`
- [ ] `func (p *Ping) SetAck(ack bool)`
- [ ] `func (p *Ping) Reset()`
- [ ] `func (p *Ping) Data() []byte`
- [ ] `func (p *Ping) SetData(b []byte)`
- [ ] `func (p *Ping) Write(b []byte) (int, error)`
- [ ] `func (p *Ping) Deserialize(frh *FrameHeader) error`
- [ ] `func (p *Ping) Serialize(fr *FrameHeader)`
- [ ] `type Priority struct` (RFC 9113 §6.3)
- [ ] `func (pry *Priority) Type() FrameType`
- [ ] `func (pry *Priority) Reset()`
- [ ] `func (pry *Priority) Stream() uint32`
- [ ] `func (pry *Priority) SetStream(stream uint32)`
- [ ] `func (pry *Priority) Weight() byte`
- [ ] `func (pry *Priority) SetWeight(w byte)`
- [ ] `func (pry *Priority) Exclusive() bool`
- [ ] `func (pry *Priority) SetExclusive(v bool)`
- [ ] `func (pry *Priority) Deserialize(fr *FrameHeader) error`
- [ ] `func (pry *Priority) Serialize(fr *FrameHeader)`
- [ ] `type PushPromise struct` (RFC 9113 §6.6)
- [ ] `func (pp *PushPromise) Type() FrameType`
- [ ] `func (pp *PushPromise) PromisedStream() uint32`
- [ ] `func (pp *PushPromise) Headers() []byte`
- [ ] `func (pp *PushPromise) Reset()`
- [ ] `func (pp *PushPromise) SetHeader(h []byte)`
- [ ] `func (pp *PushPromise) Write(b []byte) (int, error)`
- [ ] `func (pp *PushPromise) Deserialize(fr *FrameHeader) error`
- [ ] `func (pp *PushPromise) Serialize(fr *FrameHeader)`
- [ ] `type RstStream struct` (RFC 9113 §6.4)
- [ ] `func (rst *RstStream) Type() FrameType`
- [ ] `func (rst *RstStream) Code() ErrorCode`
- [ ] `func (rst *RstStream) SetCode(code ErrorCode)`
- [ ] `func (rst *RstStream) Reset()`
- [ ] `func (rst *RstStream) Error() error`
- [ ] `func (rst *RstStream) Deserialize(fr *FrameHeader) error`
- [ ] `func (rst *RstStream) Serialize(fr *FrameHeader)`
- [ ] `type WindowUpdate struct` (RFC 9113 §6.9)
- [ ] `func (wu *WindowUpdate) Type() FrameType`
- [ ] `func (wu *WindowUpdate) Reset()`
- [ ] `func (wu *WindowUpdate) Increment() int`
- [ ] `func (wu *WindowUpdate) SetIncrement(inc int)`
- [ ] `func (wu *WindowUpdate) Deserialize(fr *FrameHeader) error`
- [ ] `func (wu *WindowUpdate) Serialize(fr *FrameHeader)`
- [ ] `const DefaultHeaderTableSize` (RFC 9113 §6.5.2)
- [ ] `const DefaultDataFrameSize` (RFC 9113 §6.5.2)
- [ ] `const HeaderTableSize` (RFC 9113 §6.5.2)
- [ ] `const EnablePush` (RFC 9113 §6.5.2)
- [ ] `const MaxConcurrentStreams` (RFC 9113 §6.5.2)
- [ ] `const MaxWindowSize` (RFC 9113 §6.5.2)
- [ ] `const MaxFrameSize` (RFC 9113 §6.5.2)
- [ ] `const MaxHeaderListSize` (RFC 9113 §6.5.2)
- [ ] `const EnableConnectProtocol` (RFC 8441 §3)
- [ ] `type Settings struct` (RFC 9113 §6.5)
- [ ] `field Settings.MaxStreams uint32`
- [ ] `func (st *Settings) Type() FrameType`
- [ ] `func (st *Settings) Reset()`
- [ ] `func (st *Settings) CopyTo(dst *Settings)`
- [ ] `func (st *Settings) SetHeaderTableSize(size uint32)`
- [ ] `func (st *Settings) HeaderTableSize() uint32`
- [ ] `func (st *Settings) SetPush(enabled bool)`
- [ ] `func (st *Settings) Push() bool`
- [ ] `func (st *Settings) SetMaxConcurrentStreams(m uint32)`
- [ ] `func (st *Settings) MaxConcurrentStreams() uint32`
- [ ] `func (st *Settings) SetMaxWindowSize(size uint32)`
- [ ] `func (st *Settings) MaxWindowSize() uint32`
- [ ] `func (st *Settings) SetMaxFrameSize(size uint32)`
- [ ] `func (st *Settings) MaxFrameSize() uint32`
- [ ] `func (st *Settings) SetMaxHeaderListSize(size uint32)`
- [ ] `func (st *Settings) MaxHeaderListSize() uint32`
- [ ] `func (st *Settings) SetEnableConnect(enabled bool)`
- [ ] `func (st *Settings) EnableConnect() bool`
- [ ] `func (st *Settings) IsAck() bool`
- [ ] `func (st *Settings) SetAck(ack bool)`
- [ ] `func (st *Settings) Read(payload []byte) error`
- [ ] `func (st *Settings) Encode()`
- [ ] `func (st *Settings) Deserialize(fr *FrameHeader) error`
- [ ] `func (st *Settings) Serialize(fr *FrameHeader)`
- [ ] `type ErrorCode uint32` (RFC 9113 §7)
- [ ] `const NoError` (RFC 9113 §7)
- [ ] `const ProtocolError` (RFC 9113 §7)
- [ ] `const InternalError` (RFC 9113 §7)
- [ ] `const FlowControlError` (RFC 9113 §7)
- [ ] `const SettingsTimeoutError` (RFC 9113 §7)
- [ ] `const StreamClosedError` (RFC 9113 §7)
- [ ] `const FrameSizeError` (RFC 9113 §7)
- [ ] `const RefusedStreamError` (RFC 9113 §7)
- [ ] `const StreamCanceled` (RFC 9113 §7)
- [ ] `const CompressionError` (RFC 9113 §7)
- [ ] `const ConnectionError` (RFC 9113 §7)
- [ ] `const EnhanceYourCalm` (RFC 9113 §7)
- [ ] `const InadequateSecurity` (RFC 9113 §7)
- [ ] `const HTTP11Required` (RFC 9113 §7)
- [ ] `type Error struct` (RFC 9113 §7)
- [ ] `func (e Error) Is(target error) bool`
- [ ] `func (e Error) Code() ErrorCode`
- [ ] `func (e Error) Debug() string`
- [ ] `func (e Error) Error() string`
- [ ] `func NewError(code ErrorCode, debug string) Error`
- [ ] `func NewGoAwayError(code ErrorCode, debug string) Error`
- [ ] `func NewResetStreamError(code ErrorCode, debug string) Error`
- [ ] `var ErrServerSupport, ErrNoAvailableStreams, ErrTimeout, ErrUnexpectedSize, ErrWriterClosed, ErrWrongPreface, ErrMalformedString, ErrGoAwayRetryable, ErrControlFrameFlood, ErrUnknownFrameType, ErrMissingBytes, ErrPayloadExceeds, ErrCompression, ErrInvalidPingPayload, ErrStreamClosed, ErrInvalidWindowIncrement, ErrWindowAboveLimits`
- [ ] `var StringPath, StringStatus, StringAuthority, StringScheme, StringMethod, StringServer, StringContentLength, StringContentType, StringUserAgent, StringHTTP2` (RFC 9113 §8.3)
- [ ] `func ReadPreface(r io.Reader) bool` (RFC 9113 §3.4)
- [ ] `func WritePreface(w io.Writer) error` (RFC 9113 §3.4)
- [ ] `func PerformHandshake(preface bool, bw *bufio.Writer, st *Settings, maxWin int32) error`
- [ ] `func ToLowerCopy(b []byte) []byte`
- [ ] `func FasthttpResponseHeaders(dst *Headers, hp *hpack.HPACK, res *h1.Response)`
- [ ] `func SerializeResponseHeaders(dst *Headers, hp *hpack.HPACK, statusCode int, headers http.Header, bodyLen int)`
- [ ] `type ConnectionFramePool struct`
- [ ] `func NewConnectionFramePool(capacity int) *ConnectionFramePool`
- [ ] `func (p *ConnectionFramePool) AcquireFrame(frameType FrameType) Frame`
- [ ] `func (p *ConnectionFramePool) ReleaseFrame(fr Frame)`
- [ ] `func (p *ConnectionFramePool) Release()`

#### Package `proto/h2/overlay`
- [ ] `type Frame []byte` (RFC 9113 §4.1)
- [ ] `func (f Frame) Length() uint32`
- [ ] `func (f Frame) Type() uint8`
- [ ] `func (f Frame) Flags() uint8`
- [ ] `func (f Frame) StreamID() uint32`
- [ ] `func (f Frame) Payload() []byte`
- [ ] `func (f Frame) IsValid() bool`
- [ ] `type DataFrame Frame` (RFC 9113 §6.1)
- [ ] `func (d DataFrame) IsEndStream() bool`
- [ ] `func (d DataFrame) IsPadded() bool`
- [ ] `func (d DataFrame) PadLength() uint8`
- [ ] `func (d DataFrame) Data() []byte`
- [ ] `type HeadersFrame Frame` (RFC 9113 §6.2)
- [ ] `func (h HeadersFrame) IsEndStream() bool`
- [ ] `func (h HeadersFrame) IsEndHeaders() bool`
- [ ] `func (h HeadersFrame) HeaderBlockFragment() []byte`

#### Package `proto/h3`
- [ ] `type ErrorCode uint64` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3NoError` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3GeneralProtocolError` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3InternalError` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3StreamCreationError` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3ClosedCriticalStream` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3FrameUnexpected` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3FrameError` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3ExcessiveLoad` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3IDError` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3SettingsError` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3MissingSettings` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3RequestRejected` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3RequestCancelled` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3RequestIncomplete` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3MessageError` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3ConnectError` (RFC 9114 §8.1)
- [ ] `const ErrCodeH3VersionFallback` (RFC 9114 §8.1)
- [ ] `const ErrCodeQpackDecompressionFailed` (RFC 9204 §6)
- [ ] `const ErrCodeQpackEncoderStreamError` (RFC 9204 §6)
- [ ] `const ErrCodeQpackDecoderStreamError` (RFC 9204 §6)
- [ ] `var ErrServerClosed, ErrFrameUnexpected, ErrHeaderTooLarge, ErrStreamClosed, ErrInvalidStreamType, ErrMissingSettings, ErrDuplicateControlStream, ErrClosedCriticalStream, ErrQPACKDecompressFailed, ErrMissingStatusHeader, ErrInvalidHostHeader, ErrMissingMethodOrPath, ErrMalformedHeader, ErrH3SettingsError`
- [ ] `const FrameTypeData` (RFC 9114 §7.2.1)
- [ ] `const FrameTypeHeaders` (RFC 9114 §7.2.2)
- [ ] `const FrameTypeCancelPush` (RFC 9114 §7.2.3)
- [ ] `const FrameTypeSettings` (RFC 9114 §7.2.4)
- [ ] `const FrameTypePushPromise` (RFC 9114 §7.2.5)
- [ ] `const FrameTypeGoAway` (RFC 9114 §7.2.6)
- [ ] `const FrameTypeMaxPushID` (RFC 9114 §7.2.7)
- [ ] `const FrameTypeWebTransportStream` (WebTransport §4.1)
- [ ] `const StreamTypeControl` (RFC 9114 §6.2.1)
- [ ] `const StreamTypePush` (RFC 9114 §6.2.2)
- [ ] `const StreamTypeQPACKEncoder` (RFC 9204 §4.2)
- [ ] `const StreamTypeQPACKDecoder` (RFC 9204 §4.2)
- [ ] `const StreamTypeWebTransport` (WebTransport §4.2)
- [ ] `const SettingQpackMaxTableCapacity` (RFC 9204 §5)
- [ ] `const SettingMaxFieldSectionSize` (RFC 9114 §7.2.4.1)
- [ ] `const SettingQpackBlockedStreams` (RFC 9204 §5)
- [ ] `const SettingEnableConnectProtocol` (RFC 9220 §3)
- [ ] `const SettingH3Datagram` (RFC 9297)
- [ ] `type Settings struct` (RFC 9114 §7.2.4)
- [ ] `field Settings.Other map[uint64]uint64`
- [ ] `field Settings.MaxFieldSectionSize int64`
- [ ] `field Settings.QpackMaxTableCap uint64`
- [ ] `field Settings.QpackBlockedStreams uint64`
- [ ] `field Settings.EnableDatagrams bool`
- [ ] `field Settings.EnableConnect bool`
- [ ] `func (s *Settings) Encode() []byte`
- [ ] `func DecodeSettings(r io.Reader, payloadLen uint64) (*Settings, error)`
- [ ] `func ReadFrameHeader(r varint.Reader) (frameType, payloadLen uint64, err error)`
- [ ] `func AppendHeadersHeader(b []byte, length uint64) []byte`
- [ ] `func AppendDataHeader(b []byte, length uint64) []byte`
- [ ] `type QPACKStreamError struct` (RFC 9204 §6)
- [ ] `field QPACKStreamError.Code ErrorCode`
- [ ] `field QPACKStreamError.InternalCode uint64`
- [ ] `field QPACKStreamError.Message string`
- [ ] `field QPACKStreamError.IsEncoder bool`
- [ ] `func (e *QPACKStreamError) Error() string`
- [ ] `func (e *QPACKStreamError) Is(target error) bool`
- [ ] `type QPACKCodec struct` (RFC 9204)
- [ ] `func NewQPACKCodec() *QPACKCodec`
- [ ] `func NewQPACKCodecWithOptions(maxDynamicTableCapacity, maxBlockedStreams uint64, onError func(error)) *QPACKCodec`
- [ ] `func (q *QPACKCodec) Err() error`
- [ ] `func (q *QPACKCodec) ErrChan() <-chan error`
- [ ] `func (q *QPACKCodec) SetErrorHandler(fn func(err error))`
- [ ] `func (q *QPACKCodec) Decoder() *qpack.Decoder`
- [ ] `func (q *QPACKCodec) Encoder() *qpack.Encoder`
- [ ] `func (q *QPACKCodec) EncodeRequestHeaders(streamID uint64, w io.Writer, req *http.Request, orderedKeys []string) (retErr error)`
- [ ] `func (q *QPACKCodec) DecodeResponseHeaders(streamID uint64, headerBlock []byte, res *http.ResponseHeader) (statusCode int, retErr error)`
- [ ] `func (q *QPACKCodec) DecodeResponseTrailers(streamID uint64, headerBlock []byte) (trailers map[string][]string, retErr error)`
- [ ] `func (q *QPACKCodec) DecodeRequestHeaders(streamID uint64, headerBlock []byte, reqHeaders *headkit.Headers) (method, path, scheme, authority string, retErr error)`
- [ ] `func (q *QPACKCodec) EncodeResponseHeaders(streamID uint64, statusCode int, headers headkit.Headers, bodyLen int) (block []byte)`

---

## 5. Verification Method

To independently verify the facts, observations, and criteria documented in this report:

1. **Verify Unit Test Suites**:
   ```bash
   go test -v ./proto/compress/... ./proto/h2/... ./proto/h3/...
   ```
   Confirm all test cases pass and verify the warning `[no tests to run]` on `proto/h2/overlay`.

2. **Verify Zero-Allocation Invariants and Baseline Latencies**:
   ```bash
   go test "-bench=." "-benchmem" ./proto/h2/... ./proto/h3/... ./proto/compress/... ./server/h3
   ```
   Confirm:
   - `BenchmarkInSituOverlay`: `0 B/op`, `0 allocs/op`
   - `BenchmarkAcquireRelease_PerGoroutinePool_Parallel`: `0 B/op`, `0 allocs/op`
   - `BenchmarkH3_FrameHeaderPack`: `0 B/op`, `0 allocs/op`

3. **Verify Race Safety**:
   ```bash
   go test -race ./proto/compress/... ./proto/h2/... ./proto/h3/...
   ```
   Confirm exit code 0 and 0 race warnings.

4. **Verify Fuzz Targets**:
   ```bash
   go test "-fuzz=^FuzzHPACKDecode$" "-fuzztime=2s" ./proto/h2
   go test "-fuzz=^FuzzFrameRead$" "-fuzztime=2s" ./proto/h2
   go test "-fuzz=^FuzzH3FrameHeaderRead$" "-fuzztime=2s" ./proto/h3
   ```
   Confirm exit code 0 across all three targets.

5. **Verify Clean Linter Baseline**:
   ```bash
   golangci-lint run ./proto/compress/... ./proto/h2/... ./proto/h3/...
   ```
   Confirm 0 issues reported.

6. **Verify Downstream Re-Exports**:
   Inspect `client/h2/export.go` and `client/h3/export.go` to confirm the exact aliases that Worker must not break.
