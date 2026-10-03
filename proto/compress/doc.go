// Package compress provides HTTP compression algorithms.
//
// Stdlib counterpart: compress/gzip
// Rejected compromise: Heap allocations and hidden goroutines.
// Accepted cost: Manual buffer reuse.
// Allocations: zero
package compress
