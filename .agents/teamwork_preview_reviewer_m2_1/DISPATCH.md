# Dispatch Assignment — Milestone M2 Reviewer 1 (Standards & Clean Code)

## Identity
- Role: M2 Reviewer 1 (Standards & Clean Code)
- Type: teamwork_preview_reviewer
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_reviewer_m2_1
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
Read these files before starting review:
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout, and invariants)
3. `d:\CodingProjects\mach\.agents\TEST_READY.md` (test runner command & expected suites)
4. Worker handoff: `d:\CodingProjects\mach\.agents\teamwork_preview_worker_m2_3\handoff.md`

## Review Focus
1. Verify exact 3-line BSD license header on EVERY `.go` file in `proto/http/`.
2. Verify comprehensive, authoritative RFC docstrings (RFC 9110, RFC 9112, etc.) on ALL exported types, functions, methods, and constants in `proto/http/` (especially all 120 constants in `proto/http/headers.go`).
3. Verify that all 6 obsolete files (`http.go`, `chunk.go`, `streaming.go`, `header_request.go`, `header_response.go`, `header_helpers.go`) have been permanently removed.
4. Run `golangci-lint run --timeout 5m ./proto/http/...` and verify 0 issues.
5. Run unit and E2E tests: `go test -v ./proto/http/...` and `go test -v ./tests/e2e/...`.

## Output
Write `handoff.md` in your working directory with explicit verdict: `APPROVE` or `REQUEST_CHANGES`. Notify parent via `send_message`.

## 2026-09-22T19:37:56Z
You are M2 Reviewer 1 (Standards & Clean Code) for Milestone M2.
Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_reviewer_m2_1
Parent: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac

Read:
1. d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
2. d:\CodingProjects\mach\.agents\PROJECT.md
3. d:\CodingProjects\mach\.agents\teamwork_preview_reviewer_m2_1\DISPATCH.md
4. d:\CodingProjects\mach\.agents\teamwork_preview_worker_m2_3\handoff.md

Verify:
- Exact 3-line BSD header on all .go files in proto/http/
- RFC docstrings on all exported symbols and constants in proto/http/
- Deletion of all 6 obsolete monolith files
- golangci-lint run --timeout 5m ./proto/http/... (must be 0 issues)
- go test -v ./proto/http/... and go test -v ./tests/e2e/...

Write handoff.md with verdict APPROVE or REQUEST_CHANGES and send_message to parent.
