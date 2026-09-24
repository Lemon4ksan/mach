## 2026-09-22T14:28:00Z
You are the E2E Test Writer (Track MT1) for the mach protocol engine refactoring project.
Your working directory is: d:\CodingProjects\mach\.agents\teamwork_preview_test_writer_mt1_gen2
Original user request file: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
Master Project Plan: d:\CodingProjects\mach\.agents\PROJECT.md
Test Infrastructure Specification: d:\CodingProjects\mach\.agents\TEST_INFRA.md

Mission & Task:
1. You MUST read:
   - `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md`
   - `d:\CodingProjects\mach\.agents\PROJECT.md`
   - `d:\CodingProjects\mach\.agents\TEST_INFRA.md`
2. Implement the comprehensive, opaque-box, requirement-driven E2E test suite under `d:\CodingProjects\mach\tests\e2e/`:
   - Structure tests across the 4 tiers defined in `TEST_INFRA.md`:
     * Tier 1: Feature Coverage (>=5 test cases per protocol: H1, H2, H3)
     * Tier 2: Boundary & Corner Cases (>=5 test cases per protocol: limits, empty frames, zero window, overflow, timeouts)
     * Tier 3: Cross-Feature Combinations (pairwise interactions: chunked pipelining, stream reset + multiplexing, trailer streaming, scoped memory borrowing)
     * Tier 4: Real-World Application Scenarios (high-throughput pipeline, concurrent multi-stream exchanges, large payloads, client pool manager)
   - Follow the directory layout in `TEST_INFRA.md`:
     * `tests/e2e/helpers_test.go`: loopback listeners, mock servers, in-memory certs/pipes
     * `tests/e2e/h1_test.go`: HTTP/1.1 test suite (Tiers 1-4)
     * `tests/e2e/h2_test.go`: HTTP/2 test suite (Tiers 1-4)
     * `tests/e2e/h3_test.go`: HTTP/3 test suite (Tiers 1-4)
     * `tests/e2e/pool_test.go`: Client connection pool manager suite
   - Package must be `package e2e_test`.
   - Every file MUST begin with the mandatory 3-line BSD license header:
     ```go
     // Copyright (c) 2026 Lemon4ksan All rights reserved.
     // Use of this source code is governed by a BSD-style
     // license that can be found in the LICENSE file.
     ```
   - Tests MUST ONLY exercise the public API surface (`mach`, `client`, `client/h1`, `client/h2`, `client/h3`, `server/h1`, `server/h2`, `server/h3`, `proto/http`). NO internal unexported accesses.
3. Verify the entire E2E test suite:
   Run `go test -v -race -timeout 120s ./tests/e2e/...` and ensure 100% of tests pass with 0 race detector warnings.
4. Publish `d:\CodingProjects\mach\.agents\TEST_READY.md` containing:
   - Test runner command
   - Coverage matrix & summary across all 4 tiers and protocols
   - Feature checklist matching `PROJECT.md § Feature Inventory`
5. Write your handoff report to `d:\CodingProjects\mach\.agents\teamwork_preview_test_writer_mt1_gen2\handoff.md`, update your `progress.md`, and notify the orchestrator.
