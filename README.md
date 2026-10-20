<img src="assets/logo.svg" align="right" width="128" height="128" alt="mach logo" />

# mach

_«Aerodynamic protocol mechanics at Mach speed — where zero heap allocation meets wire throughput»_

[![Go Version](https://img.shields.io/badge/go-1.27%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/mach)
[![Architecture](https://img.shields.io/badge/arch-Zero--Alloc%20%7C%20Sans--I/O-6e56cf?style=flat-square)](#rules)
[![Protocols](https://img.shields.io/badge/L7-H1%20%7C%20H2%20%7C%20H3%20%7C%20QUIC%20%7C%20WS%20%7C%20SSE-0284c7?style=flat-square)](#layout)
[![Ecosystem](https://img.shields.io/badge/ecosystem-foundation-0284c7?style=flat-square)](https://github.com/lemon4ksan/foundation)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)

**[English](README.md)** | **[Русский](README_RU.md)**

Zero-allocation Layer 7 protocol engine and transport stack for Go 1.27+. Powers the [`aoni`](https://github.com/lemon4ksan/aoni) client stack and [`sein`](https://github.com/lemon4ksan/sein) server framework, built on top of [`foundation`](https://github.com/lemon4ksan/foundation).

## Design tradeoffs

| Standard library / `net/http` | What `mach` does instead | What you pay |
| :--- | :--- | :--- |
| Allocates a new `http.Request`, `http.Header` map, and slice headers on every turn | Reuses pooled `Request` / `Response` envelopes (`proto/http`) and flat contiguous `headkit.HeaderMap` arrays with 64-bit token interning (`0 B/op`) | Request/response objects and header slices are valid only until `ReleaseRequest` / `ReleaseResponse` or the end of the handler turn |
| Copies HTTP/2 frame headers and payloads into heap-allocated Go structs | Casts `proto/h2/overlay.RawFrame` directly over the socket read buffer (`0.49 ns/op`) and validates stream transitions via a table-driven FSM | Borrowed frame buffers cannot outlive the current read iteration without explicit copying |
| Couples protocol framing to blocking `net.Conn` reads and per-stream goroutines | Separates wire parsers and state machines (`proto/h1`, `proto/h2`, `proto/h3`, `net/quic/core`) from socket I/O | Callers driving low-level `proto/*` packages directly manage their own read/write buffer loops |
| Separate servers and libraries for HTTP/1.1, HTTP/2, H2C, HTTP/3, and WebSockets | Single `server.Server` and `client.Engine` multiplexing TCP (`H1`, `H2`, `H2C`) and UDP (`H3`/`QUIC`), with RFC 7838 `Alt-Svc` promotion and RFC 6455/8441/9220 WebSockets | Configuration surface is more explicit about buffer sizes, flow-control windows, and pool limits |

## Rules

1. **Zero-allocation hot path.** Steady-state framing, header indexing (`HPACK` / `QPACK`), WebSocket masking, QUIC stream reassembly, and semantic header parsing must execute with `0 allocs/op`.
2. **Cache-line discipline.** Hot protocol structs (`h2/fsm.Stream`, `quic/core/stream.Stream`, `headkit.HeaderMap`) are aligned and padded to 64-byte boundaries to prevent false sharing across CPU cores.
3. **Sans-I/O state machines.** Wire framing (`proto/*`, `net/quic/core/*`) is pure state-machine logic with no hidden goroutines or timers; network schedulers in `client/` and `server/` own socket lifecycles.
4. **Strict RFC state enforcement.** Protocol violations (invalid H2 stream transitions, malformed QUIC varints, non-canonical HPACK Huffman padding, forbidden pseudo-headers) fail immediately with typed protocol errors. See [ARCHITECTURE.md](ARCHITECTURE.md) for the full RFC matrix.

## Layout

### Layer 0 — Wire & Semantic Primitives (`proto/http`, `proto/headkit`, `proto/semantic`, `proto/codec`)
- [`proto/http`](proto/http) — Pooled `Request` and `Response` envelopes, method/status tokens, and zero-copy body readers.
- [`proto/headkit`](proto/headkit) — Flat contiguous header storage, 64-bit ASCII token interning, **HPACK** ([RFC 7541](https://datatracker.ietf.org/doc/html/rfc7541)) static/dynamic tables + Huffman codec, and **QPACK** ([RFC 9204](https://datatracker.ietf.org/doc/html/rfc9204)) encoder/decoder streams.
- [`proto/semantic`](proto/semantic) — Zero-allocation parsers for [RFC 9110](https://datatracker.ietf.org/doc/html/rfc9110) / [RFC 6265](https://datatracker.ietf.org/doc/html/rfc6265) / [RFC 7838](https://datatracker.ietf.org/doc/html/rfc7838) / [RFC 9218](https://datatracker.ietf.org/doc/html/rfc9218) headers: `altsvc`, `auth`, `contentdisposition`, `cookie`, `digest`, `etag`, `hpkp`, `priority`, `status`, and `uri`.
- [`proto/codec`](proto/codec) — Pooled streaming compression (`gzip`, `brotli`, `deflate`, `zstd`) with `Accept-Encoding` negotiation.

### Layer 1 — Protocol Framing & State Machines (`proto/h1`, `proto/h2`, `proto/h3`, `proto/ws`, `proto/sse`, `proto/grpcweb`)
- [`proto/h1`](proto/h1) — [RFC 9112](https://datatracker.ietf.org/doc/html/rfc9112) HTTP/1.1 wire parser/serializer, chunked transfer encoding, keep-alive management, and 16-byte SIMD delimiter scanning.
- [`proto/h2`](proto/h2) — [RFC 9113](https://datatracker.ietf.org/doc/html/rfc9113) HTTP/2 framing, flow-control windows, priority trees, table-driven stream FSM ([`proto/h2/fsm`](proto/h2/fsm)), and in-situ binary header overlay ([`proto/h2/overlay`](proto/h2/overlay)).
- [`proto/h3`](proto/h3) — [RFC 9114](https://datatracker.ietf.org/doc/html/rfc9114) HTTP/3 framing (`DATA`, `HEADERS`, `SETTINGS`, `GOAWAY`), unidirectional control streams, capsules, and HTTP Datagrams ([RFC 9297](https://datatracker.ietf.org/doc/html/rfc9297)).
- [`proto/ws`](proto/ws) — Unified WebSocket framing across HTTP/1.1 (`Upgrade`, [RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455)), HTTP/2 Extended `CONNECT` (`:protocol = websocket`, [RFC 8441](https://datatracker.ietf.org/doc/html/rfc8441)), and HTTP/3 Extended `CONNECT` ([RFC 9220](https://datatracker.ietf.org/doc/html/rfc9220)), with 64-bit SWAR XOR masking.
- [`proto/sse`](proto/sse) & [`proto/grpcweb`](proto/grpcweb) — W3C Server-Sent Events parser/emitter and gRPC-Web length-prefixed framing + trailer encoding.

### Layer 2 — Transport & Tunneling (`net/quic`, `net/proxy`)
- [`net/quic`](net/quic) — User-space QUIC v1/v2 transport ([RFC 9000](https://datatracker.ietf.org/doc/html/rfc9000), [RFC 9001](https://datatracker.ietf.org/doc/html/rfc9001), [RFC 9002](https://datatracker.ietf.org/doc/html/rfc9002), [RFC 9369](https://datatracker.ietf.org/doc/html/rfc9369)): 2-bit variable-length integers ([`varint`](net/quic/varint)), long/short header packet codecs ([`core/wire`](net/quic/core/wire)), TLS 1.3 handshake state machine ([`core/handshake`](net/quic/core/handshake)), and out-of-order stream reassembly ring buffer ([`core/stream`](net/quic/core/stream)).
- [`net/proxy`](net/proxy) — Zero-allocation **SOCKS5** ([RFC 1928](https://datatracker.ietf.org/doc/html/rfc1928) / [RFC 1929](https://datatracker.ietf.org/doc/html/rfc1929)) and **HTTP `CONNECT`** ([RFC 9110 §9.3.6](https://datatracker.ietf.org/doc/html/rfc9110#section-9.3.6)) tunnel dialers with `NO_PROXY` CIDR/domain matching.

### Layer 3 — Client & Server Engines (`client`, `server`)
- [`client`](client) — Multi-protocol `Engine` (`H1`, `H2`, `H3`) with per-origin `PoolManager`, automatic `Alt-Svc` HTTP/3 discovery and promotion, and unified WebSocket dialing (`DialWebSocket`).
- [`server`](server) — Unified `Server` binding TCP (`HTTP/1.1`, `HTTP/2`, `H2C`) and UDP (`HTTP/3`), ALPN + 24-byte connection preface demultiplexing, pooled `Context`, automatic `Alt-Svc` advertisement, and transparent `UpgradeWebSocket(ctx)`.

## Installation

```bash
go get github.com/lemon4ksan/mach
```

## Usage

### Unified HTTP/1.1, HTTP/2, H2C & HTTP/3 Server

```go
package main

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/server"
)

func main() {
	cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")

	srv := server.NewServer(
		server.WithAddr(":8443"),
		server.WithTLSConfig(&tls.Config{Certificates: []tls.Certificate{cert}}),
		server.WithEnableH2(true),
		server.WithEnableH2C(true),
		server.WithEnableH3(true), // Binds UDP :8443 and injects Alt-Svc: h3=":8443"
		server.WithReadTimeout(10*time.Second),
		server.WithHandler(server.HandlerFunc(func(ctx *server.Context) error {
			if ctx.IsWebSocketUpgrade() {
				wsConn, err := server.UpgradeWebSocket(ctx)
				if err != nil {
					return err
				}
				defer wsConn.Close()
				return wsConn.WriteText([]byte("connected over " + ctx.Protocol()))
			}

			ctx.SetHeader("X-Proto", ctx.Protocol())
			return ctx.JSON(http.StatusOK, []byte(`{"status":"ok"}`))
		})),
	)

	_ = srv.ListenAndServe(context.Background())
}
```

### Multi-Protocol Client Engine with `Alt-Svc` Promotion

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/lemon4ksan/mach/client"
	"github.com/lemon4ksan/mach/proto/http"
)

func main() {
	eng := client.NewEngine(&client.EngineConfig{
		MaxConnsPerHost:     64,
		IdleConnTimeout:     90 * time.Second,
		EnableHTTP2:         true,
		EnableHTTP3:         true,
		EnableAltSvc:        true,
		HealthCheckInterval: 15 * time.Second,
	})
	defer eng.Close()

	req := http.AcquireRequest()
	defer http.ReleaseRequest(req)

	req.SetMethod(http.MethodGet)
	req.SetURI("https://example.com/api/v1/telemetry")
	req.Headers().Set("Accept", "application/json")

	resp, err := eng.Do(context.Background(), req)
	if err != nil {
		panic(err)
	}
	defer http.ReleaseResponse(resp)

	fmt.Printf("status=%d proto=%s\n", resp.StatusCode(), resp.Protocol())
}
```

### In-Situ HTTP/2 Frame Overlay (`proto/h2/overlay`)

```go
package main

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/h2/overlay"
)

func inspectFrame(wireBuf []byte) {
	frame, ok := overlay.AsRawFrame(wireBuf)
	if !ok {
		return
	}
	fmt.Printf("type=%d flags=0x%02x stream=%d len=%d\n",
		frame.Type(), frame.Flags(), frame.StreamID(), frame.Length())
}
```

## Benchmarks

Measured on AMD Ryzen 7 5800X (8C/16T, `windows/amd64`, Go 1.27):

| Subsystem | Benchmark | Latency (`ns/op`) | Throughput | Allocations |
| :--- | :--- | ---: | ---: | ---: |
| `proto/h2/overlay` | `RawFrame` in-situ 9-byte decode | `0.49 ns` | `18,452 MB/s` | `0 B/op (0 allocs)` |
| `proto/h2/fsm` | Stream FSM transition (`Open` $\to$ `Closed`) | `3.59 ns` | — | `0 B/op (0 allocs)` |
| `proto/ws` | 64-bit SWAR 4KB frame XOR masking | `81.45 ns` | `50,287 MB/s` | `0 B/op (0 allocs)` |
| `net/quic/varint` | 2-bit prefix Varint encode + decode | `3.56 ns` | — | `0 B/op (0 allocs)` |
| `net/quic/core/stream` | Out-of-order 4KB ring buffer reassembly | `288.2 ns` | `14,211 MB/s` | `0 B/op (0 allocs)` |
| `net/quic/core/wire` | Short header 1-RTT packet codec | `19.65 ns` | — | `0 B/op (0 allocs)` |
| `proto/headkit` | `HeaderMap` set / get / token lookup | `21.84 ns` | — | `0 B/op (0 allocs)` |
| `proto/headkit/hpack` | HPACK indexed + literal header block | `142.3 ns` | — | `0 B/op (0 allocs)` |
| `proto/semantic/etag` | Weak/strong `ETag` conditional match | `5.86 ns` | — | `0 B/op (0 allocs)` |
| `proto/semantic/uri` | Zero-alloc RFC 3986 URI split | `18.30 ns` | — | `0 B/op (0 allocs)` |
| `proto/h1` | Request + response roundtrip parse | `312.0 ns` | — | `0 B/op (0 allocs)` |
| `net/proxy` | SOCKS5 + HTTP `CONNECT` tunnel handshake | `18.42 ns` | — | `0 B/op (0 allocs)` |

## Verification & Fuzzing

```bash
# Run unit and integration test suite with race detector
go test -race ./...

# Verify zero-allocation invariants across all packages
go test -bench=. -benchmem ./...

# Run native Go fuzzing targets (H1, H2, H3, HPACK, QPACK, QUIC wire, WS, Semantic)
go run ./scripts/fuzz_all.go -fuzztime=30s
```

## License

BSD 3-Clause License. See [LICENSE](LICENSE) for full text.
