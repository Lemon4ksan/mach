# Dispatch Assignment — Milestone M3 Challenger 2 (Concurrency, Escalation 3 & Fuzzing)

## Identity
- Role: M3 Challenger 2 (Concurrency, Escalation 3 & Fuzzing)
- Type: teamwork_preview_challenger
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_challenger_m3_2
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout §5, invariants §6)
3. Worker handoff: `d:\CodingProjects\mach\.agents\teamwork_preview_worker_m3_1\handoff.md`

## Challenge Focus
1. Concurrency & Race Detector Gate:
   - Run `$env:GOWORK="off"; go test -v -race ./client/...` (MUST report 0 race detector warnings across all 4 packages).
   - Run `$env:GOWORK="off"; go test -v -race -timeout 120s ./tests/e2e/...` (62/62 pass, 0 warnings).
2. Escalation 3 Verification (Data Race on `ctx.StreamID`):
   - Run 10 consecutive iterations under race detector:
     `$env:GOWORK="off"; go test -race -count=10 -run "TestH2_Tier1_StreamCancellationRST|TestH2_Tier3_MultiplexingWithConcurrentResets" ./tests/e2e/...`
   - MUST pass all 10 iterations with 0 data race warnings.
3. Heavy Protocol Fuzzing:
   - Run `$env:GOWORK="off"; go run ./scripts/fuzz_all.go -fuzztime=5s` (all 8 targets MUST pass with 0 panics and 0 errors).

## Output
Write `handoff.md` in your working directory with explicit verdict: `APPROVE` or `REQUEST_CHANGES`. Notify parent via `send_message`.

## 2026-09-22T20:06:33Z
You are M3 Challenger 2 (Concurrency, Escalation 3 & Fuzzing) for Milestone M3.
Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_challenger_m3_2
Parent: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac

