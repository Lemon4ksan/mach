## 2026-09-22T14:37:16Z

You are the Worker for Milestone M1 (Core Protocol Frame & Codec Decomposition) of the mach refactoring project.
Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_worker_m1_1
Original user request file: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
Master Project Plan: d:\CodingProjects\mach\.agents\PROJECT.md

Explorer Reports to read:
1. d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m1_1_gen2\handoff.md
2. d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m1_2_gen2\handoff.md
3. d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m1_3_gen2\handoff.md

MANDATORY INTEGRITY WARNING:
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

Scope & Exclusive File Write Ownership Boundaries:
- proto/h2/frame_data.go (new)
- proto/h2/frame_headers.go (new)
- proto/h2/frame_control.go (new)
- proto/h2/frame_window.go (new)
- proto/h2/frame_ext.go (new)
- proto/h2/frames.go (remove or purge)
- proto/h2/overlay/frame_test.go (add unit test assertions)
- proto/h3/qpack.go (decomposed)
- proto/h3/qpack_client.go (new)
- proto/h3/qpack_server.go (new)
- proto/h3/qpack_rules.go (new)
- proto/compress/compress.go (decomposed)
- proto/compress/gzip.go (new)
- proto/compress/flate.go (new)
- proto/compress/brotli.go (docstring fix)
- proto/compress/zstd.go (docstring fix)
- .agents/teamwork_preview_worker_m1_1/*

Instructions:
1. Read ORIGINAL_REQUEST.md, PROJECT.md, and the 3 Explorer handoff reports.
2. Implement the modular decompositions using the exact blueprints provided by Explorers 1 and 2.
3. Ensure every file starts with the mandatory 3-line BSD license header.
4. Provide comprehensive RFC docstrings (RFC 9113, RFC 9204, RFC 9114, RFC 1952, RFC 1951, RFC 7932, RFC 8878) and concurrency/lifecycle notes for all exported symbols.
5. Add unit tests for in-situ overlay in proto/h2/overlay/frame_test.go.
6. Run builds, tests, and micro-benchmarks to verify:
   - go test -v -race ./proto/h2/... ./proto/h3/... ./proto/compress/...
   - go test -v -race ./client/h2/... ./client/h3/... ./server/h2/... ./server/h3/...
   - BenchmarkInSituOverlay (0 B/op, 0 allocs/op)
   - BenchmarkAcquireRelease_PerGoroutinePool (0 B/op, 0 allocs/op)
   - BenchmarkH3_FrameHeaderPack (0 B/op, 0 allocs/op)
7. Document all commands and results in your handoff report (d:\CodingProjects\mach\.agents\teamwork_preview_worker_m1_1\handoff.md), update progress.md, and notify parent.
