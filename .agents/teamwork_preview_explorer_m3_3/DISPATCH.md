# Dispatch Assignment — Milestone M3 Explorer 3 (Client Pool, H1, Standards & Gate)

## Identity
- Role: M3 Explorer 3 (Client Pool, H1, Standards & Gate)
- Type: teamwork_preview_explorer
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_3
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout §5, invariants §6)
3. `d:\CodingProjects\mach\.agents\TEST_READY.md` (test runner command & expected suites)

## Investigation Scope
Investigate `client/pool.go`, `client/h1/conn.go`, and repository standards across `client/`:
1. Full symbol inventory: map `client/pool.go` (connection pool manager) and `client/h1/conn.go` (HTTP/1.1 client connection).
2. Public API surface audit:
   - Check Downstream Consumer Contract in `PROJECT.md` §4.3: `client/h1.ClientConn: Do(ctx, req, res), Close()`.
   - Check connection pool interfaces and types in `client/pool.go`.
3. Standards & Docstrings Audit:
   - Identify missing BSD license headers and missing docstrings across `client/pool.go` and `client/h1/`.
   - Draft comprehensive RFC 9112 / RFC 9110 docstrings and concurrency expectations.
4. M3 Verification Gate Design:
   - Define exact test commands, race detector runs, benchmark targets, and linter parameters for Milestone M3 acceptance.
   - Define write boundaries for the M3 Worker.

## Output
Write `handoff.md` in your working directory with sections: Observation, Logic Chain, Caveats, Conclusion, Verification Method. Notify parent via `send_message`.

## 2026-09-22T19:47:41Z
You are M3 Explorer 3 (Client Pool, H1, Standards & Gate) for Milestone M3.
Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_3
Parent: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac

Read:
1. d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
2. d:\CodingProjects\mach\.agents\PROJECT.md
3. d:\CodingProjects\mach\.agents\TEST_READY.md
4. d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_3\DISPATCH.md

Investigate client/pool.go, client/h1/conn.go, and repo standards across client/:
- Full symbol inventory of client/pool.go and client/h1/conn.go
- Public API surface audit per PROJECT.md §4.3
- BSD headers and RFC docstrings audit
- Design M3 Verification Gate (test commands, race test, benchmarks, linter, worker write boundaries)

Write handoff.md with Observation, Logic Chain, Caveats, Conclusion, Verification Method and send_message to parent.
