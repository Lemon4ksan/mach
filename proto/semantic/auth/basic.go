// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"bytes"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// ErrInvalidBase64 is returned when HTTP Basic credentials fail Base64 decoding.
var ErrInvalidBase64 = errors.New("auth: invalid base64 encoding")

// BasicChallenge represents an RFC 7617 HTTP Basic Authentication challenge from a WWW-Authenticate header.
// The zero value represents an empty challenge.
type BasicChallenge struct {
	// Realm identifies the protected resource boundary (RFC 7617 §2).
	Realm string
	// Charset specifies character encoding, typically "UTF-8" (RFC 7617 §2.1).
	Charset string
}

// AppendFormat appends the WWW-Authenticate header challenge to dst without heap allocations (RFC 7617 §2).
func (c BasicChallenge) AppendFormat(dst []byte) []byte {
	dst = append(dst, "Basic realm=\""...)
	dst = append(dst, c.Realm...)
	dst = append(dst, '"')

	if c.Charset != "" {
		dst = append(dst, ", charset=\""...)
		dst = append(dst, c.Charset...)
		dst = append(dst, '"')
	}

	return dst
}

// String formats the Challenge as a standard WWW-Authenticate header value (RFC 7617 §2).
func (c BasicChallenge) String() string {
	var buf [128]byte
	return string(c.AppendFormat(buf[:0]))
}

// AppendBasic formats an RFC 7617 Authorization header value ("Basic <base64>") and appends it to dst.
func AppendBasic[S ~string | ~[]byte](dst []byte, username, password S) []byte {
	u := stringOrBytes(username)
	p := stringOrBytes(password)
	totalLen := len(u) + 1 + len(p)

	dst = append(dst, "Basic "...)
	encLen := base64.StdEncoding.EncodedLen(totalLen)
	dstStart := len(dst)
	dst = append(dst, make([]byte, encLen)...)

	if totalLen <= 128 {
		var buf [128]byte
		n := copy(buf[:], u)
		buf[n] = ':'
		copy(buf[n+1:], p)
		base64.StdEncoding.Encode(dst[dstStart:], buf[:totalLen])
	} else {
		cred := make([]byte, totalLen)
		n := copy(cred, u)
		cred[n] = ':'
		copy(cred[n+1:], p)
		base64.StdEncoding.Encode(dst[dstStart:], cred)
	}

	return dst
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

var (
	errNotBasicScheme = errors.New("auth: not a Basic scheme")
	errMissingColon   = errors.New("auth: missing colon in credentials")
)

// ParseBasic extracts the username and password from a standard "Authorization: Basic <credentials>" header (RFC 7617 §2).
// When authHeader is []byte, in-place base64 decoding is performed with zero allocations.
func ParseBasic[S ~string | ~[]byte](authHeader S) (username, password S, err error) {
	var zero S
	switch h := any(authHeader).(type) {
	case []byte:
		h = bytes.TrimSpace(h)
		if len(h) < 6 || !bytes.EqualFold(h[:6], []byte("Basic ")) {
			return zero, zero, errNotBasicScheme
		}

		payload := bytes.TrimSpace(h[6:])
		n, errDecode := base64.StdEncoding.Decode(payload, payload)
		if errDecode != nil {
			return zero, zero, ErrInvalidBase64
		}

		decoded := payload[:n]
		before, after, ok := bytes.Cut(decoded, []byte{':'})
		if !ok {
			return zero, zero, errMissingColon
		}

		return any(before).(S), any(after).(S), nil

	case string:
		h = strings.TrimSpace(h)
		if len(h) < 6 || !strings.EqualFold(h[:6], "Basic ") {
			return zero, zero, errNotBasicScheme
		}

		payload := strings.TrimSpace(h[6:])
		var buf [256]byte
		var decoded []byte

		if base64.StdEncoding.DecodedLen(len(payload)) <= len(buf) {
			n, errDecode := base64.StdEncoding.Decode(buf[:], bytesconv.S2B(payload))
			if errDecode != nil {
				return zero, zero, ErrInvalidBase64
			}
			decoded = buf[:n]
		} else {
			var errDecode error
			decoded, errDecode = base64.StdEncoding.DecodeString(payload)
			if errDecode != nil {
				return zero, zero, ErrInvalidBase64
			}
		}

		raw := bytesconv.B2S(decoded)
		before, after, ok := strings.Cut(raw, ":")
		if !ok {
			return zero, zero, errMissingColon
		}

		return any(before).(S), any(after).(S), nil

	default:
		str := stringOrBytes(authHeader)
		u, p, err := ParseBasic(str)
		if err != nil {
			return zero, zero, err
		}
		return S(u), S(p), nil
	}
}

// ParseBasicChallenge extracts the realm and optional charset parameter from a "WWW-Authenticate: Basic ..." header (RFC 7617 §2).
func ParseBasicChallenge[S ~string | ~[]byte](challengeHeader S) (BasicChallenge, bool) {
	ch := stringOrBytes(challengeHeader)
	ch = strings.TrimSpace(ch)
	if len(ch) < 6 || !strings.EqualFold(ch[:6], "Basic ") {
		return BasicChallenge{}, false
	}

	parsedParams, _ := ExtractChallengeParams(ch, "Basic")
	var res BasicChallenge
	foundRealm := false

	if realm, ok := parsedParams["realm"]; ok {
		res.Realm = realm
		foundRealm = true
	}

	if charset, ok := parsedParams["charset"]; ok {
		if strings.EqualFold(charset, "UTF-8") {
			res.Charset = "UTF-8"
		}
	}

	return res, foundRealm
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

func stringOrBytes[S ~string | ~[]byte](s S) string {
	switch v := any(s).(type) {
	case string:
		return v
	case []byte:
		return bytesconv.B2S(v)
	default:
		return ""
	}
}
