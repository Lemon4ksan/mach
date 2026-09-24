## 2026-09-22T14:14:51Z
You are M1 Explorer 2 (QPACK & Compression Explorer) for Milestone 1.
Your working directory is: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m1_2
Original user request file: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
Project plan file: d:\CodingProjects\mach\.agents\PROJECT.md

Task:
1. Read d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md and d:\CodingProjects\mach\.agents\PROJECT.md.
2. Investigate `proto/h3/qpack.go` (690 lines) and `proto/compress/compress.go` (536 lines).
3. Plan the exact modular decomposition of `proto/h3/qpack.go` into:
   - `proto/h3/qpack.go`: QPACKCodec struct, constructor, options, error channel/callback tracking.
   - `proto/h3/qpack_client.go`: Client-side request header encoding, response header/trailer decoding.
   - `proto/h3/qpack_server.go`: Server-side request header decoding, response header encoding.
   - `proto/h3/qpack_rules.go`: RFC 9114 forbidden header checks and header validation.
4. Plan the exact modular decomposition of `proto/compress/compress.go` into:
   - `proto/compress/compress.go`: Compression constants, level helpers, file type detection.
   - `proto/compress/gzip.go`: Gzip reader/writer pools, stackless writers, limit readers.
   - `proto/compress/flate.go`: Deflate reader/writer pools, stackless writers, limit readers.
5. Provide exact docstrings citing RFC 9204, RFC 9114, RFC 1952, RFC 1951, and concurrency invariants.
6. Write your complete decomposition blueprint to `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m1_2\handoff.md`, update progress.md, and send a message to parent.
DO NOT modify source code files directly — your role is technical exploration and blueprint specification.
