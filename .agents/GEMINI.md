---
name: mach-guidelines
description: Rules and boundaries for AI agents working on mach
trigger: always_on
---

# Working on mach

`mach` is the L7 protocol engine built on top of `foundation`. It provides high-performance, zero-allocation protocol codecs (HTTP/1, HTTP/2, HTTP/3, HPACK, QPACK) for both client and server.

## Scope

- Stay inside the packages the task names.
- Zero-allocation hot paths for request/response processing. Connection setup may allocate, but processing must be zero-drag.
- Asymmetric isolation: L7 server handler logic must be fully isolated from L4/L7 malformed traffic and network congestion.
- No business logic or application models. `mach` provides the protocol building blocks, not the application itself (that belongs in `sein` or `aoni`).

## Documentation

`doc.go`, `example_test.go`, and `alloc_test.go` are the canonical package documentation. When you change behavior, update `doc.go` and allocation tests.

## Quality

- Lint, race tests, and allocation budgets (`alloc_test.go`) must pass.
- Everything is tested with `-race`; any race warning is a bug.
- All files are in English.
