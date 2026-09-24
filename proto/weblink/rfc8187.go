// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package weblink

import (
	"net/url"
	"strings"
	"unicode/utf8"
)

const hexTableUpper = "0123456789ABCDEF"

func isRFC8187AttrChar(b byte) bool {
	if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
		return true
	}

	switch b {
	case '!', '#', '$', '&', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}

	return false
}

func encodeRFC8187(value, language string) string {
	var sb strings.Builder
	sb.Grow(len("UTF-8'") + len(language) + len("'") + len(value)*3)
	sb.WriteString("UTF-8'")
	sb.WriteString(language)
	sb.WriteByte('\'')

	for i := 0; i < len(value); i++ {
		b := value[i]
		if isRFC8187AttrChar(b) {
			sb.WriteByte(b)
		} else {
			sb.WriteByte('%')
			sb.WriteByte(hexTableUpper[b>>4])
			sb.WriteByte(hexTableUpper[b&0x0F])
		}
	}

	return sb.String()
}

func decodeRFC8187(extValue string) (charset, language, value string, err error) {
	firstQuote := strings.IndexByte(extValue, '\'')
	if firstQuote == -1 {
		return "", "", extValue, nil
	}

	secondQuote := strings.IndexByte(extValue[firstQuote+1:], '\'')
	if secondQuote == -1 {
		return "", "", extValue, nil
	}

	secondQuote += firstQuote + 1

	charset = strings.ToLower(strings.TrimSpace(extValue[:firstQuote]))
	language = extValue[firstQuote+1 : secondQuote]
	rawEncoded := extValue[secondQuote+1:]

	unescaped, unescapeErr := url.PathUnescape(rawEncoded)
	if unescapeErr != nil {
		unescaped = rawEncoded
	}

	if utf8.ValidString(unescaped) {
		value = unescaped
	} else {
		value = strings.ToValidUTF8(unescaped, "")
	}

	return charset, language, value, nil
}

func decodeRFC8187Value(extValue string) string {
	_, _, val, _ := decodeRFC8187(extValue)
	return val
}
