# Dispatch Assignment — Milestone M2 Worker (Replacement Gen 3)

## 2026-09-22T19:00:19Z

## Identity
- Role: M2 Implementation Worker
- Type: teamwork_preview_worker
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_worker_m2_3
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
Read these files before starting work:
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout, and invariants)
3. Explorer Reports:
   - `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m2_1\handoff.md` (Headers decomposition & docstring catalog)
   - `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m2_2\handoff.md` (Request & Response decomposition & docstrings)
   - `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m2_3\handoff.md` (Body, Chunk, Compress, Multipart decomposition & gate specifications)

## Mandatory Integrity Warning
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

## Objective & Scope
Execute Milestone M2: HTTP Message Model & Parser Modularization across `proto/http`:
1. Purge `.tmp/` scratch scripts (`.tmp/split_header.go`, `.tmp/split_http.go`, `.tmp/split_http2.go`, and `.tmp/` dir).
2. Decompose Body & Transfer Coding into:
   - `proto/http/body_chunked.go` (chunk.go + http.go chunked methods)
   - `proto/http/body_identity.go` (identity, fixed-size & writeBufio)
   - `proto/http/body_compress.go` (compression stream adapters)
   - `proto/http/multipart.go` (multipart encoder/decoder)
   - Update `proto/http/pool.go` (request/response ByteBuffer pools and `SetBodySizePoolLimit`)
   - Update `proto/http/errors.go` (absorb `ErrGetOnly`, `maxInterimResponses`, etc.)
   - Permanently delete `proto/http/http.go` and `proto/http/chunk.go`.
3. Decompose HTTP Request into:
   - `proto/http/request.go` (core struct, URI, Host, Reset, String)
   - `proto/http/request_body.go` (in-memory body, body swapping, decompression, scoped borrow)
   - `proto/http/request_stream.go` (streaming body, consolidates `streaming.go` RequestStream & pooling)
   - `proto/http/request_wire.go` (Read, Write, WriteVectored, 100-continue)
   - `proto/http/request_forms.go` (PostArgs, MultipartForm)
   - Permanently delete `proto/http/streaming.go` (absorbed into `request_stream.go`).
4. Decompose HTTP Response into:
   - `proto/http/response.go` (core struct, StatusCode, Addr, Reset, String)
   - `proto/http/response_body.go` (in-memory body, body swapping, decompression, scoped borrow)
   - `proto/http/response_stream.go` (streaming body, SendFile, ReadStreamScoped)
   - `proto/http/response_wire.go` (Read, Write, on-the-fly wire compression WriteGzip/WriteBrotli/etc.)
5. Decompose HTTP Headers into:
   - `proto/http/header.go` (core types, header, RequestHeader, ResponseHeader, ErrNothingRead, ErrSmallBuffer, sentinels)
   - `proto/http/header_parse.go` (Read, Write, parseFirstLine, parseHeaders, AppendBytes)
   - `proto/http/header_fields.go` (Peek, Set, Add, Del, All, normalization, absorbed helpers)
   - `proto/http/header_cookies.go` (RFC 6265 cookies for RequestHeader & ResponseHeader)
   - `proto/http/header_trailers.go` (RFC 9112 §7.1.2 trailers, parseTrailerHeaders, validation)
   - `proto/http/header_scoped.go` (scoped zero-alloc borrow methods)
   - `proto/http/headers.go` (standard header constants with comprehensive docstrings)
   - Retain `proto/http/headerscanner.go` (SIMD header scanner)
   - Permanently delete `proto/http/header_request.go`, `proto/http/header_response.go`, `proto/http/header_helpers.go`.

## Strict Invariants:
- BSD license header on every Go file (`// Copyright (c) 2026 Lemon4ksan All rights reserved.\n// Use of this source code is governed by a BSD-style\n// license that can be found in the LICENSE file.\n\npackage http`).
- Full RFC docstrings (RFC 9110, RFC 9112, etc.) on ALL exported types, functions, methods, and constants.
- Zero heap allocations (`0 B/op`, `0 allocs/op`) on hot paths (`BenchmarkFullPipeline_ScopedBorrow`, `BenchmarkPool_PerPStorage_Parallel`, `BenchmarkBorrow_Scoped`, `BenchmarkCookie_Scoped`, `BenchmarkURI_Scoped`).
- 100% public API compatibility. No exported symbols removed or signature altered.
- All tests pass: `go test -v -race -timeout 90s ./proto/http/...`, `go test ./...`, and `golangci-lint run ./proto/http/...`.

## Output:
- Write detailed progress to `.agents/teamwork_preview_worker_m2_3/progress.md`.
- Upon completion, write self-contained `handoff.md` in `.agents/teamwork_preview_worker_m2_3/` with test/benchmark results and notify parent via `send_message`.
