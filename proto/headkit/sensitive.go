// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit

import (
	"net/http"
	"strings"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// DefaultRedactedMask is the standard replacement placeholder for sensitive values.
const DefaultRedactedMask = "[REDACTED]"

// IsSensitive reports whether headerName contains authentication credentials,
// session tokens, or private identity data per RFC 9110 §15.
// Accepts both [string] and [[]byte] with zero heap allocations.
func IsSensitive[S ~string | ~[]byte](headerName S) bool {
	b := stringOrBytesToBytes(headerName)
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\r' || b[0] == '\n') {
		b = b[1:]
	}

	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\t' || b[len(b)-1] == '\r' || b[len(b)-1] == '\n') {
		b = b[:len(b)-1]
	}

	if len(b) == 0 {
		return false
	}

	h := bytesconv.B2S(b)
	switch len(h) {
	case 5: // token
		return bytesconv.EqualFoldASCII(h, "token")
	case 6: // cookie, secret
		return bytesconv.EqualFoldASCII(h, "cookie") || bytesconv.EqualFoldASCII(h, "secret")
	case 7: // api-key
		return bytesconv.EqualFoldASCII(h, "api-key")
	case 8: // x-secret
		return bytesconv.EqualFoldASCII(h, "x-secret")
	case 9: // x-api-key
		return bytesconv.EqualFoldASCII(h, "x-api-key")
	case 10: // set-cookie
		return bytesconv.EqualFoldASCII(h, "set-cookie")
	case 11: // private-key
		return bytesconv.EqualFoldASCII(h, "private-key")
	case 12: // x-auth-token
		return bytesconv.EqualFoldASCII(h, "x-auth-token")
	case 13: // authorization
		return bytesconv.EqualFoldASCII(h, "authorization")
	case 14: // x-access-token
		return bytesconv.EqualFoldASCII(h, "x-access-token")
	case 15: // x-client-secret
		return bytesconv.EqualFoldASCII(h, "x-client-secret")
	case 16: // www-authenticate
		return bytesconv.EqualFoldASCII(h, "www-authenticate")
	case 18: // proxy-authenticate
		return bytesconv.EqualFoldASCII(h, "proxy-authenticate")
	case 19: // proxy-authorization
		return bytesconv.EqualFoldASCII(h, "proxy-authorization")
	default:
		return false
	}
}

// RedactValue masks the secret value of a sensitive header while preserving recognizable prefix formats
// (e.g. "Bearer [REDACTED]" or "Basic [REDACTED]").
func RedactValue[SH, SV ~string | ~[]byte](headerName SH, value SV) string {
	bVal := stringOrBytesToBytes(value)
	if len(bVal) == 0 || !IsSensitive(headerName) {
		return string(bVal)
	}

	bHdr := stringOrBytesToBytes(headerName)
	h := bytesconv.B2S(bHdr)
	v := bytesconv.B2S(bVal)

	if bytesconv.EqualFoldASCII(h, "authorization") || bytesconv.EqualFoldASCII(h, "proxy-authorization") {
		scheme, _, ok := strings.Cut(v, " ")
		if ok && scheme != "" {
			return scheme + " " + DefaultRedactedMask
		}
	}

	return DefaultRedactedMask
}

// RedactHeader returns a deep copy of h with all sensitive credentials securely masked.
func RedactHeader(h http.Header) http.Header {
	if h == nil {
		return nil
	}

	redacted := make(http.Header, len(h))
	for k, vv := range h {
		if IsSensitive(k) {
			masked := make([]string, len(vv))
			for i, v := range vv {
				masked[i] = RedactValue(k, v)
			}

			redacted[k] = masked
		} else {
			copied := make([]string, len(vv))
			copy(copied, vv)
			redacted[k] = copied
		}
	}

	return redacted
}
