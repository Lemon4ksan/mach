// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package etag implements RFC 7232, RFC 9110, and RFC 9111 HTTP Entity Tags, conditional request matching, and 304 response reconstruction.
package etag

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/lemon4ksan/foundation/generic"
)

// StrongMatch checks whether two ETags match under strong comparison semantics (RFC 7232 §2.3.2).
// Both must not be weak, and the opaque tags must be identical.
func StrongMatch(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)

	if a == "" || b == "" || IsWeak(a) || IsWeak(b) {
		return false
	}

	return a == b
}

// WeakMatch checks whether two ETags match under weak comparison semantics (RFC 7232 §2.3.2).
// Opaque-tags must match regardless of weak "W/" indicator.
func WeakMatch(a, b string) bool {
	a = Normalize(a)
	b = Normalize(b)

	if a == "" || b == "" {
		return false
	}

	return a == b
}

// IsWeak reports whether etagVal is a weak entity tag (starts with "W/" or "w/").
func IsWeak(etagVal string) bool {
	trimmed := strings.TrimSpace(etagVal)
	return strings.HasPrefix(trimmed, "W/\"") || strings.HasPrefix(trimmed, "w/\"")
}

// Normalize strips whitespace and any leading "W/" or "w/" weak prefix.
func Normalize(etagVal string) string {
	s := strings.TrimSpace(etagVal)
	if strings.HasPrefix(s, "W/\"") || strings.HasPrefix(s, "w/\"") {
		return s[2:]
	}

	return s
}

type cachedETagEntry struct {
	etag   string
	status string
	proto  string
	header http.Header
	body   []byte
}

const DefaultMaxEntries = 1024

// Automaton manages ETag recording, If-None-Match header injection, and 304 body reconstruction.
type Automaton struct {
	mu         sync.RWMutex
	maxEntries int
	entries    map[string]cachedETagEntry
}

// NewAutomaton creates a new RFC 9111 [Automaton] instance with default capacity (1024).
func NewAutomaton() *Automaton {
	return NewAutomatonWithCapacity(DefaultMaxEntries)
}

// NewAutomatonWithCapacity creates a new RFC 9111 [Automaton] with the specified capacity limit.
func NewAutomatonWithCapacity(maxEntries int) *Automaton {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}

	return &Automaton{
		maxEntries: maxEntries,
		entries:    make(map[string]cachedETagEntry, maxEntries),
	}
}

// DefaultAutomaton is the package-level shared ETag automaton instance.
var DefaultAutomaton = NewAutomaton()

// Record stores the ETag and response payload bytes for the specified cache key.
func (a *Automaton) Record(key, etagVal string, resp *http.Response, bodyBytes []byte) {
	if etagVal == "" || len(bodyBytes) == 0 {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Bound memory: evict an entry if capacity reached
	if len(a.entries) >= a.maxEntries {
		for k := range a.entries {
			delete(a.entries, k)
			break
		}
	}

	a.entries[key] = cachedETagEntry{
		etag:   etagVal,
		status: resp.Status,
		proto:  resp.Proto,
		header: resp.Header.Clone(),
		body:   bodyBytes,
	}
}

// GetETag returns the stored ETag for key, or empty string if not found.
func (a *Automaton) GetETag(key string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.entries[key].etag
}

// Reconstruct304 returns a 200 OK http.Response populated with the previously cached payload bytes for key.
// Returns nil if key is not found in the automaton cache.
func (a *Automaton) Reconstruct304(key string) *http.Response {
	a.mu.RLock()
	entry, ok := a.entries[key]
	a.mu.RUnlock()

	if !ok {
		return nil
	}

	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Proto:         entry.proto,
		Header:        entry.header.Clone(),
		Body:          io.NopCloser(bytes.NewReader(entry.body)),
		ContentLength: int64(len(entry.body)),
	}
}

// GetETagOptional returns the stored ETag wrapped in generic.Optional.
func (a *Automaton) GetETagOptional(key string) generic.Optional[string] {
	a.mu.RLock()
	defer a.mu.RUnlock()

	entry, ok := a.entries[key]
	if !ok || entry.etag == "" {
		return generic.None[string]()
	}

	return generic.Some(entry.etag)
}

// Reconstruct304Optional returns a reconstructed 200 OK http.Response wrapped in generic.Optional.
//
//nolint:bodyclose
func (a *Automaton) Reconstruct304Optional(key string) generic.Optional[*http.Response] {
	resp := a.Reconstruct304(key)
	if resp == nil {
		return generic.None[*http.Response]()
	}

	return generic.Some(resp)
}
