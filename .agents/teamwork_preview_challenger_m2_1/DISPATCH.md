# Dispatch Assignment — Milestone M2 Challenger 1 (Zero-Alloc & Silicon Benchmarks)

## Identity
- Role: M2 Challenger 1 (Zero-Alloc & Silicon Benchmarks)
- Type: teamwork_preview_challenger
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_challenger_m2_1
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
Read these files before starting challenge:
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout, and invariants)
3. Worker handoff: `d:\CodingProjects\mach\.agents\teamwork_preview_worker_m2_3\handoff.md`

## Challenge Focus
1. Empirically verify zero-allocation silicon performance on hot paths:
   - `BenchmarkFullPipeline_ScopedBorrow` (MUST be 0 B/op, 0 allocs/op)
   - `BenchmarkPool_PerPStorage_Parallel` (MUST be 0 B/op, 0 allocs/op)
   - `BenchmarkBorrow_Scoped` (MUST be 0 B/op, 0 allocs/op)
   - `BenchmarkCookie_Scoped` (MUST be 0 B/op, 0 allocs/op)
   - `BenchmarkURI_Scoped` (MUST be 0 B/op, 0 allocs/op)
2. Execute:
   `go test -bench="BenchmarkFullPipeline_ScopedBorrow|BenchmarkPool_PerPStorage_Parallel|BenchmarkBorrow_Scoped|BenchmarkCookie_Scoped|BenchmarkURI_Scoped" -benchmem -run="^$" ./proto/http/...`
3. Stress test allocation behavior under parallel execution (`-cpu 1,2,4,8`).
4. Ensure no hidden allocations or performance degradations occurred.

## Output
Write `handoff.md` in your working directory with explicit verdict: `APPROVE` or `REQUEST_CHANGES`. Notify parent via `send_message`.
