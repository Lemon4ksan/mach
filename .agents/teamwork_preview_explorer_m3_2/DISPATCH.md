# Dispatch Assignment — Milestone M3 Explorer 2 (Client H3 Architecture & Decomposition)

## Identity
- Role: M3 Explorer 2 (Client H3 Architecture & Decomposition)
- Type: teamwork_preview_explorer
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout §5, invariants §6)
3. `d:\CodingProjects\mach\.agents\TEST_READY.md` (test runner command & expected suites)

## Investigation Scope
Investigate `client/h3/conn.go` and associated files in `client/h3`:
1. Full symbol inventory: map every struct, method, function, variable, and constant with line numbers.
2. Target decomposition mapping into single-responsibility files per `PROJECT.md` §5:
   - `conn.go`: struct ClientConn, constructor, lifecycle
   - `control.go`: control stream & unidirectional stream demux
   - `request.go`: Do, DoScoped, request sending
   - `response.go`: readResponse, readResponseScoped
   - `export.go`: downstream type aliases (must preserve exact signatures per §4.2)
3. Zero-Allocation Invariants:
   - Verify scoped borrowing and QPACK encoder/decoder lifecycle without heap leaks.
4. Concurrency & QUIC interaction:
   - Identify stream opening and lifecycle concurrency expectations.
5. Propose comprehensive RFC 9114 / RFC 9204 docstrings and BSD headers for all exported symbols.

## Output
Write `handoff.md` in your working directory with sections: Observation, Logic Chain, Caveats, Conclusion, Verification Method. Notify parent via `send_message`.

## 2026-09-22T19:47:41Z
<USER_REQUEST>
You are M3 Explorer 2 (Client H3 Architecture & Decomposition) for Milestone M3.
Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2
Parent: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac

Read:
1. d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
2. d:\CodingProjects\mach\.agents\PROJECT.md
3. d:\CodingProjects\mach\.agents\TEST_READY.md
4. d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2\DISPATCH.md

Investigate client/h3/conn.go and client/h3:
- Complete symbol inventory with line numbers
- Target decomposition mapping into conn.go, control.go, request.go, response.go, export.go per PROJECT.md §5
- Verify zero-alloc invariants, scoped borrow, QPACK codec integration, and QUIC stream lifecycles
- Draft RFC 9114 / RFC 9204 docstrings and BSD headers for all exported symbols

Write handoff.md with Observation, Logic Chain, Caveats, Conclusion, Verification Method and send_message to parent.
</USER_REQUEST>
