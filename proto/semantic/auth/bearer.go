// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"strings"
)

// Standard OAuth 2.0 Bearer error codes in WWW-Authenticate headers (RFC 6750 §3.1 & §6.2).
const (
	// ErrBearerInvalidRequest indicates the request is missing a required parameter, includes an
	// unsupported parameter, or is otherwise malformed (HTTP 400 Bad Request, RFC 6750 §3.1).
	ErrBearerInvalidRequest = "invalid_request"

	// ErrBearerInvalidToken indicates the access token provided is expired, revoked, or malformed
	// (HTTP 401 Unauthorized, RFC 6750 §3.1).
	ErrBearerInvalidToken = "invalid_token"

	// ErrBearerInsufficientScope indicates the request requires higher privileges than provided
	// by the access token (HTTP 403 Forbidden, RFC 6750 §3.1).
	ErrBearerInsufficientScope = "insufficient_scope"
)

// BearerChallenge represents a parsed HTTP Bearer authentication challenge from a WWW-Authenticate header (RFC 6750 §3).
// The zero value represents an empty challenge.
type BearerChallenge struct {
	// Realm indicates the scope of protection (RFC 6750 §3).
	Realm string
	// Scope indicates the scope of the access request (RFC 6750 §3).
	Scope string
	// Error indicates the error code if token validation failed (RFC 6750 §3.1).
	Error string
	// ErrorDescription provides human-readable explanation of the error (RFC 6750 §3).
	ErrorDescription string
	// ErrorURI points to human-readable documentation for the error (RFC 6750 §3).
	ErrorURI string
}

// AppendFormat appends the Challenge as a standard WWW-Authenticate header value to dst without heap allocations (RFC 6750 §3).
func (c BearerChallenge) AppendFormat(dst []byte) []byte {
	dst = append(dst, "Bearer"...)
	first := true
	appendParam := func(k, v string) {
		if v == "" {
			return
		}
		if first {
			dst = append(dst, ' ')
			first = false
		} else {
			dst = append(dst, ',', ' ')
		}
		dst = append(dst, k...)
		dst = append(dst, '=', '"')
		dst = append(dst, v...)
		dst = append(dst, '"')
	}

	appendParam("realm", c.Realm)
	appendParam("scope", c.Scope)
	appendParam("error", c.Error)
	appendParam("error_description", c.ErrorDescription)
	appendParam("error_uri", c.ErrorURI)

	return dst
}

// String formats the Challenge as a standard WWW-Authenticate header value (RFC 6750 §3).
func (c BearerChallenge) String() string {
	var buf [256]byte
	return string(c.AppendFormat(buf[:0]))
}

// AppendBearer appends an RFC 6750 Authorization header value ("Bearer <token>") to dst without heap allocations.
func AppendBearer[S ~string | ~[]byte](dst []byte, token S) []byte {
	dst = append(dst, "Bearer "...)
	return append(dst, stringOrBytes(token)...)
}

// FormatBearer formats an access token into an RFC 6750 Authorization header value ("Bearer <token>").
func FormatBearer(token string) string {
	token = strings.TrimSpace(token)
	var buf [128]byte
	if len(token)+7 <= len(buf) {
		b := append(buf[:0], "Bearer "...)
		b = append(b, token...)
		return string(b)
	}
	return "Bearer " + token
}

// IsValidBearerToken reports whether token conforms to the RFC 6750 §2.1 b64token ABNF production:
// 1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="
func IsValidBearerToken[S ~string | ~[]byte](token S) bool {
	raw := stringOrBytes(token)
	n := len(raw)
	if n == 0 {
		return false
	}

	_ = raw[n-1]
	padding := false
	for i := 0; i < n; i++ {
		b := raw[i]
		if b == '=' {
			padding = true
			continue
		}
		if padding {
			return false
		}
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
			continue
		}
		switch b {
		case '-', '.', '_', '~', '+', '/':
			continue
		default:
			return false
		}
	}
	return true
}

// ParseBearer extracts the bearer token from an HTTP Authorization or Proxy-Authorization header ([string] or [[]byte]) (RFC 6750 §2.1).
func ParseBearer[S ~string | ~[]byte](authHeader S) (token S, ok bool) {
	authHeader = trimSpaceGeneric(authHeader)
	if !hasBearerPrefix(authHeader) {
		var zero S
		return zero, false
	}

	token = trimSpaceGeneric(authHeader[7:])
	if len(token) == 0 || !IsValidBearerToken(token) {
		var zero S
		return zero, false
	}

	return token, true
}

func trimSpaceGeneric[S ~string | ~[]byte](s S) S {
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

func hasBearerPrefix[S ~string | ~[]byte](s S) bool {
	if len(s) < 7 {
		return false
	}
	return (s[0] == 'B' || s[0] == 'b') &&
		(s[1] == 'e' || s[1] == 'E') &&
		(s[2] == 'a' || s[2] == 'A') &&
		(s[3] == 'r' || s[3] == 'R') &&
		(s[4] == 'e' || s[4] == 'E') &&
		(s[5] == 'r' || s[5] == 'R') &&
		s[6] == ' '
}

// ParseBearerChallenge parses a WWW-Authenticate header ([string] or [[]byte]) containing a Bearer challenge (RFC 6750 §3).
func ParseBearerChallenge[S ~string | ~[]byte](challengeHeader S) (BearerChallenge, bool) {
	ch := stringOrBytes(challengeHeader)
	ch = strings.TrimSpace(ch)
	if len(ch) < 6 || !strings.EqualFold(ch[:6], "Bearer") {
		return BearerChallenge{}, false
	}

	rest := strings.TrimSpace(ch[6:])
	var challenge BearerChallenge
	if rest == "" {
		return challenge, true
	}

	for rest != "" {
		eqIdx := strings.IndexByte(rest, '=')
		if eqIdx == -1 {
			break
		}

		key := strings.ToLower(strings.TrimSpace(rest[:eqIdx]))
		rest = strings.TrimSpace(rest[eqIdx+1:])

		var val string
		if strings.HasPrefix(rest, "\"") {
			rest = rest[1:]
			endQuote := strings.IndexByte(rest, '"')
			if endQuote == -1 {
				val = rest
				rest = ""
			} else {
				val = rest[:endQuote]
				rest = strings.TrimSpace(rest[endQuote+1:])
				if strings.HasPrefix(rest, ",") {
					rest = strings.TrimSpace(rest[1:])
				}
			}
		} else {
			commaIdx := strings.IndexByte(rest, ',')
			if commaIdx == -1 {
				val = strings.TrimSpace(rest)
				rest = ""
			} else {
				val = strings.TrimSpace(rest[:commaIdx])
				rest = strings.TrimSpace(rest[commaIdx+1:])
			}
		}

		switch key {
		case "realm":
			challenge.Realm = val
		case "scope":
			challenge.Scope = val
		case "error":
			challenge.Error = val
		case "error_description":
			challenge.ErrorDescription = val
		case "error_uri":
			challenge.ErrorURI = val
		}
	}

	return challenge, true
}
