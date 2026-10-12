// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit

import (
	"iter"
	"strconv"
	"strings"
	"time"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Directives yields an iterator traversing comma-separated key=value or flag directives from header.
// Handles optional quotes (e.g., key="val,ue") and whitespace with zero heap allocations.
// Supports both [string] and [[]byte].
//
// Specification Adherence:
// Conforms to RFC 9110 §5.6.1 (Lists of Directives) and RFC 9111 §5.2 (Cache-Control Directives).
func Directives[S ~string | ~[]byte](header S) iter.Seq2[S, S] {
	return parseDirectivesIter(header, ',')
}

// ParamDirectives yields an iterator traversing semicolon-separated key=value directives (e.g., in Alt-Svc or Content-Type)
// with zero heap allocations. Supports both [string] and [[]byte].
func ParamDirectives[S ~string | ~[]byte](header S) iter.Seq2[S, S] {
	return parseDirectivesIter(header, ';')
}

func parseDirectivesIter[S ~string | ~[]byte](header S, delim byte) iter.Seq2[S, S] {
	return func(yield func(key, value S) bool) {
		if len(header) == 0 {
			return
		}

		s := header
		for len(s) > 0 {
			// Skip leading whitespace and delimiter
			s = trimLeftDirectives(s, delim)
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
				} else if c == delim && !inQuote {
					break
				}
			}

			var token S
			if end < len(s) {
				token = s[:end]
				s = s[end+1:]
			} else {
				token = s
				s = s[:0]
			}

			token = trimSpaceDirectives(token)
			if len(token) == 0 {
				continue
			}

			k, v, ok := cutDirectives(token, '=')

			k = toLowerDirectives(trimSpaceDirectives(k))
			if ok {
				v = trimSpaceDirectives(v)
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

func trimLeftDirectives[S ~string | ~[]byte](s S, delim byte) S {
	start := 0
	for start < len(s) {
		c := s[start]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == delim {
			start++
			continue
		}

		break
	}

	return s[start:]
}

func trimSpaceDirectives[S ~string | ~[]byte](s S) S {
	start := 0
	for start < len(s) {
		c := s[start]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			start++
			continue
		}

		break
	}

	end := len(s)
	for end > start {
		c := s[end-1]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			end--
			continue
		}

		break
	}

	return s[start:end]
}

func cutDirectives[S ~string | ~[]byte](s S, sep byte) (before, after S, found bool) {
	for i := range len(s) {
		if s[i] == sep {
			return s[:i], s[i+1:], true
		}
	}

	return s, s[:0], false
}

func toLowerDirectives[S ~string | ~[]byte](s S) S {
	for i := range len(s) {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			switch v := any(s).(type) {
			case string:
				return any(strings.ToLower(v)).(S)
			case []byte:
				return any(toLowerBytesZeroAlloc(v)).(S)
			default:
				return s
			}
		}
	}

	return s
}

// DirectivesMap provides convenient typed lookups over parsed header directives.
type DirectivesMap struct {
	m map[string]string
}

// ParseDirectives eagerly parses comma-separated directives into a [DirectivesMap].
// Supports both [string] and [[]byte].
func ParseDirectives[S ~string | ~[]byte](header S) DirectivesMap {
	m := make(map[string]string)
	for k, v := range Directives(header) {
		m[string(k)] = string(v)
	}

	return DirectivesMap{m: m}
}

// ParseParamDirectives eagerly parses semicolon-separated directives into a [DirectivesMap].
// Supports both [string] and [[]byte].
func ParseParamDirectives[S ~string | ~[]byte](header S) DirectivesMap {
	m := make(map[string]string)
	for k, v := range ParamDirectives(header) {
		m[string(k)] = string(v)
	}

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

func toLowerBytesZeroAlloc(b []byte) []byte {
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			res := make([]byte, len(b))
			copy(res, b[:i])

			for j := i; j < len(b); j++ {
				ch := b[j]
				if ch >= 'A' && ch <= 'Z' {
					ch += 'a' - 'A'
				}

				res[j] = ch
			}

			return res
		}
	}

	return b
}
