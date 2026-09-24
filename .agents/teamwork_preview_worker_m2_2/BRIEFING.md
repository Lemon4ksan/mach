# BRIEFING — 2026-09-22T15:57:30Z

## Mission
Execute Milestone M2: HTTP Message Model & Parser Modularization across `proto/http` without regressions, maintaining 100% public API compatibility, 0 B/op on hot paths, full RFC docstrings, and passing tests/lint.

## 🔒 My Identity
- Archetype: teamwork_preview_worker
- Roles: implementer, qa, specialist
- Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_worker_m2_2
- Original parent: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac
- Milestone: M2 - HTTP Message Model & Parser Modularization

## 🔒 Key Constraints
- BSD license header on every Go file (`// Copyright (c) 2026 Lemon4ksan All rights reserved.\n// Use of this source code is governed by a BSD-style\n// license that can be found in the LICENSE file.\n\npackage http`).
- Full RFC docstrings (RFC 9110, RFC 9112, RFC 7578, etc.) on ALL exported types, functions, methods, and constants.
- Zero heap allocations (0 B/op, 0 allocs/op) on hot paths (BenchmarkFullPipeline_ScopedBorrow, BenchmarkPool_PerPStorage_Parallel, BenchmarkBorrow_Scoped, BenchmarkCookie_Scoped, BenchmarkURI_Scoped).
- 100% public API compatibility. No exported symbols removed or signature altered.
- Pass tests: `go test -v -race -timeout 90s ./proto/http/...` and `go test ./...`.
- Pass linter: `golangci-lint run ./proto/http/...`.
- Purge `.tmp/` scratch scripts.
- Only `.agents/` metadata in `.agents/` folder.

## Current Parent
- Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac
- Updated: 2026-09-22T15:57:30Z

## Task Summary
- **What to build**: Modularize `proto/http` by decomposing monolithic files (`http.go`, `chunk.go`, `streaming.go`, `header_request.go`, `header_response.go`, `header_helpers.go`) into focused single-responsibility files (Headers, Request, Response, Body/Transfer/Compress/Multipart), update `pool.go` and `errors.go`, purge `.tmp/`, ensure comprehensive RFC docstrings and 100% API compatibility.
- **Success criteria**: All tests pass with `-race`, all benchmarks pass with zero allocs on hot paths, linter passes cleanly, file layout strictly matches M2 architecture.
- **Interface contracts**: `.agents/PROJECT.md`, `.agents/ORIGINAL_REQUEST.md`, Explorer handoff reports.
- **Code layout**: `proto/http/` files as specified in dispatch.

## Key Decisions Made
- Follow the exact target layout and docstring catalogs provided in the 3 Explorer handoff reports.

## Artifact Index
- `DISPATCH.md` — Dispatch assignment
- `BRIEFING.md` — Situational awareness
- `progress.md` — Progress tracker and heartbeat
- `handoff.md` — Final handoff report

## Change Tracker
- **Files modified**: [TBD]
- **Build status**: [TBD]
- **Pending issues**: [TBD]

## Quality Status
- **Build/test result**: [TBD]
- **Lint status**: [TBD]
- **Tests added/modified**: [TBD]
