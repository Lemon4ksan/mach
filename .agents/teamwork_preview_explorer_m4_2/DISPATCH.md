## 2026-09-22T20:16:00Z
You are Explorer M4.2 (teamwork_preview_explorer).
Your working directory is: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_2
Project root: d:\CodingProjects\mach
Parent conversation ID: 5990a2d7-7ec1-47d8-9672-52a9ad7ba846

MANDATORY READING:
- Original User Request: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
- Master Plan: d:\CodingProjects\mach\.agents\PROJECT.md (especially §5 Code Layout for server/ and §6.4 Silicon Performance)
- E2E Test Suite & Escalations: d:\CodingProjects\mach\.agents\TEST_READY.md
- Predecessor handoff: d:\CodingProjects\mach\.agents\orchestrator_3\handoff.md

YOUR MISSION (Milestone M4: Server H1 & H3 Modularization & Per-P Buffer Pooling):
1. Investigate the current implementation of `server/h1/` and `server/h3/`. Examine all files in both packages.
2. Investigate how buffer pooling is currently done in `server/h1/` and `server/h3/`, and compare with the Per-P storage buffer pooling patterns established in `proto/http/pool.go` and `server/h1/conn.go`.
3. Design the standardization of Per-P storage buffer pooling across `server/h1/` and `server/h3/`:
   - Identify hot path allocations (e.g. read buffers, write buffers, scratch slices).
   - Ensure zero-allocation hot paths (`0 B/op, 0 allocs/op`).
   - Ensure thread-safety and correct return/recycling semantics to prevent buffer reuse data races.
4. Review `server/h3/` structure: check if `server/h3/server_conn.go`, `server/h3/dispatch.go`, `server/h3/stream.go` adhere to `PROJECT.md` §5.
5. Invariant checking: exact 3-line BSD header, RFC 9112 / RFC 9110 / RFC 9114 / RFC 9204 docstrings on all exported symbols.
6. Note: Environment invariant: if running Go commands to inspect symbols or test, use `$env:GOWORK="off"`.
7. You are a READ-ONLY EXPLORER. Do NOT modify source code.
8. Deliver a structured report to `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_2\handoff.md` containing:
   - Current Architecture of `server/h1/` and `server/h3/`
   - Detailed Blueprint for Per-P Storage Buffer Pooling in `server/h1/` and `server/h3/`
   - Code Layout and Modularization analysis
   - Zero-allocation hot path verification plan
   - Concrete, step-by-step guidance for Worker M4.1
When complete, notify parent via `send_message`.
