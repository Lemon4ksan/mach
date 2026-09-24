# Dispatch Assignment — Milestone M2 Forensic Auditor

## Identity
- Role: M2 Forensic Auditor
- Type: teamwork_preview_auditor
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_auditor_m2_1
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
Read these files before starting audit:
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout, and invariants)
3. Worker handoff: `d:\CodingProjects\mach\.agents\teamwork_preview_worker_m2_3\handoff.md`

## Forensic Audit Protocol
Execute independent integrity forensics across `proto/http/`:
1. Static Analysis:
   - Check for hardcoded test results, expected output strings, or dummy/facade implementations.
   - Confirm genuine protocol parsing and framing logic in `body_chunked.go`, `body_identity.go`, `body_compress.go`, `multipart.go`, `request_wire.go`, `response_wire.go`, `header_parse.go`.
   - Verify that all 6 obsolete files (`http.go`, `chunk.go`, `streaming.go`, `header_request.go`, `header_response.go`, `header_helpers.go`) are deleted and `.tmp/` scratch scripts are gone.
2. Boundary Enforcement:
   - Check `git status` to verify the worker only created or modified permitted files in `proto/http/` and did not modify forbidden areas (`client/`, `server/`, `proto/h2/`, `proto/h3/`, `tests/e2e/`).
3. BSD License & Documentation Audit:
   - Check every `.go` file in `proto/http/` for the standard 3-line BSD license header.
   - Verify comprehensive docstrings with RFC citations on all exported types, functions, methods, and constants.
4. Independent Execution Validation:
   - Run `go build ./proto/http/...` and `go test -race ./proto/http/...`.
   - Verify zero-allocation benchmarks on hot paths.

## Output
Write `handoff.md` in your working directory with explicit verdict: `CLEAN` or `INTEGRITY VIOLATION`. Notify parent via `send_message`.

## 2026-09-22T19:37:56Z
You are M2 Forensic Auditor for Milestone M2.
Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_auditor_m2_1
Parent: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac

Read:
1. d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
2. d:\CodingProjects\mach\.agents\PROJECT.md
3. d:\CodingProjects\mach\.agents\teamwork_preview_auditor_m2_1\DISPATCH.md
4. d:\CodingProjects\mach\.agents\teamwork_preview_worker_m2_3\handoff.md

Verify:
- Forensic anti-cheat: static analysis, runtime verification, no hardcoded test values, genuine protocol implementations
- Boundary enforcement: check git status, worker modified only permitted proto/http files
- BSD headers and RFC docstrings audit
- Independent build & test execution

Write handoff.md with verdict CLEAN or INTEGRITY VIOLATION and send_message to parent.
