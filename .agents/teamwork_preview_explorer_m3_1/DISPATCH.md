# Dispatch Assignment — Milestone M3 Explorer 1 (Client H2 Monolith Decomposition)

## Identity
- Role: M3 Explorer 1 (Client H2 Monolith Decomposition)
- Type: teamwork_preview_explorer
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_1
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout §5, invariants §6)
3. `d:\CodingProjects\mach\.agents\TEST_READY.md` (test runner command & expected suites)

## Investigation Scope
Investigate `client/h2/conn.go` (1,667 lines) and associated files in `client/h2`:
1. Full symbol inventory: map every struct, method, function, variable, constant, and atomic field with line numbers.
2. Target decomposition mapping into single-responsibility files per `PROJECT.md` §5:
   - `conn.go`: struct Conn, NewConn, exported API facade
   - `stream_table.go`: stream table open-addressing & overflow buckets
   - `flow_control.go`: connection & stream window accounting
   - `read_loop.go`: socket read loop & frame demuxing
   - `write_loop.go`: socket write loop & SPSC ring batching
   - `request_writer.go`: request framing, DATA chunking, 100-continue
   - `headers.go`: HPACK encoding/decoding & forbidden filters
   - `push.go`: server push promise handling
   - `dialer.go`: network dialing & connection factory
   - `context.go`: stream context & lifecycle state
   - `export.go`: downstream type aliases (must preserve exact signatures)
3. Silicon Performance Invariants (F12):
   - Identify CPU cacheline padding (`_ cpu.CacheLinePad`) on atomic counters to prevent SMP false sharing.
   - Retain `ringbuf.SPSCRingBuffer` for lock-free batching.
4. Pre-existing defect analysis:
   - Check `TEST_READY.md` §5 regarding `client/h2` flow control uninitialized `serverWindow` and race on `ctx.StreamID`.
5. Propose comprehensive RFC 9113 docstrings and BSD headers for all exported symbols.

## Output
Write `handoff.md` in your working directory with sections: Observation, Logic Chain, Caveats, Conclusion, Verification Method. Notify parent via `send_message`.

## 2026-09-22T19:47:41Z
You are M3 Explorer 1 (Client H2 Monolith Decomposition) for Milestone M3.
Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_1
Parent: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac

Read:
1. d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
2. d:\CodingProjects\mach\.agents\PROJECT.md
3. d:\CodingProjects\mach\.agents\TEST_READY.md
4. d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_1\DISPATCH.md

Investigate client/h2/conn.go (1,667 lines):
- Complete symbol inventory with line numbers
- Concrete mapping into the 9 single-responsibility files per PROJECT.md §5
- Retain CPU cacheline padding (_ cpu.CacheLinePad) and SPSCRingBuffer
- Check TEST_READY.md §5 regarding client/h2 flow control uninitialized serverWindow and race on ctx.StreamID
- Draft RFC 9113 docstrings and BSD headers for all exported symbols

Write handoff.md with Observation, Logic Chain, Caveats, Conclusion, Verification Method and send_message to parent.

