# Dispatch Assignment — Milestone M3 Reviewer 1 (Standards & Clean Code)

## Identity
- Role: M3 Reviewer 1 (Standards & Clean Code)
- Type: teamwork_preview_reviewer
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_reviewer_m3_1
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout §5, invariants §6)
3. `d:\CodingProjects\mach\.agents\TEST_READY.md` (test runner command & expected suites)
4. Worker handoff: `d:\CodingProjects\mach\.agents\teamwork_preview_worker_m3_1\handoff.md`

## Review Focus
1. Verify exact 3-line BSD license header on EVERY `.go` file in `client/` (`client/pool.go`, `client/h1/`, `client/h2/`, `client/h3/`).
2. Verify comprehensive, authoritative RFC docstrings (RFC 9112, 9110, 9113, 9114, 9204) on ALL exported types, functions, methods, and constants in `client/`.
3. Check `golangci-lint run ./client/...` (must be 0 issues; remember to run with `$env:GOWORK="off"`).
4. Run unit tests across all 4 client packages: `go test -v ./client/...` (ensure no `[no test files]`).
5. Run E2E test suite: `go test -v ./tests/e2e/...` (62/62 pass).

## Output
Write `handoff.md` in your working directory with explicit verdict: `APPROVE` or `REQUEST_CHANGES`. Notify parent via `send_message`.

## 2026-09-22T20:06:33Z
You are M3 Reviewer 1 (Standards & Clean Code) for Milestone M3.
Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_reviewer_m3_1
Parent: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac

Read:
1. d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
2. d:\CodingProjects\mach\.agents\PROJECT.md
3. d:\CodingProjects\mach\.agents\teamwork_preview_reviewer_m3_1\DISPATCH.md
4. d:\CodingProjects\mach\.agents\teamwork_preview_worker_m3_1\handoff.md

Verify:
- Exact 3-line BSD header on all .go files in client/ (pool, h1, h2, h3)
- Comprehensive RFC docstrings on all exported symbols and constants in client/
- golangci-lint run ./client/... (must be 0 issues; run with $env:GOWORK="off")
- go test -v ./client/... (all packages pass, no [no test files])
- go test -v ./tests/e2e/... (62/62 pass)

Write handoff.md with verdict APPROVE or REQUEST_CHANGES and send_message to parent.
