# BRIEFING — 2026-09-22T19:47:41Z

## Mission
Investigate client/h2/conn.go (1,667 lines), create complete symbol inventory, concrete mapping to 9 single-responsibility files per PROJECT.md §5, verify silicon invariants (cacheline padding, SPSCRingBuffer), analyze TEST_READY.md §5 defects, and draft RFC 9113 docstrings & BSD headers.

## 🔒 My Identity
- Archetype: explorer
- Roles: investigation, synthesis
- Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_1
- Original parent: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac
- Milestone: M3 (Client Protocol Engine Decomposition)

## 🔒 Key Constraints
- Read-only investigation — do NOT implement
- Full symbol inventory with line numbers
- Concrete mapping into the 9 single-responsibility files per PROJECT.md §5
- Retain CPU cacheline padding (_ cpu.CacheLinePad) and SPSCRingBuffer
- Check TEST_READY.md §5 regarding client/h2 flow control uninitialized serverWindow and race on ctx.StreamID
- Draft RFC 9113 docstrings and BSD headers for all exported symbols
- Self-contained handoff.md report with 5 components: Observation, Logic Chain, Caveats, Conclusion, Verification Method

## Current Parent
- Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac
- Updated: not yet

## Investigation State
- **Explored paths**: `client/h2/conn.go`, `client/h2/context.go`, `client/h2/export.go`, `client/h2/conn_test.go`, `tests/e2e/h2_test.go`, `tests/e2e/helpers_test.go`, `d:/CodingProjects/aoni/internal/transport/pool.go`
- **Key findings**:
  1. Complete symbol mapping for all 1,667 lines of `conn.go` into 9 single-responsibility files.
  2. Silicon invariants verified: `_ cpu.CacheLinePad` at line 98 and `outRing *ringbuf.SPSCRingBuffer` at line 107.
  3. Escalation 4 verified: `serverWindow` starts at 0 instead of 65,535 (RFC 9113 §5.2.1), causing client request body stalls without explicit server WINDOW_UPDATE.
  4. Escalation 3 verified: data race on `ctx.StreamID` between `CancelStream` and `writeRequest`.
  5. Full RFC 9113 docstrings and BSD headers drafted for all exported symbols.
- **Unexplored areas**: None within client/h2 scope.

## Key Decisions Made
- Decompose `client/h2/conn.go` into 9 single-responsibility files matching PROJECT.md §5: `conn.go`, `stream_table.go`, `flow_control.go`, `read_loop.go`, `write_loop.go`, `request_writer.go`, `headers.go`, `push.go`, `dialer.go`.
- Retain existing `context.go` and `export.go`, updating `context.go` to use `atomic.Uint32` for `StreamID`.
- Propose `nc.serverWindow.Store(65535)` in `NewConn` to fix RFC 9113 flow control window bug.

## Artifact Index
- d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_1\BRIEFING.md — Persistent context & memory
- d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_1\progress.md — Liveness heartbeat & progress log
- d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_1\handoff.md — Final 5-component handoff report

