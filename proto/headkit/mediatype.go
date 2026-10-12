// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit

import (
	"bytes"
	"errors"
	"strings"

	"github.com/lemon4ksan/foundation/bufkit"
)

// ErrInvalidMediaType is returned when a Content-Type value cannot be parsed.
var ErrInvalidMediaType = errors.New("headkit: invalid media type")

// BaseMediaType extracts and normalizes the base media type (e.g. "application/json; charset=utf-8" -> "application/json")
// without allocating parameter maps. Supports both [string] and [[]byte].
func BaseMediaType[S ~string | ~[]byte](v S) S {
	rawType, _, _ := cutDirectives(v, ';')
	return toLowerDirectives(trimSpaceDirectives(rawType))
}

// ParseMediaType parses a Content-Type or Content-Disposition value into its base media type
// and parameter map according to RFC 2045 §5.1 and RFC 9110 §8.3.
// Supports both [string] and [[]byte].
func ParseMediaType[S ~string | ~[]byte](v S) (S, DirectivesMap, error) {
	if len(v) == 0 {
		return v[:0], DirectivesMap{}, ErrInvalidMediaType
	}

	rawType, rest, ok := cutDirectives(v, ';')

	mediaType := toLowerDirectives(trimSpaceDirectives(rawType))
	if len(mediaType) == 0 || !containsByte(mediaType, '/') {
		return v[:0], DirectivesMap{}, ErrInvalidMediaType
	}

	if !ok || len(trimSpaceDirectives(rest)) == 0 {
		return mediaType, DirectivesMap{}, nil
	}

	params := ParseParamDirectives(rest)

	return mediaType, params, nil
}

// ExtractBoundary extracts the multipart boundary parameter from a Content-Type string or byte slice (RFC 2046 §5.1.1).
func ExtractBoundary[S ~string | ~[]byte](contentType S) string {
	b := stringOrBytesToBytes(contentType)
	if !bytes.Contains(b, []byte("boundary")) {
		return ""
	}

	_, params, err := ParseMediaType(contentType)
	if err != nil {
		return ""
	}

	return params.Get("boundary")
}

// ExtractCharset extracts the character set parameter from a Content-Type string or byte slice (RFC 9110 §8.3).
func ExtractCharset[S ~string | ~[]byte](contentType S) string {
	b := stringOrBytesToBytes(contentType)
	if !bytes.Contains(b, []byte("charset")) {
		return ""
	}

	_, params, err := ParseMediaType(contentType)
	if err != nil {
		return ""
	}

	return params.Get("charset")
}

// IsJSON reports whether contentType represents a JSON media type (RFC 8259, RFC 6839).
// Supports both [string] and [[]byte] with zero heap allocations.
func IsJSON[S ~string | ~[]byte](contentType S) bool {
	mt := BaseMediaType(contentType)
	b := stringOrBytesToBytes(mt)
	return bytes.Equal(b, []byte("application/json")) || bytes.HasSuffix(b, []byte("+json"))
}

// IsXML reports whether contentType represents an XML media type (RFC 7303, RFC 6839).
// Supports both [string] and [[]byte] with zero heap allocations.
func IsXML[S ~string | ~[]byte](contentType S) bool {
	mt := BaseMediaType(contentType)
	b := stringOrBytesToBytes(mt)

	return bytes.Equal(b, []byte("application/xml")) || bytes.Equal(b, []byte("text/xml")) ||
		bytes.HasSuffix(b, []byte("+xml"))
}

// IsForm reports whether contentType is standard URL-encoded form data (application/x-www-form-urlencoded).
// Supports both [string] and [[]byte] with zero heap allocations.
func IsForm[S ~string | ~[]byte](contentType S) bool {
	mt := BaseMediaType(contentType)
	b := stringOrBytesToBytes(mt)
	return bytes.Equal(b, []byte("application/x-www-form-urlencoded"))
}

// IsMultipart reports whether contentType is multipart form data (multipart/form-data).
// Supports both [string] and [[]byte] with zero heap allocations.
func IsMultipart[S ~string | ~[]byte](contentType S) bool {
	mt := BaseMediaType(contentType)
	b := stringOrBytesToBytes(mt)
	return bytes.Equal(b, []byte("multipart/form-data")) || bytes.HasPrefix(b, []byte("multipart/"))
}

func containsByte[S ~string | ~[]byte](s S, b byte) bool {
	for i := range len(s) {
		if s[i] == b {
			return true
		}
	}

	return false
}

// FormatMediaType formats a media type and parameter map into an RFC 2045 formatted string.
func FormatMediaType(mediaType string, params map[string]string) string {
	if len(params) == 0 {
		return strings.ToLower(strings.TrimSpace(mediaType))
	}

	sb := bufkit.NewChain()
	defer sb.Release()

	_, _ = sb.WriteString(strings.ToLower(strings.TrimSpace(mediaType)))

	for k, v := range params {
		_, _ = sb.WriteString("; ")
		_, _ = sb.WriteString(strings.ToLower(strings.TrimSpace(k)))
		_, _ = sb.WriteString("=")

		if strings.ContainsAny(v, " ;\"=(),/") {
			_, _ = sb.WriteString(strconvQuote(v))
		} else {
			_, _ = sb.WriteString(v)
		}
	}

	return string(sb.Bytes())
}

func strconvQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}
