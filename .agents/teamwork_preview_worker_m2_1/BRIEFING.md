# BRIEFING — 2026-09-22T15:20:00Z

## Mission
Execute Milestone M2 (HTTP Message Model & Parser Modularization) of the mach refactoring project, implementing modular decomposition of HTTP headers, requests, responses, body transfer coding, multipart, and pool without regressions, ensuring 0 B/op zero-allocation performance and full RFC compliance.

## 🔒 My Identity
- Archetype: worker
- Roles: implementer, qa, specialist
- Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_worker_m2_1
- Original parent: 5d05cf1e-7247-466c-b645-4e25e1408e3e
- Milestone: M2

## 🔒 Key Constraints
- DO NOT CHEAT: Genuine implementations only, no dummy/facade implementations, no hardcoding test results.
- Mandatory 3-line BSD license header on every Go file.
- Comprehensive RFC docstrings (RFC 9110, RFC 9112, RFC 7578, RFC 3986) with concurrency and lifecycle notes for all exported symbols.
- Performance invariants: BenchmarkFullPipeline_ScopedBorrow (0 B/op, 0 allocs/op), BenchmarkPool_PerPStorage_Parallel (0 B/op, 0 allocs/op), BenchmarkBorrow_Scoped (0 B/op, 0 allocs/op).
- Passing tests: `go test -v -race ./proto/http/...`, `go test -v -race ./tests/e2e/...`, `golangci-lint run ./proto/http/...`.
- Purge obsolete scratch scripts in .tmp/ (.tmp/split_header.go, .tmp/split_http.go, .tmp/split_http2.go).
- Scope boundaries: only edit allowed files in proto/http/ and delete obsolete files as specified.

## Current Parent
- Conversation ID: 5d05cf1e-7247-466c-b645-4e25e1408e3e
- Updated: not yet

## Task Summary
- **What to build**: Modular decomposition of proto/http package into:
  - Header files: header.go, header_parse.go, header_fields.go, header_cookies.go, header_trailers.go, header_scoped.go, headers.go (and delete header_request.go, header_response.go, header_helpers.go)
  - Request files: request.go, request_body.go, request_stream.go, request_wire.go, request_forms.go
  - Response files: response.go, response_body.go, response_stream.go, response_wire.go
  - Delete streaming.go, http.go, chunk.go
  - Body & pool files: body_chunked.go, body_identity.go, body_compress.go, multipart.go, pool.go
  - Delete .tmp/split_header.go, .tmp/split_http.go, .tmp/split_http2.go
- **Success criteria**: All tests pass with -race, benchmarks retain 0 allocs/op, linter clean, clean package layout.
- **Interface contracts**: PROJECT.md, ORIGINAL_REQUEST.md
- **Code layout**: proto/http/

## Key Decisions Made
- [TBD]

## Change Tracker
- **Files modified**: [TBD]
- **Build status**: [TBD]
- **Pending issues**: [TBD]

## Quality Status
- **Build/test result**: [TBD]
- **Lint status**: [TBD]
- **Tests added/modified**: [TBD]

## Loaded Skills
None requested.

## Artifact Index
- d:\CodingProjects\mach\.agents\teamwork_preview_worker_m2_1\progress.md — liveness heartbeat and subtask progress
- d:\CodingProjects\mach\.agents\teamwork_preview_worker_m2_1\handoff.md — 5-component handoff report
