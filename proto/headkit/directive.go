// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit

import (
	"bytes"
	"iter"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Directives yields an iterator traversing comma-separated key=value or flag directives from header.
// Handles optional quotes (e.g., key="val,ue") and whitespace with zero heap allocations.
//
// Specification Adherence:
// Conforms to RFC 9110 §5.6.1 (Lists of Directives) and RFC 9111 §5.2 (Cache-Control Directives).
func Directives(header string) iter.Seq2[string, string] {
	return func(yield func(key, value string) bool) {
		if header == "" {
			return
		}

		s := header
		for len(s) > 0 {
			// Skip leading whitespace and commas
			s = strings.TrimLeft(s, " ,\t\r\n")
			if len(s) == 0 {
				break
			}

			// Find next delimiter taking quotes into account
			var end int
			inQuote := false

			for end = 0; end < len(s); end++ {
				c := s[end]
				if inQuote && c == '\\' && end+1 < len(s) {
					end++
					continue
				}
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

			k, v, ok := strings.Cut(token, "=")
			k = strings.ToLower(strings.TrimSpace(k))
			if ok {
				v = strings.TrimSpace(v)
				if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
					v = v[1 : len(v)-1]
				}
			}

			if !yield(k, v) {
				return
			}
		}
	}
}

// DirectivesBytes yields an iterator traversing comma-separated key=value directives from byte slice b
// with zero heap allocations.
func DirectivesBytes(b []byte) iter.Seq2[[]byte, []byte] {
	return func(yield func(key, value []byte) bool) {
		if len(b) == 0 {
			return
		}

		s := b
		for len(s) > 0 {
			s = bytes.TrimLeft(s, " ,\t\r\n")
			if len(s) == 0 {
				break
			}

			var end int
			inQuote := false

			for end = 0; end < len(s); end++ {
				c := s[end]
				if inQuote && c == '\\' && end+1 < len(s) {
					end++
					continue
				}
				if c == '"' {
					inQuote = !inQuote
				} else if c == ',' && !inQuote {
					break
				}
			}

			var token []byte
			if end < len(s) {
				token = s[:end]
				s = s[end+1:]
			} else {
				token = s
				s = nil
			}

			token = bytes.TrimSpace(token)
			if len(token) == 0 {
				continue
			}

			k, v, ok := bytes.Cut(token, []byte{'='})
			k = bytes.ToLower(bytes.TrimSpace(k))
			if ok {
				v = bytes.TrimSpace(v)
				if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
					v = v[1 : len(v)-1]
				}
			}

			if !yield(k, v) {
				return
			}
		}
	}
}

// ParamDirectives yields an iterator traversing semicolon-separated key=value directives (e.g., in Alt-Svc or Content-Type)
// with zero heap allocations.
func ParamDirectives(header string) iter.Seq2[string, string] {
	return func(yield func(key, value string) bool) {
		if header == "" {
			return
		}

		s := header
		for len(s) > 0 {
			s = strings.TrimLeft(s, " ;\t\r\n")
			if len(s) == 0 {
				break
			}

			var end int
			inQuote := false

			for end = 0; end < len(s); end++ {
				c := s[end]
				if inQuote && c == '\\' && end+1 < len(s) {
					end++
					continue
				}
				if c == '"' {
					inQuote = !inQuote
				} else if c == ';' && !inQuote {
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

			k, v, ok := strings.Cut(token, "=")
			k = strings.ToLower(strings.TrimSpace(k))
			if ok {
				v = strings.TrimSpace(v)
				if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
					v = v[1 : len(v)-1]
				}
			}

			if !yield(k, v) {
				return
			}
		}
	}
}

// DirectivesMap provides convenient typed lookups over parsed header directives.
type DirectivesMap struct {
	m map[string]string
}

// ParseDirectives eagerly parses comma-separated directives into a [DirectivesMap].
func ParseDirectives(header string) DirectivesMap {
	m := maps.Collect(Directives(header))

	return DirectivesMap{m: m}
}

// ParseParamDirectives eagerly parses semicolon-separated directives into a [DirectivesMap].
func ParseParamDirectives(header string) DirectivesMap {
	m := maps.Collect(ParamDirectives(header))

	return DirectivesMap{m: m}
}

// Has reports whether directive key is present.
func (d DirectivesMap) Has(key string) bool {
	if d.m == nil {
		return false
	}

	_, ok := d.m[strings.ToLower(key)]
	return ok
}

// Get returns the value associated with key, or an empty string if missing.
func (d DirectivesMap) Get(key string) string {
	if d.m == nil {
		return ""
	}

	return d.m[strings.ToLower(key)]
}

// Int returns the integer value for key, or def if missing or unparseable.
func (d DirectivesMap) Int(key string, def int) int {
	val := d.Get(key)
	if val == "" {
		return def
	}

	if parsed, err := strconv.Atoi(val); err == nil {
		return parsed
	}

	return def
}

// Duration returns the duration value in seconds for key (RFC 9111 delta-seconds), or def if missing.
func (d DirectivesMap) Duration(key string, def time.Duration) time.Duration {
	val := d.Get(key)
	if val == "" {
		return def
	}

	if secs, err := strconv.ParseInt(val, 10, 64); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}

	return def
}

// Bool returns true if key is present and not explicitly "false" or "0".
func (d DirectivesMap) Bool(key string) bool {
	if !d.Has(key) {
		return false
	}

	v := d.Get(key)
	if v == "" || v == "1" || bytesconv.EqualFoldASCII(v, "true") {
		return true
	}

	return false
}

// Len returns the number of directives.
func (d DirectivesMap) Len() int {
	return len(d.m)
}

// All returns an iterator over all directives.
func (d DirectivesMap) All() iter.Seq2[string, string] {
	return func(yield func(key, value string) bool) {
		for k, v := range d.m {
			if !yield(k, v) {
				return
			}
		}
	}
}
