// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"bytes"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/mach/proto/semantic/altsvc"
)

// AltSvcCache stores and tracks origin-to-alternative-service mappings per RFC 7838.
type AltSvcCache struct {
	mu      sync.RWMutex
	entries map[string]*altSvcEntry
}

type altSvcEntry struct {
	targetAddr  string
	expiresAt   time.Time
	failedUntil time.Time
}

// NewAltSvcCache creates a new thread-safe AltSvcCache.
func NewAltSvcCache() *AltSvcCache {
	return &AltSvcCache{
		entries: make(map[string]*altSvcEntry),
	}
}

// GetH3 returns the cached alternative service address for HTTP/3, if fresh and not failing.
// Optimized for zero heap allocations on the hot path (EXP-1207).
func (c *AltSvcCache) GetH3(origin string) (string, bool) {
	if c == nil {
		return "", false
	}

	c.mu.RLock()
	entry, ok := c.entries[origin]
	if !ok {
		if strings.HasPrefix(origin, "https://") {
			entry, ok = c.entries[origin[8:]]
		} else if strings.HasPrefix(origin, "http://") {
			entry, ok = c.entries[origin[7:]]
		}
	}
	if !ok && hasUpper(origin) {
		lower := strings.ToLower(origin)
		entry, ok = c.entries[lower]
		if !ok {
			if strings.HasPrefix(lower, "https://") {
				entry, ok = c.entries[lower[8:]]
			} else if strings.HasPrefix(lower, "http://") {
				entry, ok = c.entries[lower[7:]]
			}
		}
	}
	if !ok {
		c.mu.RUnlock()

		return "", false
	}

	now := time.Now()
	if now.After(entry.expiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		delete(c.entries, origin)
		if strings.HasPrefix(origin, "https://") {
			delete(c.entries, origin[8:])
		} else if strings.HasPrefix(origin, "http://") {
			delete(c.entries, origin[7:])
		}
		c.mu.Unlock()

		return "", false
	}

	if now.Before(entry.failedUntil) {
		c.mu.RUnlock()

		return "", false
	}

	addr := entry.targetAddr
	c.mu.RUnlock()

	return addr, true
}

func hasUpper(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			return true
		}
	}

	return false
}

// Set records an alternative service for the origin per RFC 7838 §3.
func (c *AltSvcCache) Set(origin string, svc altsvc.Service) {
	if c == nil {
		return
	}

	if !strings.HasPrefix(strings.ToLower(svc.ProtocolID), "h3") {
		return
	}

	canonical := canonicalOrigin(origin)
	clean := strings.TrimPrefix(canonical, "https://")
	clean = strings.TrimPrefix(clean, "http://")

	host, _, err := net.SplitHostPort(clean)
	if err != nil {
		host = clean
	}

	targetHost := svc.Host
	if targetHost == "" {
		targetHost = host
	}

	targetAddr := net.JoinHostPort(targetHost, strconv.Itoa(svc.Port))
	maxAge := svc.MaxAge
	if maxAge <= 0 {
		maxAge = altsvc.DefaultMaxAge
	}

	entry := &altSvcEntry{
		targetAddr: targetAddr,
		expiresAt:  time.Now().Add(maxAge),
	}

	c.mu.Lock()
	c.entries[canonical] = entry
	c.entries[origin] = entry
	if strings.HasPrefix(canonical, "https://") {
		c.entries[canonical[8:]] = entry
	} else if strings.HasPrefix(canonical, "http://") {
		c.entries[canonical[7:]] = entry
	}
	if strings.HasPrefix(origin, "https://") {
		c.entries[origin[8:]] = entry
	} else if strings.HasPrefix(origin, "http://") {
		c.entries[origin[7:]] = entry
	}
	if host != "" {
		c.entries[host] = entry
		c.entries["https://"+host] = entry
	}
	c.mu.Unlock()
}

// MarkFailed puts an origin's alternative service in temporary failure backoff (RFC 7838 §3.4).
func (c *AltSvcCache) MarkFailed(origin string, backoff time.Duration) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[origin]
	if !ok {
		if strings.HasPrefix(origin, "https://") {
			entry, ok = c.entries[origin[8:]]
		} else if strings.HasPrefix(origin, "http://") {
			entry, ok = c.entries[origin[7:]]
		}
	}
	if !ok {
		canonical := canonicalOrigin(origin)
		entry, ok = c.entries[canonical]
	}

	if ok {
		entry.failedUntil = time.Now().Add(backoff)
	}
}

// Clear removes all cached alternative services for the origin (e.g. upon "Alt-Svc: clear").
func (c *AltSvcCache) Clear(origin string) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, origin)
	if strings.HasPrefix(origin, "https://") {
		delete(c.entries, origin[8:])
	} else if strings.HasPrefix(origin, "http://") {
		delete(c.entries, origin[7:])
	}
	canonical := canonicalOrigin(origin)
	delete(c.entries, canonical)
	if strings.HasPrefix(canonical, "https://") {
		delete(c.entries, canonical[8:])
	}
}

// ClearAll purges all cached alternative service entries.
func (c *AltSvcCache) ClearAll() {
	if c == nil {
		return
	}

	c.mu.Lock()
	clear(c.entries)
	c.mu.Unlock()
}

// UpdateFromHeader parses the raw Alt-Svc header and updates cache entries accordingly.
func (c *AltSvcCache) UpdateFromHeader(origin string, rawHeader []byte) {
	if c == nil || len(rawHeader) == 0 {
		return
	}

	trimmed := bytes.TrimSpace(rawHeader)
	if bytes.EqualFold(trimmed, []byte("clear")) {
		c.Clear(origin)

		return
	}

	altsvc.ForEach(rawHeader, func(svc altsvc.Service) bool {
		if strings.HasPrefix(strings.ToLower(svc.ProtocolID), "h3") {
			c.Set(origin, svc)
		}

		return true
	})
}

func canonicalOrigin(origin string) string {
	origin = strings.ToLower(strings.TrimSpace(origin))
	if strings.HasPrefix(origin, "https://") || strings.HasPrefix(origin, "http://") {
		return origin
	}

	if _, _, err := net.SplitHostPort(origin); err == nil {
		return "https://" + origin
	}

	return "https://" + net.JoinHostPort(origin, "443")
}
