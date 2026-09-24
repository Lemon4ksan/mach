# BRIEFING — 2026-09-22T20:21:00Z

## Mission
Milestone M4: Server H1 & H3 Modularization & Per-P Buffer Pooling exploration. Complete investigation of server/h1 and server/h3 architecture, analyze current pooling vs proto/http/pool.go, design standardized Per-P buffer pooling, check PROJECT.md §5 layout compliance, verify docstring and BSD header invariants, and produce structured handoff report with step-by-step guidance for Worker M4.1.

## 🔒 My Identity
- Archetype: explorer
- Roles: Teamwork explorer (read-only investigation, analysis, synthesis, handoff report)
- Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_2
- Original parent: 5990a2d7-7ec1-47d8-9672-52a9ad7ba846
- Milestone: M4 (Server H1 & H3 Modularization & Per-P Buffer Pooling)

## 🔒 Key Constraints
- Read-only investigation — do NOT implement or modify source code
- Files for content delivery, Messages for coordination
- Format messages as Context / Content / Action
- Self-contained 5-component handoff report (Observation, Logic Chain, Caveats, Conclusion, Verification Method)
- $env:GOWORK="off" for Go commands
- Invariant checking: exact 3-line BSD header, RFC docstrings on exported symbols

## Current Parent
- Conversation ID: 5990a2d7-7ec1-47d8-9672-52a9ad7ba846
- Updated: 2026-09-22T20:21:00Z

## Investigation State
- **Explored paths**:
  - `server/h1/`: `conn.go`, `request.go`, `response.go`, `chunked.go`, `status.go`, `header.go`, `errors.go`, `generate.go`, `h1_test.go`, `h1_fuzz_test.go`
  - `server/h3/`: `server_conn.go`, `h3_server_test.go`, `h3_bench_test.go`
  - `client/h3/`: `conn.go`, `control.go`, `request.go`, `response.go`, `export.go`
  - `proto/http/pool.go`
  - `foundation/silicon/pool/perp_storage.go`
  - `tests/e2e/h1_test.go`, `tests/e2e/h3_test.go`
- **Key findings**:
  - `server/h3/`: Monolithic `server_conn.go` lacks `dispatch.go` and `stream.go` per `PROJECT.md` §5; has 0 buffer pooling. Can achieve zero allocations via Per-P storage (`serverReqStorage`, `serverResStorage`, `h3HeaderBlockStorage`, `h3ReaderStorage`, `h3BodyBufferStorage`) and by using `*bufio.Reader` directly satisfying `varint.Reader` (0 allocs).
  - `server/h1/`: Per-P storage has release asymmetry (`bytesconv.ReleaseByteBuffer` instead of `writerStorage.Put`), leaks socket pointer in idle `*bufio.Reader` without `br.Reset(nil)`, and lacks body buffer size bounding.
  - Escalation 1 confirmed: `request.go:254` deletes `ContentLength`, preventing keepAlive=false in `conn.go:155` for request smuggling mitigation. Fixed via `req.MustClose`.
  - Zero-alloc hot path optimizations identified for cookie writing (`c.WriteTo(bw)`), chunked reading (pooled `ByteBuffer`), and stack header buffers.
- **Unexplored areas**: None in scope. All files and invariants examined.

## Key Decisions Made
- Fully specified decomposition of `server/h3/` into `server_conn.go`, `dispatch.go`, and `stream.go`.
- Designed standardized Per-P storage buffer pooling for both `server/h1` and `server/h3`.
- Formulated exact step-by-step guidance for Worker M4.1 in `handoff.md`.

## Artifact Index
- DISPATCH.md — Dispatch log
- BRIEFING.md — Persistent context & memory
- progress.md — Liveness & progress heartbeat
- handoff.md — Final structured report for Worker M4.1
