# BRIEFING — 2026-09-22T19:48:55Z

## Mission
Investigate client/h3 architecture, produce complete symbol inventory, map decomposition into 5 single-responsibility files per PROJECT.md §5, verify zero-alloc invariants and QUIC stream lifecycle, and draft RFC 9114/9204 docstrings and BSD headers.

## 🔒 My Identity
- Archetype: teamwork_preview_explorer
- Roles: M3 Explorer 2 (Client H3 Architecture & Decomposition)
- Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2
- Original parent: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac
- Milestone: M3 (Client Protocol Engine Decomposition)

## 🔒 Key Constraints
- Read-only investigation — do NOT implement directly in source files.
- Deliver findings, symbol inventory, decomposition mapping, and drafted code in agent directory artifacts (handoff.md).
- Preserve exact public API signatures per PROJECT.md §4.2, §4.3.
- All Go files must have standard 3-line BSD header and RFC 9114 / RFC 9204 docstrings.
- Verify zero-allocation hot paths and QUIC stream lifecycle concurrency.

## Current Parent
- Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac
- Updated: not yet

## Investigation State
- **Explored paths**:
  - `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md`
  - `d:\CodingProjects\mach\.agents\PROJECT.md`
  - `d:\CodingProjects\mach\.agents\TEST_READY.md`
  - `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2\DISPATCH.md`
  - `client/h3/conn.go` (560 lines)
  - `client/h3/export.go` (28 lines)
  - `client/h3/conn_test.go` (493 lines)
  - `proto/h3/qpack_client.go`
  - `proto/http/header_scoped.go`
  - `tests/e2e/h3_test.go`
  - `tests/e2e/helpers_test.go`
- **Key findings**:
  - `client/h3/conn.go` (560 lines) decomposed into 4 files + `export.go` per PROJECT.md §5: `conn.go` (lifecycle), `control.go` (unidirectional demux), `request.go` (Do/DoScoped, send), `response.go` (readResponse, pools), `export.go` (type aliases).
  - 100% public API stability: `ClientConn`, `NewClientConn`, `Do`, `DoScoped`, `IsClosed`, `Close`, `Transport`, `UnderlyingCloser`, and all export aliases preserved.
  - Package-internal test APIs preserved: `sendRequestTo`, `readResponseFrom`, `readResponseScoped` retained on `ClientConn` so `conn_test.go` passes unmodified.
  - Zero-alloc optimizations identified:
    1. Pool `requestHeaderBufferPool` for QPACK header encoding buffer eliminating `bytes.Buffer{}` heap escape.
    2. Vectored send on payload > 8KB avoiding `make([]byte, 0, totalLen)` heap allocations.
    3. StreamID dynamic extraction in `readResponseScoped` avoiding hardcoded 0.
    4. Explicit `ErrCodeH3NoError` (0x0100) and `ErrCodeH3RequestCancelled` (0x010c) RFC named constants.
  - Full RFC 9114, RFC 9204, RFC 9000, RFC 9221 docstrings and BSD headers drafted.
- **Unexplored areas**: None. Investigation complete and fully verified.

## Key Decisions Made
- Decompose `client/h3` into exactly 5 files matching `PROJECT.md` §5.
- Draft drop-in replacement files (`proposed_conn.go`, `proposed_control.go`, `proposed_request.go`, `proposed_response.go`, `proposed_export.go`) in agent directory.
- Preserve all existing exported and test-internal signatures with 100% test compatibility.

## Artifact Index
- `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2\BRIEFING.md` — persistent memory
- `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2\progress.md` — liveness heartbeat
- `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2\proposed_conn.go` — decomposed conn.go
- `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2\proposed_control.go` — decomposed control.go
- `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2\proposed_request.go` — decomposed request.go
- `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2\proposed_response.go` — decomposed response.go
- `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2\proposed_export.go` — decomposed export.go
- `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2\handoff.md` — 5-component handoff report
