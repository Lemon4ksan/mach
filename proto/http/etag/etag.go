// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etag

import (
	"github.com/lemon4ksan/mach/proto/semantic/etag"
)

// DefaultMaxEntries forwarded from proto/semantic/etag.
const DefaultMaxEntries = etag.DefaultMaxEntries

// Automaton forwarded from proto/semantic/etag.
type Automaton = etag.Automaton

var (
	// DefaultAutomaton forwarded from proto/semantic/etag.
	DefaultAutomaton = etag.DefaultAutomaton
)

// StrongMatch forwarded from proto/semantic/etag.
func StrongMatch[S ~string | ~[]byte](a, b S) bool {
	return etag.StrongMatch(a, b)
}

// WeakMatch forwarded from proto/semantic/etag.
func WeakMatch[S ~string | ~[]byte](a, b S) bool {
	return etag.WeakMatch(a, b)
}

// IsWeak forwarded from proto/semantic/etag.
func IsWeak[S ~string | ~[]byte](etagVal S) bool {
	return etag.IsWeak(etagVal)
}

// Normalize forwarded from proto/semantic/etag.
func Normalize[S ~string | ~[]byte](etagVal S) S {
	return etag.Normalize(etagVal)
}

// NewAutomaton forwarded from proto/semantic/etag.
func NewAutomaton() *Automaton {
	return etag.NewAutomaton()
}

// NewAutomatonWithCapacity forwarded from proto/semantic/etag.
func NewAutomatonWithCapacity(maxEntries int) *Automaton {
	return etag.NewAutomatonWithCapacity(maxEntries)
}
