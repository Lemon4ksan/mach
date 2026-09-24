## 2026-09-23T07:31:05Z
You are Worker M4.2 (teamwork_preview_worker), the implementation worker for Milestone M4 (Server Protocol Engine Decomposition).
Your working directory is: d:\CodingProjects\mach\.agents\teamwork_preview_worker_m4_2
Project root: d:\CodingProjects\mach
Parent conversation ID: 5990a2d7-7ec1-47d8-9672-52a9ad7ba846

MANDATORY INTEGRITY WARNING:
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

MANDATORY READING:
- Original User Request: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
- Master Plan: d:\CodingProjects\mach\.agents\PROJECT.md (especially §5 Code Layout for server/)
- E2E Test Suite & Escalations: d:\CodingProjects\mach\.agents\TEST_READY.md (Escalations 1 and 2)
- Explorer M4.1 Handoff Blueprint: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_1\handoff.md
- Explorer M4.2 Handoff Blueprint: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_2\handoff.md
- Explorer M4.3 Handoff Blueprint: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_3\handoff.md

WRITE OWNERSHIP:
You have exclusive write ownership of all files under `d:\CodingProjects\mach\server/`.
No other concurrent agents will touch `server/`.

YOUR MISSION (Milestone M4: Server Protocol Engine Decomposition):
Execute the complete, synthesised blueprint provided by Explorers M4.1, M4.2, and M4.3:

1. `server/h2/` Modular Decomposition:
   - Decompose `server/h2/server_conn.go` (570 lines) into 5 focused components per Explorer M4.1 blueprint:
     - `server/h2/server_conn.go`: ServerConn struct, constructor `NewServerConn`, Per-P storage pool, lifecycle (`Serve`, `Close`, `Closed`, `Release`), public types (`ServerHandlerFunc`, `ServerRequest`, `ServerResponse`).
     - `server/h2/read_loop.go`: Socket read loop, preface validation, frame demuxing, settings/ping/rst/goaway dispatch.
     - `server/h2/write_loop.go`: Outbound response serialization, HPACK encoding, DATA chunking, frame writing.
     - `server/h2/stream.go`: Stream state machine (`serverStream`), stream lifecycle, header validation, RFC 8441 extended CONNECT, handler execution.
     - `server/h2/flow_control.go`: Flow control window accounting, WINDOW_UPDATE reception and emission.
   - Resolve Escalation 2: Data race on `sc.streams` in `Release()`.
     - Implement `streamsWg sync.WaitGroup` to track in-flight stream goroutines.
     - Acquire `sc.streamsMu.Lock()` around `clear(sc.streams)` in both `Release()` and `NewServerConn`.
     - Add `isReleased atomic.Bool` and per-stream context cancellation.
     - Retain `_ cpu.CacheLinePad` for atomic counters.

2. `server/h1/` Standardization & Smuggling Defect Resolution:
   - Resolve Escalation 1 per Explorer M4.3 & M4.2 blueprints:
     - In `server/h1/request.go`: add `CloseConnection bool` to `Request`, flag it in `finishRequestBodyRead` when `hasTE && hasCL` is detected, reset it in `Request.Reset()`.
     - In `server/h1/conn.go:152-157`: evaluate `if req.CloseConnection { keepAlive = false }`.
     - Add regression test `TestConnHandler_RequestSmuggling_ConnectionClose` to `server/h1/h1_test.go`.
   - Standardize Per-P storage buffer pooling:
     - Fix asymmetric `writerStorage`: allocate `*bytesconv.ByteBuffer` and return via `writerStorage.Put(bw)`.
     - Reset `br.Reset(nil)` before returning to `readerStorage`.
     - Cap pooled request/response body sizes before returning (if cap > 64KB, reallocate to 1KB).
     - For cookie serialization in `server/h1/response.go`: use zero-alloc `c.AppendBytes` or `bw.B = c.AppendBytes(bw.B)` if supported by `zerocopy.Cookie`, or write cookie bytes directly to `bw`.
     - Use pooled `ByteBuffer` in `ReadAllChunked`.

3. `server/h3/` Modularization & Per-P Storage Buffer Pooling:
   - Decompose `server/h3/server_conn.go` into 3 files per `PROJECT.md` §5 and Explorer M4.2 blueprint:
     - `server/h3/server_conn.go`: `ServerConn` struct, constructor `NewServerConn`, lifecycle (`Serve`, `Close`).
     - `server/h3/dispatch.go`: `acceptUniStreams()` and `handleUniStream()` router.
     - `server/h3/stream.go`: `ServerHandlerFunc`, `ServerRequest`, `ServerResponse`, Per-P storage pools (`serverReqStorage`, `serverResStorage`, `h3HeaderBlockStorage`, `h3ReaderStorage`, `h3BodyBufferStorage`), and `handleRequestStream()`.
   - Eliminate hot path allocations: use `*bufio.Reader` satisfying `varint.Reader`, stack header buffer `[4096]byte`.

4. Standards & Invariants:
   - Exact 3-line BSD license header on EVERY `.go` file in `server/`.
   - Comprehensive RFC docstrings (RFC 9112, 9110, 9113, 9114, 9204) on all exported symbols per Explorer M4.3 catalog.
   - 100% public API compatibility.
   - Zero-allocation hot paths.

5. Verification:
   Always use `$env:GOWORK="off"` when running Go commands.
   Execute and verify:
   - `go test -v -race -count=1 ./server/...` (all server tests pass, 0 race warnings)
   - `go test -v -race -count=5 -run "TestConnHandler_RequestSmuggling|TestH1_Tier2_RequestSmuggling" ./server/h1/... ./tests/e2e/...`
   - `go test -v -race -count=5 -run "TestH2Server_EndToEnd|TestH2_Tier3_AbruptConnectionDisconnectDuringInflight" ./server/h2/... ./tests/e2e/...`
   - `go test -v -race ./tests/e2e/...` (62/62 E2E tests pass)
   - `golangci-lint run ./server/...` (0 issues)
   - `go vet ./server/...` (0 warnings)

6. Document your work:
   Write your completion report to `d:\CodingProjects\mach\.agents\teamwork_preview_worker_m4_2\handoff.md` with:
   - Summary of changes and files modified/created/deleted
   - Verification command outputs
   - Invariant compliance confirmations
Notify parent via `send_message` when done.
