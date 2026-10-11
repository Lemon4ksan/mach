// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import "strings"

// ForEachChallengeParam scans auth-param key-value pairs from an authentication challenge header (RFC 9110 §11.2 & RFC 7616 §3.3).
// It returns false if the scheme was not found.
func ForEachChallengeParam[S ~string | ~[]byte](header S, scheme string, yield func(key, val string) bool) bool {
	h := stringOrBytes(header)
	h = strings.TrimSpace(h)
	schemeLower := strings.ToLower(scheme)

	idx := strings.Index(strings.ToLower(h), schemeLower)
	if idx < 0 {
		return false
	}

	if idx > 0 {
		prev := h[idx-1]
		if prev != ' ' && prev != ',' && prev != '\t' {
			return false
		}
	}

	rest := h[idx+len(scheme):]
	if len(rest) > 0 && rest[0] != ' ' && rest[0] != '\t' && rest[0] != ',' && rest[0] != '=' {
		return false
	}

	rest = strings.TrimSpace(rest)

	for len(rest) > 0 {
		rest = strings.TrimLeft(rest, " ,\t")
		if len(rest) == 0 {
			break
		}

		eqIdx := strings.IndexByte(rest, '=')
		if eqIdx < 0 {
			break
		}

		spaceIdx := strings.IndexByte(rest, ' ')
		if spaceIdx >= 0 && spaceIdx < eqIdx {
			break
		}

		key := strings.ToLower(strings.TrimSpace(rest[:eqIdx]))
		rest = strings.TrimSpace(rest[eqIdx+1:])

		var val string
		if len(rest) > 0 && rest[0] == '"' {
			rest = rest[1:]
			var sb strings.Builder
			escaped := false
			foundQuote := false

			for i, c := range rest {
				if escaped {
					sb.WriteRune(c)
					escaped = false
					continue
				}

				if c == '\\' {
					escaped = true
					continue
				}

				if c == '"' {
					rest = rest[i+1:]
					foundQuote = true
					break
				}

				sb.WriteRune(c)
			}

			if !foundQuote {
				rest = ""
			}

			val = sb.String()
		} else {
			commaIdx := strings.IndexByte(rest, ',')
			spaceIdx2 := strings.IndexByte(rest, ' ')

			endIdx := commaIdx
			if spaceIdx2 >= 0 && (endIdx < 0 || spaceIdx2 < endIdx) {
				endIdx = spaceIdx2
			}

			if endIdx < 0 {
				val = strings.TrimSpace(rest)
				rest = ""
			} else {
				val = strings.TrimSpace(rest[:endIdx])
				rest = rest[endIdx:]
			}
		}

		if !yield(key, val) {
			return true
		}
	}

	return true
}

// ExtractChallengeParams parses auth-param key-value pairs into a map from an authentication challenge header (RFC 9110 §11.2 & RFC 7616 §3.3).
func ExtractChallengeParams(header, scheme string) (map[string]string, bool) {
	m := make(map[string]string)
	ok := ForEachChallengeParam(header, scheme, func(key, val string) bool {
		m[key] = val
		return true
	})
	if !ok {
		return nil, false
	}
	return m, true
}
