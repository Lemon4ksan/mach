## 2026-09-22T20:16:00Z
You are Explorer M4.1 (teamwork_preview_explorer).
Your working directory is: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_1
Project root: d:\CodingProjects\mach
Parent conversation ID: 5990a2d7-7ec1-47d8-9672-52a9ad7ba846

MANDATORY READING:
- Original User Request: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
- Master Plan: d:\CodingProjects\mach\.agents\PROJECT.md (especially §5 Code Layout for server/)
- E2E Test Suite & Escalations: d:\CodingProjects\mach\.agents\TEST_READY.md (especially §5 Escalation 2)
- Predecessor handoff: d:\CodingProjects\mach\.agents\orchestrator_3\handoff.md

YOUR MISSION (Milestone M4: Server H2 Decomposition Blueprint):
1. Investigate the existing HTTP/2 server implementation in `server/h2/`. Examine all files, especially `server/h2/server_conn.go` (and any other files in `server/h2/`).
2. Plan the clean decomposition of `server/h2/server_conn.go` (and related logic) into 5 focused, single-responsibility files:
   - `server/h2/server_conn.go`: ServerConn struct, constructor `NewServerConn`, configuration, exported facade, lifecycle (Serve, Close, Release).
   - `server/h2/read_loop.go`: Frame demuxing read loop, settings ACK handling, ping reflection, error handling.
   - `server/h2/write_loop.go`: Response framing, DATA batching, window updates, outbound frame queue / serialization.
   - `server/h2/stream.go`: Stream state machine (`serverStream`), stream lifecycle, half-closed tracking, error propagation, handler invocation.
   - `server/h2/flow_control.go`: Stream & connection window management, WINDOW_UPDATE emission and tracking.
3. Investigate Escalation 2 in `TEST_READY.md` §5: Data race on `sc.streams` in `Release()`.
   - Inspect line 114 or equivalent where `clear(sc.streams)` or map access occurs without locking or while goroutines are in-flight.
   - Provide concrete fix architecture (e.g. `sync.WaitGroup` to track in-flight stream goroutines before pooling/clearing, or mutex protection).
4. Verify all silicon invariants: zero-allocation hot paths, lock-free or low-contention structures, preservation of cacheline pads if any.
5. Invariant checking: exact 3-line BSD header, RFC 9113 / RFC 7541 docstrings on all exported symbols.
6. Note: Environment invariant: if running Go commands to inspect symbols or test, use `$env:GOWORK="off"`.
7. You are a READ-ONLY EXPLORER. Do NOT modify source code.
8. Deliver a structured report to `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_1\handoff.md` containing:
   - Current Code Architecture & Structure of `server/h2/`
   - Detailed Decomposition Blueprint for the 5 target files (symbols, structs, methods, responsibility breakdown)
   - Precise Fix Blueprint for Escalation 2 (data race on `sc.streams` in `Release()`)
   - RFC docstrings and license header checklist
   - Concrete, step-by-step guidance for Worker M4.1
When complete, notify parent via `send_message`.
