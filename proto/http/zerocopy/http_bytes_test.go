// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"bufio"
	"bytes"
	"errors"
	"math"
	"net"
	"testing"
	"time"
)

func TestAppendHTMLEscape(t *testing.T) {
	input := "<b>\"Tom & Jerry's\"</b>"
	expected := "&lt;b&gt;&#34;Tom &amp; Jerry&#39;s&#34;&lt;/b&gt;"

	res := string(AppendHTMLEscape(nil, input))
	if res != expected {
		t.Fatalf("AppendHTMLEscape = %q, want %q", res, expected)
	}

	resBytes := string(AppendHTMLEscapeBytes(nil, []byte(input)))
	if resBytes != expected {
		t.Fatalf("AppendHTMLEscapeBytes = %q, want %q", resBytes, expected)
	}

	// Clean input
	clean := "clean text 123"
	if string(AppendHTMLEscape(nil, clean)) != clean {
		t.Fatalf("AppendHTMLEscape on clean text changed")
	}
}

func TestIPv4Conversions(t *testing.T) {
	// AppendIPv4
	ip := net.IPv4(192, 168, 1, 100)
	buf := AppendIPv4(nil, ip)
	if string(buf) != "192.168.1.100" {
		t.Fatalf("AppendIPv4 = %q, want 192.168.1.100", buf)
	}

	// Non-IPv4 passed to AppendIPv4
	nonV4 := AppendIPv4(nil, net.IP{1, 2, 3})
	if string(nonV4) != "non-v4 ip passed to AppendIPv4" {
		t.Fatalf("AppendIPv4 non-v4 = %q", nonV4)
	}

	// ParseIPv4
	parsed, err := ParseIPv4(nil, []byte("10.0.0.1"))
	if err != nil || !parsed.Equal(net.IPv4(10, 0, 0, 1)) {
		t.Fatalf("ParseIPv4 10.0.0.1 failed: %v, %v", parsed, err)
	}

	// ParseIPv4 with preallocated dst
	dst := make(net.IP, net.IPv4len)
	parsed2, err := ParseIPv4(dst, []byte("255.255.255.0"))
	if err != nil || !parsed2.Equal(net.IPv4(255, 255, 255, 0)) {
		t.Fatalf("ParseIPv4 with preallocated dst failed: %v, %v", parsed2, err)
	}

	// ParseIPv4 error cases
	if _, err := ParseIPv4(nil, nil); !errors.Is(err, errEmptyIPStr) {
		t.Fatalf("expected errEmptyIPStr, got %v", err)
	}
	if _, err := ParseIPv4(nil, []byte("192.168.1")); err == nil {
		t.Fatal("expected error on missing dot")
	}
	if _, err := ParseIPv4(nil, []byte("192.168.1.999")); err == nil {
		t.Fatal("expected error on octet > 255")
	}
	if _, err := ParseIPv4(nil, []byte("192.168.1.abc")); err == nil {
		t.Fatal("expected error on non-digit octet")
	}
	if _, err := ParseIPv4(nil, []byte("999.1.1.1")); err == nil {
		t.Fatal("expected error on first octet > 255")
	}
}

func TestHTTPDateConversions(t *testing.T) {
	// Mon, 02 Jan 2006 15:04:05 GMT
	tm := time.Date(2026, time.September, 21, 14, 30, 0, 0, time.UTC)
	formatted := AppendHTTPDate(nil, tm)
	if !bytes.HasSuffix(formatted, []byte("GMT")) {
		t.Fatalf("AppendHTTPDate missing GMT suffix: %s", formatted)
	}

	parsed, err := ParseHTTPDate(formatted)
	if err != nil || !parsed.Equal(tm) {
		t.Fatalf("ParseHTTPDate failed: %v, %v", parsed, err)
	}

	// ParseRFC1123DateGMT errors
	// Invalid length
	if _, ok := ParseRFC1123DateGMT([]byte("short")); ok {
		t.Fatal("expected false for short date")
	}
	// Invalid weekday
	if _, ok := ParseRFC1123DateGMT([]byte("Xyz, 02 Jan 2006 15:04:05 GMT")); ok {
		t.Fatal("expected false for invalid weekday")
	}
	// Missing commas or colons
	if _, ok := ParseRFC1123DateGMT([]byte("Mon- 02 Jan 2006 15:04:05 GMT")); ok {
		t.Fatal("expected false for missing comma")
	}
	// Missing GMT
	if _, ok := ParseRFC1123DateGMT([]byte("Mon, 02 Jan 2006 15:04:05 UTC")); ok {
		t.Fatal("expected false for UTC suffix")
	}
	// Invalid day
	if _, ok := ParseRFC1123DateGMT([]byte("Mon, 00 Jan 2006 15:04:05 GMT")); ok {
		t.Fatal("expected false for day 00")
	}
	if _, ok := ParseRFC1123DateGMT([]byte("Mon, 32 Jan 2006 15:04:05 GMT")); ok {
		t.Fatal("expected false for day 32")
	}
	// Invalid month
	if _, ok := ParseRFC1123DateGMT([]byte("Mon, 02 Xxx 2006 15:04:05 GMT")); ok {
		t.Fatal("expected false for invalid month")
	}
	// Invalid year
	if _, ok := ParseRFC1123DateGMT([]byte("Mon, 02 Jan 200x 15:04:05 GMT")); ok {
		t.Fatal("expected false for invalid year")
	}
	// Invalid hour, minute, second
	if _, ok := ParseRFC1123DateGMT([]byte("Mon, 02 Jan 2006 24:04:05 GMT")); ok {
		t.Fatal("expected false for hour 24")
	}
	if _, ok := ParseRFC1123DateGMT([]byte("Mon, 02 Jan 2006 15:60:05 GMT")); ok {
		t.Fatal("expected false for minute 60")
	}
	if _, ok := ParseRFC1123DateGMT([]byte("Mon, 02 Jan 2006 15:04:60 GMT")); ok {
		t.Fatal("expected false for second 60")
	}
	// Feb 31 calendar invalid
	if _, ok := ParseRFC1123DateGMT([]byte("Mon, 31 Feb 2006 15:04:05 GMT")); ok {
		t.Fatal("expected false for Feb 31")
	}
}

func TestDateHelpers(t *testing.T) {
	// IsWeekday3
	weekdays := []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun", "MON", "Wed"}
	for _, w := range weekdays {
		if !IsWeekday3(w[0], w[1], w[2]) {
			t.Fatalf("expected true for weekday %s", w)
		}
	}
	if IsWeekday3('f', 'o', 'o') {
		t.Fatal("expected false for 'foo'")
	}

	// Parse2Digits & Parse4Digits
	v, ok := Parse2Digits('4', '2')
	if !ok || v != 42 {
		t.Fatalf("Parse2Digits failed: %d, %v", v, ok)
	}
	if _, ok := Parse2Digits('a', '2'); ok {
		t.Fatal("expected false on non-digit")
	}

	v4, ok := Parse4Digits('2', '0', '2', '6')
	if !ok || v4 != 2026 {
		t.Fatalf("Parse4Digits failed: %d, %v", v4, ok)
	}
	if _, ok := Parse4Digits('2', '0', 'a', '6'); ok {
		t.Fatal("expected false on non-digit in Parse4Digits")
	}
	if _, ok := Parse4Digits('x', '0', '2', '6'); ok {
		t.Fatal("expected false on non-digit in Parse4Digits first pair")
	}

	// ParseMonth3
	allMonths := []struct {
		str   string
		month time.Month
	}{
		{"jan", time.January},
		{"feb", time.February},
		{"mar", time.March},
		{"apr", time.April},
		{"may", time.May},
		{"jun", time.June},
		{"jul", time.July},
		{"aug", time.August},
		{"sep", time.September},
		{"oct", time.October},
		{"nov", time.November},
		{"dec", time.December},
	}
	for _, m := range allMonths {
		got, ok := ParseMonth3(m.str[0], m.str[1], m.str[2])
		if !ok || got != m.month {
			t.Fatalf("ParseMonth3(%s) = %v, want %v", m.str, got, m.month)
		}
	}
	if _, ok := ParseMonth3('x', 'y', 'z'); ok {
		t.Fatal("expected false for xyz month")
	}
}

func TestAppendUintAndParseUint(t *testing.T) {
	// AppendUint
	res := AppendUint(nil, 42)
	if string(res) != "42" {
		t.Fatalf("AppendUint = %s, want 42", res)
	}

	// Panic on negative
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on negative AppendUint")
		}
	}()
	_ = AppendUint(nil, -5)
}

func TestParseUintEdgeCases(t *testing.T) {
	// Valid
	v, err := ParseUint([]byte("12345"))
	if err != nil || v != 12345 {
		t.Fatalf("ParseUint failed: %d, %v", v, err)
	}

	// Empty
	if _, err := ParseUint(nil); !errors.Is(err, errEmptyInt) {
		t.Fatalf("expected errEmptyInt, got %v", err)
	}

	// Non-digit first char via ParseUintBuf
	if _, _, err := ParseUintBuf([]byte("abc")); !errors.Is(err, errUnexpectedFirstChar) {
		t.Fatalf("expected errUnexpectedFirstChar from ParseUintBuf, got %v", err)
	}

	// Non-digit trailing char via ParseUint
	if _, err := ParseUint([]byte("123a")); !errors.Is(err, errUnexpectedTrailingChar) {
		t.Fatalf("expected errUnexpectedTrailingChar, got %v", err)
	}
	if _, err := ParseUint([]byte("abc")); !errors.Is(err, errUnexpectedTrailingChar) {
		t.Fatalf("expected errUnexpectedTrailingChar for abc, got %v", err)
	}

	// Overflow
	overflowStr := []byte("999999999999999999999999999999")
	if _, _, err := ParseUintBuf(overflowStr); !errors.Is(err, errTooLongInt) {
		t.Fatalf("expected errTooLongInt, got %v", err)
	}
}

func TestParseIPv4Octet(t *testing.T) {
	// Valid
	oct, parsed, err := ParseIPv4Octet([]byte("255"))
	if err != nil || oct != 255 || parsed != 255 {
		t.Fatalf("ParseIPv4Octet failed: %d, %d, %v", oct, parsed, err)
	}

	// Empty
	if _, _, err := ParseIPv4Octet(nil); !errors.Is(err, errEmptyInt) {
		t.Fatalf("expected errEmptyInt, got %v", err)
	}

	// Unexpected first char
	if _, _, err := ParseIPv4Octet([]byte("x1")); !errors.Is(err, errUnexpectedFirstChar) {
		t.Fatalf("expected errUnexpectedFirstChar, got %v", err)
	}

	// Unexpected trailing char
	if _, _, err := ParseIPv4Octet([]byte("1x")); !errors.Is(err, errUnexpectedTrailingChar) {
		t.Fatalf("expected errUnexpectedTrailingChar, got %v", err)
	}

	// Part too large (> 255)
	if _, _, err := ParseIPv4Octet([]byte("256")); !errors.Is(err, errIPv4PartTooLarge) {
		t.Fatalf("expected errIPv4PartTooLarge, got %v", err)
	}
	if _, _, err := ParseIPv4Octet([]byte("300")); !errors.Is(err, errIPv4PartTooLarge) {
		t.Fatalf("expected errIPv4PartTooLarge for 300, got %v", err)
	}
}

func TestParseUfloat(t *testing.T) {
	f, err := ParseUfloat([]byte("3.1415"))
	if err != nil || math.Abs(f-3.1415) > 1e-9 {
		t.Fatalf("ParseUfloat failed: %f, %v", f, err)
	}

	if _, err := ParseUfloat([]byte("-2.5")); err == nil {
		t.Fatal("expected error on negative ufloat")
	}

	if _, err := ParseUfloat([]byte("invalid")); err == nil {
		t.Fatal("expected error on invalid ufloat")
	}
}

func TestHexIntReadWriteAndFormat(t *testing.T) {
	// FormatHexUint
	var buf [16]byte
	n := FormatHexUint(&buf, 0x1A2B)
	if string(buf[:n]) != "1a2b" {
		t.Fatalf("FormatHexUint = %s, want 1a2b", string(buf[:n]))
	}

	nZero := FormatHexUint(&buf, 0)
	if string(buf[:nZero]) != "0" {
		t.Fatalf("FormatHexUint(0) = %s, want 0", string(buf[:nZero]))
	}

	// Panic on negative FormatHexUint
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic on negative FormatHexUint")
			}
		}()
		_ = FormatHexUint(&buf, -1)
	}()

	// WriteHexInt
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	if err := WriteHexInt(w, 255); err != nil {
		t.Fatalf("WriteHexInt failed: %v", err)
	}
	_ = w.Flush()
	if b.String() != "ff" {
		t.Fatalf("WriteHexInt = %s, want ff", b.String())
	}

	// Panic on negative WriteHexInt
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic on negative WriteHexInt")
			}
		}()
		_ = WriteHexInt(w, -1)
	}()

	// ReadHexInt
	r := bufio.NewReader(bytes.NewReader([]byte("ff\r\n")))
	val, err := ReadHexInt(r)
	if err != nil || val != 255 {
		t.Fatalf("ReadHexInt = %d, %v, want 255", val, err)
	}

	// Empty hex num
	rEmpty := bufio.NewReader(bytes.NewReader([]byte("zz")))
	if _, err := ReadHexInt(rEmpty); !errors.Is(err, ErrEmptyHexNum) {
		t.Fatalf("expected ErrEmptyHexNum, got %v", err)
	}

	// Too large hex num
	rTooLarge := bufio.NewReader(bytes.NewReader([]byte("1234567890abcdef1234567890abcdef")))
	if _, err := ReadHexInt(rTooLarge); !errors.Is(err, ErrTooLargeHexNum) {
		t.Fatalf("expected ErrTooLargeHexNum, got %v", err)
	}

	// EOF without delimiter
	rEOF := bufio.NewReader(bytes.NewReader([]byte("2a")))
	valEOF, err := ReadHexInt(rEOF)
	if err != nil || valEOF != 42 {
		t.Fatalf("ReadHexInt on EOF = %d, %v", valEOF, err)
	}
}

func TestByteEncodingAndHelpers(t *testing.T) {
	// LowercaseBytes
	b := []byte("HeLLo WoRLD 123!")
	LowercaseBytes(b)
	if string(b) != "hello world 123!" {
		t.Fatalf("LowercaseBytes failed: %s", b)
	}

	// AppendUnquotedArg
	unquoted := AppendUnquotedArg(nil, []byte("hello+world%21"))
	if string(unquoted) != "hello world!" {
		t.Fatalf("AppendUnquotedArg failed: %s", unquoted)
	}

	// AppendQuotedArg
	quoted := AppendQuotedArg(nil, []byte("hello world!"))
	if !bytes.Contains(quoted, []byte("+")) {
		t.Fatalf("AppendQuotedArg missing +: %s", quoted)
	}

	// AppendQuotedPath
	asterisk := AppendQuotedPath(nil, []byte("*"))
	if string(asterisk) != "*" {
		t.Fatalf("AppendQuotedPath for * failed: %s", asterisk)
	}

	quotedPath := AppendQuotedPath(nil, []byte("/path with spaces/&special"))
	if !bytes.Contains(quotedPath, []byte("%20")) {
		t.Fatalf("AppendQuotedPath missing %%20: %s", quotedPath)
	}

	// B2S and S2B from missing.go
	str := "test-string"
	bs := S2B(str)
	if string(bs) != str {
		t.Fatalf("S2B failed: %s", bs)
	}
	backToStr := B2S(bs)
	if backToStr != str {
		t.Fatalf("B2S failed: %s", backToStr)
	}
}
