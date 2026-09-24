## 2026-09-22T15:19:51Z
You are the Worker for Milestone M2 (HTTP Message Model & Parser Modularization) of the mach refactoring project.
Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_worker_m2_1
Original user request file: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
Master Project Plan: d:\CodingProjects\mach\.agents\PROJECT.md

Explorer Reports:
1. d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m2_1\handoff.md
2. d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m2_2\handoff.md
3. d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m2_3\handoff.md

MANDATORY INTEGRITY WARNING:
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

Scope & Exclusive File Write Ownership Boundaries:
- proto/http/header.go, header_parse.go, header_fields.go, header_cookies.go, header_trailers.go, header_scoped.go, headers.go
- delete proto/http/header_request.go, header_response.go, header_helpers.go
- proto/http/request.go, request_body.go, request_stream.go, request_wire.go, request_forms.go
- proto/http/response.go, response_body.go, response_stream.go, response_wire.go
- delete proto/http/streaming.go
- proto/http/body_chunked.go, body_identity.go, body_compress.go, multipart.go, pool.go
- delete proto/http/http.go, proto/http/chunk.go
- delete .tmp/split_header.go, .tmp/split_http.go, .tmp/split_http2.go
- .agents/teamwork_preview_worker_m2_1/*

Instructions:
1. Read ORIGINAL_REQUEST.md, PROJECT.md, and all 3 Explorer handoff reports.
2. Implement the modular decompositions across headers, requests, responses, and body transfer coding.
3. Purge obsolete scratch scripts in .tmp/.
4. Ensure every file starts with the mandatory 3-line BSD license header.
5. Provide comprehensive RFC docstrings (RFC 9110, RFC 9112, RFC 7578, RFC 3986) with concurrency and lifecycle notes for all exported symbols.
6. Verify implementation:
   - go test -v -race ./proto/http/...
   - go test -v -race ./tests/e2e/...
   - BenchmarkFullPipeline_ScopedBorrow (0 B/op, 0 allocs/op)
   - BenchmarkPool_PerPStorage_Parallel (0 B/op, 0 allocs/op)
   - BenchmarkBorrow_Scoped (0 B/op, 0 allocs/op)
   - golangci-lint run ./proto/http/...
7. Write handoff report to d:\CodingProjects\mach\.agents\teamwork_preview_worker_m2_1\handoff.md, update progress.md, and notify parent.
