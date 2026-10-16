// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cachestatus implements the Cache-Status HTTP Response Header Field (RFC 9211).
//
// It provides zero-allocation parsing and serialization of Structured Fields cache metadata,
// enabling proxy caches and L7 gateways to inspect and report cache hit/miss semantics
// without heap overhead.
package cachestatus
