## 2026-09-22T20:16:00Z

You are Explorer M4.3 (teamwork_preview_explorer).
Your working directory is: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_3
Project root: d:\CodingProjects\mach
Parent conversation ID: 5990a2d7-7ec1-47d8-9672-52a9ad7ba846

MANDATORY READING:
- Original User Request: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
- Master Plan: d:\CodingProjects\mach\.agents\PROJECT.md (especially §6 Quality Standards)
- E2E Test Suite & Escalations: d:\CodingProjects\mach\.agents\TEST_READY.md (especially §5 Escalation 1)
- Predecessor handoff: d:\CodingProjects\mach\.agents\orchestrator_3\handoff.md

YOUR MISSION (Milestone M4: Server Standards, Docstrings & Smuggling Defect Resolution):
1. Investigate Escalation 1 from `TEST_READY.md` §5: Request Smuggling Connection Close Defect in `server/h1/`.
   - Inspect `server/h1/request.go:254` (`finishRequestBodyRead` deleting `header.ContentLength`) and `server/h1/conn.go:155` (where `TransferEncoding` and `ContentLength` check is performed).
   - Formulate the exact, mathematically sound fix adhering to RFC 9112 §6.3 Item 3 and §11.2 (Request Smuggling Mitigation):
     Ensure that any request with conflicting `Content-Length` and `Transfer-Encoding` forces the connection to close (`keepAlive = false`), so the socket is never reused for pipelined/smuggled requests.
2. Conduct a comprehensive symbol audit across all packages in `server/` (`server/h1`, `server/h2`, `server/h3`):
   - Enumerate all exported types, functions, methods, interfaces, and constants.
   - Verify presence and compliance of RFC docstrings (RFC 9112, RFC 9110, RFC 9113, RFC 9114, RFC 9204).
   - Verify presence of the exact 3-line BSD license header on every Go file in `server/`.
3. Check linter status (`golangci-lint run ./server/...`) and test suite status (`go test ./server/...`).
4. Note: Environment invariant: if running Go commands, use `$env:GOWORK="off"`.
5. You are a READ-ONLY EXPLORER. Do NOT modify source code.
6. Deliver a structured report to `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_3\handoff.md` containing:
   - Precise Root Cause & Fix Blueprint for Escalation 1 (smuggling connection close)
   - Comprehensive Exported Symbol & Docstring Audit for `server/h1`, `server/h2`, `server/h3`
   - 3-line BSD License Header Verification across `server/`
   - Linter and Test Suite analysis
   - Concrete, step-by-step guidance for Worker M4.1
When complete, notify parent via `send_message`.
