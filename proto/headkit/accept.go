// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit

import (
	"iter"
	"slices"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// AcceptItem represents a quality-weighted content-negotiation token (RFC 9110 §12.4.2 & §12.5).
type AcceptItem struct {
	Value  string
	Q      float32
	Params DirectivesMap
}

// Accepts yields an iterator traversing quality-weighted items from an Accept, Accept-Encoding,
// or Accept-Language header string with zero heap allocations per token.
func Accepts(header string) iter.Seq[AcceptItem] {
	return func(yield func(item AcceptItem) bool) {
		if header == "" {
			return
		}

		for _, rawToken := range Directives(header) {
			_ = rawToken // unused in key/val split
		}

		// Direct iteration over comma-separated chunks
		s := header
		for len(s) > 0 {
			s = strings.TrimLeft(s, " ,\t\r\n")
			if len(s) == 0 {
				break
			}

			var end int
			inQuote := false

			for end = 0; end < len(s); end++ {
				c := s[end]
				if c == '"' {
					inQuote = !inQuote
				} else if c == ',' && !inQuote {
					break
				}
			}

			var token string
			if end < len(s) {
				token = s[:end]
				s = s[end+1:]
			} else {
				token = s
				s = ""
			}

			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}

			item := parseAcceptToken(token)
			if !yield(item) {
				return
			}
		}
	}
}

func parseAcceptToken(token string) AcceptItem {
	parts := strings.Split(token, ";")
	val := strings.ToLower(strings.TrimSpace(parts[0]))
	q := float32(1.0)
	var params map[string]string

	for i := 1; i < len(parts); i++ {
		p := strings.TrimSpace(parts[i])
		if p == "" {
			continue
		}

		k, v, ok := strings.Cut(p, "=")
		k = strings.ToLower(strings.TrimSpace(k))
		if ok {
			v = strings.TrimSpace(v)
			if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
				v = v[1 : len(v)-1]
			}
		}

		if k == "q" {
			if parsedQ, err := strconv.ParseFloat(v, 32); err == nil {
				if parsedQ >= 0 && parsedQ <= 1.0 {
					q = float32(parsedQ)
				}
			}
		} else {
			if params == nil {
				params = make(map[string]string)
			}
			params[k] = v
		}
	}

	return AcceptItem{
		Value:  val,
		Q:      q,
		Params: DirectivesMap{m: params},
	}
}

// SortedAccepts parses header and returns items sorted by quality Q in descending order (highest preference first).
func SortedAccepts(header string) []AcceptItem {
	var items []AcceptItem
	for item := range Accepts(header) {
		if item.Q > 0 { // RFC 9110 §12.4.2: q=0 means unacceptable
			items = append(items, item)
		}
	}

	slices.SortStableFunc(items, func(a, b AcceptItem) int {
		if b.Q > a.Q {
			return 1
		} else if b.Q < a.Q {
			return -1
		}
		return 0
	})

	return items
}

// Negotiate selects the best matching offer from offers according to the client's Accept header (RFC 9110 §12.5).
// Supports exact matches, wildcard matching (*/* or type/*), and respects q=0 (not acceptable).
// Returns an empty string if no acceptable match is found.
func Negotiate(acceptHeader string, offers []string) string {
	if len(offers) == 0 {
		return ""
	}

	if acceptHeader == "" || acceptHeader == "*/*" {
		return offers[0]
	}

	sorted := SortedAccepts(acceptHeader)
	if len(sorted) == 0 {
		return offers[0]
	}

	for _, pref := range sorted {
		prefVal := pref.Value
		if prefVal == "*/*" || prefVal == "*" {
			return offers[0]
		}

		// Exact match
		for _, offer := range offers {
			if bytesconv.EqualFoldASCII(offer, prefVal) {
				return offer
			}
		}

		// Wildcard match (e.g. "text/*")
		if strings.HasSuffix(prefVal, "/*") {
			prefix := prefVal[:len(prefVal)-1] // "text/"
			for _, offer := range offers {
				if strings.HasPrefix(strings.ToLower(offer), prefix) {
					return offer
				}
			}
		}
	}

	return ""
}
