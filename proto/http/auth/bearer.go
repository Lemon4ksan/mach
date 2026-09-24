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
type BearerChallenge struct {
	Realm            string
	Scope            string
	Error            string
	ErrorDescription string
	ErrorURI         string
}

// String formats the Challenge as a standard WWW-Authenticate header value (RFC 6750 §3).
func (c BearerChallenge) String() string {
	var sb strings.Builder
	sb.WriteString("Bearer")

	first := true
	appendParam := func(k, v string) {
		if v == "" {
			return
		}

		if first {
			sb.WriteByte(' ')

			first = false
		} else {
			sb.WriteString(", ")
		}

		sb.WriteString(k)
		sb.WriteString("=\"")
		sb.WriteString(v)
		sb.WriteByte('"')
	}

	appendParam("realm", c.Realm)
	appendParam("scope", c.Scope)
	appendParam("error", c.Error)
	appendParam("error_description", c.ErrorDescription)
	appendParam("error_uri", c.ErrorURI)

	return sb.String()
}

// FormatBearer formats an access token into an RFC 6750 Authorization header value ("Bearer <token>").
func FormatBearer(token string) string {
	return "Bearer " + strings.TrimSpace(token)
}

// IsValidBearerToken reports whether token conforms to the RFC 6750 §2.1 b64token ABNF production:
// 1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="
func IsValidBearerToken(token string) bool {
	n := len(token)
	if n == 0 {
		return false
	}

	_ = token[n-1]

	padding := false
	for i := range n {
		b := token[i]
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

// ParseBearer extracts the bearer token from an HTTP Authorization or Proxy-Authorization header (RFC 6750 §2.1).
func ParseBearer(authHeader string) (token string, ok bool) {
	authHeader = strings.TrimSpace(authHeader)
	if len(authHeader) < 7 || !strings.EqualFold(authHeader[:7], "Bearer ") {
		return "", false
	}

	token = strings.TrimSpace(authHeader[7:])
	if token == "" || !IsValidBearerToken(token) {
		return "", false
	}

	return token, true
}

// ParseBearerChallenge parses a WWW-Authenticate header containing a Bearer challenge (RFC 6750 §3).
func ParseBearerChallenge(challengeHeader string) (BearerChallenge, bool) {
	challengeHeader = strings.TrimSpace(challengeHeader)
	if len(challengeHeader) < 6 || !strings.EqualFold(challengeHeader[:6], "Bearer") {
		return BearerChallenge{}, false
	}

	rest := strings.TrimSpace(challengeHeader[6:])

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
