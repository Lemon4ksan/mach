// Package h1 provides an HTTP/1.1 client protocol implementation featuring asymmetric isolation.
//
// Stdlib counterpart: net/http
// Rejected compromise: Heap allocations and hidden goroutines.
// Accepted cost: Manual buffer reuse.
// Allocations: zero
package h1
