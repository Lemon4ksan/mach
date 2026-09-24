# Dispatch Assignment — Milestone M3 Worker (Client Protocol Engine Decomposition)

## Identity
- Role: M3 Implementation Worker
- Type: teamwork_preview_worker
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_worker_m3_1
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
Read these files before starting work:
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout §5, invariants §6)
3. `d:\CodingProjects\mach\.agents\TEST_READY.md` (test runner command & expected suites)
4. Explorer Reports:
   - `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_1\handoff.md` (Client H2 decomposition, Escalations 3 & 4)
   - `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_2\handoff.md` (Client H3 decomposition & proposed files)
   - `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_3\handoff.md` (Client Pool socket leak fix, H1 docstrings & tests, 5-pillar gate)

## Mandatory Integrity Warning
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

## Objective & Scope
Execute Milestone M3 (Client Protocol Engine Decomposition):
1. **Decompose `client/h2`** into 9 single-responsibility files per `PROJECT.md` §5 & `explorer_m3_1`:
   - `client/h2/conn.go`: struct Conn, NewConn, exported API facade (`Do`, `Write`, `Close`, `Closed`, `Handshake`, `CanOpenStream`, `CancelStream`, `SetOrderedHeaders`, `sendSettingsAck`)
   - `client/h2/stream_table.go`: stream table open-addressing & overflow shards (`getStream`, `storeStream`, `deleteStream`, `broadcastErrorToAllStreams`, `purgeStreamsAfterID`)
   - `client/h2/flow_control.go`: connection & stream window accounting (`waitForWindowUpdate`, `broadcastWindowUpdate`, `calculateChunkSize`, `handleWindowUpdate`, `updateServerWindow`, `updateStreamWindow`, `updateWindow`)
   - `client/h2/read_loop.go`: socket read loop & frame demuxing (`readLoop`, `readNext`, `handleConnectionFrame`, `handlePingAck`, `recordRTT`, `handleGoAway`, `handlePing`, `handleSettings`, `readStream`)
   - `client/h2/write_loop.go`: socket write loop & SPSC ring batching (`writeLoop`, `selectWriteEvent`, `recoverWriteLoop`, `writePing`)
   - `client/h2/request_writer.go`: request framing, DATA chunking, 100-continue (`writeRequest`, `writeData`, `finish`, `isExpectContinue`, `waitExpectContinue`)
   - `client/h2/headers.go`: HPACK encoding/decoding & forbidden filters (`encodeRequestHeaders`, `readHeader`, `readTrailers`, `appendOrderedHeaders`, `getFastHTTPCookieHeader`, `isForbiddenH2Header`, `isForbiddenH2HeaderStr`, `peekHeaderCaseInsensitive`)
   - `client/h2/push.go`: server push promise handling (`handlePushPromise`, `decodePushHeaders`, `awaitPushedResponse`, `resetStream`)
   - `client/h2/dialer.go`: network dialing & connection factory (`Dialer`, `Dial`, `DialContext`, `tryDial`)
   - Update `client/h2/context.go`: change `StreamID` to `atomic.Uint32` with `.Store()` and `.Load()` to resolve Escalation 3 data race!
   - Update `client/h2/conn.go`: in `NewConn`, initialize `nc.serverWindow.Store(65535)` per RFC 9113 §5.2.1 to resolve Escalation 4 zero-window stall!
   - Ensure `_ cpu.CacheLinePad` and `ringbuf.SPSCRingBuffer` are strictly retained.
   - Retain and document `client/h2/export.go`.
2. **Decompose `client/h3`** into 4 single-responsibility files + updated `export.go` per `PROJECT.md` §5 & `explorer_m3_2`:
   - `client/h3/conn.go`: struct ClientConn, constructor `NewClientConn`, lifecycle `IsClosed`, `Close`
   - `client/h3/control.go`: control stream setup and unidirectional stream demux (`setupControlStream`, `readUnidirectionalStreams`, `handleUnidirectionalStream`, `handleControlStream`, `handleGoAway`)
   - `client/h3/request.go`: `Do`, `DoScoped`, `sendRequest`, `sendRequestTo`
   - `client/h3/response.go`: `readResponse`, `readResponseScoped`, `readResponseFrom`, and buffer pools (`dataBufPool`, `h3HeaderBlockStorage`)
   - `client/h3/export.go`: add missing `type Settings = coreh3.Settings` per `PROJECT.md` §4.2, and document all exported symbols.
   - (You may use the thoroughly crafted proposed files in `.agents/teamwork_preview_explorer_m3_2/proposed_*.go` as base references).
3. **Upgrade `client/pool.go`** per `explorer_m3_3`:
   - Fix socket descriptor leak: check if discarded connection implements `io.Closer` (or `CloseConn` hook) and invoke `Close()` upon idle timeout eviction, health check failure, and `Put` rejection.
   - Add `Close() error` to `PoolManager`.
   - Create `client/pool_test.go` with unit tests and benchmarks.
4. **Upgrade `client/h1`** per `explorer_m3_3`:
   - Add comprehensive RFC 9112 and RFC 9110 docstrings to `client/h1/conn.go`.
   - Create `client/h1/conn_test.go` with unit tests.
5. **Standards & Clean Code**:
   - Exact 3-line BSD license header on all Go source files.
   - Comprehensive RFC docstrings on all exported types, functions, methods, and constants across `client/`.
   - 0 linter issues under `golangci-lint run ./client/...`.
   - Pass all tests: `go test -v -race ./client/...` and `go test -v -race -timeout 120s ./tests/e2e/...`.
   - Micro-benchmarks confirm 0 B/op and 0 allocs/op on hot paths.

## Write Boundaries:
- Permitted: `client/` (`client/pool.go`, `client/pool_test.go`, `client/h1/*`, `client/h2/*`, `client/h3/*`) and your working directory `.agents/teamwork_preview_worker_m3_1/`.
- FORBIDDEN: `proto/`, `server/`, `tests/e2e/`, and other agent directories.

## Output
Write `progress.md` during execution. When done, write self-contained `handoff.md` in your working directory and notify parent via `send_message`.
