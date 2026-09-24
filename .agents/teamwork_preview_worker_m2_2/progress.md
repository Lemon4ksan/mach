# Progress — Milestone M2 Worker

Last visited: 2026-09-22T15:57:30Z

## Current Status
- Initializing task.
- Reviewing reference documents: `ORIGINAL_REQUEST.md`, `PROJECT.md`, and the 3 explorer reports (`explorer_m2_1/handoff.md`, `explorer_m2_2/handoff.md`, `explorer_m2_3/handoff.md`).

## Checklist
- [ ] 0. Read all reference documents and inspect current git status and existing codebase
- [ ] 1. Purge `.tmp/` scratch scripts (`.tmp/split_header.go`, `.tmp/split_http.go`, `.tmp/split_http2.go`, and `.tmp/` dir)
- [ ] 2. Decompose Body & Transfer Coding:
  - [ ] `proto/http/body_chunked.go`
  - [ ] `proto/http/body_identity.go`
  - [ ] `proto/http/body_compress.go`
  - [ ] `proto/http/multipart.go`
  - [ ] Update `proto/http/pool.go`
  - [ ] Update `proto/http/errors.go`
  - [ ] Delete `proto/http/http.go` and `proto/http/chunk.go` (after request/response also decomposed)
- [ ] 3. Decompose HTTP Request:
  - [ ] `proto/http/request.go`
  - [ ] `proto/http/request_body.go`
  - [ ] `proto/http/request_stream.go`
  - [ ] `proto/http/request_wire.go`
  - [ ] `proto/http/request_forms.go`
  - [ ] Delete `proto/http/streaming.go`
- [ ] 4. Decompose HTTP Response:
  - [ ] `proto/http/response.go`
  - [ ] `proto/http/response_body.go`
  - [ ] `proto/http/response_stream.go`
  - [ ] `proto/http/response_wire.go`
- [ ] 5. Decompose HTTP Headers:
  - [ ] `proto/http/header.go`
  - [ ] `proto/http/header_parse.go`
  - [ ] `proto/http/header_fields.go`
  - [ ] `proto/http/header_cookies.go`
  - [ ] `proto/http/header_trailers.go`
  - [ ] `proto/http/header_scoped.go`
  - [ ] `proto/http/headers.go`
  - [ ] Retain `proto/http/headerscanner.go`
  - [ ] Delete `proto/http/header_request.go`, `proto/http/header_response.go`, `proto/http/header_helpers.go`
- [ ] 6. Verify BSD headers and RFC docstrings on all files and symbols
- [ ] 7. Build and run tests (`go test -v -race -timeout 90s ./proto/http/...`, `go test ./...`)
- [ ] 8. Run benchmarks for hot paths (`BenchmarkFullPipeline_ScopedBorrow`, `BenchmarkPool_PerPStorage_Parallel`, `BenchmarkBorrow_Scoped`, etc.)
- [ ] 9. Run linter (`golangci-lint run ./proto/http/...`) and fix any issues
- [ ] 10. Write `handoff.md` and report completion to parent
