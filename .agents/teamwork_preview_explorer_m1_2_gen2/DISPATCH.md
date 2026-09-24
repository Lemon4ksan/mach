## 2026-09-22T14:28:00Z
You are Explorer 2 for Milestone M1 (QPACK & Compression Decomposition) of the mach refactoring project.
Your working directory is: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m1_2_gen2
Original user request file: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
Master Project Plan: d:\CodingProjects\mach\.agents\PROJECT.md

Scope & Focus:
Decomposition of `proto/h3/qpack.go` (690 lines) and `proto/compress/compress.go` (536 lines):
- `proto/h3/qpack.go` into:
  * `proto/h3/qpack.go`: `QPACKCodec` struct, `NewQPACKCodec`, `NewQPACKCodecWithOptions`, error tracking (`recordError`, `Err`, `ErrChan`, `SetErrorHandler`, `QPACKStreamError`).
  * `proto/h3/qpack_client.go`: Client-side methods: `EncodeRequestHeaders`, `getOrderedHeaders`, `DecodeResponseHeaders`, `DecodeResponseTrailers`, `responseHeaderHandler`, `trailersHandler`.
  * `proto/h3/qpack_server.go`: Server-side methods: `DecodeRequestHeaders`, `EncodeResponseHeaders`, `requestHeaderHandler`.
  * `proto/h3/qpack_rules.go`: Forbidden header rules (`isForbiddenH3Header`, `isForbiddenH3HeaderStr`).
- `proto/compress/compress.go` into:
  * `proto/compress/compress.go`: Compression levels (`CompressNoCompression`, etc.), common error limits, file type detection.
  * `proto/compress/gzip.go`: Gzip reader/writer pools, stackless gzip writers, `WriteGzip`, `WriteGunzip`, `AppendGzipBytes`, `AppendGunzipBytes`.
  * `proto/compress/flate.go`: Flate reader/writer pools, stackless deflate writers, `WriteDeflate`, `WriteInflate`, `AppendDeflateBytes`, `AppendInflateBytes`.

Tasks:
1. You MUST read `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` and `d:\CodingProjects\mach\.agents\PROJECT.md`.
2. Inspect `proto/h3/qpack.go`, `proto/h3/qpack_test.go`, `proto/compress/compress.go`, `proto/compress/compress_test.go`, `proto/compress/brotli.go`, `proto/compress/zstd.go`.
3. Inventory all types, methods, functions, and constants in `proto/h3/qpack.go` and `proto/compress/compress.go`.
4. Check downstream consumption by `client/h3/export.go`, `client/h3/conn.go`, `server/h3/server_conn.go`, and `aoni` to verify 100% public API compatibility.
5. Catalog docstring violations in `proto/h3/qpack.go` (7 violations) and `proto/compress/compress.go` (6 violations) + `brotli.go` and `zstd.go` (3 violations) and draft RFC-compliant docstrings (RFC 9204 for QPACK, RFC 9114 for HTTP/3 rules, RFC 1952 for Gzip, RFC 1951 for Deflate).
6. Provide concrete file mapping and function distribution for the upcoming Worker implementation.
7. Write your findings to `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m1_2_gen2\handoff.md`, update your `progress.md`, and notify the orchestrator.
