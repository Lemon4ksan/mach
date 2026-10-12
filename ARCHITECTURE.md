# mach Architecture Reference: Zero-Drag Protocol Mechanics

This document provides a comprehensive technical overview of the architecture, memory topology, protocol state machines, and concurrency models powering **mach v0.2.0**.

---

## 1. Architectural Philosophy: The Zero-Drag Manifesto

The `mach` engine is designed to eliminate computational drag from high-throughput network pipelines. It operates on four core principles:

1. **Agnostic Wire Core**:
   The engine transforms raw socket bytes into protocol frames and request/response structures. It contains zero application routing, middleware chains, or business logic. Higher-level frameworks ([`aoni`](https://github.com/lemon4ksan/aoni) for clients and [`sein`](https://github.com/lemon4ksan/sein) for servers) build on top of `mach`.

2. **Zero Heap Allocation on Hot Path**:
   Active request/response exchanges, frame encoding/decoding, header compression/decompression, and payload vector masking execute with **zero heap allocations**. Memory is managed through per-core pools, thread-local arenas, and scoped borrow semantics.

3. **Sans-I/O & Event-Driven State Machines**:
   Protocol parsers and state verifiers (HTTP/1.1 chunked streams, HTTP/2 RFC 9113 §5.1 stream transitions, and WebSocket framing) operate as pure state machines. They do not spawn unmanaged background goroutines and are driven synchronously by caller loops or explicit reactors.

4. **Forever-Frozen Standard**:
   Mechanics strictly adhere to official IETF standards without proprietary extensions or compromises:
   * **RFC 9112**: HTTP/1.1 Wire Protocol
   * **RFC 9113**: HTTP/2 Framing, Stream Multiplexing & Flow Control
   * **RFC 7541**: HPACK Header Compression
   * **RFC 9000 / RFC 9001 / RFC 9002**: QUIC Transport, Security & Congestion
   * **RFC 9114**: HTTP/3 over QUIC
   * **RFC 9204**: QPACK Header Compression
   * **RFC 6455**: WebSocket Protocol
   * **RFC 8441**: Bootstrapping WebSockets with HTTP/2 Extended CONNECT
   * **RFC 9220**: Bootstrapping WebSockets with HTTP/3 Extended CONNECT
   * **RFC 7838**: HTTP Alternative Services (Alt-Svc)
   * **RFC 1928 / RFC 1929**: SOCKS Protocol Version 5 & Authentication
   * **RFC 9110 §9.3.6**: HTTP CONNECT Tunneling

---

## 2. Core Architecture Topology

### 2.1 Unified Server Architecture (`server.Server`)

The `server.Server` acts as a multi-protocol L7 ingress dispatcher. It concurrently manages dual listeners:
* **TCP Listener**: Handles incoming HTTP/1.1, HTTP/2 (via TLS ALPN `"h2"`), and cleartext HTTP/2 prior knowledge (H2C via 24-byte preface detection).
* **UDP Listener**: Handles incoming QUIC sessions and HTTP/3 streams via TLS ALPN `"h3"`.

```
                    ┌──────────────────────────────────────────────┐
                    │            Unified server.Server             │
                    └──────────────────────┬───────────────────────┘
                                           │
                 ┌─────────────────────────┴─────────────────────────┐
                 │                                                   │
        TCP Listener (:443)                                 UDP Listener (:443)
                 │                                                   │
     ┌───────────┴───────────┐                               QUIC Packet Demux
     │                       │                                       │
Cleartext / H2C         TLS ALPN Handshake                    1-RTT Crypto Handshake
     │                   ┌───┴───┐                                   │
24-byte Peek             │       │                                   │
     ├── h2c ─────────┐  h1      h2                                  h3
     └── http/1.1 ──┐ │  │       │                                   │
                    ▼ ▼  ▼       ▼                                   ▼
             ┌───────────────┐ ┌───────────────┐               ┌───────────────┐
             │   server/h1   │ │   server/h2   │               │   server/h3   │
             │ Engine (2.6M) │ │ Engine (14.1M)│               │ Engine (14.2G)│
             └───────┬───────┘ └───────┬───────┘               └───────┬───────┘
                     │                 │                               │
                     └─────────────────┼───────────────────────────────┘
                                       │
                                       ▼
                       ┌───────────────────────────────┐
                       │   Per-Core Context Pool       │
                       │   Zero-Alloc Request Context  │
                       └───────────────┬───────────────┘
                                       │
                 ┌─────────────────────┴─────────────────────┐
                 │                                           │
         Is WebSocket Upgrade?                        Standard Request
                 │                                           │
     ┌───────────┴───────────┐                               │
     │                       │                               │
H1: RFC 6455            H2/H3: Extended CONNECT              │
(101 Switching)         (200 OK CONNECT)                     │
     │                       │                               │
     └───────────┬───────────┘                               │
                 ▼                                           ▼
      ┌──────────────────────┐                    ┌──────────────────────┐
      │  ws.Conn (50.3 GB/s) │                    │ Application Handler  │
      │  SWAR Vector Masking │                    │ ctx.SetBodyString()  │
      └──────────────────────┘                    └──────────────────────┘
```

#### Automatic Alt-Svc Emission
When `server.Server` is configured with an `AltSvcPort` (or when `ServeQUIC` is active), all HTTP/1.1 and HTTP/2 response headers automatically inject:
```http
Alt-Svc: h3=":443"; ma=86400
```
This signals clients to immediately upgrade subsequent requests to HTTP/3 over QUIC without manual header plumbing.

---

### 2.2 Intelligent Client Engine (`client.Engine`)

The `client.Engine` manages the entire client-side transport lifecycle:

1. **Alt-Svc Discovery & Cache**:
   Tracks alternative services per destination origin (`scheme://host:port`). If a valid, non-expired HTTP/3 alternative is cached, `Engine.Do` attempts a 0-RTT/1-RTT QUIC handshake to the UDP endpoint.
2. **Graceful Fallback & Penalty Backoff**:
   Per RFC 7838 §3.4, if the HTTP/3 endpoint drops packets or fails to respond, the engine immediately falls back to the TCP origin, marks the alternative service as failed, and imposes a 5-minute penalty backoff to protect latency.
3. **Origin Connection Pooling (`PoolManager`)**:
   Maintains a thread-safe, idle-evicting connection pool:
   * **HTTP/2**: Active connections are tracked in an active table. Multiple concurrent requests multiplex across streams on the same connection.
   * **HTTP/1.1**: Idle connections are returned to the pool for keep-alive reuse.
4. **Proxy & Tunneling Integration**:
   The engine transparently accepts custom dialers, SOCKS5 proxies (`RFC 1928`), and HTTP CONNECT tunnels (`RFC 9110`), routing TCP connections through intermediary relays.

---

## 3. High-Velocity Wire Subsystems

### 3.1 64-bit SWAR Vector Masking (`proto/ws`)

WebSocket RFC 6455 §5.3 requires all frames sent from client to server to be masked with a 32-bit masking key using modulo-4 XOR:

$$\text{transformed-octet}[i] = \text{original-octet}[i] \oplus \text{masking-key}[i \pmod 4]$$

Standard byte-by-byte scalar loops process 1 byte per cycle, bottlenecking throughput at ~2.45 GB/s. `mach/proto/ws` vectorizes this kernel using 64-bit SWAR (SIMD Within A Register):

1. The 32-bit mask key is replicated across a 64-bit register:
   ```go
   k64 := uint64(k32) | (uint64(k32) << 32)
   ```
2. If the payload offset is unaligned relative to 4 bytes, `k64` is rotated accordingly:
   ```go
   shift := (offset % 4) * 8
   k64 = (k64 >> shift) | (k64 << (64 - shift))
   ```
3. The main payload loop is unrolled 4x across 32-byte blocks (4 × `uint64`), executing 4 parallel 64-bit XOR instructions per iteration.
4. Remaining trailing bytes (<32 bytes) are processed with single 64-bit XORs and scalar fallbacks.

**Throughput Result**: Sustains **50.3 GB/s**, completely eliminating masking overhead as a bottleneck.

---

### 3.2 In-Situ Binary Overlays (`proto/h2/overlay`)

Conventional protocol decoders allocate intermediate Go structs to represent parsed frames. At millions of frames per second, heap allocation and GC scanning destroy throughput.

`mach/proto/h2/overlay` eliminates heap allocations entirely by providing zero-copy binary overlays:

```
0                   1                   2                   3
0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                 Length (24 bits)              |  Type (8)     |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   Flags (8)   |R|                 Stream ID (31 bits)         |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                     Frame Payload (0...)                    ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

* `CastFrameHeader(b []byte) FrameHeader`: Casts a 9-byte wire slice into an in-situ accessor.
* Field extractions (`Length()`, `Type()`, `Flags()`, `StreamID()`) use direct big-endian bit shifts without pointer dereferencing or struct allocations.
* **Latency**: **0.49 ns/op** (35x faster than heap struct decode, 0 B/op).

---

### 3.3 HTTP/2 Sharded Stream Table

In high-concurrency HTTP/2 servers, thousands of concurrent streams read and write simultaneously. A single global lock creates severe CPU contention across cores.

`server/h2` implements a cache-aligned, sharded stream table:
* Stream IDs are hashed into $N$ distinct shards (where $N$ is a power of 2 sized to available CPU cores).
* Each shard maintains independent spin-locks or fine-grained read/write mutexes with 64-byte padding to prevent false sharing.
* **Velocity Record**: Sustains **14.1M ops/s** under high-goroutine contention with 0 B/op memory overhead.

---

### 3.4 QUIC Zero-Copy Stream Reassembly Ring

In QUIC (RFC 9000), UDP packets frequently arrive out of order. `net/quic/core/stream` uses a high-performance circular reassembly ring:
* Fast Path: In-order packets are passed directly to the stream consumer without copying.
* Gap Buffer: Out-of-order frames are indexed by stream offset into pre-allocated memory slices.
* **Velocity Record**: **14.2 GB/s** reassembly throughput with zero GC pressure.

---

## 4. Multi-Protocol WebSocket Mechanics

`mach` provides seamless WebSocket upgrades across three distinct transport protocols:

| Transport | Upgrade Handshake | Request Semantics | Framing Protocol |
| :--- | :--- | :--- | :--- |
| **HTTP/1.1** | RFC 6455 | `GET /chat` + `Upgrade: websocket` | 101 Switching Protocols + raw socket hijack |
| **HTTP/2** | RFC 8441 | `CONNECT /chat` + `:protocol = websocket` | 200 OK + dedicated multiplexed stream |
| **HTTP/3** | RFC 9220 | `CONNECT /chat` + `:protocol = websocket` | 200 OK + dedicated QUIC stream & datagrams |

### Universal Context Interface
Regardless of whether a connection originates from HTTP/1.1, HTTP/2, or HTTP/3, the server application code remains completely uniform:

```go
if ctx.IsWebSocket() {
    wsConn, err := ctx.UpgradeWebSocket()
    if err != nil {
        return err
    }
    defer wsConn.Close()

    // Full-duplex ws.Conn session
}
```

The underlying engine handles the protocol-specific handshakes automatically.

---

## 5. Memory Architecture & Pool Topology

To maintain zero heap allocations, `mach` uses a tiered memory hierarchy:

```
┌───────────────────────────────────────────────────────────────┐
│ Level 1: Thread-Local / Per-Goroutine Ring Buffers (1.89 ns)  │
│          Instant reuse for ephemeral frame parsing            │
└───────────────────────────────┬───────────────────────────────┘
                                │ fallback
┌───────────────────────────────▼───────────────────────────────┐
│ Level 2: Per-Core Pools (foundation/pool) (4.39 ns)          │
│          Core-pinned Context, Request, and Response reuse     │
└───────────────────────────────┬───────────────────────────────┘
                                │ fallback
┌───────────────────────────────▼───────────────────────────────┐
│ Level 3: Global sync.Pool / Arena Buffers (4.57 ns)           │
│          Cross-thread payload buffers and decompression rings │
└───────────────────────────────────────────────────────────────┘
```

1. **Per-Goroutine Ring Buffers (`1.89 ns/op`)**:
   Used for frame header decoding and immediate scratch operations. Avoids cross-thread synchronization entirely.
2. **Per-Core Pools (`4.39 ns/op`)**:
   Used by `server.Server` to acquire and recycle `server.Context` instances. Sharded by CPU core ID to eliminate global lock contention.
3. **Global Arenas (`sync.Pool`)**:
   Recycle 32 KB and 64 KB byte slices for payload buffering, WebSocket masking, and streaming compression.

---

## 6. Formal RFC Compliance Mapping

| RFC | Section / Feature | `mach` Implementation Package |
| :--- | :--- | :--- |
| **RFC 9112** | HTTP/1.1 Chunked Transfer, Keep-Alive, Pipelining | `mach/proto/http`, `mach/server/h1`, `mach/client/h1` |
| **RFC 9113** | HTTP/2 Framing, Stream Multiplexing, Flow Control | `mach/proto/h2`, `mach/server/h2`, `mach/client/h2` |
| **RFC 7541** | HPACK Compression, Static & Dynamic Tables | `mach/proto/codec/hpack` |
| **RFC 9000** | QUIC Transport, Stream Multiplexing, Version Negotiation | `mach/net/quic` |
| **RFC 9001** | QUIC TLS 1.3 Key Schedule, Handshake Cryptography | `mach/net/quic/core/handshake` |
| **RFC 9002** | QUIC Loss Detection & Congestion Control | `mach/net/quic/core/stream` |
| **RFC 9114** | HTTP/3 Wire Framing, Control Streams, Datagrams | `mach/proto/h3`, `mach/server/h3`, `mach/client/h3` |
| **RFC 9204** | QPACK Compression, Encoder/Decoder Streams | `mach/proto/codec/qpack` |
| **RFC 6455** | WebSocket Wire Framing, SWAR Masking, Opcode FSM | `mach/proto/ws` |
| **RFC 8441** | Bootstrapping WebSockets with HTTP/2 Extended CONNECT | `mach/server/websocket.go`, `mach/client/websocket.go` |
| **RFC 9220** | Bootstrapping WebSockets with HTTP/3 Extended CONNECT | `mach/server/websocket.go`, `mach/client/websocket.go` |
| **RFC 7838** | HTTP Alternative Services (Alt-Svc Caching & Fallback) | `mach/client/altsvc.go`, `mach/proto/semantic/altsvc` |
| **RFC 1928** | SOCKS Protocol Version 5 (TCP CONNECT, Domain/IPv4/IPv6) | `mach/net/proxy/socks` |
| **RFC 1929** | SOCKS Version 5 Username/Password Authentication | `mach/net/proxy/socks` |
| **RFC 9110** | HTTP CONNECT Method Tunneling (§9.3.6) | `mach/net/proxy/connect.go` |

---

## 7. Verification & Fuzzing Architecture

`mach` enforces continuous mathematical and memory purity through 8 native Go fuzz targets (`scripts/fuzz_all.go`):

1. `FuzzH1RequestWire`: Mutated HTTP/1.1 wire requests, malformed header boundaries, CRLF injections.
2. `FuzzH1ResponseWire`: Truncated HTTP/1.1 response bodies, chunked transfer framing mutations.
3. `FuzzH2FrameDecode`: Malformed HTTP/2 frame headers, length overflow, invalid stream IDs.
4. `FuzzHPACKDecode`: Dynamic table overflow, malformed Huffman encodings, boundary corruption.
5. `FuzzH3FrameParse`: Varint mutations, malformed capsule datagrams, invalid HTTP/3 frame types.
6. `FuzzQPACKDecode`: Out-of-order header references, dynamic table eviction edge cases.
7. `FuzzWSWire`: Invalid WebSocket opcodes, unmasked server frame injections, fragmented control frames.
8. `FuzzSOCKS5Handshake`: Corrupted SOCKS5 negotiation vectors, invalid authentication payloads.

All fuzzers are run continuously in CI to ensure zero-panic guarantees against arbitrary network payloads.
