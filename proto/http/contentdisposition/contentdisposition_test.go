// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package contentdisposition_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http/contentdisposition"
	"github.com/lemon4ksan/foundation/testing/assert"
)

func TestContentDisposition_RFC6266_And_RFC8187(t *testing.T) {
	t.Parallel()

	// Pure ASCII filename
	asciiFormatted := contentdisposition.FormatContentDisposition("attachment", "document.pdf")
	assert.Equal(t, `attachment; filename="document.pdf"`, asciiFormatted)

	cdAscii := contentdisposition.ParseContentDisposition(asciiFormatted)
	assert.Equal(t, contentdisposition.DispositionAttachment, cdAscii.Type)
	assert.Equal(t, "document.pdf", cdAscii.Filename)

	// UTF-8 filename with RFC 8187 encoding
	utf8Formatted := contentdisposition.FormatContentDisposition("attachment", "отчет_2026.pdf")
	assert.Contains(t, utf8Formatted, `filename*=UTF-8''`)

	cdUtf8 := contentdisposition.ParseContentDisposition(utf8Formatted)
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

	charset, lang, val, err := contentdisposition.DecodeRFC8187(encoded)
	assert.NoError(t, err)
	assert.Equal(t, "utf-8", charset)
	assert.Equal(t, "de", lang)
	assert.Equal(t, "letztes Kapitel", val)
}
