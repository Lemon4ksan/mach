// Package fsm implements a Sans-I/O finite state machine for HTTP/2 protocol processing conforming to RFC 9113.
//
// # Architecture
//
// The state machine completely decouples protocol logic and frame decoding from network I/O:
//
//   - Sans-I/O Engine ([StateMachine]):
//     Consumes incoming wire bytes via [StateMachine.Feed] without spawning background goroutines
//     or performing socket read/write calls.
//   - Direct Event Dispatch ([EventHandler]):
//     Emits synchronous strongly typed protocol events (OnStreamData, OnHeaders, OnSettings, OnPing,
//     OnWindowUpdate, OnStreamReset, OnGoAway) to avoid heap escapes and interface{} boxing on hot paths.
//   - In-Situ Frame Decoding:
//     Integrates directly with [overlay.Frame] zero-copy views, decoding frame headers and payload
//     slices directly from caller-supplied buffers.
//
// # RFC Specifications
//
//   - RFC 9113: HTTP/2 (Binary Framing, Stream Multiplexing, Flow Control)
//
// # Compared to the standard library
//
// Stdlib counterpart: golang.org/x/net/http2 (internal state tracking)
//
// Unlike standard library HTTP/2 parsers that bundle socket I/O into hidden internal read loops,
// this Sans-I/O state machine operates purely on byte buffers, enabling deterministic protocol testing,
// vector acceleration, and zero goroutine overhead per connection.
//
// Allocations: zero heap allocations during feed and event dispatch.
package fsm
