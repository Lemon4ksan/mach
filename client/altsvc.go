// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"bytes"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/lemon4ksan/foundation/generic"

	"github.com/lemon4ksan/mach/proto/semantic/altsvc"
)

// AltSvcCache stores and tracks origin-to-alternative-service mappings per RFC 7838.
type AltSvcCache struct {
	entries generic.SyncMap[string, *altSvcEntry]
}

type altSvcEntry struct {
	targetAddr  string
	expiresAt   time.Time
	failedUntil time.Time
}

// NewAltSvcCache creates a new thread-safe AltSvcCache.
func NewAltSvcCache() *AltSvcCache {
	return &AltSvcCache{}
}

// GetH3 returns the cached alternative service address for HTTP/3, if fresh and not failing.
// Optimized for zero heap allocations on the hot path (EXP-1207).
func (c *AltSvcCache) GetH3(origin string) (string, bool) {
	if c == nil {
		return "", false
	}

	entry, ok := c.entries.Load(origin)
	if !ok {
		if strings.HasPrefix(origin, "https://") {
			entry, ok = c.entries.Load(origin[8:])
		} else if strings.HasPrefix(origin, "http://") {
			entry, ok = c.entries.Load(origin[7:])
		}
	}

	if !ok && hasUpper(origin) {
		lower := strings.ToLower(origin)

		entry, ok = c.entries.Load(lower)
		if !ok {
			if strings.HasPrefix(lower, "https://") {
				entry, ok = c.entries.Load(lower[8:])
			} else if strings.HasPrefix(lower, "http://") {
				entry, ok = c.entries.Load(lower[7:])
			}
		}
	}

	if !ok {
		return "", false
	}

	now := time.Now()
	if now.After(entry.expiresAt) {
		c.entries.Delete(origin)

		if strings.HasPrefix(origin, "https://") {
			c.entries.Delete(origin[8:])
		} else if strings.HasPrefix(origin, "http://") {
			c.entries.Delete(origin[7:])
		}

		return "", false
	}

	if now.Before(entry.failedUntil) {
		return "", false
	}

	return entry.targetAddr, true
}

// GetH3Optional returns the cached alternative service address for HTTP/3 wrapped in an Optional[string].
// Preserves zero heap allocations on the hot path (EXP-1207).
func (c *AltSvcCache) GetH3Optional(origin string) generic.Optional[string] {
	addr, ok := c.GetH3(origin)
	return generic.From(addr, ok)
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

	c.entries.Store(canonical, entry)
	c.entries.Store(origin, entry)

	if strings.HasPrefix(canonical, "https://") {
		c.entries.Store(canonical[8:], entry)
	} else if strings.HasPrefix(canonical, "http://") {
		c.entries.Store(canonical[7:], entry)
	}

	if strings.HasPrefix(origin, "https://") {
		c.entries.Store(origin[8:], entry)
	} else if strings.HasPrefix(origin, "http://") {
		c.entries.Store(origin[7:], entry)
	}

	if host != "" {
		c.entries.Store(host, entry)
		c.entries.Store("https://"+host, entry)
	}
}

// MarkFailed puts an origin's alternative service in temporary failure backoff (RFC 7838 §3.4).
func (c *AltSvcCache) MarkFailed(origin string, backoff time.Duration) {
	if c == nil {
		return
	}

	entry, ok := c.entries.Load(origin)
	if !ok {
		if strings.HasPrefix(origin, "https://") {
			entry, ok = c.entries.Load(origin[8:])
		} else if strings.HasPrefix(origin, "http://") {
			entry, ok = c.entries.Load(origin[7:])
		}
	}

	if !ok {
		canonical := canonicalOrigin(origin)
		entry, ok = c.entries.Load(canonical)
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

	c.entries.Delete(origin)

	if strings.HasPrefix(origin, "https://") {
		c.entries.Delete(origin[8:])
	} else if strings.HasPrefix(origin, "http://") {
		c.entries.Delete(origin[7:])
	}

	canonical := canonicalOrigin(origin)
	c.entries.Delete(canonical)

	if strings.HasPrefix(canonical, "https://") {
		c.entries.Delete(canonical[8:])
	}
}

// ClearAll purges all cached alternative service entries.
func (c *AltSvcCache) ClearAll() {
	if c == nil {
		return
	}

	c.entries.Clear()
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
