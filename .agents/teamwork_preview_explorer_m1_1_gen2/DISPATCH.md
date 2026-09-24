## 2026-09-22T14:27:43Z
You are Explorer 1 for Milestone M1 (Core Protocol Frame Decomposition) of the mach refactoring project.
Working directory: d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m1_1_gen2
Original user request file: d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md
Master Project Plan: d:\CodingProjects\mach\.agents\PROJECT.md

Instructions:
1. You MUST read d:\CodingProjects\mach\.agents\ORIGINAL_REQUEST.md and d:\CodingProjects\mach\.agents\PROJECT.md.
2. Read and analyze `proto/h2/frames.go`, `proto/h2/frame.go`, `proto/h2/header.go`, `proto/h2/frame_pool.go`, and test files.
3. Catalog all types, methods, functions, and constants in `frames.go`.
4. Design the modular split into `frame_data.go`, `frame_headers.go`, `frame_control.go`, `frame_window.go`, and `frame_ext.go`.
5. Catalog all 87 docstring violations in `proto/h2/frames.go` and draft compliant RFC 9113 docstrings with section citations.
6. Ensure zero-allocation frame pooling invariants (ConnectionFramePool off-heap slab allocator vs sync.Pool) are upheld.
7. Write handoff report to `d:\CodingProjects\mach\.agents\teamwork_preview_explorer_m1_1_gen2\handoff.md`, update progress.md, and notify parent.
