# Dispatch Assignment — Milestone M3 Challenger 1 (Silicon Invariants & Escalation 4)

## Identity
- Role: M3 Challenger 1 (Silicon Invariants & Escalation 4)
- Type: teamwork_preview_challenger
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_challenger_m3_1
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout §5, invariants §6)
3. Worker handoff: `d:\CodingProjects\mach\.agents\teamwork_preview_worker_m3_1\handoff.md`

## Challenge Focus
1. Silicon Performance Invariants:
   - Check `client/h2/conn.go`: verify `_ cpu.CacheLinePad` is retained directly after hot atomic counters.
   - Check `client/h2/write_loop.go`: verify `ringbuf.SPSCRingBuffer` is retained for lock-free batching.
   - Run benchmarks: `$env:GOWORK="off"; go test -bench . -benchmem ./client/...`
2. Escalation 4 Verification:
   - Verify `serverWindow` in `client/h2/conn.go:NewConn` is initialized to 65,535 octets per RFC 9113 §5.2.1.
   - Run `go test -v -race -run TestH2_Tier2_FlowControlZeroWindowStalling ./tests/e2e/...`
3. Stress test client pooling and round-trip performance under multi-CPU concurrency.

## Output
Write `handoff.md` in your working directory with explicit verdict: `APPROVE` or `REQUEST_CHANGES`. Notify parent via `send_message`.

## 2026-09-22T20:06:33Z
You are M3 Challenger 1 (Silicon Invariants & Escalation 4) for Milestone M3.
Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_challenger_m3_1
Parent: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac

Read:
1. d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
2. d:\CodingProjects\mach\.agents\PROJECT.md
3. d:\CodingProjects\mach\.agents\teamwork_preview_challenger_m3_1\DISPATCH.md
4. d:\CodingProjects\mach\.agents\teamwork_preview_worker_m3_1\handoff.md

Verify:
- _ cpu.CacheLinePad retained in client/h2/conn.go
- ringbuf.SPSCRingBuffer retained in client/h2/write_loop.go
- Hot path benchmarks: $env:GOWORK="off"; go test -bench . -benchmem ./client/...
- Escalation 4 verified: serverWindow initialized to 65535 per RFC 9113 §5.2.1; run go test -v -race -run TestH2_Tier2_FlowControlZeroWindowStalling ./tests/e2e/...

Write handoff.md with verdict APPROVE or REQUEST_CHANGES and send_message to parent.

