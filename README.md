# mach

Silicon-speed protocol engines and transport layers for Go 1.27+. `mach` serves as the brutal, zero-allocation network core that powers both the **`aoni`** client and **`sein`** server frameworks. 

It handles the heaviest bytes-to-frames mechanics: protocol parsing, stream multiplexing, HPACK/QPACK compression, cryptography, and QUIC congestion control.

```bash
go get github.com/lemon4ksan/mach
```

## Architecture: The Zero-Drag Manifesto

The name **mach** (from the Mach number) represents the speed of sound and the aerodynamic concept of *Zero Drag*. In networking, this translates to zero heap allocations, zero blocking, and zero garbage collection overhead on the hot path. 

Unlike standard `net/http` or monolithic libraries, `mach` implements a strict separation of protocol mechanics from business logic:
* **Agnostic Core**: The engine knows nothing about routing, middleware, or application logic. It accepts raw bytes from a socket and yields protocol frames.
* **Forever-Frozen Standard**: The core mechanics are rigidly locked to IETF RFCs (RFC 9000, RFC 7540, RFC 9114, RFC 9204). No product-specific hacks are allowed at this layer.
* **Symmetric Isolation**: Client and server architectures often require fundamentally different memory layouts and optimizations. `mach` isolates them explicitly (e.g., `mach/h1` vs `mach/server/h1`).

## Protocol Index

```text
mach/
├── quic/                 # IETF QUIC (RFC 9000). Zero-alloc congestion control, AEAD payload encryption.
├── qpack/                # HTTP/3 QPACK (RFC 9204) encoder/decoder. Static table lookups.
├── h1/                   # HTTP/1.1 Client Engine. Pipelining, chunked transfers.
├── h2/                   # HTTP/2 Client Engine (RFC 7540). Stream multiplexing, HPACK (RFC 7541).
├── h3/                   # HTTP/3 Client Engine (RFC 9114). UDP-based zero-RTT handshakes.
└── server/               # Server-optimized protocol implementations for the `sein` framework
    ├── h1/               # HTTP/1.1 Server Engine
    ├── h2/               # HTTP/2 Server Engine
    └── h3/               # HTTP/3 Server Engine
```

## Performance & Optimization Rules

1. **Strict Zero-Allocation**: During a stable connection life-cycle, the engine must make exactly `0 allocs/op`. All memory is recycled via multi-tiered arenas and `sync.Pool`.
2. **Cache-Line Alignment**: Protocol structs (like `Request`, `Response`, `Frame`) must be packed and padded to 64-byte boundaries to prevent false sharing across CPU cores.
3. **No Hidden Goroutines**: The engine avoids spawning hidden background workers per connection. State machines should be driven by the caller's execution thread or an explicit reactor pool.

## Testing & Benchmarking

Any modifications to the core engine must prove zero performance degradation.

Run the test suite with race detection:
```bash
go test -race ./...
```

Verify zero-allocation constraints on the hot path:
```bash
go test -bench=Benchmark -benchmem ./...
```

## License

Licensed under the **BSD 3-Clause License**. See [LICENSE](LICENSE) for details.