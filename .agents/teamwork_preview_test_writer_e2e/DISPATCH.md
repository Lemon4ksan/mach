## 2026-09-22T14:15:33Z
You are the E2E Test Writer for the mach protocol engine refactoring project.
Your working directory is: d:\CodingProjects\mach\.agents\teamwork_preview_test_writer_e2e
Original user request file: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
Project plan file: d:\CodingProjects\mach\.agents\PROJECT.md

Task:
1. Read d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md and d:\CodingProjects\mach\.agents\PROJECT.md.
2. Design and implement the opaque-box, requirement-driven E2E test suite for mach across HTTP/1.1, HTTP/2, and HTTP/3:
   - Adhere to the 4-tier test methodology:
     * Tier 1: Feature Coverage (>=5 test cases per protocol feature)
     * Tier 2: Boundary & Corner Cases (>=5 test cases per feature: max limits, empty frames, zero window, overflow, header bounds)
     * Tier 3: Cross-Feature Combinations (pairwise interactions: chunked pipelining, stream reset + multiplexing, trailer streaming)
     * Tier 4: Real-World Application Scenarios (high-throughput pipeline, concurrent multi-stream exchanges, large payloads)
   - Ensure tests exercise public APIs as an external consumer (like aoni does).
3. Create `d:\CodingProjects\mach\.agents\TEST_INFRA.md` documenting test architecture, runner command, directory layout, and coverage thresholds.
4. Write test files under `d:\CodingProjects\mach\tests\e2e\` (create directory as needed). Ensure all test files include the standard 3-line BSD license header.
5. Verify tests run and pass cleanly (`go test -v -race ./tests/e2e/...`).
6. Publish `d:\CodingProjects\mach\.agents\TEST_READY.md` containing the test runner command, coverage matrix, and tier breakdown.
7. Write your handoff report to `d:\CodingProjects\mach\.agents\teamwork_preview_test_writer_e2e\handoff.md`, update progress.md, and send a completion message to parent.
