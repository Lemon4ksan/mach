# Dispatch Instructions — Orchestrator Generation 4

## 2026-09-22T20:13:00Z
You are the Project Orchestrator (Generation 4) for the `mach` protocol engine refactoring project.

Your working directory is: `d:\CodingProjects\mach\.agents\orchestrator_4`
Project root: `d:\CodingProjects\mach`
Original user request file: `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md`
Master Project Plan: `d:\CodingProjects\mach\.agents\PROJECT.md`
Predecessor handoff: `d:\CodingProjects\mach\.agents\orchestrator_3\handoff.md`
Gate status: `d:\CodingProjects\mach\.agents\orchestrator_3\GATE_STATUS.md`

Your parent is: `7940c431-b817-44e7-b43c-a8565548b485`

STATE & HANDOFFS:
- MT1 (E2E Test Suite): COMPLETE (62/62 tests pass, documented in TEST_READY.md)
- M1 (Core Frames & Codecs): COMPLETE (Gate PASS)
- M2 (HTTP Message Model & Parser Modularization): COMPLETE (Gate PASS)
- M3 (Client Protocol Engine Decomposition): COMPLETE (Gate PASS)

YOUR MISSION:
Resume work directly at Milestone M4 (Server Protocol Engine Decomposition):
1. Review `PROJECT.md` §5 (Code Layout for `server/`).
2. Decompose `server/h2/server_conn.go` (570 lines) into 5 focused components:
   - `server/h2/server_conn.go` (lifecycle, constructor `NewServerConn`, exported facade)
   - `server/h2/read_loop.go` (frame demuxing, settings ACK, ping reflection)
   - `server/h2/write_loop.go` (response framing, DATA batching, window updates)
   - `server/h2/stream.go` (stream state machine, half-closed tracking, error propagation)
   - `server/h2/flow_control.go` (stream & connection window management)
3. Standardize Per-P storage buffer pooling in `server/h1/` and `server/h3/`.
4. Resolve remaining server defects noted in `TEST_READY.md` §5:
   - Ensure `server/h1/` connection is properly closed on request smuggling attempts (conflicting Content-Length and Transfer-Encoding).
   - Ensure stream release races in `server/h2/` are guarded.
5. Invariants:
   - Exact 3-line BSD header on all Go files in `server/`.
   - Comprehensive RFC docstrings (RFC 9112, 9110, 9113, 9114, 9204) on all exported symbols.
   - 100% public API compatibility.
   - Zero-allocation hot paths (`0 B/op, 0 allocs/op`).
   - Clean linter (`golangci-lint run ./server/...`).
   - Full race detector pass (`go test -v -race ./server/...` and `./tests/e2e/...`).
6. Run the iteration loop: Explorers (3x) -> Worker -> Reviewers (2x) + Challengers (2x) + Forensic Auditor (1x) -> Gate PASS.
7. Proceed to Milestone M5 (Final Quality Invariants & Acceptance Gate), Phase 2 Adversarial Coverage Hardening (Tier 5), and victory claim!

## 2026-09-23T04:21:58Z
The server has restarted. Please resume execution of Milestone M4 immediately and continue through M5. Check status of child subagents and tasks, revive or restart them as needed, and proceed according to the master project plan.
