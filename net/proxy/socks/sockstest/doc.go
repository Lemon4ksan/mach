// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sockstest provides utilities for SOCKS testing.
//
// It includes a local mock server and helpers to parse or synthesize SOCKS wire frames.
// Use this package to simulate proxy authentication, connection acceptance, and protocol
// errors without spawning a real proxy daemon.
//
// # Compared to the standard library
//
// Stdlib counterpart: golang.org/x/net/proxy (which lacks dedicated test harnesses)
//
// Rejected compromise: none (pure testing utility).
//
// Accepted cost: the fake server binds actual localhost sockets which may flake in constrained environments.
//
// Allocations: unconstrained
package sockstest
