// Package h3 provides HTTP/3 frame parsing and encoding.
//
// Stdlib counterpart: github.com/quic-go/qpack
// Rejected compromise: Heap allocations and hidden goroutines.
// Accepted cost: Manual buffer reuse.
// Allocations: zero
package h3
