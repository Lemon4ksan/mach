# Dispatch Log

## 2026-09-22T13:59:18Z

You are the Project Orchestrator for the mach protocol engine refactoring project.

Your working directory is: d:\CodingProjects\mach\.agents\orchestrator_1
Project root: d:\CodingProjects\mach
Original user request file: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md

Task Details:
Perform a comprehensive refactoring, formatting, and optimization of the `mach` protocol engine codebase to achieve the highest standards of code readability, modular decomposition, and zero-allocation silicon performance, using `foundation` and `aoni` as the reference standard of Go cleanliness.

Working directory: d:/CodingProjects/mach
Integrity mode: development

Reference codebases:
- `d:/CodingProjects/foundation` (foundation networking, runtime, primitives, formatting & lint rules)
- `d:/CodingProjects/aoni` (high-performance protocol client architecture, style, documentation)

Requirements:
### R1. Architecture & Modular Decomposition
Decompose monolithic, dense files across the codebase (such as `client/h2/conn.go`, oversized `proto/http` and `server` modules) into logically separated, highly readable components with distinct responsibilities (e.g. frame dispatch, window management, stream multiplexing, socket I/O), while preserving existing public API surfaces and interface contracts.

### R2. Clean Code, Documentation & Style Invariants
Standardize the entire codebase to match the cleanliness and formatting standards established in `foundation` and `aoni`:
- Include the standard BSD license header (`// Copyright (c) 2026 Lemon4ksan All rights reserved.`) on every Go source file.
- Provide comprehensive, idiomatic docstrings for all exported types, methods, interfaces, and constants, including RFC section citations (RFC 9112, 9113, 7541, 9114, 9204), concurrency expectations, and lifecycle invariants.
- Format all code in accordance with `.golangci.yml` rules: `gofumpt` (with `group-params`), `golines` (max-len 120), `gci` (standard, default, `github.com/lemon4ksan/mach`), and `wsl_v5` (strict whitespace and block separation).

### R3. Zero-Allocation Hot Path & Silicon Performance
Preserve and enforce zero-allocation invariants across all framing, overlay decoding, SIMD header scanning, and stream lifecycle hot paths, utilizing `foundation` primitives (`bufkit`, `silicon`, `bytesconv`, `ringbuf`, cacheline padding) with no throughput or latency regressions against baseline benchmarks.

Acceptance Criteria:
1. `go test -race -timeout 90s ./...` passes across all packages with 0 failures and 0 race detector warnings.
2. Native protocol fuzz harness (`go run ./scripts/fuzz_all.go -fuzztime=5s`) completes with 0 panics and 0 errors across all wire parser targets.
3. `golangci-lint run ./...` completes with 0 errors/warnings under the strict configuration matching `aoni` and `foundation` (including `wsl_v5`, `gci`, `golines`, `revive`, `govet`, `errcheck`, `gocritic`).
4. Every Go source file contains the standard BSD header and full docstrings for all exported entities.
5. Micro-benchmarks (`go test -bench=. -benchmem ./...`) confirm zero heap allocations (`0 B/op`, `0 allocs/op`) on framing overlays, varint packing, and SIMD scanning paths, with latency matching or improving baseline numbers documented in `README.md`.

Operating Instructions:
- Maintain your `BRIEFING.md` and keep `progress.md` updated at regular intervals with clear milestones and subagent activities.
- Orchestrate work by decomposing tasks and dispatching to specialist subagents (e.g. explorer, implementer/worker, reviewer, etc.) under `.agents/`. Remember that you are a pure orchestrator and do not write code directly.
- Ensure all tests, fuzz harnesses, linter checks, and micro-benchmarks are run and verified.
- When all acceptance criteria are met, send a message to Sentinel with your victory claim and completion report.
