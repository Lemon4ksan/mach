// Package h2 provides a finite state machine for HTTP/2 connections.
//
// Stdlib counterpart: golang.org/x/net/http2
// Rejected compromise: Heap allocations and hidden goroutines.
// Accepted cost: Manual buffer reuse.
// Allocations: zero
package h2
