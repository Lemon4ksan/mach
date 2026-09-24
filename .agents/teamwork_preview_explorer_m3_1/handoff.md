# Milestone M3 Explorer 1 Handoff Report: Client H2 Monolith Decomposition

**Author**: M3 Explorer 1 (Client H2 Monolith Decomposition)  
**Date**: 2026-09-22T19:50:00Z  
**Target Package**: `github.com/lemon4ksan/mach/client/h2`  
**Working Directory**: `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m3_1`  
**Handoff Type**: Hard (Investigation complete, actionable decomposition specification ready for implementer)

---

## 1. Observation

### 1.1 Existing File Footprint in `client/h2`
Inspection of `d:\CodingProjects\mach\client\h2` reveals four existing files:
- `conn.go`: 1,667 lines (35,425 bytes), containing the monolithic H2 client connection implementation.
- `conn_test.go`: 221 lines (4,754 bytes), containing mock server and client unit tests.
- `context.go`: 52 lines (1,075 bytes), defining `Context`, `streamState`, and `ClientOpts`.
- `export.go`: 56 lines (1,422 bytes), defining downstream type aliases for `aoni` and external consumers.

Current test status:
```powershell
go test -v -race ./client/h2/...
# Output: PASS (TestClientConn_MockServer passed, 0 race warnings)

go test -v -race -run TestH2 ./tests/e2e/...
# Output: PASS (18/18 H2 e2e tests passed, 0 race warnings)
```

### 1.2 Monolithic Structure & Complete Symbol Inventory of `client/h2/conn.go` (1,667 Lines)

| Line Range | Symbol / Declaration | Category | Target Destination File |
|---|---|---|---|
| 1–37 | Package header & imports | Boilerplate | Distributed per file |
| 39–40 | `const maxConsecutiveControlFrames = 1000` | Constant (RFC 9113 §10.5 DoS limit) | `read_loop.go` |
| 42–45 | `type FrameWithHeaders interface { Headers() []byte }` | Interface (RFC 9113 §4.3) | `headers.go` |
| 47–55 | `type ConnOpts struct` (PingInterval, DisablePingChecking, OnDisconnect, OnRTT, OnPushPromise, Settings) | Struct (Configuration) | `conn.go` |
| 57–63 | `const streamTableSize = 2048`, `streamTableMask`, `streamMaxProbes = 8`, `streamNumShards = 16`, `streamShardMask` | Constants (Open addressing & shards) | `stream_table.go` |
| 65–68 | `type streamShard struct { mu sync.RWMutex; overflow map[uint32]*Context }` | Struct (Overflow bucket) | `stream_table.go` |
| 70–113 | `type Conn struct` (socket, hpack, windows, reqStreams, reqShards, channels, ringbuf) | Struct (Connection representation) | `conn.go` |
| 88–98 | `_ cpu.CacheLinePad` isolating atomic counters (`serverWindow`, `serverStreamWindow`, `openStreams`, `nextID`) | Silicon Invariant (F12) | `conn.go` |
| 107 | `outRing *ringbuf.SPSCRingBuffer[coreh2.FrameHeader]` | Silicon Invariant (F12) | `conn.go` |
| 115–156 | `func NewConn(c net.Conn, opts ConnOpts) *Conn` | Constructor | `conn.go` |
| 158–161 | `func (c *Conn) SetOrderedHeaders(keys []string)` | Exported Method | `conn.go` |
| 163–179 | `func (c *Conn) getStream(streamID uint32) *Context` | Internal Method (Lookup) | `stream_table.go` |
| 181–199 | `func (c *Conn) storeStream(ctx *Context)` | Internal Method (CAS / Overflow) | `stream_table.go` |
| 201–219 | `func (c *Conn) deleteStream(streamID uint32)` | Internal Method (Removal) | `stream_table.go` |
| 221–246 | `func (c *Conn) broadcastErrorToAllStreams(err error)` | Internal Method (Error broadcast) | `stream_table.go` |
| 248–277 | `func (c *Conn) purgeStreamsAfterID(lastStreamID uint32, err error)` | Internal Method (GOAWAY purge) | `stream_table.go` |
| 279–308 | `func (c *Conn) CancelStream(ctx *Context)` | Exported Method (RST_STREAM) | `conn.go` |
| 310–350 | `func (c *Conn) Close() error` | Exported Method (GOAWAY & Close) | `conn.go` |
| 352–396 | `func (c *Conn) Handshake() error` | Exported Method (Preface & Settings) | `conn.go` |
| 398–412 | `func (c *Conn) sendSettingsAck()` | Internal Method (SETTINGS ACK) | `conn.go` |
| 414–421 | `func (c *Conn) CanOpenStream() bool` | Exported Method (Stream limit check) | `conn.go` |
| 423–426 | `func (c *Conn) Closed() bool` | Exported Method (State check) | `conn.go` |
| 428–443 | `func (c *Conn) Write(r *Context) error` | Exported Method (Enqueue request) | `conn.go` |
| 445–469 | `func (c *Conn) writeLoop()` | Internal Goroutine (Egress loop) | `write_loop.go` |
| 471–572 | `func (c *Conn) selectWriteEvent(pingChan <-chan time.Time) (bool, error)` | Internal Method (Batching & demux) | `write_loop.go` |
| 574–588 | `func (c *Conn) recoverWriteLoop(lastErr *error)` | Internal Method (Panic recovery) | `write_loop.go` |
| 590–600 | `func (c *Conn) finish(r *Context, stream uint32, err error)` | Internal Method (Stream completion) | `request_writer.go` |
| 602–656 | `func (c *Conn) readLoop()` | Internal Goroutine (Ingress loop) | `read_loop.go` |
| 658–661 | `func isExpectContinue(req *h1.Request) bool` | Internal Helper (100-continue) | `request_writer.go` |
| 663–673 | `func (c *Conn) waitExpectContinue(ctx *Context)` | Internal Helper (Expect timer) | `request_writer.go` |
| 675–761 | `func (c *Conn) writeRequest(ctx *Context) error` | Internal Method (HEADERS frame) | `request_writer.go` |
| 763–812 | `func (c *Conn) writeData(fh *coreh2.FrameHeader, ctx *Context, body []byte) error` | Internal Method (DATA framing) | `request_writer.go` |
| 814–832 | `func (c *Conn) waitForWindowUpdate(ctx *Context, remaining int)` | Internal Method (Window wait cond) | `flow_control.go` |
| 834–838 | `func (c *Conn) broadcastWindowUpdate()` | Internal Method (Window wake-up) | `flow_control.go` |
| 840–858 | `func (c *Conn) calculateChunkSize(ctx *Context, remaining int) int` | Internal Method (Min window size) | `flow_control.go` |
| 860–877 | `func isForbiddenH2Header(key, value []byte) bool` | Internal Helper (RFC 9113 §8.2.2) | `headers.go` |
| 879–890 | `func isForbiddenH2HeaderStr(key string) bool` | Internal Helper (RFC 9113 §8.2.2) | `headers.go` |
| 892 | `var defaultPseudoOrder = [4]string{":method", ":authority", ":scheme", ":path"}` | Variable (Header order) | `headers.go` |
| 894–970 | `func (c *Conn) encodeRequestHeaders(h *coreh2.Headers, req *h1.Request)` | Internal Method (HPACK encode) | `headers.go` |
| 972–998 | `func getFastHTTPCookieHeader(req *h1.Request) []byte` | Internal Helper (Cookie collapse) | `headers.go` |
| 1000–1008 | `func peekHeaderCaseInsensitive(req *h1.Request, key string) []byte` | Internal Helper (Case search) | `headers.go` |
| 1010–1063 | `func (c *Conn) appendOrderedHeaders(...)` | Internal Method (Preserve order) | `headers.go` |
| 1065–1093 | `func (c *Conn) readNext() (*coreh2.FrameHeader, error)` | Internal Method (Frame parsing) | `read_loop.go` |
| 1095–1126 | `func (c *Conn) handleConnectionFrame(fr *coreh2.FrameHeader) error` | Internal Method (Control demux) | `read_loop.go` |
| 1128–1144 | `func (c *Conn) handlePingAck(ping *coreh2.Ping)` | Internal Method (RTT tracking) | `read_loop.go` |
| 1146–1153 | `func (c *Conn) recordRTT(rtt time.Duration)` | Internal Method (RTT callback) | `read_loop.go` |
| 1155–1176 | `func (c *Conn) handleWindowUpdate(fr *coreh2.FrameHeader) error` | Internal Method (WINDOW_UPDATE) | `flow_control.go` |
| 1178–1190 | `func (c *Conn) updateServerWindow(inc int32) error` | Internal Method (Conn window) | `flow_control.go` |
| 1192–1209 | `func (c *Conn) updateStreamWindow(streamID uint32, inc int32) error` | Internal Method (Stream window) | `flow_control.go` |
| 1211–1218 | `func (c *Conn) handleGoAway(ga *coreh2.GoAway)` | Internal Method (GOAWAY demux) | `read_loop.go` |
| 1220–1240 | `func (c *Conn) writePing() error` | Internal Method (PING frame send) | `write_loop.go` |
| 1243–1255 | `func (c *Conn) handleSettings(st *coreh2.Settings)` | Internal Method (SETTINGS apply) | `read_loop.go` |
| 1257–1265 | `func (c *Conn) handlePing(ping *coreh2.Ping)` | Internal Method (PING reply) | `read_loop.go` |
| 1267–1330 | `func (c *Conn) readStream(fr *coreh2.FrameHeader, reqCtx *Context) error` | Internal Method (Stream demux) | `read_loop.go` |
| 1332–1373 | `func (c *Conn) handlePushPromise(pp *coreh2.PushPromise) error` | Internal Method (PUSH_PROMISE) | `push.go` |
| 1375–1408 | `func (c *Conn) decodePushHeaders(headerBlock []byte, pushReq *h1.Request) error` | Internal Method (Push decode) | `push.go` |
| 1410–1418 | `func (c *Conn) awaitPushedResponse(...)` | Internal Goroutine (Pushed wait) | `push.go` |
| 1420–1433 | `func (c *Conn) resetStream(streamID uint32, code coreh2.ErrorCode)` | Internal Method (RST_STREAM) | `push.go` |
| 1435–1472 | `func (c *Conn) readTrailers(b []byte, reqCtx *Context) error` | Internal Method (HPACK trailers) | `headers.go` |
| 1474–1491 | `func (c *Conn) updateWindow(streamID uint32, size int)` | Internal Method (Window emission) | `flow_control.go` |
| 1493 | `const defaultMaxHeaderListSize = 10 * 1024 * 1024` | Constant (Max header size) | `headers.go` |
| 1495–1558 | `func (c *Conn) readHeader(b []byte, res *h1.Response) (int, error)` | Internal Method (HPACK decode) | `headers.go` |
| 1560–1568 | `type Dialer struct` | Struct (Dialer factory) | `dialer.go` |
| 1570–1573 | `func (d *Dialer) Dial(opts ConnOpts) (*Conn, error)` | Exported Method | `dialer.go` |
| 1575–1586 | `func (d *Dialer) DialContext(ctx context.Context, opts ConnOpts) (*Conn, error)` | Exported Method | `dialer.go` |
| 1588–1644 | `func (d *Dialer) tryDial(ctx context.Context) (net.Conn, error)` | Internal Method (TLS dialer) | `dialer.go` |
| 1646–1666 | `func (c *Conn) Do(ctx context.Context, req *h1.Request, res *h1.Response) error` | Exported Method (Facade Do) | `conn.go` |

### 1.3 Pre-existing Defect Analysis (TEST_READY.md §5)

1. **Escalation 4: Flow Control Uninitialized `c.serverWindow`**
   - Observation: In `conn.go:89`, `serverWindow atomic.Int32` is declared. In `NewConn` (lines 116–156) and `Handshake` (lines 353–396), `serverWindow` is never initialized. By default in Go, an `atomic.Int32` starts at 0.
   - Per RFC 9113 §5.2.1:
     > "The initial flow-control window is 65,535 octets for both new streams and the overall connection."
   - Consequences: In `calculateChunkSize` (lines 846–851):
     ```go
     serverWin := c.serverWindow.Load()
     streamWin := ctx.streamWindow.Load()
     win := min(int(streamWin), int(serverWin))
     ```
     `win` computes `min(65535, 0) = 0`. Any request with `len(body) > 0` immediately blocks on `c.windowCond.Wait()` indefinitely if the server does not send an explicit initial `WINDOW_UPDATE` on stream 0.
   - Confirmation in test harness: `tests/e2e/helpers_test.go:303–336` defines `h2WindowUpdateConn`, which explicitly forces a connection-level `WINDOW_UPDATE` frame on stream 0 from the server during handshake specifically to prevent tests from deadlocking on this uninitialized zero window.
   - Recommended Fix: In `NewConn` (line 136), initialize `nc.serverWindow.Store(65535)`.

2. **Escalation 3: Data Race on `ctx.StreamID`**
   - Observation: In `context.go:38`, `StreamID uint32` is defined as a non-atomic field.
   - In `conn.go:692` (`writeRequest`), `ctx.StreamID = id` writes the assigned stream ID.
   - In `conn.go:281` (`CancelStream`), `ctx.StreamID == 0` reads the stream ID.
   - In `conn.go:1646` (`Do`):
     ```go
     select {
     case <-ctx.Done():
         c.CancelStream(reqCtx)
         return ctx.Err()
     case err := <-errCh:
         return err
     }
     ```
     If `ctx` is cancelled while `writeRequest` is dequeuing and processing the request, `Do` calls `CancelStream(reqCtx)` on goroutine A while `writeRequest` executes `ctx.StreamID = id` on goroutine B, causing a data race.
   - Furthermore, if `CancelStream` executes before `ctx.StreamID` is written, `ctx.StreamID == 0` causes `CancelStream` to return prematurely without cancelling the stream.
   - Recommended Fix: In `context.go`, change `StreamID` to `atomic.Uint32`. In `conn.go`/`request_writer.go`, use `ctx.StreamID.Store(id)` and `ctx.StreamID.Load()`. Add an early check `if ctx.State() == streamClosed { return context.Canceled }` in `writeRequest` before assigning `id`.

---

## 2. Logic Chain

1. **Decomposition Boundary Justification**:
   - `conn.go` contains 1,667 lines mingling 9 distinct concerns: connection lifecycle, table indexing, flow control arithmetic, ingress framing, egress batching, HTTP request formatting, HPACK processing, server push handling, and TLS dialing.
   - Separating these 9 concerns into the exact files prescribed in `PROJECT.md` §5 aligns directly with Go package design principles:
     - `conn.go`: Core struct, constructor, and public methods (`Do`, `Write`, `Close`, `Closed`, `Handshake`, `CanOpenStream`, `CancelStream`, `SetOrderedHeaders`).
     - `stream_table.go`: High-performance open-addressing table (`reqStreams`) with 8-probe linear probing and 16-shard RWMutex overflow maps (`reqShards`), plus stream lifecycle lookups and broadcasts.
     - `flow_control.go`: RFC 9113 §5.2 / §6.9 window updates, capacity math, chunk calculations, and `sync.Cond` wait loops.
     - `read_loop.go`: RFC 9113 §4 socket frame parsing, control frame demuxing (SETTINGS, PING, GOAWAY, RST_STREAM), RTT tracking, and frame flood mitigation.
     - `write_loop.go`: Socket egress goroutine, lock-free SPSC ring buffer frame draining, vectored socket writes, and periodic PING heartbeats.
     - `request_writer.go`: Request framing (HEADERS), body streaming (DATA), 100-continue negotiation (`waitExpectContinue`), and stream termination (`finish`).
     - `headers.go`: RFC 7541 / RFC 9113 §4.3 HPACK header encoding/decoding, forbidden header filtering (`isForbiddenH2Header`), pseudo-header ordering, cookie aggregation, and trailer parsing.
     - `push.go`: RFC 9113 §6.6 / §8.4 server push promise handling (`PUSH_PROMISE`), pushed header decoding, and promised stream dispatch.
     - `dialer.go`: TCP/TLS connection dialing and ALPN negotiation (`h2`) with fallback protection.
     - `context.go`: Stream context and lifecycle states (`streamIdle`, `streamOpen`, `streamHalfClosed`, `streamClosed`).
     - `export.go`: Downstream type aliases (`HPACK`, `Frame`, `Headers`, `Settings`, `Data`, `WindowUpdate`) preserving public API compatibility.

2. **Preservation of Silicon Performance Invariants (F12)**:
   - In `conn.go:98`, `_ cpu.CacheLinePad` is placed directly after hot atomic counters:
     ```go
     serverWindow             atomic.Int32
     serverStreamWindow       uint32
     maxWindow                int32
     currentWindow            int32
     openStreams              atomic.Int32
     pingUnacks               int32
     consecutiveControlFrames int32
     nextID                   atomic.Uint32

     _ cpu.CacheLinePad
     ```
     This prevents SMP cacheline bouncing between reader/writer goroutines updating atomic counters and the stream tables / ring buffers below. This must remain intact in `Conn`.
   - In `conn.go:107`, `outRing *ringbuf.SPSCRingBuffer[coreh2.FrameHeader]` provides zero-allocation, lock-free batching for frame headers queued to the write loop. The batching logic in `selectWriteEvent` (popping up to 16 frames into a vector buffer) must be preserved in `write_loop.go`.

3. **Public API Contract Invariance**:
   - `aoni` and downstream tests only invoke:
     - `h2.Conn` (`Do`, `Write`, `Close`, `Closed`, `Handshake`, `CanOpenStream`, `CancelStream`, `SetOrderedHeaders`)
     - `h2.ConnOpts`
     - `h2.NewConn`
     - `h2.Dialer` (`Dial`, `DialContext`)
     - Exported aliases in `export.go`
   - By retaining all exported type names, field names, and method signatures in `conn.go` and `dialer.go`, full binary and source compatibility is guaranteed.

---

## 3. Caveats

1. **Internal Package Scope**:
   All 9 files belong to `package h2`. Private fields and methods (e.g. `c.writeRequest`, `c.getStream`, `c.updateWindow`) are directly accessible across all files without accessor overhead or interface indirection.
2. **`ClientOpts` vs `ConnOpts`**:
   `context.go` contains an unused legacy struct `ClientOpts`. `ConnOpts` in `conn.go` is the active struct used by `NewConn` and `aoni`. Both should be retained for backward compatibility, with `ConnOpts` documented as the primary option type.
3. **No Code Modification Rule**:
   As an explorer, this investigation is strictly read-only. No source files under `client/h2` have been modified. The complete decomposition blueprint, symbol mappings, docstrings, and bug fixes are documented below for the implementer agent.

---

## 4. Conclusion & Concrete Implementation Blueprint

### 4.1 Target File Organization (9 Decomposed Files + 2 Retained Files)

```text
d:/CodingProjects/mach/client/h2/
├── conn.go               (struct Conn, ConnOpts, NewConn, exported API facade)
├── stream_table.go       (stream table open-addressing & overflow shards)
├── flow_control.go       (connection & stream window accounting)
├── read_loop.go          (socket read loop, control frame demuxing, DoS limits)
├── write_loop.go         (socket write loop & SPSC ring batching)
├── request_writer.go     (request framing, DATA chunking, 100-continue)
├── headers.go            (HPACK encoding/decoding & forbidden filters)
├── push.go               (server push promise handling)
├── dialer.go             (network dialing & connection factory)
├── context.go            (stream context, atomic.Uint32 StreamID, streamState)
├── export.go             (downstream type aliases & factory functions)
└── conn_test.go          (existing unit tests)
```

### 4.2 Detailed File-by-File Symbol & Content Specification

#### File 1: `client/h2/conn.go`
- **Imports**: `bufio`, `context`, `fmt`, `io`, `net`, `sync`, `sync/atomic`, `time`, `github.com/lemon4ksan/foundation/net/hpack`, `github.com/lemon4ksan/foundation/silicon/ringbuf`, `github.com/lemon4ksan/foundation/sync/spinlock`, `golang.org/x/sys/cpu`, `coreh2 "github.com/lemon4ksan/mach/proto/h2"`, `h1 "github.com/lemon4ksan/mach/proto/http"`.
- **Symbols**:
  - `ConnOpts` struct
  - `Conn` struct (with `_ cpu.CacheLinePad` at line 98 and `outRing` at line 107)
  - `NewConn(c net.Conn, opts ConnOpts) *Conn` (initialize `nc.serverWindow.Store(65535)` per Escalation 4)
  - `(*Conn).Do(ctx context.Context, req *h1.Request, res *h1.Response) error`
  - `(*Conn).Write(r *Context) error`
  - `(*Conn).CanOpenStream() bool`
  - `(*Conn).Close() error`
  - `(*Conn).Closed() bool`
  - `(*Conn).Handshake() error`
  - `(*Conn).SetOrderedHeaders(keys []string)`
  - `(*Conn).CancelStream(ctx *Context)`
  - `(*Conn).sendSettingsAck()`

#### File 2: `client/h2/stream_table.go`
- **Imports**: `sync`, `sync/atomic`, `coreh2 "github.com/lemon4ksan/mach/proto/h2"`.
- **Symbols**:
  - `const streamTableSize = 2048`
  - `const streamTableMask = streamTableSize - 1`
  - `const streamMaxProbes = 8`
  - `const streamNumShards = 16`
  - `const streamShardMask = streamNumShards - 1`
  - `type streamShard struct { mu sync.RWMutex; overflow map[uint32]*Context }`
  - `(*Conn).getStream(streamID uint32) *Context` (uses `ctx.StreamID.Load() == streamID`)
  - `(*Conn).storeStream(ctx *Context)` (uses `streamID := ctx.StreamID.Load()`)
  - `(*Conn).deleteStream(streamID uint32)` (uses `cur.StreamID.Load() == streamID`)
  - `(*Conn).broadcastErrorToAllStreams(err error)`
  - `(*Conn).purgeStreamsAfterID(lastStreamID uint32, err error)` (uses `ctx.StreamID.Load() > lastStreamID`)

#### File 3: `client/h2/flow_control.go`
- **Imports**: `time`, `coreh2 "github.com/lemon4ksan/mach/proto/h2"`.
- **Symbols**:
  - `(*Conn).waitForWindowUpdate(ctx *Context, remaining int)`
  - `(*Conn).broadcastWindowUpdate()`
  - `(*Conn).calculateChunkSize(ctx *Context, remaining int) int`
  - `(*Conn).handleWindowUpdate(fr *coreh2.FrameHeader) error`
  - `(*Conn).updateServerWindow(inc int32) error`
  - `(*Conn).updateStreamWindow(streamID uint32, inc int32) error`
  - `(*Conn).updateWindow(streamID uint32, size int)`

#### File 4: `client/h2/read_loop.go`
- **Imports**: `encoding/binary`, `errors`, `sync/atomic`, `time`, `coreh2 "github.com/lemon4ksan/mach/proto/h2"`, `h1 "github.com/lemon4ksan/mach/proto/http"`.
- **Symbols**:
  - `const maxConsecutiveControlFrames = 1000`
  - `(*Conn).readLoop()`
  - `(*Conn).readNext() (*coreh2.FrameHeader, error)`
  - `(*Conn).handleConnectionFrame(fr *coreh2.FrameHeader) error`
  - `(*Conn).handlePingAck(ping *coreh2.Ping)`
  - `(*Conn).recordRTT(rtt time.Duration)`
  - `(*Conn).handleGoAway(ga *coreh2.GoAway)`
  - `(*Conn).handlePing(ping *coreh2.Ping)`
  - `(*Conn).handleSettings(st *coreh2.Settings)`
  - `(*Conn).readStream(fr *coreh2.FrameHeader, reqCtx *Context) error`

#### File 5: `client/h2/write_loop.go`
- **Imports**: `encoding/binary`, `errors`, `fmt`, `io`, `time`, `github.com/lemon4ksan/foundation/silicon/clock`, `github.com/lemon4ksan/foundation/silicon/sysnet`, `coreh2 "github.com/lemon4ksan/mach/proto/h2"`.
- **Symbols**:
  - `(*Conn).writeLoop()`
  - `(*Conn).selectWriteEvent(pingChan <-chan time.Time) (bool, error)`
  - `(*Conn).recoverWriteLoop(lastErr *error)`
  - `(*Conn).writePing() error`

#### File 6: `client/h2/request_writer.go`
- **Imports**: `bytes`, `context`, `errors`, `time`, `coreh2 "github.com/lemon4ksan/mach/proto/h2"`, `h1 "github.com/lemon4ksan/mach/proto/http"`.
- **Symbols**:
  - `isExpectContinue(req *h1.Request) bool`
  - `(*Conn).waitExpectContinue(ctx *Context)`
  - `(*Conn).writeRequest(ctx *Context) error` (checks `ctx.State() == streamClosed` and uses `ctx.StreamID.Store(id)`)
  - `(*Conn).writeData(fh *coreh2.FrameHeader, ctx *Context, body []byte) error`
  - `(*Conn).finish(r *Context, stream uint32, err error)`

#### File 7: `client/h2/headers.go`
- **Imports**: `bytes`, `net/http`, `net/http/httptrace`, `net/textproto`, `strconv`, `strings`, `github.com/lemon4ksan/foundation/generic`, `github.com/lemon4ksan/foundation/net/hpack`, `github.com/lemon4ksan/foundation/silicon/bytesconv`, `coreh2 "github.com/lemon4ksan/mach/proto/h2"`, `h1 "github.com/lemon4ksan/mach/proto/http"`.
- **Symbols**:
  - `type FrameWithHeaders interface { Headers() []byte }`
  - `isForbiddenH2Header(key, value []byte) bool`
  - `isForbiddenH2HeaderStr(key string) bool`
  - `var defaultPseudoOrder = [4]string{":method", ":authority", ":scheme", ":path"}`
  - `(*Conn).encodeRequestHeaders(h *coreh2.Headers, req *h1.Request)`
  - `getFastHTTPCookieHeader(req *h1.Request) []byte`
  - `peekHeaderCaseInsensitive(req *h1.Request, key string) []byte`
  - `(*Conn).appendOrderedHeaders(h *coreh2.Headers, req *h1.Request, hf *hpack.HeaderField)`
  - `const defaultMaxHeaderListSize = 10 * 1024 * 1024`
  - `(*Conn).readTrailers(b []byte, reqCtx *Context) error`
  - `(*Conn).readHeader(b []byte, res *h1.Response) (int, error)`

#### File 8: `client/h2/push.go`
- **Imports**: `github.com/lemon4ksan/foundation/net/hpack`, `coreh2 "github.com/lemon4ksan/mach/proto/h2"`, `h1 "github.com/lemon4ksan/mach/proto/http"`.
- **Symbols**:
  - `(*Conn).handlePushPromise(pp *coreh2.PushPromise) error`
  - `(*Conn).decodePushHeaders(headerBlock []byte, pushReq *h1.Request) error`
  - `(*Conn).awaitPushedResponse(ctx *Context, pushReq *h1.Request, pushResp *h1.Response)`
  - `(*Conn).resetStream(streamID uint32, code coreh2.ErrorCode)`

#### File 9: `client/h2/dialer.go`
- **Imports**: `context`, `crypto/tls`, `net`, `time`, `coreh2 "github.com/lemon4ksan/mach/proto/h2"`.
- **Symbols**:
  - `type Dialer struct`
  - `(*Dialer).Dial(opts ConnOpts) (*Conn, error)`
  - `(*Dialer).DialContext(ctx context.Context, opts ConnOpts) (*Conn, error)`
  - `(*Dialer).tryDial(ctx context.Context) (net.Conn, error)`

#### File 10: `client/h2/context.go` (Audit & Refactor)
- **Updates**:
  - Change `StreamID uint32` to `StreamID atomic.Uint32` to eliminate Escalation 3 data race.
  - Add helper methods `ID() uint32` and `SetID(id uint32)`:
    ```go
    func (ctx *Context) ID() uint32 { return ctx.StreamID.Load() }
    func (ctx *Context) SetID(id uint32) { ctx.StreamID.Store(id) }
    ```

#### File 11: `client/h2/export.go` (Audit)
- **Status**: Already separated; verify BSD headers and RFC docstrings.

---

### 4.3 Drafted RFC 9113 Docstrings & BSD Headers for All Exported Symbols

#### Mandatory BSD License Header (Invariant §6.1)
```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
```

#### RFC 9113 Docstrings (Invariant §6.2)

```go
// Conn manages a multiplexed HTTP/2 client connection over an underlying net.Conn socket
// in compliance with RFC 9113 §3 (Starting HTTP/2), §4 (HTTP Frames), §5 (Streams and Multiplexing),
// and §6 (Frame Definitions).
//
// Concurrency:
// Conn is safe for concurrent use across multiple goroutines. Egress frames are serialized
// via an internal lock-free SPSC ring buffer and write mutex, while stream states are mapped
// using lock-free open addressing with partitioned overflow buckets.
//
// Silicon Invariants:
// Hot atomic counters are isolated on a dedicated 64-byte cacheline (_ cpu.CacheLinePad)
// to eliminate false sharing across CPU cores during high-concurrency request pipelining.
type Conn struct ...

// ConnOpts defines connection configuration options for client-side HTTP/2 sessions
// (RFC 9113 §6.5).
type ConnOpts struct {
	// PingInterval specifies the frequency of periodic PING keepalive probes (RFC 9113 §6.7).
	PingInterval time.Duration

	// DisablePingChecking disables connection termination when peer PING acks stall.
	DisablePingChecking bool

	// OnDisconnect is an optional callback invoked when the connection terminates.
	OnDisconnect func(ctx context.Context, c *Conn)

	// OnRTT is an optional callback reporting measured round-trip time from PING frames.
	OnRTT func(time.Duration)

	// OnPushPromise is an optional callback invoked upon receiving server push streams (RFC 9113 §8.4).
	OnPushPromise func(pushReq *h1.Request, pushResp *h1.Response)

	// Settings specifies initial local HTTP/2 SETTINGS parameters advertised to the server (RFC 9113 §6.5).
	Settings *coreh2.Settings
}

// NewConn instantiates a new HTTP/2 client connection wrapping socket c.
//
// Lifecycle:
// NewConn initializes internal SPSC ring buffers, flow control windows (initial send window
// set to 65,535 octets per RFC 9113 §5.2.1), and HPACK compression tables. Handshake() must
// be invoked prior to executing requests.
func NewConn(c net.Conn, opts ConnOpts) *Conn

// Handshake performs the HTTP/2 connection preface exchange and SETTINGS negotiation
// per RFC 9113 §3.4 (Starting HTTP/2 with Prior Knowledge) and §6.5 (SETTINGS Frame).
// It spawns background readLoop and writeLoop goroutines upon successful negotiation.
func (c *Conn) Handshake() error

// Do executes a single HTTP request/response exchange over a multiplexed HTTP/2 stream
// per RFC 9113 §8.1 (HTTP Message Exchanges).
//
// Lifecycle:
// Blocks until the peer delivers a complete response, an error occurs, or ctx is cancelled.
// If ctx is cancelled before completion, CancelStream is triggered to transmit RST_STREAM
// with CANCEL (RFC 9113 §6.4, §8.1).
func (c *Conn) Do(ctx context.Context, req *h1.Request, res *h1.Response) error

// Write enqueues a request context into the connection's egress submission channel.
// Returns coreh2.ErrNoAvailableStreams if the channel is saturated or coreh2.ErrStreamClosed
// if the connection has terminated.
func (c *Conn) Write(r *Context) error

// CanOpenStream reports whether the connection can open a new concurrent stream without
// exceeding peer-advertised SETTINGS_MAX_CONCURRENT_STREAMS (RFC 9113 §5.1.2) or exhausting
// the 31-bit stream identifier space (RFC 9113 §5.1.1).
func (c *Conn) CanOpenStream() bool

// CancelStream terminates an active HTTP/2 stream by transmitting an RST_STREAM frame
// with error code CANCEL (0x08) per RFC 9113 §5.1 and §6.4.
func (c *Conn) CancelStream(ctx *Context)

// Close gracefully terminates the HTTP/2 connection by transmitting a GOAWAY frame
// with error code NO_ERROR (0x00) per RFC 9113 §6.8 and closing the underlying network socket.
func (c *Conn) Close() error

// Closed reports whether the connection has been closed or experienced a fatal socket error.
func (c *Conn) Closed() bool

// SetOrderedHeaders configures custom HPACK header emission order to preserve
// deterministic header sequence matching browser or peer signatures (RFC 9113 §8.2).
func (c *Conn) SetOrderedHeaders(keys []string)

// FrameWithHeaders defines an interface for HTTP/2 frames carrying HPACK-encoded header
// block fragments per RFC 9113 §4.3 (Header Compression and Decompression).
type FrameWithHeaders interface {
	Headers() []byte
}

// Dialer establishes outbound HTTP/2 TLS connections using custom network dialers
// and performs ALPN protocol negotiation ("h2") per RFC 9113 §3.3.
type Dialer struct ...

// Dial establishes an HTTP/2 TLS connection to Dialer.Addr and performs the initial handshake.
func (d *Dialer) Dial(opts ConnOpts) (*Conn, error)

// DialContext establishes an HTTP/2 TLS connection with context cancellation support.
func (d *Dialer) DialContext(ctx context.Context, opts ConnOpts) (*Conn, error)

// Context encapsulates stream-level lifecycle state, request/response message envelopes,
// and stream flow-control windows for an active HTTP/2 exchange (RFC 9113 §5.1).
type Context struct {
	Request        *h1.Request
	Response       *h1.Response
	Err            chan error
	Trailers       map[string][]string
	StreamID       atomic.Uint32
	streamWindow   atomic.Int32
	streamRxWindow atomic.Int32
	state          atomic.Int32
	headersParsed  bool
}

// DefaultPingInterval specifies the default 10-second period between keepalive PING frames (RFC 9113 §6.7).
const DefaultPingInterval = time.Second * 10
```

---

## 5. Verification Method

To verify the findings and the eventual decomposed implementation independently:

1. **Unit Test Verification**:
   ```powershell
   go test -v -race ./client/h2/...
   ```
   Ensures all mock server tests pass without race conditions.

2. **E2E Integration Verification**:
   ```powershell
   go test -v -race -run TestH2 ./tests/e2e/...
   ```
   Verifies all 18 H2 end-to-end tests (HPACK compression, DATA frame streaming, multi-stream multiplexing, stream resets, flow control window updates, custom header ordering, zero-window stalling, control frame DoS limit, PING/ACK round-trip, GOAWAY shutdown, concurrent disconnects, burst concurrency).

3. **Data Race Verification on `ctx.StreamID`**:
   Run `TestH2_Tier1_StreamCancellationRST` and `TestH2_Tier3_MultiplexingWithConcurrentResets` under `-race` with multiple iterations:
   ```powershell
   go test -race -count=5 -run "TestH2_Tier1_StreamCancellationRST|TestH2_Tier3_MultiplexingWithConcurrentResets" ./tests/e2e/...
   ```

4. **Zero-Window Stall Invalidation Test (Escalation 4 Fix Verification)**:
   Verify that dialing an H2 server without `h2WindowUpdateConn` and sending a request with a 64KB body succeeds immediately without blocking, confirming `c.serverWindow` is initialized to 65,535 octets per RFC 9113 §5.2.1.
