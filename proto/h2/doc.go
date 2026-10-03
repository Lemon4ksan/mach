// Package h2 provides HTTP/2 frame parsing and encoding.
//
// Stdlib counterpart: golang.org/x/net/http2
// Rejected compromise: Heap allocations and hidden goroutines.
// Accepted cost: Manual buffer reuse.
// Allocations: zero
package h2
