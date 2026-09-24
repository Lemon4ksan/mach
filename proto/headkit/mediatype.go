// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit

import (
	"errors"
	"strings"
)

// ErrInvalidMediaType is returned when a Content-Type value cannot be parsed.
var ErrInvalidMediaType = errors.New("headkit: invalid media type")

// BaseMediaType extracts and normalizes the base media type (e.g. "application/json; charset=utf-8" -> "application/json")
// without allocating parameter maps.
func BaseMediaType(v string) string {
	rawType, _, _ := strings.Cut(v, ";")
	return strings.ToLower(strings.TrimSpace(rawType))
}

// ParseMediaType parses a Content-Type or Content-Disposition value into its base media type
// and parameter map according to RFC 2045 §5.1 and RFC 9110 §8.3.
func ParseMediaType(v string) (string, DirectivesMap, error) {
	if v == "" {
		return "", DirectivesMap{}, ErrInvalidMediaType
	}

	rawType, rest, ok := strings.Cut(v, ";")
	mediaType := strings.ToLower(strings.TrimSpace(rawType))
	if mediaType == "" || !strings.Contains(mediaType, "/") {
		return "", DirectivesMap{}, ErrInvalidMediaType
	}

	if !ok || strings.TrimSpace(rest) == "" {
		return mediaType, DirectivesMap{}, nil
	}

	params := ParseParamDirectives(rest)

	return mediaType, params, nil
}

// ExtractBoundary extracts the multipart boundary parameter from a Content-Type string (RFC 2046 §5.1.1).
func ExtractBoundary(contentType string) string {
	if !strings.Contains(contentType, "boundary") {
		return ""
	}

	_, params, err := ParseMediaType(contentType)
	if err != nil {
		return ""
	}

	return params.Get("boundary")
}

// ExtractCharset extracts the character set parameter from a Content-Type string (RFC 9110 §8.3).
func ExtractCharset(contentType string) string {
	if !strings.Contains(contentType, "charset") {
		return ""
	}

	_, params, err := ParseMediaType(contentType)
	if err != nil {
		return ""
	}

	return params.Get("charset")
}

// IsJSON reports whether contentType represents a JSON media type (RFC 8259, RFC 6839).
func IsJSON(contentType string) bool {
	mt := BaseMediaType(contentType)
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}

// IsXML reports whether contentType represents an XML media type (RFC 7303, RFC 6839).
func IsXML(contentType string) bool {
	mt := BaseMediaType(contentType)
	return mt == "application/xml" || mt == "text/xml" || strings.HasSuffix(mt, "+xml")
}

// IsForm reports whether contentType is standard URL-encoded form data (application/x-www-form-urlencoded).
func IsForm(contentType string) bool {
	return BaseMediaType(contentType) == "application/x-www-form-urlencoded"
}

// IsMultipart reports whether contentType is multipart form data (multipart/form-data).
func IsMultipart(contentType string) bool {
	mt := BaseMediaType(contentType)
	return mt == "multipart/form-data" || strings.HasPrefix(mt, "multipart/")
}

// FormatMediaType formats a media type and parameter map into an RFC 2045 formatted string.
func FormatMediaType(mediaType string, params map[string]string) string {
	if len(params) == 0 {
		return strings.ToLower(strings.TrimSpace(mediaType))
	}

	var sb strings.Builder
	sb.WriteString(strings.ToLower(strings.TrimSpace(mediaType)))

	for k, v := range params {
		sb.WriteString("; ")
		sb.WriteString(strings.ToLower(strings.TrimSpace(k)))
		sb.WriteString("=")
		if strings.ContainsAny(v, " ;\"=(),/") {
			sb.WriteString(strconvQuote(v))
		} else {
			sb.WriteString(v)
		}
	}

	return sb.String()
}

func strconvQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}
