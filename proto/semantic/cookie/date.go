// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cookie

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

var (
	httpDateGMT = time.FixedZone("GMT", 0)

	strGMT = []byte("GMT")
)

// AppendHTTPDate formats date in HTTP-date GMT format (RFC 9110 §5.6.7), appends it to dst, and returns the slice.
func AppendHTTPDate(dst []byte, date time.Time) []byte {
	dst = date.In(time.UTC).AppendFormat(dst, time.RFC1123)
	copy(dst[len(dst)-3:], strGMT)
	return dst
}

// ParseRFC1123DateGMT parses an RFC 1123 GMT date byte slice into a [time.Time].
func ParseRFC1123DateGMT(b []byte) (time.Time, bool) {
	// Expects "Mon, 02 Jan 2006 15:04:05 GMT".
	if len(b) != 29 {
		return time.Time{}, false
	}

	if !IsWeekday3(b[0], b[1], b[2]) {
		return time.Time{}, false
	}

	if b[3] != ',' || b[4] != ' ' || b[7] != ' ' || b[11] != ' ' ||
		b[16] != ' ' || b[19] != ':' || b[22] != ':' || b[25] != ' ' {
		return time.Time{}, false
	}

	if b[26] != 'G' || b[27] != 'M' || b[28] != 'T' {
		return time.Time{}, false
	}

	day, ok := Parse2Digits(b[5], b[6])
	if !ok || day < 1 || day > 31 {
		return time.Time{}, false
	}

	month, ok := ParseMonth3(b[8], b[9], b[10])
	if !ok {
		return time.Time{}, false
	}

	year, ok := Parse4Digits(b[12], b[13], b[14], b[15])
	if !ok {
		return time.Time{}, false
	}

	hour, ok := Parse2Digits(b[17], b[18])
	if !ok || hour > 23 {
		return time.Time{}, false
	}

	minute, ok := Parse2Digits(b[20], b[21])
	if !ok || minute > 59 {
		return time.Time{}, false
	}

	second, ok := Parse2Digits(b[23], b[24])
	if !ok || second > 59 {
		return time.Time{}, false
	}

	t := time.Date(year, month, day, hour, minute, second, 0, httpDateGMT)
	// Reject calendar-invalid dates like "31 Feb", which time.Date normalizes.
	if t.Year() != year || t.Month() != month || t.Day() != day {
		return time.Time{}, false
	}

	return t, true
}

// IsWeekday3 reports whether three-byte sequence a, b, c represents a 3-letter weekday abbreviation.
func IsWeekday3(a, b, c byte) bool {
	a |= 0x20
	b |= 0x20
	c |= 0x20

	k := uint32(a)<<16 | uint32(b)<<8 | uint32(c)
	switch k {
	case uint32('m')<<16 | uint32('o')<<8 | uint32('n'),
		uint32('t')<<16 | uint32('u')<<8 | uint32('e'),
		uint32('w')<<16 | uint32('e')<<8 | uint32('d'),
		uint32('t')<<16 | uint32('h')<<8 | uint32('u'),
		uint32('f')<<16 | uint32('r')<<8 | uint32('i'),
		uint32('s')<<16 | uint32('a')<<8 | uint32('t'),
		uint32('s')<<16 | uint32('u')<<8 | uint32('n'):
		return true
	default:
		return false
	}
}

// Parse2Digits parses two decimal digit bytes a and b into an integer.
func Parse2Digits(a, b byte) (int, bool) {
	if a < '0' || a > '9' || b < '0' || b > '9' {
		return 0, false
	}

	return int(a-'0')*10 + int(b-'0'), true
}

// Parse4Digits parses four decimal digit bytes a, b, c, and d into an integer.
func Parse4Digits(a, b, c, d byte) (int, bool) {
	v1, ok := Parse2Digits(a, b)
	if !ok {
		return 0, false
	}

	v2, ok := Parse2Digits(c, d)
	if !ok {
		return 0, false
	}

	return v1*100 + v2, true
}

// ParseMonth3 parses three-byte sequence a, b, c into a [time.Month].
func ParseMonth3(a, b, c byte) (time.Month, bool) {
	a |= 0x20
	b |= 0x20
	c |= 0x20

	k := uint32(a)<<16 | uint32(b)<<8 | uint32(c)
	switch k {
	case uint32('j')<<16 | uint32('a')<<8 | uint32('n'):
		return time.January, true
	case uint32('f')<<16 | uint32('e')<<8 | uint32('b'):
		return time.February, true
	case uint32('m')<<16 | uint32('a')<<8 | uint32('r'):
		return time.March, true
	case uint32('a')<<16 | uint32('p')<<8 | uint32('r'):
		return time.April, true
	case uint32('m')<<16 | uint32('a')<<8 | uint32('y'):
		return time.May, true
	case uint32('j')<<16 | uint32('u')<<8 | uint32('n'):
		return time.June, true
	case uint32('j')<<16 | uint32('u')<<8 | uint32('l'):
		return time.July, true
	case uint32('a')<<16 | uint32('u')<<8 | uint32('g'):
		return time.August, true
	case uint32('s')<<16 | uint32('e')<<8 | uint32('p'):
		return time.September, true
	case uint32('o')<<16 | uint32('c')<<8 | uint32('t'):
		return time.October, true
	case uint32('n')<<16 | uint32('o')<<8 | uint32('v'):
		return time.November, true
	case uint32('d')<<16 | uint32('e')<<8 | uint32('c'):
		return time.December, true
	}

	return 0, false
}

// ParseCookieExpires parses an expiration date from cookie header bytes.
func ParseCookieExpires(src []byte) (time.Time, error) {
	if t, ok := ParseRFC1123DateGMT(src); ok {
		return t, nil
	}

	s := bytesconv.B2S(src)

	// UTC-anchored RFC1123 parsing behavior for non-GMT.
	t, err := time.ParseInLocation(http.TimeFormat, s, time.UTC)
	if err == nil {
		return t, nil
	}

	// Legacy cookie date compatibility used by net/http.
	return time.Parse("Mon, 02-Jan-2006 15:04:05 MST", s)
}

func appendUint(dst []byte, n int) []byte {
	if n < 0 {
		panic("BUG: int must be positive")
	}

	return strconv.AppendUint(dst, uint64(n), 10)
}

func parseUint(b []byte) (int, error) {
	if len(b) == 0 {
		return -1, errors.New("empty integer")
	}

	v := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return -1, errors.New("unexpected char")
		}

		v = 10*v + int(c-'0')
	}

	return v, nil
}
