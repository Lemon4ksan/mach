# Original User Request

## 2026-09-22T13:58:46Z

Perform a comprehensive refactoring, formatting, and optimization of the `mach` protocol engine codebase to achieve the highest standards of code readability, modular decomposition, and zero-allocation silicon performance, using `foundation` and `aoni` as the reference standard of Go cleanliness.

Working directory: d:/CodingProjects/mach
Integrity mode: development

Reference codebases:
- `d:/CodingProjects/foundation` (foundation networking, runtime, primitives, formatting & lint rules)
- `d:/CodingProjects/aoni` (high-performance protocol client architecture, style, documentation)

## Requirements

### R1. Architecture & Modular Decomposition
Decompose monolithic, dense files across the codebase (such as `client/h2/conn.go`, oversized `proto/http` and `server` modules) into logically separated, highly readable components with distinct responsibilities (e.g. frame dispatch, window management, stream multiplexing, socket I/O), while preserving existing public API surfaces and interface contracts.

### R2. Clean Code, Documentation & Style Invariants
Standardize the entire codebase to match the cleanliness and formatting standards established in `foundation` and `aoni`:
- Include the standard BSD license header (`// Copyright (c) 2026 Lemon4ksan All rights reserved.`) on every Go source file.
- Provide comprehensive, idiomatic docstrings for all exported types, methods, interfaces, and constants, including RFC section citations (RFC 9112, 9113, 7541, 9114, 9204), concurrency expectations, and lifecycle invariants.
- Format all code in accordance with `.golangci.yml` rules: `gofumpt` (with `group-params`), `golines` (max-len 120), `gci` (standard, default, `github.com/lemon4ksan/mach`), and `wsl_v5` (strict whitespace and block separation).

### R3. Zero-Allocation Hot Path & Silicon Performance
Preserve and enforce zero-allocation invariants across all framing, overlay decoding, SIMD header scanning, and stream lifecycle hot paths, utilizing `foundation` primitives (`bufkit`, `silicon`, `bytesconv`, `ringbuf`, cacheline padding) with no throughput or latency regressions against baseline benchmarks.

## Acceptance Criteria

### Correctness & Race Safety
- [ ] `go test -race -timeout 90s ./...` passes across all packages with 0 failures and 0 race detector warnings.
- [ ] Native protocol fuzz harness (`go run ./scripts/fuzz_all.go -fuzztime=5s`) completes with 0 panics and 0 errors across all wire parser targets.

### Linter & Code Cleanliness
- [ ] `golangci-lint run ./...` completes with 0 errors/warnings under the strict configuration matching `aoni` and `foundation` (including `wsl_v5`, `gci`, `golines`, `revive`, `govet`, `errcheck`, `gocritic`).
- [ ] Every Go source file contains the standard BSD header and full docstrings for all exported entities.

### Benchmark & Allocation Invariants
- [ ] Micro-benchmarks (`go test -bench=. -benchmem ./...`) confirm zero heap allocations (`0 B/op`, `0 allocs/op`) on framing overlays, varint packing, and SIMD scanning paths, with latency matching or improving baseline numbers documented in `README.md`.

## 2026-09-22T15:54:15Z

Perform a comprehensive refactoring, formatting, and optimization of the `mach` protocol engine codebase to achieve the highest standards of code readability, modular decomposition, and zero-allocation silicon performance, using `foundation` and `aoni` as the reference standard of Go cleanliness.

Working directory: d:/CodingProjects/mach
Integrity mode: development

Resume context:
- Previous run completed Milestone M1 (decompositions of proto/h2, proto/h3, proto/compress) and MT1 (tests/e2e/ test suite with 62 passing tests).
- Milestone M2 is partially started in proto/http/.
- All specifications, progress state, and detailed explorer handoffs exist on disk under .agents/ (see .agents/PROJECT.md, .agents/orchestrator_2/progress.md, and .agents/teamwork_preview_explorer_m2_*/handoff.md).
- Resume execution directly from Milestone M2.

Reference codebases:
- `d:/CodingProjects/foundation` (foundation networking, runtime, primitives, formatting & lint rules)
- `d:/CodingProjects/aoni` (high-performance protocol client architecture, style, documentation)

## Requirements

### R1. Architecture & Modular Decomposition
Decompose monolithic, dense files across the codebase (such as `client/h2/conn.go`, oversized `proto/http` and `server` modules) into logically separated, highly readable components with distinct responsibilities (e.g. frame dispatch, window management, stream multiplexing, socket I/O), while preserving existing public API surfaces and interface contracts.

### R2. Clean Code, Documentation & Style Invariants
Standardize the entire codebase to match the cleanliness and formatting standards established in `foundation` and `aoni`:
- Include the standard BSD license header (`// Copyright (c) 2026 Lemon4ksan All rights reserved.`) on every Go source file.
- Provide comprehensive, idiomatic docstrings for all exported types, methods, interfaces, and constants, including RFC section citations (RFC 9112, 9113, 7541, 9114, 9204), concurrency expectations, and lifecycle invariants.
- Format all code in accordance with `.golangci.yml` rules: `gofumpt` (with `group-params`), `golines` (max-len 120), `gci` (standard, default, `github.com/lemon4ksan/mach`), and `wsl_v5` (strict whitespace and block separation).

### R3. Zero-Allocation Hot Path & Silicon Performance
Preserve and enforce zero-allocation invariants across all framing, overlay decoding, SIMD header scanning, and stream lifecycle hot paths, utilizing `foundation` primitives (`bufkit`, `silicon`, `bytesconv`, `ringbuf`, cacheline padding) with no throughput or latency regressions against baseline benchmarks.

## Acceptance Criteria

### Correctness & Race Safety
- [ ] `go test -race -timeout 90s ./...` passes across all packages with 0 failures and 0 race detector warnings.
- [ ] Native protocol fuzz harness (`go run ./scripts/fuzz_all.go -fuzztime=5s`) completes with 0 panics and 0 errors across all wire parser targets.

### Linter & Code Cleanliness
- [ ] `golangci-lint run ./...` completes with 0 errors/warnings under the strict configuration matching `aoni` and `foundation` (including `wsl_v5`, `gci`, `golines`, `revive`, `govet`, `errcheck`, `gocritic`).
- [ ] Every Go source file contains the standard BSD header and full docstrings for all exported entities.

### Benchmark & Allocation Invariants
- [ ] Micro-benchmarks (`go test -bench=. -benchmem ./...`) confirm zero heap allocations (`0 B/op`, `0 allocs/op`) on framing overlays, varint packing, and SIMD scanning paths, with latency matching or improving baseline numbers documented in `README.md`.

## 2026-09-22T18:51:26Z

Server restarted. Please resume execution of Milestone M2 immediately and continue through M3, M4, and M5.

## 2026-09-23T04:21:17Z

Server restarted. Please resume execution of Milestone M4 immediately and proceed through M5.



