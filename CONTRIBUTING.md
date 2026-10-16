# Contributing to mach

Thank you for considering a contribution! `mach` is the aerodynamic, silicon-speed protocol engine powering the `aoni` and `sein` ecosystems. We welcome bug fixes, performance optimizations, and RFC conformance refinements.

## Ground Rules

- **Zero-Allocation Hot Paths.** Wire frame processing, binary overlays, and SWAR masking operations must maintain 0 B/op and 0 allocs/op.
- **Strict Standards Compliance.** All protocol codecs and state machines must strictly adhere to IETF RFCs (RFC 9110, RFC 9112, RFC 9113, RFC 9114, RFC 6455, RFC 8441, RFC 9220).
- **No Application Business Logic.** `mach` is a protocol substrate, not an application framework. Keep higher-level constructs in `aoni` or `sein`.
- **Backward Compatibility.** Public API contracts are permanent. Breaking changes require substantial architectural rationale and major version increments.

## Getting Started

```bash
git clone https://github.com/lemon4ksan/mach
cd mach
go mod download
make test   # fast unit tests
make race   # test with race detector
make lint   # golangci-lint
```

## Workflow

1. **Fork** the repository and create a feature branch from `main`.
2. Write code following the existing zero-allocation patterns and `.golangci.yml`.
3. Add unit, regression, or fuzz tests. Verify that `make test`, `make race`, and `make lint` pass with 0 errors.
4. Submit a Pull Request targeting `main` using the provided PR template.

## Commit Style

Use [Conventional Commits](https://www.conventionalcommits.org/):

```text
feat(h2): implement dynamic stream window prioritization
fix(quic): prevent out-of-order frame truncation in reassembly ring
perf(ws): vectorize SWAR masking loop for AVX-512 register widths
docs(doc.go): document Alt-Svc connection promotion state machine
```

## Tests & Benchmarks

| Command | Purpose |
|---|---|
| `make test` | Fast unit tests across all protocol packages |
| `make race` | Tests with Go data race detector |
| `make bench` | Silicon wire throughput and allocation microbenchmarks |
| `make fuzz` | Continuous wire parser and state machine fuzzing |
| `make lint` | Run `golangci-lint` verification |
| `make format` | Code formatting and BSD-3-Clause license attribution |

## Questions

For architecture discussions and protocol RFC clarifications, please open a [GitHub Discussion](https://github.com/lemon4ksan/mach/discussions).
