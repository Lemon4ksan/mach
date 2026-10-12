// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sockstest implements test harnesses, mock servers, and frame synthesizers for SOCKS protocols.
//
// # Architecture
//
// In-memory and loopback servers simulate proxy authentication workflows, connection negotiations,
// and wire-level protocol failures without requiring external proxy daemons:
//
//   - Mock Server Infrastructure:
//     Binds local loopback listeners to exercise client handshakes against arbitrary authentication configurations.
//   - Frame Synthesis:
//     Generates conforming and deliberately corrupted SOCKS wire frames for negative testing and fuzz validation.
//
// # RFC Specifications
//
//   - RFC 1928: SOCKS Protocol Version 5
//   - RFC 1929: Username/Password Authentication for SOCKS V5
package sockstest
