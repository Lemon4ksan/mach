# BRIEFING — 2026-09-22T14:07:45Z

## Mission
Investigate and benchmark performance baselines, race conditions, fuzz tests, hot paths, and allocation profiles for the mach protocol engine refactoring project.

## 🔒 My Identity
- Archetype: explorer
- Roles: Performance & Baselines Explorer
- Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_survey_perf
- Original parent: 18869d5f-1e3b-48b9-9132-3f4ec56d6ab3
- Milestone: survey & baseline performance characterization

## 🔒 Key Constraints
- Read-only investigation — do NOT implement
- Do NOT modify any source code files
- Write only to .agents/teamwork_preview_explorer_survey_perf
- Provide exact benchmark and race detector numbers and findings

## Current Parent
- Conversation ID: 18869d5f-1e3b-48b9-9132-3f4ec56d6ab3
- Updated: 2026-09-22T14:07:45Z

## Investigation State
- **Explored paths**:
  - `README.md`, `ORIGINAL_REQUEST.md`, `scripts/fuzz_all.go`
  - `proto/h2/overlay`, `proto/h2`, `proto/h3`, `proto/http`, `proto/compress`, `proto/http/stackless`
  - `client/h1`, `client/h2`, `client/h3`, `server/h1`, `server/h2`, `server/h3`, `fsm/h2`, `x/raptor`
- **Key findings**:
  - `go test -race -timeout 90s ./...`: PASSED across all packages (0 failures, 0 race warnings).
  - `go run ./scripts/fuzz_all.go -fuzztime=5s`: PASSED 8/8 targets in 2m38s (0 panics, 0 errors).
  - Micro-benchmarks: 21 benchmarks run across 5 packages. InSitu overlay (0 B/op, 0 allocs), Varint pack (0 B/op, 0 allocs), Scoped borrow pipeline (0 B/op, 0 allocs), Per-P pool (0 B/op, 0 allocs).
  - Hot paths & foundation primitives analyzed: `simd`, `bytesconv`, `ringbuf`, `pool.NewPerPStorage`, `cpu.CacheLinePad`, `borrow`.
  - Non-zero allocation hotspots cataloged: `c.Do()` in `client/h2` & `client/h1` (errCh, Context, goroutine), `NewServerConn` in `server/h2`, `headerscanner` boundary in `proto/http`, QPACK dynamic encode/decode.
- **Unexplored areas**: None within scope.

## Key Decisions Made
- All tests and benchmarks executed natively on host environment (`12th Gen Intel Core i5-12400F`, Windows amd64, Go 1.27.0).
- Detailed comparison table created correlating measured numbers against `README.md` claims.

## Artifact Index
- d:\CodingProjects\mach\.agents\teamwork_preview_explorer_survey_perf\DISPATCH.md — Incoming assignment
- d:\CodingProjects\mach\.agents\teamwork_preview_explorer_survey_perf\progress.md — Liveness & progress tracking
- d:\CodingProjects\mach\.agents\teamwork_preview_explorer_survey_perf\handoff.md — Final 5-component handoff report
