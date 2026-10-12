// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package contentdisposition implements Content-Disposition header parsing, RFC 8187 parameter encoding/decoding,
// and safe filename sanitization strictly conforming to RFC 6266 and RFC 8187.
package contentdisposition

import (
	"mime"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/pathkit"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/seal/text/encoding/htmlindex"
)

// Standard Content-Disposition disposition types per RFC 6266 §4.2 and RFC 7578 §4.2.
const (
	// DispositionInline indicates default in-place processing and rendering of the response (RFC 6266 §4.2).
	DispositionInline = "inline"

	// DispositionAttachment prompts the user to save the response locally ("Save As...") (RFC 6266 §4.2).
	DispositionAttachment = "attachment"

	// DispositionFormData denotes a form field payload in multipart/form-data bodies (RFC 7578 §4.2).
	DispositionFormData = "form-data"
)

// ContentDisposition represents a parsed HTTP Content-Disposition header according to RFC 6266.
// The zero value represents an empty disposition.
type ContentDisposition struct {
	// Type is the disposition type (e.g., "inline", "attachment", or extension token) per RFC 6266 §4.2.
	Type string

	// Filename is the extracted, decoded, and sanitized filename per RFC 6266 §4.3.
	Filename string

	// Parameters holds additional disposition parameters per RFC 6266 §4.4.
	Parameters map[string]string
}

// FileName cleans a string or byte slice by stripping path traversal sequences, null bytes,
// control characters, and Windows reserved device names per RFC 6266 §4.3.
func FileName[S ~string | ~[]byte](filename S) string {
	raw := stringOrBytes(filename)

	var sb strings.Builder
	sb.Grow(len(raw))

	for i := range len(raw) {
		b := raw[i]
		if b >= 32 && b != 127 {
			sb.WriteByte(b)
		}
	}

	cleaned := strings.ReplaceAll(sb.String(), "\\", "/")
	if len(cleaned) >= 2 && cleaned[1] == ':' &&
		((cleaned[0] >= 'a' && cleaned[0] <= 'z') || (cleaned[0] >= 'A' && cleaned[0] <= 'Z')) {
		cleaned = cleaned[2:]
	}

	res := pathkit.New(pathkit.Clean(strings.TrimSpace(cleaned))).Base()

	for strings.HasPrefix(res, "..") || strings.HasPrefix(res, ".") {
		res = strings.TrimPrefix(res, "..")
		res = strings.TrimPrefix(res, ".")
	}

	res = strings.ReplaceAll(res, ":", "_")
	res = strings.TrimSpace(res)

	if IsWindowsReservedName(res) || res == "" || res == "." || res == "/" ||
		res == "\\" {
		return "downloaded_file"
	}

	return res
}

// Parse parses a Content-Disposition header ([string] or [[]byte]) per RFC 6266 §4.1–§4.4.
// Unknown or unhandled disposition types are normalized to "attachment" (RFC 6266 §4.2).
// When both "filename*" and "filename" parameters are present, "filename*" takes precedence (RFC 6266 §4.3).
func Parse[S ~string | ~[]byte](header S) ContentDisposition {
	contentDispositionHeader := stringOrBytes(header)
	if contentDispositionHeader == "" {
		return ContentDisposition{
			Type:       DispositionAttachment,
			Filename:   "downloaded_file",
			Parameters: nil,
		}
	}

	dispType, params, err := mime.ParseMediaType(contentDispositionHeader)
	if err != nil {
		return ContentDisposition{
			Type:       DispositionAttachment,
			Filename:   "downloaded_file",
			Parameters: nil,
		}
	}

	dispType = generic.Coalesce(strings.ToLower(strings.TrimSpace(dispType)), DispositionAttachment)

	var filename string
	if extFn, ok := params["filename*"]; ok && extFn != "" {
		// RFC 6266 §4.3: Pick filename* over filename when both are present
		_, _, filename = DecodeRFC8187(extFn)
	} else if stdFn, ok := params["filename"]; ok {
		filename = stdFn
	}

	sanitized := "downloaded_file"
	if filename != "" {
		sanitized = FileName(filename)
	}

	return ContentDisposition{
		Type:       dispType,
		Filename:   sanitized,
		Parameters: params,
	}
}

// ExtractFilename extracts, RFC 8187-decodes, and cleans the filename parameter
// from a Content-Disposition HTTP header ([string] or [[]byte]) according to RFC 6266 §4.3.
func ExtractFilename[S ~string | ~[]byte](contentDispositionHeader S) string {
	cd := Parse(contentDispositionHeader)
	return cd.Filename
}

// AppendFormatContentDisposition serializes an RFC 6266 Content-Disposition header and appends it to dst without heap allocations.
func AppendFormatContentDisposition(dst []byte, dispType, filename string) []byte {
	dispType = generic.Coalesce(strings.TrimSpace(dispType), DispositionAttachment)

	if filename == "" {
		return append(dst, dispType...)
	}

	isPureASCII := true
	for i := range len(filename) {
		if filename[i] > 127 || filename[i] == '"' || filename[i] == '\\' {
			isPureASCII = false
			break
		}
	}

	dst = append(dst, dispType...)
	if isPureASCII {
		dst = append(dst, ';', ' ', 'f', 'i', 'l', 'e', 'n', 'a', 'm', 'e', '=', '"')
		dst = append(dst, filename...)
		dst = append(dst, '"')

		return dst
	}

	// RFC 6266 Appendix D & RFC 8187 §3.2.1: ASCII fallback + filename*
	dst = append(dst, ';', ' ', 'f', 'i', 'l', 'e', 'n', 'a', 'm', 'e', '=', '"')
	for i := range len(filename) {
		b := filename[i]
		if b >= 32 && b <= 126 && b != '"' && b != '\\' && b != '%' {
			dst = append(dst, b)
		} else {
			dst = append(dst, '_')
		}
	}

	dst = append(dst, '"', ';', ' ', 'f', 'i', 'l', 'e', 'n', 'a', 'm', 'e', '*', '=')
	dst = AppendEncodeRFC8187(dst, filename, "")

	return dst
}

// FormatContentDisposition constructs an RFC 6266 compliant Content-Disposition header value
// using the recommendations in RFC 6266 Appendix D (providing ASCII fallback and RFC 8187 filename*).
func FormatContentDisposition(dispType, filename string) string {
	var buf [256]byte

	return string(AppendFormatContentDisposition(buf[:0], dispType, filename))
}

const hexTableUpper = "0123456789ABCDEF"

// IsRFC8187AttrChar reports whether b is a valid RFC 8187 attr-char (RFC 8187 §3.2.1).
func IsRFC8187AttrChar(b byte) bool {
	if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
		return true
	}

	switch b {
	case '!', '#', '$', '&', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}

	return false
}

// AppendEncodeRFC8187 encodes value into RFC 8187 extended parameter value notation and appends it to dst without heap allocations.
func AppendEncodeRFC8187(dst []byte, value, language string) []byte {
	dst = append(dst, "UTF-8'"...)
	dst = append(dst, language...)
	dst = append(dst, '\'')

	for i := range len(value) {
		b := value[i]
		if IsRFC8187AttrChar(b) {
			dst = append(dst, b)
		} else {
			dst = append(dst, '%', hexTableUpper[b>>4], hexTableUpper[b&0x0F])
		}
	}

	return dst
}

// EncodeRFC8187 encodes value into RFC 8187 extended parameter value notation ("UTF-8'lang'encoded-value")
// using UTF-8 character encoding and percent-encoding for non-attr-char octets (RFC 8187 §3.2.1).
func EncodeRFC8187(value, language string) string {
	var buf [256]byte

	return string(AppendEncodeRFC8187(buf[:0], value, language))
}

// DecodeRFC8187 decodes an RFC 8187 extended parameter value ([string] or [[]byte]) ("charset'language'value-chars")
// into its constituent charset, language, and decoded value (RFC 8187 §3.2.1).
func DecodeRFC8187[S ~string | ~[]byte](extValue S) (charset, language, value string) {
	raw := stringOrBytes(extValue)

	firstQuote := strings.IndexByte(raw, '\'')
	if firstQuote == -1 {
		return "", "", raw
	}

	secondQuote := strings.IndexByte(raw[firstQuote+1:], '\'')
	if secondQuote == -1 {
		return "", "", raw
	}

	secondQuote += firstQuote + 1

	charset = strings.ToLower(strings.TrimSpace(raw[:firstQuote]))
	language = raw[firstQuote+1 : secondQuote]
	rawEncoded := raw[secondQuote+1:]

	unescaped, unescapeErr := url.PathUnescape(rawEncoded)
	if unescapeErr != nil {
		unescaped = rawEncoded
	}

	switch charset {
	case "utf-8", "utf8", "":
		if utf8.ValidString(unescaped) {
			value = unescaped
		} else {
			value = strings.ToValidUTF8(unescaped, "")
		}

	case "iso-8859-1", "latin1":
		value = ISO88591ToUTF8(unescaped)

	default:
		if enc, getErr := htmlindex.Get(charset); getErr == nil {
			if decoded, decErr := enc.NewDecoder().String(unescaped); decErr == nil {
				value = decoded
				return charset, language, value
			}
		}

		if utf8.ValidString(unescaped) {
			value = unescaped
		} else {
			value = strings.ToValidUTF8(unescaped, "")
		}
	}

	return charset, language, value
}

// AppendISO88591ToUTF8 converts ISO-8859-1 raw bytes ([string] or [[]byte]) to UTF-8 and appends them to dst.
func AppendISO88591ToUTF8[S ~string | ~[]byte](dst []byte, s S) []byte {
	str := stringOrBytes(s)
	for i := range len(str) {
		r := rune(str[i])

		var buf [utf8.UTFMax]byte

		n := utf8.EncodeRune(buf[:], r)
		dst = append(dst, buf[:n]...)
	}

	return dst
}

// ISO88591ToUTF8 translates ISO-8859-1 raw bytes ([string] or [[]byte]) into UTF-8 representation.
func ISO88591ToUTF8[S ~string | ~[]byte](s S) string {
	var buf [256]byte

	return string(AppendISO88591ToUTF8(buf[:0], s))
}

// IsWindowsReservedName checks whether filename stem conflicts with Win32 legacy DOS devices (RFC 6266 §4.3).
func IsWindowsReservedName[S ~string | ~[]byte](filename S) bool {
	raw := stringOrBytes(filename)
	if before, _, ok := strings.Cut(raw, "."); ok {
		raw = before
	}

	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	}

	return false
}

func stringOrBytes[S ~string | ~[]byte](s S) string {
	switch v := any(s).(type) {
	case string:
		return v
	case []byte:
		return bytesconv.B2S(v)
	default:
		return string(s)
	}
}
