# BRIEFING — 2026-09-22T20:19:30Z

## Mission
Investigate server/h2/ implementation, plan clean decomposition into 5 files, and architect the fix for Escalation 2 (data race on sc.streams in Release()).

## 🔒 My Identity
- Archetype: explorer
- Roles: investigator, architect, synthesizer
- Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_1
- Original parent: 5990a2d7-7ec1-47d8-9672-52a9ad7ba846
- Milestone: M4 (Server H2 Decomposition Blueprint)

## 🔒 Key Constraints
- Read-only investigation — do NOT implement
- Environment invariant: $env:GOWORK="off" for all Go commands
- Clean decomposition into 5 target files: server_conn.go, read_loop.go, write_loop.go, stream.go, flow_control.go
- Resolve Escalation 2 (data race on sc.streams in Release())
- Exact 3-line BSD header and RFC 9113 / RFC 7541 docstrings on all exported symbols
- Zero-allocation hot paths, lock-free or low-contention structures, preservation of cacheline pads

## Current Parent
- Conversation ID: 5990a2d7-7ec1-47d8-9672-52a9ad7ba846
- Updated: 2026-09-22T20:16:00Z

## Investigation State
- **Explored paths**:
  - `server/h2/server_conn.go` (monolith of 570 lines)
  - `server/h2/server_test.go` (unit & end-to-end tests)
  - `tests/e2e/h2_test.go` and `tests/e2e/helpers_test.go` (18 H2 E2E tests)
  - `client/h2/` (reference standard for 9-file decomposition)
  - `server/h1/` & `server/h3/` (server patterns and conventions)
- **Key findings**:
  - `server/h2/server_conn.go` mixes connection lifecycle, frame reading/demuxing, frame writing/serialization, stream state/dispatching, and flow control into 570 lines.
  - Escalation 2 confirmed: `sc.Release()` executes `clear(sc.streams)` without holding `sc.streamsMu`. Concurrently executing `dispatchStream` goroutines race on `delete(sc.streams, st.id)`. Returning `sc` to `serverConnStorage` while streams are in-flight causes severe pool contamination and use-after-free hazards.
  - Fix designed: `sc.streamsWg.Add(1)` on stream start, `defer sc.streamsWg.Done()` in `dispatchStream`, `sc.streamsWg.Wait()` in `sc.Release()`, and mutex protection around `clear(sc.streams)` in both `Release()` and `NewServerConn`. Also `isReleased atomic.Bool` to guarantee idempotency.
  - Clean decomposition into 5 target files mapped out with zero breaking changes to public APIs.
  - Zero-allocation hot path optimization identified: resetting `sc.br` and `sc.bw` via `.Reset(netConn)` rather than allocating new `bufio` readers/writers on every connection.
  - SMP false sharing protection: isolating atomic counters (`isClosed`, `connSendWindow`, `isReleased`) with `_ cpu.CacheLinePad`.
- **Unexplored areas**: None. Investigation complete.

## Key Decisions Made
- Decompose `server/h2/` into 5 single-responsibility files: `server_conn.go`, `read_loop.go`, `write_loop.go`, `stream.go`, `flow_control.go`.
- Escalation 2 fix incorporates both `sync.WaitGroup` tracking and `sync.RWMutex` locking around map operations, plus stream context cancellation.
- Retain all public API signatures (`ServerConn`, `NewServerConn`, `ServerHandlerFunc`, `ServerRequest`, `ServerResponse`, `Serve`, `Release`), while cleanly adding `Close() error` and `Closed() bool`.

## Artifact Index
- DISPATCH.md — incoming dispatch instructions
- BRIEFING.md — persistent working memory
- progress.md — liveness heartbeat
- handoff.md — final handoff report
