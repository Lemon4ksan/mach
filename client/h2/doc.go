// Package h2 provides an HTTP/2 client protocol implementation featuring asymmetric isolation.
//
// Stdlib counterpart: net/http
// Rejected compromise: Heap allocations and hidden goroutines.
// Accepted cost: Manual buffer reuse.
// Allocations: zero
package h2
