## 2026-09-23T05:05:20Z
<USER_REQUEST>
You are Explorer M4.2 Gen 2 (teamwork_preview_explorer).
Your working directory is: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_2_gen2
Project root: d:\CodingProjects\mach
Parent conversation ID: 5990a2d7-7ec1-47d8-9672-52a9ad7ba846

MANDATORY READING:
- Original User Request: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
- Master Plan: d:\CodingProjects\mach\.agents\PROJECT.md (§6.4 Silicon Performance)
- Challenger M4.1 Challenge Report: d:\CodingProjects\mach\.agents\teamwork_preview_challenger_m4_1\handoff.md
- Worker M4.2 Handoff: d:\CodingProjects\mach\.agents\teamwork_preview_worker_m4_2\handoff.md

YOUR MISSION (Milestone M4 Iteration 2: Per-P Pool Buffer Bounding Blueprint):
1. Investigate the buffer bloat defect identified in Challenger M4.1's report:
   - In `server/h1/conn.go:81`: `writerStorage.Put(bw)` does not check `cap(bw.B)`, allowing sporadic high-watermark payloads (e.g. 10MB) to permanently bloat Per-P shards.
   - In `server/h3/stream.go:143`: `h3BodyBufferStorage.Put(bodyBuf)` does not check `cap(bodyBuf.B)`.
2. Inspect all Per-P storage pools in `server/h1/`, `server/h2/`, `server/h3/` to ensure consistent capacity bounding:
   - Standardize rule: if buffer capacity exceeds `64*1024` (64KB), reset underlying slice to `nil` or initial capacity before returning to `Put()`.
3. Note: You are READ-ONLY EXPLORER. Do NOT modify source code directly.
4. Deliver a concrete fix blueprint with exact line diffs to `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m4_2_gen2\handoff.md`.
Notify parent via `send_message` when done.
</USER_REQUEST>
