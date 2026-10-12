// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package contentdisposition_test

import (
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"

	"github.com/lemon4ksan/mach/proto/semantic/contentdisposition"
)

func TestContentDisposition_RFC6266_And_RFC8187(t *testing.T) {
	t.Parallel()

	// Pure ASCII filename
	asciiFormatted := contentdisposition.FormatContentDisposition("attachment", "document.pdf")
	assert.Equal(t, `attachment; filename="document.pdf"`, asciiFormatted)

	cdAscii := contentdisposition.Parse(asciiFormatted)
	assert.Equal(t, contentdisposition.DispositionAttachment, cdAscii.Type)
	assert.Equal(t, "document.pdf", cdAscii.Filename)

	// Byte slice parsing
	cdBytes := contentdisposition.Parse([]byte(asciiFormatted))
	assert.Equal(t, contentdisposition.DispositionAttachment, cdBytes.Type)
	assert.Equal(t, "document.pdf", cdBytes.Filename)

	// UTF-8 filename with RFC 8187 encoding
	utf8Formatted := contentdisposition.FormatContentDisposition("attachment", "отчет_2026.pdf")
	assert.Contains(t, utf8Formatted, `filename*=UTF-8''`)

	cdUtf8 := contentdisposition.Parse(utf8Formatted)
	assert.Equal(t, "отчет_2026.pdf", cdUtf8.Filename)

	// Path traversal protection
	assert.Equal(t, "secret.txt", contentdisposition.FileName("../../secret.txt"))
	assert.Equal(t, "passwords.db", contentdisposition.FileName("C:\\Windows\\System32\\..\\passwords.db"))
	assert.Equal(t, "downloaded_file", contentdisposition.FileName("../.."))
	assert.Equal(t, "downloaded_file", contentdisposition.FileName("CON"))
	assert.Equal(t, "downloaded_file", contentdisposition.FileName("NUL.txt"))

	// RFC 8187 Encode & Decode
	encoded := contentdisposition.EncodeRFC8187("letztes Kapitel", "de")
	assert.Equal(t, "UTF-8'de'letztes%20Kapitel", encoded)

	charset, lang, val := contentdisposition.DecodeRFC8187(encoded)
	assert.Equal(t, "utf-8", charset)
	assert.Equal(t, "de", lang)
	assert.Equal(t, "letztes Kapitel", val)

	// Byte slice decoding
	charsetBytes, langBytes, valBytes := contentdisposition.DecodeRFC8187([]byte(encoded))
	assert.Equal(t, "utf-8", charsetBytes)
	assert.Equal(t, "de", langBytes)
	assert.Equal(t, "letztes Kapitel", valBytes)
}

func TestContentDisposition_Hardening(t *testing.T) {
	t.Parallel()

	// 1. ISO-8859-1 conversion with multi-byte and high-order octets
	rawISO := "\xC2\xA0\xE9\xFC"
	converted := contentdisposition.ISO88591ToUTF8(rawISO)
	assert.Equal(t, "\u00C2\u00A0\u00E9\u00FC", converted)

	// 2. NTFS Alternate Data Streams (ADS) colon sanitization
	assert.Equal(t, "test.txt__$DATA", contentdisposition.FileName("test.txt::$DATA"))
	assert.Equal(t, "malicious.exe_payload", contentdisposition.FileName("malicious.exe:payload"))

	// 3. ExtractFilenameOptional
	opt1 := contentdisposition.ExtractFilenameOptional(`attachment; filename="report.pdf"`)
	assert.True(t, opt1.IsPresent())
	assert.Equal(t, "report.pdf", opt1.MustValue())

	optEmpty := contentdisposition.ExtractFilenameOptional(`attachment`)
	assert.False(t, optEmpty.IsPresent())
	assert.Equal(t, "default.pdf", optEmpty.OrElse("default.pdf"))
}
