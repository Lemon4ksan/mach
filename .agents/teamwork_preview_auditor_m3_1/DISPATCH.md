# Dispatch Assignment — Milestone M3 Forensic Auditor

## Identity
- Role: M3 Forensic Auditor
- Type: teamwork_preview_auditor
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_auditor_m3_1
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout §5, invariants §6)
3. Worker handoff: `d:\CodingProjects\mach\.agents\teamwork_preview_worker_m3_1\handoff.md`

## Forensic Audit Protocol
Execute independent integrity forensics across `client/`:
1. Static Analysis & Anti-Cheat:
   - Verify genuine client transport implementations across `client/h2/` (9 decomposed files), `client/h3/` (4 decomposed files + export.go), `client/h1/`, and `client/pool.go`.
   - Verify zero hardcoded test outputs, zero dummy facades, zero bypass logic.
2. Boundary Enforcement:
   - Check `git status` to verify the worker modified only files in `client/` and did not modify forbidden areas (`proto/`, `server/`, `tests/e2e/`).
3. Standards Audit:
   - Check every `.go` file in `client/` for the standard 3-line BSD license header.
   - Check all exported symbols for comprehensive RFC docstrings.
4. Independent Execution:
   - Run `$env:GOWORK="off"; go build ./client/...`
   - Run `$env:GOWORK="off"; go test -v -race ./client/...`
   - Run `$env:GOWORK="off"; golangci-lint run ./client/...`

Write `handoff.md` in your working directory with explicit verdict: `CLEAN` or `INTEGRITY VIOLATION`. Notify parent via `send_message`.

## 2026-09-22T20:06:33Z
You are M3 Forensic Auditor for Milestone M3.
Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_auditor_m3_1
Parent: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac

Read:
1. d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
2. d:\CodingProjects\mach\.agents\PROJECT.md
3. d:\CodingProjects\mach\.agents\teamwork_preview_auditor_m3_1\DISPATCH.md
4. d:\CodingProjects\mach\.agents\teamwork_preview_worker_m3_1\handoff.md

Verify:
- Anti-cheat static analysis: genuine client transport logic in client/h2, client/h3, client/h1, client/pool; zero facades, zero hardcoded test outputs
- Boundary enforcement: check git status, worker modified only permitted client/ files
- Standards audit: BSD license header on all .go files in client/; complete RFC docstrings on all exported symbols
- Independent build, test, and lint execution ($env:GOWORK="off")

Write handoff.md with verdict CLEAN or INTEGRITY VIOLATION and send_message to parent.

