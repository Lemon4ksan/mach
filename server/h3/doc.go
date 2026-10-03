// Package h3 provides a zero-allocation HTTP/3 server protocol engine.
//
// By strictly decoupling L4 connection management from L7 application logic, the engine enforces asymmetric isolation: it protects the server from malformed client traffic without imposing business-logic constraints on the handler.
//
// Stdlib counterpart: net/http
// Rejected compromise: Heap allocations and hidden goroutines.
// Accepted cost: Manual buffer reuse.
// Allocations: 28 (per connection)
package h3
