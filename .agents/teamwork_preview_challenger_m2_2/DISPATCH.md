# Dispatch Assignment — Milestone M2 Challenger 2 (Fuzzing, Race Safety & LLHTTP)

## Identity
- Role: M2 Challenger 2 (Fuzzing, Race Safety & LLHTTP)
- Type: teamwork_preview_challenger
- Working Directory: d:\CodingProjects\mach\.agents\teamwork_preview_challenger_m2_2
- Parent Conversation ID: 6e20ed8f-fd2a-4c96-9fa7-c568bce992ac (Orchestrator Gen 3)
- Project Root: d:\CodingProjects\mach

## Mandatory Reference Documents
Read these files before starting challenge:
1. `d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md` (authoritative user requirements)
2. `d:\CodingProjects\mach\.agents\PROJECT.md` (master architecture, layout, and invariants)
3. Worker handoff: `d:\CodingProjects\mach\.agents\teamwork_preview_worker_m2_3\handoff.md`

## Challenge Focus
1. Concurrency & Race Safety:
   - Run `go test -v -race -timeout 90s ./proto/http/...` (MUST report 0 race detector warnings).
   - Run `go test -v -race -timeout 120s ./tests/e2e/...` (MUST report 0 warnings, 62/62 pass).
2. Wire Protocol Vector Correctness:
   - Run `TestLLHTTP_Chunked_OfficialVectors` in `proto/http` and confirm all 12/12 vectors pass.
3. Heavy Protocol Fuzzing:
   - Run `go run ./scripts/fuzz_all.go -fuzztime=5s` (all 8 targets MUST pass with 0 panics and 0 errors).

## Output
Write `handoff.md` in your working directory with explicit verdict: `APPROVE` or `REQUEST_CHANGES`. Notify parent via `send_message`.
