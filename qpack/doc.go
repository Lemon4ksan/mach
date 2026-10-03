// Package qpack provides HTTP/3 header compression (RFC 9204).
//
// Stdlib counterpart: golang.org/x/net/http3/qpack
// Rejected compromise: Heap allocations and hidden goroutines.
// Accepted cost: Manual buffer reuse.
// Allocations: zero
package qpack
