// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package contentdisposition

import (
	"github.com/lemon4ksan/mach/proto/semantic/contentdisposition"
)

// Standard Content-Disposition disposition types forwarded from proto/semantic/contentdisposition.
const (
	DispositionInline     = contentdisposition.DispositionInline
	DispositionAttachment = contentdisposition.DispositionAttachment
	DispositionFormData   = contentdisposition.DispositionFormData
)

// ContentDisposition forwarded from proto/semantic/contentdisposition.
type ContentDisposition = contentdisposition.ContentDisposition

// FileName forwarded from proto/semantic/contentdisposition.
func FileName(filename string) string {
	return contentdisposition.FileName(filename)
}

// ParseContentDisposition forwarded from proto/semantic/contentdisposition.
func ParseContentDisposition(contentDispositionHeader string) ContentDisposition {
	return contentdisposition.ParseContentDisposition(contentDispositionHeader)
}

// ExtractFilename forwarded from proto/semantic/contentdisposition.
func ExtractFilename(contentDispositionHeader string) string {
	return contentdisposition.ExtractFilename(contentDispositionHeader)
}

// AppendFormatContentDisposition forwarded from proto/semantic/contentdisposition.
func AppendFormatContentDisposition(dst []byte, dispType, filename string) []byte {
	return contentdisposition.AppendFormatContentDisposition(dst, dispType, filename)
}

// FormatContentDisposition forwarded from proto/semantic/contentdisposition.
func FormatContentDisposition(dispType, filename string) string {
	return contentdisposition.FormatContentDisposition(dispType, filename)
}

// IsRFC8187AttrChar forwarded from proto/semantic/contentdisposition.
func IsRFC8187AttrChar(b byte) bool {
	return contentdisposition.IsRFC8187AttrChar(b)
}

// AppendEncodeRFC8187 forwarded from proto/semantic/contentdisposition.
func AppendEncodeRFC8187(dst []byte, value, language string) []byte {
	return contentdisposition.AppendEncodeRFC8187(dst, value, language)
}

// EncodeRFC8187 forwarded from proto/semantic/contentdisposition.
func EncodeRFC8187(value, language string) string {
	return contentdisposition.EncodeRFC8187(value, language)
}

// DecodeRFC8187 forwarded from proto/semantic/contentdisposition.
func DecodeRFC8187(extValue string) (charset, language, value string, err error) {
	return contentdisposition.DecodeRFC8187(extValue)
}

// DecodeRFC8187Value forwarded from proto/semantic/contentdisposition.
func DecodeRFC8187Value(extValue string) string {
	return contentdisposition.DecodeRFC8187Value(extValue)
}

// AppendISO88591ToUTF8 forwarded from proto/semantic/contentdisposition.
func AppendISO88591ToUTF8(dst []byte, s string) []byte {
	return contentdisposition.AppendISO88591ToUTF8(dst, s)
}

// ISO88591ToUTF8 forwarded from proto/semantic/contentdisposition.
func ISO88591ToUTF8(s string) string {
	return contentdisposition.ISO88591ToUTF8(s)
}

// IsWindowsReservedName forwarded from proto/semantic/contentdisposition.
func IsWindowsReservedName(filename string) bool {
	return contentdisposition.IsWindowsReservedName(filename)
}
