// Package hpack provides HTTP/2 header compression (RFC 7541).
//
// Stdlib counterpart: golang.org/x/net/http2/hpack
// Rejected compromise: Heap allocations and hidden goroutines.
// Accepted cost: Manual buffer reuse.
// Allocations: zero
package hpack
