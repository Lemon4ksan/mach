# Handoff Report — Milestone M4: Silicon Performance & Zero-Allocation Empirical Challenge

**Challenger**: teamwork_preview_challenger_m4_1  
**Target Milestone**: M4 (Server Protocol Engine Decomposition & Silicon Invariants)  
**Parent**: 5990a2d7-7ec1-47d8-9672-52a9ad7ba846  
**Date**: 2026-09-23T05:01:00Z  
**Verdict**: **CHALLENGE**  

---

## 1. Observation

### 1.1 Micro-Benchmark Suite Results
Executing the required project micro-benchmark command:
```powershell
$env:GOWORK="off"; go test -run 'NONE' -bench '.' -benchmem ./server/...
```
Produced the following verbatim empirical output:
```text
goos: windows
goarch: amd64
pkg: github.com/lemon4ksan/mach/server/h1
cpu: 12th Gen Intel(R) Core(TM) i5-12400F
BenchmarkRequest_ReadRequest-12               	 2791315	       493.7 ns/op	      48 B/op	       1 allocs/op
BenchmarkResponse_WriteTo-12                  	14529181	        98.31 ns/op	       0 B/op	       0 allocs/op
BenchmarkConnHandler_ServeConn_Pipeline-12    	  278235	      4292 ns/op	       2 B/op	       1 allocs/op
PASS
ok  	github.com/lemon4ksan/mach/server/h1	5.078s

goos: windows
goarch: amd64
pkg: github.com/lemon4ksan/mach/server/h2
cpu: 12th Gen Intel(R) Core(TM) i5-12400F
BenchmarkServerConn_WriteResponse-12    	 2394936	       518.6 ns/op	     516 B/op	       2 allocs/op
PASS
ok  	github.com/lemon4ksan/mach/server/h2	1.858s

goos: windows
goarch: amd64
pkg: github.com/lemon4ksan/mach/server/h3
cpu: 12th Gen Intel(R) Core(TM) i5-12400F
BenchmarkQPACK_EncodeResponseHeaders-12    	  358659	      2876 ns/op	    1736 B/op	      29 allocs/op
BenchmarkQPACK_DecodeRequestHeaders-12     	  576493	      2310 ns/op	    1070 B/op	       9 allocs/op
BenchmarkH3_FrameHeaderPack-12             	233186658	         6.060 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	github.com/lemon4ksan/mach/server/h3	3.965s
```

### 1.2 Core Silicon Invariants from PROJECT.md §6.4
Executing verification across core hot paths:
```powershell
$env:GOWORK="off"; go test -run 'NONE' -bench 'BenchmarkInSituOverlay|BenchmarkH3_FrameHeaderPack|BenchmarkFullPipeline_ScopedBorrow|BenchmarkAcquireRelease_PerGoroutinePool_Parallel|BenchmarkPool_PerPStorage_Parallel' -benchmem ./...
```
Output:
- `BenchmarkInSituOverlay-12`: `1,000,000,000` ops, `0.8094 ns/op`, `0 B/op`, `0 allocs/op`.
- `BenchmarkAcquireRelease_PerGoroutinePool_Parallel-12`: `559,397,788` ops, `2.109 ns/op`, `0 B/op`, `0 allocs/op`.
- `BenchmarkPool_PerPStorage_Parallel-12`: `210,215,749` ops, `5.502 ns/op`, `0 B/op`, `0 allocs/op`.
- `BenchmarkFullPipeline_ScopedBorrow-12`: `8,065,034` ops, `141.8 ns/op`, `0 B/op`, `0 allocs/op`.
- `BenchmarkH3_FrameHeaderPack-12`: `405,261,236` ops, `3.450 ns/op`, `0 B/op`, `0 allocs/op`.

### 1.3 `_ cpu.CacheLinePad` SMP Alignment
Inspecting `server/h2/server_conn.go:90-95`:
```go
	_ cpu.CacheLinePad

	isClosed       atomic.Bool
	isReleased     atomic.Bool
	connSendWindow atomic.Int32
	closeErr       error
```
`_ cpu.CacheLinePad` is retained on `ServerConn` in `server/h2/`, correctly isolating atomic counters against SMP false sharing.

### 1.4 Memory Profiling of HTTP/2 Response Serialization
Running `go tool pprof` on `BenchmarkServerConn_WriteResponse` (`-memprofile mem.out`):
```text
Showing nodes accounting for 894.43MB, 99.22% of 901.46MB total
      flat  flat%   sum%        cum   cum%
  887.43MB 98.44% 98.44%   887.43MB 98.44%  github.com/lemon4ksan/foundation/net/hpack.appendString
       7MB  0.78% 99.22%        7MB  0.78%  internal/strconv.FormatInt
         0     0% 99.22%   887.43MB 98.44%  github.com/lemon4ksan/foundation/net/hpack.(*HPACK).AppendHeader
         0     0% 99.22%   887.43MB 98.44%  github.com/lemon4ksan/mach/proto/h2.(*Headers).AppendHeaderField
         0     0% 99.22%   894.43MB 99.22%  github.com/lemon4ksan/mach/proto/h2.SerializeResponseHeaders
         0     0% 99.22%   897.44MB 99.55%  github.com/lemon4ksan/mach/server/h2.(*ServerConn).writeResponse
```
Inspection of `proto/h2/utils.go:204-210`:
```go
func SerializeResponseHeaders(dst *Headers, hp *hpack.HPACK, statusCode int, headers http.Header, bodyLen int) {
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	hf.SetKeyBytes(StringStatus)
	hf.SetValue(strconv.Itoa(statusCode))
	dst.AppendHeaderField(hp, hf, false)
...
```
1. `strconv.Itoa(statusCode)`: Calls `FormatInt`, which unconditionally allocates a new string on the heap for any value >= 100 (including all standard HTTP status codes: 200, 201, 404, etc.).
2. In `dst.AppendHeaderField(hp, hf, false)`: `dst` (`*coreh2.Headers`) acquired from `framePools[FrameHeaders]` (`proto/h2/frame.go:106`) has `rawHeaders = nil`. `hp.AppendHeader` appends to `nil`, triggering a 512-byte heap allocation inside `hpack.appendString`, followed by another reallocating copy in `h.SetHeaders(dst)`.

### 1.5 Buffer Pool Retention & Bounding
Inspecting `server/h1/conn.go:26, 81-83`:
```go
var writerStorage = pool.NewPerPStorage(func() *bytesconv.ByteBuffer {
	return &bytesconv.ByteBuffer{}
})
...
defer func() {
	if !isHijacked {
		_ = bw.Flush()
		_ = conn.Close()

		br.Reset(nil)
		readerStorage.Put(br)

		bw.Reset()
		writerStorage.Put(bw)
	}
}()
```
And inspecting `server/h3/stream.go:109-111, 143-144`:
```go
h3BodyBufferStorage = pool.NewPerPStorage(func() *bytesconv.ByteBuffer {
	return &bytesconv.ByteBuffer{}
})
...
defer func() {
	bodyBuf.Reset()
	h3BodyBufferStorage.Put(bodyBuf)
}()
```
`bytesconv.ByteBuffer.Reset()` does not shrink `b.B` (`b.B = b.B[:0]`).
In `PerPStorage`, pooled objects are held in a fixed array `items [32]T` per CPU core and are **never** evicted by GC.
When a large payload (e.g. 10MB–64MB) passes through `writerStorage` (H1) or `h3BodyBufferStorage` (H3), the multi-megabyte capacity remains permanently retained in that CPU core's shard, unbounded. (Notice that in contrast, `Request.Body` and `Response.Body` in `server/h1` and `ServerRequest.Body` in `server/h3` explicitly enforce `if cap(...) > 64*1024` bounding).

---

## 2. Logic Chain

1. **Failure of Zero-Allocation Requirement on HTTP/2 Response Writing**:
   - The mission explicitly mandates: *"Verify that response writing and framing achieve 0 B/op, 0 allocs/op."*
   - Observation 1.1 and Observation 1.4 prove that `server/h2` response writing (`sc.writeResponse`) achieves **516 B/op and 2 allocs/op**, directly violating this invariant.
   - Tracing through the pprof profile:
     - Allocation 1: `strconv.Itoa(statusCode)` in `proto/h2/utils.go:209` (7 MB cumulative).
     - Allocation 2: `hpack.appendString` in `foundation/net/hpack/hpack.go:742` via `dst.AppendHeaderField` (887.43 MB cumulative).
   - Therefore, `server/h2` response writing does not meet the silicon zero-allocation standard.

2. **Unbounded Buffer Retention in Per-P Pools**:
   - The mission explicitly mandates: *"Verify buffer recycling: ensure buffers do not grow without bounds and Per-P sharded pools are properly utilized."*
   - In `server/h1/conn.go`, `Request` and `Response` bodies are guarded against buffer growth via:
     ```go
     if cap(req.Body) > 64*1024 { req.Body = make([]byte, 0, 1024) }
     ```
   - However, `bw` in `writerStorage` (`server/h1/conn.go:81`) and `bodyBuf` in `h3BodyBufferStorage` (`server/h3/stream.go:143`) do not contain any capacity bounding check before returning to their respective `PerPStorage` pools.
   - Because `PerPStorage` items reside permanently in off-heap or non-GC-evicted shard arrays, high-watermark payloads permanently bloat server resident memory.

3. **Validation of Concurrency and Race Safety**:
   - `go test -race -count=1 ./tests/e2e/...` passes 100% (62/62 tests).
   - `go test -race -count=1 ./server/...` passes 100% across all unit, fuzz, and adversarial tests.
   - Cache line padding on `ServerConn` is confirmed intact.
   - The code is structurally sound and race-free, but fails on allocation and memory-bounding invariants.

---

## 3. Caveats

- **Test Fixture Overhead**: In `server/h1/h1_bench_test.go:33` (`BenchmarkRequest_ReadRequest`), the measured `48 B/op, 1 allocs/op` is due to `bytes.NewReader(raw)` inside the benchmark loop, not the parser itself.
- **QPACK Codec Allocations**: `BenchmarkQPACK_EncodeResponseHeaders` and `DecodeRequestHeaders` reflect the current upstream QPACK codec design in `proto/h3`, which is outside `server/h3` decomposition scope.

---

## 4. Conclusion

### **VERDICT: CHALLENGE**

Milestone M4 cannot be approved in its current state due to two concrete empirical performance defects:

1. **Defect 1 (CRITICAL — Zero-Allocation Violation in `server/h2`)**:
   `BenchmarkServerConn_WriteResponse` executes with **516 B/op and 2 allocs/op**.
   - **Fix Required**:
     1. In `proto/h2/utils.go:209`: Replace `strconv.Itoa(statusCode)` with a zero-allocation status string lookup table or `status.Text(statusCode)`/byte conversion.
     2. In `proto/h2/frame.go:106` or `frame_headers.go`: Ensure `Headers` acquired from pool has pre-allocated `rawHeaders` buffer (e.g. `make([]byte, 0, 512)` in `Reset()` or pool creator) or pass a pre-allocated buffer into `SerializeResponseHeaders` to eliminate slice reallocation in `hpack.appendString`.

2. **Defect 2 (HIGH — Unbounded Buffer Bloat in Per-P Pools)**:
   - In `server/h1/conn.go:81`: Bound `bw.B` before pool return:
     ```go
     if cap(bw.B) > 64*1024 {
         bw.B = nil
     }
     bw.Reset()
     writerStorage.Put(bw)
     ```
   - In `server/h3/stream.go:143`: Bound `bodyBuf.B` before pool return:
     ```go
     if cap(bodyBuf.B) > 64*1024 {
         bodyBuf.B = nil
     }
     bodyBuf.Reset()
     h3BodyBufferStorage.Put(bodyBuf)
     ```

---

## 5. Verification Method

To independently reproduce all observations and verify the challenge findings, run the following commands from the project root (`d:\CodingProjects\mach`):

1. **Execute Server Package Micro-Benchmarks**:
   ```powershell
   $env:GOWORK="off"; go test -run 'NONE' -bench '.' -benchmem ./server/...
   ```
   *Expected Observation*: `BenchmarkServerConn_WriteResponse-12` in `server/h2` shows `516 B/op, 2 allocs/op`.

2. **Inspect HTTP/2 Allocation Profile**:
   ```powershell
   $env:GOWORK="off"; go test -run 'NONE' -bench 'BenchmarkServerConn_WriteResponse' -benchmem -memprofile mem.out ./server/h2/...
   go tool pprof -alloc_space -text mem.out
   ```
   *Expected Observation*: 98.44% of allocations in `hpack.appendString` and 0.78% in `strconv.FormatInt`.

3. **Verify Unbounded Buffer Pools via Code Inspection**:
   - Inspect `server/h1/conn.go:81-83` (`writerStorage.Put(bw)` lacks `cap` check).
   - Inspect `server/h3/stream.go:143-145` (`h3BodyBufferStorage.Put(bodyBuf)` lacks `cap` check).
