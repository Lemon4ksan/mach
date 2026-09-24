# Handoff Report: Milestone M1 — Core Protocol Frame Decomposition

**Agent**: Explorer 1 (`teamwork_preview_explorer_m1_1_gen2`)  
**Parent Conversation ID**: `5d05cf1e-7247-466c-b645-4e25e1408e3e`  
**Milestone**: M1 (Core Protocol Frame Decomposition)  
**Target Subsystem**: `proto/h2` (`frames.go` decomposition & zero-alloc frame pooling)

---

## 1. Observation

### 1.1 Source File Inventory & Metrics
Investigation of `d:\CodingProjects\mach\proto\h2` directly observed the following files and structural relationships:
- `proto/h2/frames.go`: Monolithic frame definition file (492 lines, 15,178 bytes).
  - Contains **9 struct type declarations**: `Continuation`, `Data`, `GoAway`, `Headers`, `Ping`, `Priority`, `PushPromise`, `RstStream`, `WindowUpdate`.
  - Contains **93 method declarations** across those 9 structs.
  - Contains **0 package-level constants** and **0 standalone functions** (all constants and free functions are in `frame.go`, `header.go`, `errors.go`, `settings.go`, `utils.go`).
- `proto/h2/frame.go`: Defines `FrameType`, `FrameFlags`, `Frame` interface, `framePools` (`[FrameContinuation + 1]*sync.Pool`), `AcquireFrame`, `AcquireFrameInArena`, `ReleaseFrame`.
- `proto/h2/frame_pool.go`: Implements `ConnectionFramePool` using `offheap.SlabAllocator` for Plain Old Data (POD) frames (`Ping`, `WindowUpdate`, `RstStream`, `Priority`).
- `proto/h2/settings.go`: Already separated single-responsibility file for `Settings` frame (222 lines).
- `proto/h2/header.go`: Contains `FrameHeader` serializer/deserializer and `(h *Headers) AppendHeaderField(...)` at line 292.
- `proto/h2/utils.go`: Contains wire conversion helpers (`uint24ToBytes`, `bytesToUint24`, `uint32ToBytes`, `bytesToUint32`, `appendUint32Bytes`), slice resizing (`resizeSlice`), and padding manipulation (`cutPadding`, `addPadding`).

### 1.2 Baseline Test & Benchmark Results
- **Unit & Adversarial Tests**: `go test -v ./proto/h2/...` executed with exit code 0:
  ```text
  === RUN   TestFrameHeaderFlags -> PASS
  === RUN   TestFrameHeaderBoundsCheck -> PASS
  === RUN   TestFrameHeaderParseHeaderSymmetry -> PASS
  === RUN   TestFramesSerializationRoundtrip -> PASS (8 subtests: Data, Headers, Settings, Ping, GoAway, WindowUpdate, RstStream, Priority)
  === RUN   TestPaddingHelpers -> PASS
  === RUN   TestPingInvalidPayload -> PASS
  === RUN   TestRstStreamErrorFormatting -> PASS
  === RUN   TestGoAwayErrorFormatting -> PASS
  === RUN   TestContinuationFrame -> PASS
  === RUN   TestAcquireFrameInArena -> PASS
  === RUN   TestConnectionFramePool_AcquireRelease_POD -> PASS
  === RUN   TestConnectionFramePool_AcquireRelease_NonPOD -> PASS
  === RUN   TestConnectionFramePool_NilReceiver -> PASS
  === RUN   TestConnectionFramePool_ZeroCapacity_UsesDefault -> PASS
  === RUN   TestConnectionFramePool_FreeAndReallocate -> PASS
  === RUN   TestConnectionFramePool_PerGoroutinePool -> PASS
  === RUN   TestConnectionFramePool_Release_Idempotent -> PASS
  === RUN   TestH2_FrameHeader_Adversarial -> PASS
  === RUN   TestH2_Varint_Adversarial -> PASS
  === RUN   TestH2_Huffman_Adversarial -> PASS
  === RUN   FuzzHPACKDecode -> PASS
  === RUN   FuzzFrameRead -> PASS
  PASS (0.727s)
  ```
- **Zero-Allocation Benchmark Gate**: `go test -bench=Benchmark -benchmem ./proto/h2` verified zero-allocation invariants:
  ```text
  BenchmarkFrameHeader_ReadFrame-12                       15372789   107.6 ns/op   48 B/op   1 allocs/op
  BenchmarkAcquireRelease_SyncPool_Ping-12                69251677    21.34 ns/op   0 B/op   0 allocs/op
  BenchmarkAcquireRelease_ConnPool_Ping-12                76353848    13.85 ns/op   0 B/op   0 allocs/op
  BenchmarkAcquireRelease_SyncPool_WindowUpdate-12        76476961    18.02 ns/op   0 B/op   0 allocs/op
  BenchmarkAcquireRelease_ConnPool_WindowUpdate-12        60880328    19.70 ns/op   0 B/op   0 allocs/op
  BenchmarkAcquireRelease_SyncPool_RstStream-12           39359149    29.20 ns/op   0 B/op   0 allocs/op
  BenchmarkAcquireRelease_ConnPool_RstStream-12           92379463    15.91 ns/op   0 B/op   0 allocs/op
  BenchmarkAcquireRelease_PerGoroutinePool_Parallel-12   428880068     3.254 ns/op  0 B/op   0 allocs/op
  ```

### 1.3 Exact Catalog of the 87 Docstring Violations
Executing `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 --allow-parallel-runners --no-config --enable revive ./proto/h2` identifies **exactly 87 revive docstring violations** in `proto/h2/frames.go`.

The 87 violations correspond to 87 exported methods on frame structs lacking docstrings:
1. `proto/h2/frames.go:17:1`: exported method `Continuation.Type` should have comment or be unexported
2. `proto/h2/frames.go:19:1`: exported method `Continuation.Reset` should have comment or be unexported
3. `proto/h2/frames.go:24:1`: exported method `Continuation.Headers` should have comment or be unexported
4. `proto/h2/frames.go:25:1`: exported method `Continuation.SetEndHeaders` should have comment or be unexported
5. `proto/h2/frames.go:26:1`: exported method `Continuation.EndHeaders` should have comment or be unexported
6. `proto/h2/frames.go:27:1`: exported method `Continuation.SetHeader` should have comment or be unexported
7. `proto/h2/frames.go:28:1`: exported method `Continuation.AppendHeader` should have comment or be unexported
8. `proto/h2/frames.go:31:1`: exported method `Continuation.Deserialize` should have comment or be unexported
9. `proto/h2/frames.go:42:1`: exported method `Continuation.Serialize` should have comment or be unexported
10. `proto/h2/frames.go:57:1`: exported method `Data.Type` should have comment or be unexported
11. `proto/h2/frames.go:58:1`: exported method `Data.Reset` should have comment or be unexported
12. `proto/h2/frames.go:59:1`: exported method `Data.SetEndStream` should have comment or be unexported
13. `proto/h2/frames.go:60:1`: exported method `Data.EndStream` should have comment or be unexported
14. `proto/h2/frames.go:61:1`: exported method `Data.Data` should have comment or be unexported
15. `proto/h2/frames.go:62:1`: exported method `Data.SetData` should have comment or be unexported
16. `proto/h2/frames.go:63:1`: exported method `Data.Padding` should have comment or be unexported
17. `proto/h2/frames.go:64:1`: exported method `Data.SetPadding` should have comment or be unexported
18. `proto/h2/frames.go:65:1`: exported method `Data.Append` should have comment or be unexported
19. `proto/h2/frames.go:66:1`: exported method `Data.Len` should have comment or be unexported
20. `proto/h2/frames.go:69:1`: exported method `Data.Deserialize` should have comment or be unexported
21. `proto/h2/frames.go:91:1`: exported method `Data.Serialize` should have comment or be unexported
22. `proto/h2/frames.go:120:1`: exported method `GoAway.Type` should have comment or be unexported
23. `proto/h2/frames.go:121:1`: exported method `GoAway.Reset` should have comment or be unexported
24. `proto/h2/frames.go:122:1`: exported method `GoAway.Code` should have comment or be unexported
25. `proto/h2/frames.go:123:1`: exported method `GoAway.SetCode` should have comment or be unexported
26. `proto/h2/frames.go:124:1`: exported method `GoAway.Stream` should have comment or be unexported
27. `proto/h2/frames.go:125:1`: exported method `GoAway.SetStream` should have comment or be unexported
28. `proto/h2/frames.go:126:1`: exported method `GoAway.Data` should have comment or be unexported
29. `proto/h2/frames.go:127:1`: exported method `GoAway.SetData` should have comment or be unexported
30. `proto/h2/frames.go:132:1`: exported method `GoAway.Deserialize` should have comment or be unexported
31. `proto/h2/frames.go:151:1`: exported method `GoAway.Serialize` should have comment or be unexported
32. `proto/h2/frames.go:169:1`: exported method `Headers.Type` should have comment or be unexported
33. `proto/h2/frames.go:170:1`: exported method `Headers.Headers` should have comment or be unexported
34. `proto/h2/frames.go:171:1`: exported method `Headers.SetHeaders` should have comment or be unexported
35. `proto/h2/frames.go:172:1`: exported method `Headers.AppendRawHeaders` should have comment or be unexported
36. `proto/h2/frames.go:173:1`: exported method `Headers.EndStream` should have comment or be unexported
37. `proto/h2/frames.go:174:1`: exported method `Headers.SetEndStream` should have comment or be unexported
38. `proto/h2/frames.go:175:1`: exported method `Headers.EndHeaders` should have comment or be unexported
39. `proto/h2/frames.go:176:1`: exported method `Headers.SetEndHeaders` should have comment or be unexported
40. `proto/h2/frames.go:177:1`: exported method `Headers.Stream` should have comment or be unexported
41. `proto/h2/frames.go:178:1`: exported method `Headers.SetStream` should have comment or be unexported
42. `proto/h2/frames.go:179:1`: exported method `Headers.Weight` should have comment or be unexported
43. `proto/h2/frames.go:180:1`: exported method `Headers.SetWeight` should have comment or be unexported
44. `proto/h2/frames.go:181:1`: exported method `Headers.Exclusive` should have comment or be unexported
45. `proto/h2/frames.go:182:1`: exported method `Headers.SetExclusive` should have comment or be unexported
46. `proto/h2/frames.go:183:1`: exported method `Headers.Padding` should have comment or be unexported
47. `proto/h2/frames.go:184:1`: exported method `Headers.SetPadding` should have comment or be unexported
48. `proto/h2/frames.go:186:1`: exported method `Headers.Reset` should have comment or be unexported
49. `proto/h2/frames.go:197:1`: exported method `Headers.Deserialize` should have comment or be unexported
50. `proto/h2/frames.go:238:1`: exported method `Headers.Serialize` should have comment or be unexported
51. `proto/h2/frames.go:295:1`: exported method `Ping.Type` should have comment or be unexported
52. `proto/h2/frames.go:296:1`: exported method `Ping.IsAck` should have comment or be unexported
53. `proto/h2/frames.go:297:1`: exported method `Ping.SetAck` should have comment or be unexported
54. `proto/h2/frames.go:298:1`: exported method `Ping.Reset` should have comment or be unexported
55. `proto/h2/frames.go:299:1`: exported method `Ping.Data` should have comment or be unexported
56. `proto/h2/frames.go:300:1`: exported method `Ping.SetData` should have comment or be unexported
57. `proto/h2/frames.go:303:1`: exported method `Ping.Deserialize` should have comment or be unexported
58. `proto/h2/frames.go:318:1`: exported method `Ping.Serialize` should have comment or be unexported
59. `proto/h2/frames.go:333:1`: exported method `Priority.Type` should have comment or be unexported
60. `proto/h2/frames.go:334:1`: exported method `Priority.Reset` should have comment or be unexported
61. `proto/h2/frames.go:335:1`: exported method `Priority.Stream` should have comment or be unexported
62. `proto/h2/frames.go:336:1`: exported method `Priority.SetStream` should have comment or be unexported
63. `proto/h2/frames.go:337:1`: exported method `Priority.Weight` should have comment or be unexported
64. `proto/h2/frames.go:338:1`: exported method `Priority.SetWeight` should have comment or be unexported
65. `proto/h2/frames.go:339:1`: exported method `Priority.Exclusive` should have comment or be unexported
66. `proto/h2/frames.go:340:1`: exported method `Priority.SetExclusive` should have comment or be unexported
67. `proto/h2/frames.go:342:1`: exported method `Priority.Deserialize` should have comment or be unexported
68. `proto/h2/frames.go:363:1`: exported method `Priority.Serialize` should have comment or be unexported
69. `proto/h2/frames.go:380:1`: exported method `PushPromise.Type` should have comment or be unexported
70. `proto/h2/frames.go:381:1`: exported method `PushPromise.PromisedStream` should have comment or be unexported
71. `proto/h2/frames.go:382:1`: exported method `PushPromise.Headers` should have comment or be unexported
72. `proto/h2/frames.go:384:1`: exported method `PushPromise.Reset` should have comment or be unexported
73. `proto/h2/frames.go:391:1`: exported method `PushPromise.SetHeader` should have comment or be unexported
74. `proto/h2/frames.go:397:1`: exported method `PushPromise.Deserialize` should have comment or be unexported
75. `proto/h2/frames.go:424:1`: exported method `PushPromise.Serialize` should have comment or be unexported
76. `proto/h2/frames.go:434:1`: exported method `RstStream.Type` should have comment or be unexported
77. `proto/h2/frames.go:435:1`: exported method `RstStream.Code` should have comment or be unexported
78. `proto/h2/frames.go:436:1`: exported method `RstStream.SetCode` should have comment or be unexported
79. `proto/h2/frames.go:437:1`: exported method `RstStream.Reset` should have comment or be unexported
80. `proto/h2/frames.go:440:1`: exported method `RstStream.Deserialize` should have comment or be unexported
81. `proto/h2/frames.go:454:1`: exported method `RstStream.Serialize` should have comment or be unexported
82. `proto/h2/frames.go:464:1`: exported method `WindowUpdate.Type` should have comment or be unexported
83. `proto/h2/frames.go:465:1`: exported method `WindowUpdate.Reset` should have comment or be unexported
84. `proto/h2/frames.go:466:1`: exported method `WindowUpdate.Increment` should have comment or be unexported
85. `proto/h2/frames.go:467:1`: exported method `WindowUpdate.SetIncrement` should have comment or be unexported
86. `proto/h2/frames.go:469:1`: exported method `WindowUpdate.Deserialize` should have comment or be unexported
87. `proto/h2/frames.go:488:1`: exported method `WindowUpdate.Serialize` should have comment or be unexported

**Note on Method Reconciliation (93 total methods vs 87 revive violations)**:  
Revive's `exported` rule skips methods satisfying standard library interfaces:
- `Continuation.Write(b []byte) (int, error)` (satisfies `io.Writer`)
- `Data.Write(b []byte) (int, error)` (satisfies `io.Writer`)
- `GoAway.Error() string` (satisfies `error` / `fmt.Stringer`)
- `Ping.Write(b []byte) (int, error)` (satisfies `io.Writer`)
- `PushPromise.Write(b []byte) (int, error)` (satisfies `io.Writer`)
- `RstStream.Error() error` (standard error receiver)  
93 total methods minus 6 standard library interface methods = **exactly 87 violations**. Under `PROJECT.md §6.2`, all 93 methods and all 9 struct declarations must receive comprehensive docstrings.

---

## 2. Logic Chain

### 2.1 Decomposition Topology
1. **Observation**: `proto/h2/frames.go` bundles 9 distinct frame implementations spanning data transport, header blocks, connection control, flow control, and stream extensions into a single 492-line file.
2. **Requirement & Target Layout** (`PROJECT.md §5`):
   - `frame_data.go` (DATA frame implementation)
   - `frame_headers.go` (HEADERS frame implementation)
   - `frame_control.go` (PING, GOAWAY, RST_STREAM, PRIORITY frames)
   - `frame_window.go` (WINDOW_UPDATE frame implementation)
   - `frame_ext.go` (CONTINUATION, PUSH_PROMISE frames)
3. **Logic**:
   - `Data` has distinct semantic concerns (streaming payload octets, END_STREAM, padding) governed by RFC 9113 §6.1 -> belongs in `frame_data.go`.
   - `Headers` handles HPACK field block fragments, priority signaling, END_HEADERS, and END_STREAM governed by RFC 9113 §6.2. Moving `AppendHeaderField` from `header.go:292` consolidates all `Headers` operations into `frame_headers.go`.
   - `Ping`, `GoAway`, `RstStream`, and `Priority` are connection- and stream-level control frames with fixed layouts and signaling semantics (RFC 9113 §6.7, §6.8, §6.4, §6.3) -> belong in `frame_control.go`.
   - `WindowUpdate` handles connection- and stream-level credit accounting (RFC 9113 §6.9) -> belongs in `frame_window.go`.
   - `Continuation` (RFC 9113 §6.10) and `PushPromise` (RFC 9113 §6.6 & §8.4) extend header sequences and server push -> belong in `frame_ext.go`.
   - `Settings` (RFC 9113 §6.5) is already cleanly modularized in `proto/h2/settings.go` and requires no moves.
   - Decomposing `frames.go` into these 5 target files and deleting `frames.go` achieves 100% single-responsibility separation while maintaining identical package `h2` symbols.

### 2.2 Invariant Preservation: Off-Heap Slab Allocation vs sync.Pool
1. **Observation**: `ConnectionFramePool` in `proto/h2/frame_pool.go` maintains per-goroutine off-heap slab allocators (`offheap.SlabAllocator[T]`) for the four POD types: `Ping`, `Priority`, `RstStream`, `WindowUpdate`.
2. **Memory Layout Invariant**:
   - The 4 POD frame structs contain zero slices, zero string pointers, and zero interface values. Their sizes are fixed at compile time (Ping: 9B; Priority: 8B; RstStream: 4B; WindowUpdate: 8B).
   - Non-POD frames (`Data`, `Headers`, `Continuation`, `PushPromise`, `GoAway`, `Settings`) hold dynamic slices (`[]byte`) and MUST remain backed by `sync.Pool` (`framePools`) to avoid manual heap tracking.
3. **Silicon Performance Invariant**:
   - Benchmark confirmed `BenchmarkAcquireRelease_PerGoroutinePool_Parallel` completes in **3.25 ns/op** with **0 B/op** and **0 allocs/op**.
   - During decomposition, moving the definitions of `Ping`, `Priority`, `RstStream` to `frame_control.go` and `WindowUpdate` to `frame_window.go` MUST NOT modify their struct layouts or add pointer/slice fields.
   - The global sync.Pool fallback in `proto/h2/frame.go` (`framePools`) and `AcquireFrame`/`ReleaseFrame` remain unchanged.

---

## 3. Caveats

1. **POD Struct Invariant**: `Ping`, `Priority`, `RstStream`, and `WindowUpdate` are directly allocated in off-heap kernel memory by `offheap.SlabAllocator[T]`. Under NO circumstances should any slice (`[]byte`) or pointer fields be added to these structs, as the slab allocator does not register GC roots.
2. **`AppendHeaderField` Consolidation**: `(h *Headers) AppendHeaderField(hp *hpack.HPACK, hf *hpack.HeaderField, store bool)` is currently defined in `proto/h2/header.go:292`. When implementing `frame_headers.go`, this method should be relocated from `header.go` to `frame_headers.go` to unify all `Headers` receiver methods.
3. **`ConnectionFramePool` Concurrency Expectation**: `ConnectionFramePool` is NOT thread-safe. As documented, it is intended to be owned per-goroutine (one instance for connection read loop, one for write loop). This avoids synchronization overhead.
4. **No Public API Breakage**: All type names, method signatures, flag values, and error returns must remain character-for-character identical to maintain backwards compatibility with `client/h2` and downstream consumers.

---

## 4. Conclusion & Complete Implementation Design

The decomposition of `proto/h2/frames.go` into 5 single-responsibility files must be implemented as follows. Below are the complete specifications, structural mappings, and drafted RFC 9113 docstrings for every entity.

### 4.1 Target File: `proto/h2/frame_data.go`
- **Scope**: RFC 9113 Section 6.1 (DATA).
- **Imports**: none.
- **Entities**:
  - `type Data struct`
  - `(d *Data) Type() FrameType`
  - `(d *Data) Reset()`
  - `(d *Data) SetEndStream(v bool)`
  - `(d *Data) EndStream() bool`
  - `(d *Data) Data() []byte`
  - `(d *Data) SetData(b []byte)`
  - `(d *Data) Padding() bool`
  - `(d *Data) SetPadding(v bool)`
  - `(d *Data) Append(b []byte)`
  - `(d *Data) Len() int`
  - `(d *Data) Write(b []byte) (int, error)`
  - `(d *Data) Deserialize(fr *FrameHeader) error`
  - `(d *Data) Serialize(fr *FrameHeader)`

#### Drafted Code & RFC 9113 Docstrings (`frame_data.go`):
```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

// Data conveys arbitrary, variable-length sequences of octets associated with a stream (RFC 9113 §6.1).
//
// DATA frames MUST be associated with a stream. If a DATA frame is received whose stream
// identifier field is 0x00, the recipient MUST respond with a connection error of type PROTOCOL_ERROR.
//
// Concurrency: Not thread-safe; instances are pooled and must be owned by a single goroutine.
// Lifecycle: Instances are managed by [framePools] (sync.Pool). Call [Data.Reset] prior to reuse.
type Data struct {
	endStream  bool
	hasPadding bool
	b          []byte
}

// Type returns the FrameType identifier FrameData (0x0, RFC 9113 §6.1).
func (d *Data) Type() FrameType { return FrameData }

// Reset clears payload buffers, padding flags, and stream termination state for pool reuse.
func (d *Data) Reset() { d.endStream = false; d.hasPadding = false; d.b = d.b[:0] }

// SetEndStream sets the END_STREAM flag bit (0x1), signaling this is the last frame sent on the stream (RFC 9113 §6.1).
func (d *Data) SetEndStream(v bool) { d.endStream = v }

// EndStream reports whether the END_STREAM flag bit (0x1) is set on this frame (RFC 9113 §6.1).
func (d *Data) EndStream() bool { return d.endStream }

// Data returns the raw unpadded payload octets carried by the DATA frame (RFC 9113 §6.1).
func (d *Data) Data() []byte { return d.b }

// SetData replaces the frame payload with the provided octet slice, reusing existing capacity when possible.
func (d *Data) SetData(b []byte) { d.b = append(d.b[:0], b...) }

// Padding reports whether the PADDED flag bit (0x8) is enabled on this frame (RFC 9113 §6.1).
func (d *Data) Padding() bool { return d.hasPadding }

// SetPadding configures whether the PADDED flag bit (0x8) and trailing zero octets are emitted on serialization (RFC 9113 §6.1).
func (d *Data) SetPadding(v bool) { d.hasPadding = v }

// Append appends payload octets to the existing buffer without reallocating if within slice capacity.
func (d *Data) Append(b []byte) { d.b = append(d.b, b...) }

// Len returns the current length in octets of the application data payload.
func (d *Data) Len() int { return len(d.b) }

// Write appends b to the internal payload buffer, satisfying the [io.Writer] interface.
func (d *Data) Write(b []byte) (int, error) { d.Append(b); return len(b), nil }

// Deserialize decodes the DATA frame payload from fr, validating stream binding and stripping padding (RFC 9113 §6.1).
func (d *Data) Deserialize(fr *FrameHeader) error {
	if fr.Stream() == 0 {
		return NewGoAwayError(ProtocolError, "DATA frame must be on a specific stream, not 0")
	}

	payload := fr.payload

	if fr.Flags().Has(FlagPadded) {
		var err error

		payload, err = cutPadding(payload, fr.Len())
		if err != nil {
			return err
		}
	}

	d.endStream = fr.Flags().Has(FlagEndStream)
	d.b = append(d.b[:0], payload...)

	return nil
}

// Serialize encodes the DATA frame payload and active flags into the destination FrameHeader (RFC 9113 §6.1).
func (d *Data) Serialize(fr *FrameHeader) {
	if d.endStream {
		fr.SetFlags(fr.Flags().Add(FlagEndStream))
	}

	fr.payload = fr.payload[:0]

	if d.hasPadding {
		fr.SetFlags(fr.Flags().Add(FlagPadded))

		padLen := byte(0)
		fr.payload = append(fr.payload, padLen)

		fr.payload = append(fr.payload, d.b...)
		for i := byte(0); i < padLen; i++ {
			fr.payload = append(fr.payload, 0)
		}
	} else {
		fr.payload = append(fr.payload, d.b...)
	}
}
```

---

### 4.2 Target File: `proto/h2/frame_headers.go`
- **Scope**: RFC 9113 Section 6.2 (HEADERS) & Section 5.3.1 (Stream Dependencies).
- **Imports**: `"github.com/lemon4ksan/foundation/net/hpack"`.
- **Entities**:
  - `type Headers struct`
  - `(h *Headers) Type() FrameType`
  - `(h *Headers) Headers() []byte`
  - `(h *Headers) SetHeaders(b []byte)`
  - `(h *Headers) AppendRawHeaders(b []byte)`
  - `(h *Headers) EndStream() bool`
  - `(h *Headers) SetEndStream(v bool)`
  - `(h *Headers) EndHeaders() bool`
  - `(h *Headers) SetEndHeaders(v bool)`
  - `(h *Headers) Stream() uint32`
  - `(h *Headers) SetStream(stream uint32)`
  - `(h *Headers) Weight() byte`
  - `(h *Headers) SetWeight(w byte)`
  - `(h *Headers) Exclusive() bool`
  - `(h *Headers) SetExclusive(v bool)`
  - `(h *Headers) Padding() bool`
  - `(h *Headers) SetPadding(v bool)`
  - `(h *Headers) Reset()`
  - `(h *Headers) Deserialize(frh *FrameHeader) error`
  - `(h *Headers) Serialize(frh *FrameHeader)`
  - `(h *Headers) AppendHeaderField(hp *hpack.HPACK, hf *hpack.HeaderField, store bool)` (relocated from `header.go:292`)

#### Drafted Code & RFC 9113 Docstrings (`frame_headers.go`):
```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"github.com/lemon4ksan/foundation/net/hpack"
)

// Headers carries a field block fragment and optionally opens or terminates a stream (RFC 9113 §6.2).
//
// HEADERS frames can be sent on a stream in the "idle", "reserved (local)", "open", or "half-closed (remote)"
// state. The HEADERS frame can include priority information (deprecated per RFC 9113 §5.3.2) and padding.
//
// Concurrency: Not thread-safe; instances are pooled and must be owned by a single goroutine.
// Lifecycle: Instances are managed by [framePools] (sync.Pool). Call [Headers.Reset] prior to reuse.
type Headers struct {
	hasPadding bool
	stream     uint32
	weight     uint8
	endStream  bool
	endHeaders bool
	priority   bool
	exclusive  bool
	rawHeaders []byte
}

// Type returns the FrameType identifier FrameHeaders (0x1, RFC 9113 §6.2).
func (h *Headers) Type() FrameType { return FrameHeaders }

// Headers returns the raw HPACK-encoded field block fragment octets (RFC 9113 §6.2).
func (h *Headers) Headers() []byte { return h.rawHeaders }

// SetHeaders replaces the HPACK field block fragment with the provided octet slice.
func (h *Headers) SetHeaders(b []byte) { h.rawHeaders = append(h.rawHeaders[:0], b...) }

// AppendRawHeaders appends raw HPACK-encoded field block octets to the internal buffer.
func (h *Headers) AppendRawHeaders(b []byte) { h.rawHeaders = append(h.rawHeaders, b...) }

// EndStream reports whether the END_STREAM flag bit (0x1) is set on this frame (RFC 9113 §6.2).
func (h *Headers) EndStream() bool { return h.endStream }

// SetEndStream sets the END_STREAM flag bit (0x1), indicating this frame concludes the stream (RFC 9113 §6.2).
func (h *Headers) SetEndStream(v bool) { h.endStream = v }

// EndHeaders reports whether the END_HEADERS flag bit (0x4) is set, indicating a complete field section (RFC 9113 §6.2).
func (h *Headers) EndHeaders() bool { return h.endHeaders }

// SetEndHeaders sets the END_HEADERS flag bit (0x4), signaling no subsequent CONTINUATION frames follow (RFC 9113 §6.2).
func (h *Headers) SetEndHeaders(v bool) { h.endHeaders = v }

// Stream returns the 31-bit stream dependency identifier when priority signaling is enabled (RFC 9113 §6.2 & §5.3.1).
func (h *Headers) Stream() uint32 { return h.stream }

// SetStream sets the 31-bit stream dependency identifier for priority signaling (RFC 9113 §6.2 & §5.3.1).
func (h *Headers) SetStream(stream uint32) { h.stream = stream }

// Weight returns the priority weight octet representing value weight+1 in range [1, 256] (RFC 9113 §6.2 & §5.3.2).
func (h *Headers) Weight() byte { return h.weight }

// SetWeight sets the priority weight octet (RFC 9113 §6.2 & §5.3.2).
func (h *Headers) SetWeight(w byte) { h.weight = w }

// Exclusive reports whether the exclusive dependency bit is enabled in the priority specification (RFC 9113 §6.2 & §5.3.1).
func (h *Headers) Exclusive() bool { return h.exclusive }

// SetExclusive sets or clears the exclusive dependency bit in the priority specification (RFC 9113 §6.2 & §5.3.1).
func (h *Headers) SetExclusive(v bool) { h.exclusive = v }

// Padding reports whether the PADDED flag bit (0x8) is set on this frame (RFC 9113 §6.2).
func (h *Headers) Padding() bool { return h.hasPadding }

// SetPadding configures whether the PADDED flag bit (0x8) is emitted during serialization (RFC 9113 §6.2).
func (h *Headers) SetPadding(v bool) { h.hasPadding = v }

// Reset clears all header state, priority flags, and payload buffers for pool reuse.
func (h *Headers) Reset() {
	h.hasPadding = false
	h.stream = 0
	h.weight = 0
	h.endStream = false
	h.endHeaders = false
	h.priority = false
	h.exclusive = false
	h.rawHeaders = h.rawHeaders[:0]
}

// Deserialize decodes the HEADERS frame payload from frh, parsing padding and optional priority fields (RFC 9113 §6.2).
func (h *Headers) Deserialize(frh *FrameHeader) error {
	if frh.Stream() == 0 {
		return NewGoAwayError(ProtocolError, "HEADERS frame must be on a specific stream, not 0")
	}

	flags := frh.Flags()
	payload := frh.payload

	if flags.Has(FlagPadded) {
		var err error

		payload, err = cutPadding(payload, len(payload))
		if err != nil {
			return err
		}
	}

	if flags.Has(FlagPriority) {
		if len(payload) < 5 {
			return NewGoAwayError(FrameSizeError, "invalid HEADERS frame size for priority (RFC 9113 §6.2)")
		}

		h.priority = true
		h.exclusive = (payload[0] & 0x80) != 0

		h.stream = bytesToUint32(payload) & (1<<31 - 1)
		if h.stream == frh.Stream() {
			return NewGoAwayError(ProtocolError, "stream cannot depend on itself (RFC 9113 §5.3.1)")
		}

		h.weight = payload[4]
		payload = payload[5:]
	}

	h.endStream = flags.Has(FlagEndStream)
	h.endHeaders = flags.Has(FlagEndHeaders)
	h.rawHeaders = append(h.rawHeaders, payload...)

	return nil
}

// Serialize encodes the HEADERS frame payload, priority fields, and active flags into frh (RFC 9113 §6.2).
func (h *Headers) Serialize(frh *FrameHeader) {
	if h.endStream {
		frh.SetFlags(frh.Flags().Add(FlagEndStream))
	}

	if h.endHeaders {
		frh.SetFlags(frh.Flags().Add(FlagEndHeaders))
	}

	frh.payload = frh.payload[:0]

	if h.hasPadding {
		frh.SetFlags(frh.Flags().Add(FlagPadded))
		frh.payload = append(frh.payload, 0)
	}

	if h.priority {
		frh.SetFlags(frh.Flags().Add(FlagPriority))

		var priBuf [5]byte
		uint32ToBytes(priBuf[0:4], h.stream)

		if h.exclusive {
			priBuf[0] |= 0x80
		}

		priBuf[4] = h.weight
		frh.payload = append(frh.payload, priBuf[:]...)
	}

	frh.payload = append(frh.payload, h.rawHeaders...)

	if h.hasPadding {
		padLen := byte(0)
		if len(frh.payload) > 0 {
			frh.payload[0] = padLen
		}

		for i := byte(0); i < padLen; i++ {
			frh.payload = append(frh.payload, 0)
		}
	}
}

// AppendHeaderField encodes and appends an individual HPACK header field into the field block fragment (RFC 7541 & RFC 9113 §6.2).
func (h *Headers) AppendHeaderField(hp *hpack.HPACK, hf *hpack.HeaderField, store bool) {
	h.SetHeaders(hp.AppendHeader(h.Headers(), hf, store))
}
```

---

### 4.3 Target File: `proto/h2/frame_control.go`
- **Scope**: RFC 9113 Section 6.7 (PING), Section 6.8 (GOAWAY), Section 6.4 (RST_STREAM), Section 6.3 (PRIORITY).
- **Imports**: `"fmt"`.
- **Entities**:
  - `type Ping struct` (POD)
  - `type GoAway struct` (non-POD, dynamic debug slice)
  - `type RstStream struct` (POD)
  - `type Priority struct` (POD)
  - All respective methods (37 methods total: 9 Ping, 11 GoAway, 7 RstStream, 10 Priority).

#### Drafted Code & RFC 9113 Docstrings (`frame_control.go`):
```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"fmt"
)

// Ping measures round-trip time and verifies connection liveness with an 8-octet opaque payload (RFC 9113 §6.7).
//
// PING frames MUST be sent on stream 0x00. Receipt of a PING frame with any other stream identifier
// MUST be treated as a connection error of type PROTOCOL_ERROR.
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: POD struct. Eligible for off-heap slab allocation via [ConnectionFramePool] or [framePools].
type Ping struct {
	ack  bool
	data [8]byte
}

// Type returns the FrameType identifier FramePing (0x6, RFC 9113 §6.7).
func (p *Ping) Type() FrameType { return FramePing }

// IsAck reports whether the ACK flag bit (0x1) is set, indicating a PING response (RFC 9113 §6.7).
func (p *Ping) IsAck() bool { return p.ack }

// SetAck sets or clears the ACK flag bit (0x1) on the PING frame (RFC 9113 §6.7).
func (p *Ping) SetAck(ack bool) { p.ack = ack }

// Reset clears the ACK flag and resets the PING frame state for pool reuse.
func (p *Ping) Reset() { p.ack = false }

// Data returns the 8-octet opaque payload of the PING frame (RFC 9113 §6.7).
func (p *Ping) Data() []byte { return p.data[:] }

// SetData copies up to 8 octets from b into the PING frame opaque payload (RFC 9113 §6.7).
func (p *Ping) SetData(b []byte) { copy(p.data[:], b) }

// Write copies up to 8 octets from b into the PING payload, satisfying the [io.Writer] interface.
func (p *Ping) Write(b []byte) (int, error) { copy(p.data[:], b); return len(b), nil }

// Deserialize decodes the 8-octet PING payload and ACK flag from frh, validating stream 0x00 binding (RFC 9113 §6.7).
func (p *Ping) Deserialize(frh *FrameHeader) error {
	if frh.Stream() != 0 {
		return NewGoAwayError(ProtocolError, "PING frame must be on stream 0")
	}

	p.ack = frh.Flags().Has(FlagAck)
	if len(frh.payload) != 8 {
		return NewGoAwayError(FrameSizeError, "invalid PING frame size (RFC 9113 §6.7)")
	}

	p.SetData(frh.payload)

	return nil
}

// Serialize encodes the 8-octet PING payload and ACK flag into the destination FrameHeader (RFC 9113 §6.7).
func (p *Ping) Serialize(fr *FrameHeader) {
	if p.ack {
		fr.SetFlags(fr.Flags().Add(FlagAck))
	}

	fr.setPayload(p.data[:])
}

// GoAway initiates connection shutdown or reports fatal connection-level protocol violations (RFC 9113 §6.8).
//
// GOAWAY frames MUST be sent on stream 0x00. It allows an endpoint to gracefully stop accepting new
// streams while finishing processing of previously established streams.
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: Managed by global [framePools] (sync.Pool) due to dynamic debug data slice. Call [GoAway.Reset] prior to reuse.
type GoAway struct {
	stream uint32
	code   ErrorCode
	data   []byte
}

// Type returns the FrameType identifier FrameGoAway (0x7, RFC 9113 §6.8).
func (ga *GoAway) Type() FrameType { return FrameGoAway }

// Reset clears the stream identifier, error code, and debug data buffer for pool reuse.
func (ga *GoAway) Reset() { ga.stream = 0; ga.code = 0; ga.data = ga.data[:0] }

// Code returns the 32-bit HTTP/2 error code describing the shutdown cause (RFC 9113 §6.8 & §7).
func (ga *GoAway) Code() ErrorCode { return ga.code }

// SetCode sets the 32-bit HTTP/2 error code on the GOAWAY frame (RFC 9113 §6.8 & §7).
func (ga *GoAway) SetCode(code ErrorCode) { ga.code = code & (1<<31 - 1) }

// Stream returns the 31-bit last stream identifier processed by the sender (RFC 9113 §6.8).
func (ga *GoAway) Stream() uint32 { return ga.stream }

// SetStream sets the 31-bit last stream identifier processed by the sender (RFC 9113 §6.8).
func (ga *GoAway) SetStream(stream uint32) { ga.stream = stream & (1<<31 - 1) }

// Data returns the additional opaque debug data octets carried by the GOAWAY frame (RFC 9113 §6.8).
func (ga *GoAway) Data() []byte { return ga.data }

// SetData replaces the opaque debug data with a copy of b.
func (ga *GoAway) SetData(b []byte) { ga.data = append(ga.data[:0], b...) }

// Error formats the GOAWAY stream ID, error code, and debug data into a human-readable diagnostic string.
func (ga *GoAway) Error() string {
	return fmt.Sprintf("stream=%d, code=%s, data=%s", ga.stream, ga.code, ga.data)
}

// Deserialize decodes the last stream identifier, error code, and debug data from fr (RFC 9113 §6.8).
func (ga *GoAway) Deserialize(fr *FrameHeader) error {
	if fr.Stream() != 0 {
		return NewGoAwayError(ProtocolError, "GOAWAY frame must be on stream 0")
	}

	if len(fr.payload) < 8 {
		return NewGoAwayError(FrameSizeError, "invalid GOAWAY frame size (RFC 9113 §6.8)")
	}

	ga.stream = bytesToUint32(fr.payload) & (1<<31 - 1)
	ga.code = ErrorCode(bytesToUint32(fr.payload[4:]))

	if len(fr.payload) > 8 {
		ga.data = append(ga.data[:0], fr.payload[8:]...)
	}

	return nil
}

// Serialize encodes the last stream ID, error code, and debug data into the destination FrameHeader (RFC 9113 §6.8).
func (ga *GoAway) Serialize(fr *FrameHeader) {
	fr.payload = appendUint32Bytes(fr.payload[:0], ga.stream)
	fr.payload = appendUint32Bytes(fr.payload, uint32(ga.code))
	fr.payload = append(fr.payload, ga.data...)
}

// RstStream signals immediate termination of an individual stream (RFC 9113 §6.4).
//
// RST_STREAM frames MUST be associated with a specific stream. If received on stream 0x00,
// the recipient MUST treat it as a connection error of type PROTOCOL_ERROR.
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: POD struct. Eligible for off-heap slab allocation via [ConnectionFramePool] or [framePools].
type RstStream struct {
	code ErrorCode
}

// Type returns the FrameType identifier FrameResetStream (0x3, RFC 9113 §6.4).
func (rst *RstStream) Type() FrameType { return FrameResetStream }

// Code returns the 32-bit error code indicating why the stream was reset (RFC 9113 §6.4 & §7).
func (rst *RstStream) Code() ErrorCode { return rst.code }

// SetCode sets the 32-bit error code on the RST_STREAM frame (RFC 9113 §6.4 & §7).
func (rst *RstStream) SetCode(code ErrorCode) { rst.code = code }

// Reset clears the error code to zero for pool reuse.
func (rst *RstStream) Reset() { rst.code = 0 }

// Error returns the underlying [ErrorCode] as a standard Go error.
func (rst *RstStream) Error() error { return rst.code }

// Deserialize decodes the 4-octet error code from fr, validating stream binding (RFC 9113 §6.4).
func (rst *RstStream) Deserialize(fr *FrameHeader) error {
	if fr.Stream() == 0 {
		return NewGoAwayError(ProtocolError, "RST_STREAM frame must be on a specific stream, not 0")
	}

	if len(fr.payload) != 4 {
		return NewGoAwayError(FrameSizeError, "invalid RST_STREAM frame size (RFC 9113 §6.4)")
	}

	rst.code = ErrorCode(bytesToUint32(fr.payload))

	return nil
}

// Serialize encodes the 4-octet error code into the destination FrameHeader (RFC 9113 §6.4).
func (rst *RstStream) Serialize(fr *FrameHeader) {
	fr.payload = appendUint32Bytes(fr.payload[:0], uint32(rst.code))
	fr.length = 4
}

// Priority specifies stream dependency and weight (RFC 9113 §6.3, deprecated per §5.3.2).
//
// PRIORITY frames can be sent on any stream state. Priority signaling does not alter stream state.
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: POD struct. Eligible for off-heap slab allocation via [ConnectionFramePool] or [framePools].
type Priority struct {
	exclusive bool
	stream    uint32
	weight    byte
}

// Type returns the FrameType identifier FramePriority (0x2, RFC 9113 §6.3).
func (pry *Priority) Type() FrameType { return FramePriority }

// Reset clears priority parameters to their zero values for pool reuse.
func (pry *Priority) Reset() { pry.exclusive = false; pry.stream = 0; pry.weight = 0 }

// Stream returns the 31-bit stream dependency identifier (RFC 9113 §6.3 & §5.3.1).
func (pry *Priority) Stream() uint32 { return pry.stream }

// SetStream sets the 31-bit stream dependency identifier (RFC 9113 §6.3 & §5.3.1).
func (pry *Priority) SetStream(stream uint32) { pry.stream = stream & (1<<31 - 1) }

// Weight returns the priority weight octet representing value weight+1 in range [1, 256] (RFC 9113 §6.3 & §5.3.2).
func (pry *Priority) Weight() byte { return pry.weight }

// SetWeight sets the priority weight octet (RFC 9113 §6.3 & §5.3.2).
func (pry *Priority) SetWeight(w byte) { pry.weight = w }

// Exclusive reports whether the exclusive dependency bit is enabled (RFC 9113 §6.3 & §5.3.1).
func (pry *Priority) Exclusive() bool { return pry.exclusive }

// SetExclusive sets or clears the exclusive dependency bit (RFC 9113 §6.3 & §5.3.1).
func (pry *Priority) SetExclusive(v bool) { pry.exclusive = v }

// Deserialize decodes the 5-octet stream dependency and weight payload from fr (RFC 9113 §6.3).
func (pry *Priority) Deserialize(fr *FrameHeader) error {
	if fr.Stream() == 0 {
		return NewGoAwayError(ProtocolError, "PRIORITY frame must be on a specific stream, not 0")
	}

	if len(fr.payload) != 5 {
		return NewGoAwayError(FrameSizeError, "invalid PRIORITY frame size (RFC 9113 §6.3)")
	}

	pry.exclusive = (fr.payload[0] & 0x80) != 0

	pry.stream = bytesToUint32(fr.payload) & (1<<31 - 1)
	if pry.stream == fr.Stream() {
		return NewGoAwayError(ProtocolError, "stream cannot depend on itself (RFC 9113 §5.3.1)")
	}

	pry.weight = fr.payload[4]

	return nil
}

// Serialize encodes the 5-octet stream dependency, exclusive bit, and weight into fr (RFC 9113 §6.3).
func (pry *Priority) Serialize(fr *FrameHeader) {
	fr.payload = appendUint32Bytes(fr.payload[:0], pry.stream)
	if pry.exclusive {
		fr.payload[0] |= 0x80
	}

	fr.payload = append(fr.payload, pry.weight)
}
```

---

### 4.4 Target File: `proto/h2/frame_window.go`
- **Scope**: RFC 9113 Section 6.9 (WINDOW_UPDATE).
- **Imports**: none.
- **Entities**:
  - `type WindowUpdate struct` (POD)
  - `(wu *WindowUpdate) Type() FrameType`
  - `(wu *WindowUpdate) Reset()`
  - `(wu *WindowUpdate) Increment() int`
  - `(wu *WindowUpdate) SetIncrement(inc int)`
  - `(wu *WindowUpdate) Deserialize(fr *FrameHeader) error`
  - `(wu *WindowUpdate) Serialize(fr *FrameHeader)`

#### Drafted Code & RFC 9113 Docstrings (`frame_window.go`):
```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

// WindowUpdate implements stream and connection-level flow control credits (RFC 9113 §6.9).
//
// WINDOW_UPDATE frames can be sent on stream 0x00 (connection flow control) or on an active stream.
// The window size increment MUST NOT be zero (treated as PROTOCOL_ERROR per RFC 9113 §6.9).
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: POD struct. Eligible for off-heap slab allocation via [ConnectionFramePool] or [framePools].
type WindowUpdate struct {
	increment int
}

// Type returns the FrameType identifier FrameWindowUpdate (0x8, RFC 9113 §6.9).
func (wu *WindowUpdate) Type() FrameType { return FrameWindowUpdate }

// Reset clears the window increment to zero for pool reuse.
func (wu *WindowUpdate) Reset() { wu.increment = 0 }

// Increment returns the flow-control window increment in octets (RFC 9113 §6.9).
func (wu *WindowUpdate) Increment() int { return wu.increment }

// SetIncrement sets the flow-control window increment in octets (RFC 9113 §6.9).
func (wu *WindowUpdate) SetIncrement(inc int) { wu.increment = inc }

// Deserialize decodes the 4-octet window size increment from fr, validating non-zero increments (RFC 9113 §6.9).
func (wu *WindowUpdate) Deserialize(fr *FrameHeader) error {
	if len(fr.payload) != 4 {
		wu.increment = 0
		return NewGoAwayError(FrameSizeError, "invalid WINDOW_UPDATE frame size (RFC 9113 §6.9)")
	}

	wu.increment = int(bytesToUint32(fr.payload) & (1<<31 - 1))
	if wu.increment == 0 {
		if fr.Stream() == 0 {
			return NewGoAwayError(ProtocolError, "window increment of zero on connection")
		}

		return NewResetStreamError(ProtocolError, "window increment of zero on stream")
	}

	return nil
}

// Serialize encodes the 4-octet window size increment into the destination FrameHeader (RFC 9113 §6.9).
func (wu *WindowUpdate) Serialize(fr *FrameHeader) {
	fr.payload = appendUint32Bytes(fr.payload[:0], uint32(wu.increment)) //nolint:gosec
	fr.length = 4
}
```

---

### 4.5 Target File: `proto/h2/frame_ext.go`
- **Scope**: RFC 9113 Section 6.10 (CONTINUATION) & Section 6.6 / Section 8.4 (PUSH_PROMISE).
- **Imports**: none.
- **Entities**:
  - `type Continuation struct` (non-POD)
  - `type PushPromise struct` (non-POD)
  - All respective methods (18 methods total: 10 Continuation, 8 PushPromise).

#### Drafted Code & RFC 9113 Docstrings (`frame_ext.go`):
```go
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

// Continuation extends a sequence of field block fragments across frame boundaries (RFC 9113 §6.10).
//
// A CONTINUATION frame MUST be preceded by a HEADERS, PUSH_PROMISE, or another CONTINUATION frame
// on the same stream without any intervening frames of any other type (RFC 9113 §6.10).
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: Managed by global [framePools] (sync.Pool). Call [Continuation.Reset] prior to reuse.
type Continuation struct {
	endHeaders bool
	rawHeaders []byte
}

// Type returns the FrameType identifier FrameContinuation (0x9, RFC 9113 §6.10).
func (c *Continuation) Type() FrameType { return FrameContinuation }

// Reset clears the END_HEADERS flag and raw header buffer for pool reuse.
func (c *Continuation) Reset() {
	c.endHeaders = false
	c.rawHeaders = c.rawHeaders[:0]
}

// Headers returns the extended field block fragment octets (RFC 9113 §6.10).
func (c *Continuation) Headers() []byte { return c.rawHeaders }

// SetEndHeaders sets the END_HEADERS flag bit (0x4), signaling the completion of the field block (RFC 9113 §6.10).
func (c *Continuation) SetEndHeaders(v bool) { c.endHeaders = v }

// EndHeaders reports whether the END_HEADERS flag bit (0x4) is enabled (RFC 9113 §6.10).
func (c *Continuation) EndHeaders() bool { return c.endHeaders }

// SetHeader replaces the field block fragment with the provided octet slice.
func (c *Continuation) SetHeader(b []byte) { c.rawHeaders = append(c.rawHeaders[:0], b...) }

// AppendHeader appends field block fragment octets to the internal buffer.
func (c *Continuation) AppendHeader(b []byte) { c.rawHeaders = append(c.rawHeaders, b...) }

// Write appends b to the internal header fragment, satisfying the [io.Writer] interface.
func (c *Continuation) Write(b []byte) (int, error) { c.AppendHeader(b); return len(b), nil }

// Deserialize decodes the CONTINUATION frame payload from fr, verifying non-zero stream binding (RFC 9113 §6.10).
func (c *Continuation) Deserialize(fr *FrameHeader) error {
	if fr.Stream() == 0 {
		return NewGoAwayError(ProtocolError, "CONTINUATION frame must be on a specific stream, not 0")
	}

	c.endHeaders = fr.Flags().Has(FlagEndHeaders)
	c.SetHeader(fr.payload)

	return nil
}

// Serialize encodes the CONTINUATION payload and END_HEADERS flag into the destination FrameHeader (RFC 9113 §6.10).
func (c *Continuation) Serialize(fr *FrameHeader) {
	if c.endHeaders {
		fr.SetFlags(fr.Flags().Add(FlagEndHeaders))
	}

	fr.setPayload(c.rawHeaders)
}

// PushPromise notifies the peer in advance of server-initiated streams (RFC 9113 §6.6 & §8.4).
//
// PUSH_PROMISE frames MUST be sent on a peer-initiated, open or half-closed stream.
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: Managed by global [framePools] (sync.Pool). Call [PushPromise.Reset] prior to reuse.
type PushPromise struct {
	pad    bool
	ended  bool
	stream uint32
	header []byte
}

// Type returns the FrameType identifier FramePushPromise (0x5, RFC 9113 §6.6).
func (pp *PushPromise) Type() FrameType { return FramePushPromise }

// PromisedStream returns the reserved 31-bit stream identifier promised by the server (RFC 9113 §6.6).
func (pp *PushPromise) PromisedStream() uint32 { return pp.stream }

// Headers returns the promised request field block fragment octets (RFC 9113 §6.6).
func (pp *PushPromise) Headers() []byte { return pp.header }

// Reset clears the promised stream ID, padding flag, and header buffer for pool reuse.
func (pp *PushPromise) Reset() {
	pp.pad = false
	pp.ended = false
	pp.stream = 0
	pp.header = pp.header[:0]
}

// SetHeader replaces the promised request header block fragment with the provided octet slice.
func (pp *PushPromise) SetHeader(h []byte) { pp.header = append(pp.header[:0], h...) }

// Write appends b to the promised header block fragment, satisfying the [io.Writer] interface.
func (pp *PushPromise) Write(b []byte) (int, error) {
	pp.header = append(pp.header, b...)
	return len(b), nil
}

// Deserialize decodes the promised stream ID, optional padding, and header fragment from fr (RFC 9113 §6.6).
func (pp *PushPromise) Deserialize(fr *FrameHeader) error {
	if fr.Stream() == 0 {
		return NewGoAwayError(ProtocolError, "PUSH_PROMISE frame must be on a specific stream, not 0")
	}

	payload := fr.payload

	if fr.Flags().Has(FlagPadded) {
		var err error

		payload, err = cutPadding(payload, fr.Len())
		if err != nil {
			return err
		}
	}

	if len(payload) < 4 {
		return NewGoAwayError(FrameSizeError, "invalid PUSH_PROMISE frame size (RFC 9113 §6.6)")
	}

	pp.stream = bytesToUint32(payload) & (1<<31 - 1)
	pp.header = append(pp.header[:0], payload[4:]...)
	pp.ended = fr.Flags().Has(FlagEndHeaders)

	return nil
}

// Serialize encodes the promised stream ID and header block fragment into the destination FrameHeader (RFC 9113 §6.6).
func (pp *PushPromise) Serialize(fr *FrameHeader) {
	fr.payload = appendUint32Bytes(fr.payload[:0], pp.stream)
	fr.payload = append(fr.payload, pp.header...)
}
```

---

## 5. Verification Method

To independently verify the findings, the implementation agent must run:

1. **Unit & Adversarial Testing**:
   ```bash
   go test -v -race ./proto/h2/...
   ```
   *Expected*: All unit, adversarial, round-trip, and fuzz tests pass with 0 race detector warnings and 0 failures.

2. **Zero-Allocation Micro-Benchmarks**:
   ```bash
   go test -bench=Benchmark -benchmem ./proto/h2
   ```
   *Expected*: `BenchmarkAcquireRelease_ConnPool_*` and `BenchmarkAcquireRelease_PerGoroutinePool_Parallel` report `0 B/op` and `0 allocs/op`.

3. **Linter & Docstring Verification**:
   ```bash
   golangci-lint run --allow-parallel-runners --no-config --enable revive ./proto/h2/...
   ```
   *Expected*: 0 issues reported in `frame_data.go`, `frame_headers.go`, `frame_control.go`, `frame_window.go`, and `frame_ext.go`.

4. **Invalidation Conditions**:
   - Any allocation regression (> 0 allocs/op) on POD frame acquisition.
   - Any compiler error caused by missing methods or altered struct layouts.
   - Any revive warning for missing docstring or malformed comment format on any exported entity.
