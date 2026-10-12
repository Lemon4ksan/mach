// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etag

import (
	"bytes"
	"io"
	"net/http"
	"sync"

	"github.com/lemon4ksan/foundation/generic"
)

// StrongMatch checks whether two ETags match under strong comparison semantics (RFC 9110 §8.8.2.1, obsoleting RFC 7232 §2.3.2).
// Both must not be weak, and the opaque tags must be identical.
func StrongMatch[S ~string | ~[]byte](a, b S) bool {
	aTrim := trimSpace(a)
	bTrim := trimSpace(b)

	if len(aTrim) == 0 || len(bTrim) == 0 || IsWeak(aTrim) || IsWeak(bTrim) {
		return false
	}

	return equalS(aTrim, bTrim)
}

// WeakMatch checks whether two ETags match under weak comparison semantics (RFC 9110 §8.8.2.2, obsoleting RFC 7232 §2.3.2).
// Opaque-tags must match regardless of weak "W/" indicator.
func WeakMatch[S ~string | ~[]byte](a, b S) bool {
	aNorm := Normalize(a)
	bNorm := Normalize(b)

	if len(aNorm) == 0 || len(bNorm) == 0 {
		return false
	}

	return equalS(aNorm, bNorm)
}

// IsWeak reports whether etagVal is a weak entity tag (starts with "W/" or "w/") per RFC 9110 §8.8.1.
func IsWeak[S ~string | ~[]byte](etagVal S) bool {
	trimmed := trimSpace(etagVal)
	return hasPrefixFold(trimmed, "W/\"")
}

// Normalize strips whitespace and any leading "W/" or "w/" weak prefix without heap allocations.
func Normalize[S ~string | ~[]byte](etagVal S) S {
	s := trimSpace(etagVal)
	if hasPrefixFold(s, "W/\"") {
		return s[2:]
	}

	return s
}

func trimSpace[S ~string | ~[]byte](s S) S {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r' || s[start] == '\n') {
		start++
	}

	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r' || s[end-1] == '\n') {
		end--
	}

	return s[start:end]
}

func hasPrefixFold[S ~string | ~[]byte](s S, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}

	if s[0] != 'W' && s[0] != 'w' {
		return false
	}

	for i := 1; i < len(prefix); i++ {
		if s[i] != prefix[i] {
			return false
		}
	}

	return true
}

func equalS[S ~string | ~[]byte](a, b S) bool {
	return string(a) == string(b)
}

type cachedETagEntry struct {
	etag   string
	status string
	proto  string
	header http.Header
	body   []byte
}

// DefaultMaxEntries specifies the default maximum cached entries in an Automaton instance (1024).
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

// GetETagOptional returns the stored ETag for key wrapped in an Optional[string].
func (a *Automaton) GetETagOptional(key string) generic.Optional[string] {
	etag := a.GetETag(key)
	return generic.From(etag, etag != "")
}

// Reconstruct304Optional returns an Optional wrapping the reconstructed http.Response if key is cached.
func (a *Automaton) Reconstruct304Optional(key string) generic.Optional[*http.Response] {
	resp := a.Reconstruct304(key)
	return generic.FromPtr(resp)
}
