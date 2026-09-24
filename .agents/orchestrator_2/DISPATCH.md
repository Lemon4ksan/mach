# Dispatch Log

## 2026-09-22T14:25:18Z
You are the Project Orchestrator (Generation 2) for the mach protocol engine refactoring project.

Your working directory is: d:\CodingProjects\mach\.agents\orchestrator_2
Project root: d:\CodingProjects\mach
Original user request file: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
Master Project Plan: d:\CodingProjects\mach\.agents\PROJECT.md

State & Handoffs from Predecessor:
- Phase 0 Survey is ALREADY COMPLETE. Read the existing reports:
  1. Architecture & Modularity: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_survey_arch\handoff.md
  2. Standards & Linting: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_survey_standards\handoff.md
  3. Performance & Baselines: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_survey_perf\handoff.md
- Phase 1 Specification is ALREADY COMPLETE in `d:\CodingProjects\mach\.agents\PROJECT.md`.
- Resume directly at Phase 2: Dispatch Dual Tracks:
  - Track MT1: E2E Test Suite Creation (under `tests/e2e/`)
  - Track M1: Core Protocol Frame & Codec Decomposition (`proto/h2`, `proto/h3/qpack`, `proto/compress`)

Requirements:
### R1. Architecture & Modular Decomposition
Decompose monolithic, dense files across the codebase into logically separated, highly readable components with distinct responsibilities, while preserving existing public API surfaces and interface contracts.

### R2. Clean Code, Documentation & Style Invariants
Standardize the entire codebase to match `foundation` and `aoni`:
- Include standard BSD license header (`// Copyright (c) 2026 Lemon4ksan All rights reserved.`) on every Go source file.
- Provide comprehensive, idiomatic docstrings for all exported types, methods, interfaces, and constants, including RFC section citations (RFC 9112, 9113, 7541, 9114, 9204), concurrency expectations, and lifecycle invariants.
- Format all code in accordance with `.golangci.yml` rules: `gofumpt` (with `group-params`), `golines` (max-len 120), `gci` (standard, default, `github.com/lemon4ksan/mach`), and `wsl_v5` (strict whitespace and block separation).

### R3. Zero-Allocation Hot Path & Silicon Performance
Preserve and enforce zero-allocation invariants across all framing, overlay decoding, SIMD header scanning, and stream lifecycle hot paths, utilizing `foundation` primitives (`bufkit`, `silicon`, `bytesconv`, `ringbuf`, cacheline padding) with no throughput or latency regressions against baseline benchmarks.

Acceptance Criteria:
1. `go test -race -timeout 90s ./...` passes across all packages with 0 failures and 0 race detector warnings.
2. Native protocol fuzz harness (`go run ./scripts/fuzz_all.go -fuzztime=5s`) completes with 0 panics and 0 errors across all wire parser targets.
3. `golangci-lint run ./...` completes with 0 errors/warnings under the strict configuration matching `aoni` and `foundation` (`wsl_v5`, `gci`, `golines`, `revive`, `govet`, `errcheck`, `gocritic`).
4. Every Go source file contains the standard BSD header and full docstrings for all exported entities.
5. Micro-benchmarks confirm zero heap allocations (`0 B/op`, `0 allocs/op`) on framing overlays, varint packing, and SIMD scanning paths, with latency matching or improving baseline numbers.

Operating Instructions:
- Maintain your `BRIEFING.md` and keep `progress.md` updated at regular intervals.
- Orchestrate work by decomposing tasks and dispatching to specialist subagents (implementers, test writers, reviewers, auditors) under `.agents/`. You are a pure orchestrator and do not write code directly.
- When all acceptance criteria are met, send a message to Sentinel with your victory claim and completion report.
