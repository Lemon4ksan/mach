# BRIEFING — 2026-09-22T23:21:24+03:00

## Mission
Decompose server protocol engines (`server/h2/`, `server/h1/`, `server/h3/`) per Explorers M4.1/M4.2/M4.3 blueprints, resolve Escalations 1 and 2, standardize buffer pooling, and enforce RFC compliance and BSD license headers.

## 🔒 My Identity
- Archetype: teamwork_preview_worker
- Roles: implementer, qa, specialist
- Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_worker_m4_1
- Original parent: 5990a2d7-7ec1-47d8-9672-52a9ad7ba846
- Milestone: M4 (Server Protocol Engine Decomposition)

## 🔒 Key Constraints
- Exclusive write ownership: `server/` files only.
- Strict Integrity Mandate: genuine implementation, no dummy facades, no hardcoded results.
- 100% public API compatibility across server package.
- 3-line BSD license header on every `.go` file in `server/`.
- Zero-allocation hot paths and Per-P buffer pooling.
- Comprehensive RFC docstrings on all exported symbols.
- All tests must pass with `-race`, 0 race warnings, 0 `golangci-lint` issues, 0 `go vet` warnings.

## Current Parent
- Conversation ID: 5990a2d7-7ec1-47d8-9672-52a9ad7ba846
- Updated: not yet

## Task Summary
- **What to build**:
  1. `server/h2/` modular decomposition into 5 files: `server_conn.go`, `read_loop.go`, `write_loop.go`, `stream.go`, `flow_control.go`. Resolve Escalation 2 (data race on `sc.streams` in `Release()`).
  2. `server/h1/` standardization & smuggling defect resolution (Escalation 1), Per-P buffer pooling fixes (writerStorage, readerStorage, body buffer capping, cookie zero-alloc, ReadAllChunked).
  3. `server/h3/` modular decomposition into 3 files: `server_conn.go`, `dispatch.go`, `stream.go`. Per-P buffer pooling and zero-alloc varint reader.
  4. Standards: 3-line BSD license headers, RFC docstrings (RFC 9112, 9110, 9113, 9114, 9204).
- **Success criteria**:
  - `go test -v -race -count=1 ./server/...` passes
  - `go test -v -race -count=5 -run "TestConnHandler_RequestSmuggling|TestH1_Tier2_RequestSmuggling" ./server/h1/... ./tests/e2e/...` passes
  - `go test -v -race -count=5 -run "TestH2Server_EndToEnd|TestH2_Tier3_AbruptConnectionDisconnectDuringInflight" ./server/h2/... ./tests/e2e/...` passes
  - `go test -v -race ./tests/e2e/...` (62/62 pass)
  - `golangci-lint run ./server/...` passes with 0 issues
  - `go vet ./server/...` passes with 0 warnings
- **Interface contracts**: `PROJECT.md`, `TEST_READY.md`
- **Code layout**: `PROJECT.md` §5 Code Layout for `server/`

## Change Tracker
- **Files modified**: none yet
- **Build status**: untried
- **Pending issues**: none

## Quality Status
- **Build/test result**: untried
- **Lint status**: untried
- **Tests added/modified**: pending

## Loaded Skills
- None specified in dispatch.

## Key Decisions Made
- Proceeding to read blueprints and inspect current codebase.

## Artifact Index
- `DISPATCH.md` — Assignment from parent
- `progress.md` — Liveness heartbeat
- `handoff.md` — Completion report (pending)
