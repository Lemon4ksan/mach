// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package auth implements HTTP Authentication schemes strictly conforming to IETF standards (RFC 7617 Basic, RFC 6750 Bearer).
package auth

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strings"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

var ErrInvalidBase64 = errors.New("auth: invalid base64 encoding")

// BasicChallenge represents an RFC 7617 HTTP Basic Authentication challenge from a WWW-Authenticate header.
type BasicChallenge struct {
	Realm   string
	Charset string
}

// String formats the Challenge as a standard WWW-Authenticate header value (RFC 7617 §2).
func (c BasicChallenge) String() string {
	var sb strings.Builder
	sb.WriteString("Basic realm=\"")
	sb.WriteString(c.Realm)
	sb.WriteByte('"')

	if c.Charset != "" {
		sb.WriteString(", charset=\"")
		sb.WriteString(c.Charset)
		sb.WriteByte('"')
	}

	return sb.String()
}

// FormatBasic constructs a standard "Authorization: Basic <credentials>" header value (RFC 7617 §2).
//
// Performance & Zero-Allocation Optimization:
// For credentials whose combined length ("username:password") is <= 128 bytes,
// serialization executes entirely on stack buffers without allocating heap memory.
func FormatBasic(username, password string) string {
	totalLen := len(username) + 1 + len(password)
	if totalLen <= 128 {
		var buf [128]byte

		n := copy(buf[:], username)
		buf[n] = ':'
		copy(buf[n+1:], password)

		var outBuf [256]byte

		nOut := copy(outBuf[:], "Basic ")
		base64.StdEncoding.Encode(outBuf[nOut:], buf[:totalLen])
		encodedLen := nOut + base64.StdEncoding.EncodedLen(totalLen)

		return string(outBuf[:encodedLen])
	}

	auth := username + ":" + password

	return "Basic " + base64.StdEncoding.EncodeToString(bytesconv.S2B(auth))
}

// ParseBasic extracts the username and password from a standard "Authorization: Basic <credentials>" header (RFC 7617 §2).
func ParseBasic(authHeader string) (username, password string, err error) {
	authHeader = strings.TrimSpace(authHeader)
	if len(authHeader) < 6 || !strings.EqualFold(authHeader[:6], "Basic ") {
		return "", "", errors.New("auth: not a Basic scheme")
	}

	payload := strings.TrimSpace(authHeader[6:])

	decoded, errDecode := base64.StdEncoding.DecodeString(payload)
	if errDecode != nil {
		return "", "", ErrInvalidBase64
	}

	raw := bytesconv.B2S(decoded)

	before, after, ok := strings.Cut(raw, ":")
	if !ok {
		return "", "", errors.New("auth: missing colon in credentials")
	}

	return before, after, nil
}

// ParseBasicChallenge extracts the realm and optional charset parameter from a "WWW-Authenticate: Basic ..." header (RFC 7617 §2).
func ParseBasicChallenge(challengeHeader string) (BasicChallenge, bool) {
	challengeHeader = strings.TrimSpace(challengeHeader)
	if len(challengeHeader) < 6 || !strings.EqualFold(challengeHeader[:6], "Basic ") {
		return BasicChallenge{}, false
	}

	parsedParams, _ := ExtractChallengeParams(challengeHeader, "Basic")

	var ch BasicChallenge
	foundRealm := false

	if realm, ok := parsedParams["realm"]; ok {
		ch.Realm = realm
		foundRealm = true
	}

	if charset, ok := parsedParams["charset"]; ok {
		if strings.EqualFold(charset, "UTF-8") {
			ch.Charset = "UTF-8"
		}
	}

	return ch, foundRealm
}

// InScope verifies whether a target request URI falls within the canonical protection space (RFC 7617 §2.2).
func InScope(reqURL, scopeRootURL string) bool {
	parsedReq, errReq := url.Parse(reqURL)
	parsedScope, errScope := url.Parse(scopeRootURL)
	if errReq != nil || errScope != nil {
		return false
	}

	if !strings.EqualFold(parsedReq.Scheme, parsedScope.Scheme) {
		return false
	}

	if !strings.EqualFold(parsedReq.Host, parsedScope.Host) {
		return false
	}

	scopePath := generic.Coalesce(parsedScope.Path, "/")
	if !strings.HasSuffix(scopePath, "/") {
		scopePath += "/"
	}

	reqPath := generic.Coalesce(parsedReq.Path, "/")

	if reqPath == scopePath {
		return true
	}

	return strings.HasPrefix(reqPath, scopePath)
}
