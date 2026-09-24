# Dispatch Assignment — Milestone M3 Reviewer 2 (API & Downstream Compatibility)

## Identity
- Role: M3 Reviewer 2 (API & Downstream Compatibility)
- Type: teamwork_preview_reviewer
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_reviewer_m3_2
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout §5, invariants §6)
3. `d:\CodingProjects\mach\.agents\TEST_READY.md` (test runner command & expected suites)
4. Worker handoff: `d:\CodingProjects\mach\.agents\teamwork_preview_worker_m3_1\handoff.md`

## Review Focus
1. Verify 100% public API compatibility in `client/`, `client/h1/`, `client/h2/`, `client/h3/` per `PROJECT.md` §4.1, §4.2, §4.3.
2. Verify downstream compilation and tests across all packages:
   - `$env:GOWORK="off"; go test ./client/... ./server/... ./proto/... ./tests/e2e/...`
3. Verify all 62/62 E2E tests pass under race detection:
   - `$env:GOWORK="off"; go test -v -race -timeout 120s ./tests/e2e/...`
4. Verify `client/pool.go` socket leak fix (closing evicted connections implementing `io.Closer`) and `Close() error`.

## Output
Write `handoff.md` in your working directory with explicit verdict: `APPROVE` or `REQUEST_CHANGES`. Notify parent via `send_message`.

## 2026-09-22T20:06:33Z
You are M3 Reviewer 2 (API & Downstream Compatibility) for Milestone M3.
Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_reviewer_m3_2
Parent: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac

Read:
1. d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
2. d:\CodingProjects\mach\.agents\PROJECT.md
3. d:\CodingProjects\mach\.agents\teamwork_preview_reviewer_m3_2\DISPATCH.md
4. d:\CodingProjects\mach\.agents\teamwork_preview_worker_m3_1\handoff.md

Verify:
- 100% public API compatibility in client/, client/h1/, client/h2/, client/h3/ per PROJECT.md §4.1, §4.2, §4.3
- Downstream compilation & test pass: $env:GOWORK="off"; go test ./client/... ./server/... ./proto/... ./tests/e2e/...
- 62/62 E2E tests pass under race detection: $env:GOWORK="off"; go test -v -race -timeout 120s ./tests/e2e/...
- client/pool.go socket leak fix (closing evicted connections implementing io.Closer) and Close() error

Write handoff.md with verdict APPROVE or REQUEST_CHANGES and send_message to parent.

