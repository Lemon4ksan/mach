// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/borrow"
)

func TestCookiePoolAndReset(t *testing.T) {
	c := AcquireCookie()
	if c == nil {
		t.Fatal("AcquireCookie returned nil")
	}

	c.SetKey("k")
	c.SetValue("v")
	c.SetDomain("example.com")
	c.SetPath("/api")
	c.SetMaxAge(3600)
	c.SetHTTPOnly(true)
	c.SetSecure(true)
	c.SetSameSite(CookieSameSiteStrictMode)
	c.SetPartitioned(true)

	ReleaseCookie(c)

	c2 := AcquireCookie()
	if len(c2.Key()) != 0 || len(c2.Value()) != 0 || len(c2.Domain()) != 0 || len(c2.Path()) != 0 {
		t.Fatal("reacquired Cookie was not properly reset")
	}
	if c2.MaxAge() != 0 || c2.HTTPOnly() || c2.Secure() || c2.SameSite() != CookieSameSiteDisabled || c2.Partitioned() {
		t.Fatal("reacquired Cookie flags were not reset")
	}
	ReleaseCookie(c2)
}

func TestCookieCopyTo(t *testing.T) {
	src := AcquireCookie()
	defer ReleaseCookie(src)

	src.SetKey("my_cookie")
	src.SetValue("secret")
	src.SetDomain("sub.example.com")
	src.SetPath("/admin")
	src.SetMaxAge(7200)
	exp := time.Date(2030, time.June, 15, 12, 0, 0, 0, time.UTC)
	src.SetExpire(exp)
	src.SetHTTPOnly(true)
	src.SetSecure(true)
	src.SetSameSite(CookieSameSiteLaxMode)
	src.SetPartitioned(true)

	dst := AcquireCookie()
	defer ReleaseCookie(dst)

	dst.CopyTo(src)

	if !bytes.Equal(dst.Key(), src.Key()) ||
		!bytes.Equal(dst.Value(), src.Value()) ||
		!bytes.Equal(dst.Domain(), src.Domain()) ||
		!bytes.Equal(dst.Path(), src.Path()) ||
		dst.MaxAge() != src.MaxAge() ||
		!dst.Expire().Equal(src.Expire()) ||
		dst.HTTPOnly() != src.HTTPOnly() ||
		dst.Secure() != src.Secure() ||
		dst.SameSite() != src.SameSite() ||
		dst.Partitioned() != src.Partitioned() {
		t.Fatal("CopyTo failed to copy all fields accurately")
	}

	// Verify deep copy: mutating src does not mutate dst
	src.SetKey("altered")
	if bytes.Equal(dst.Key(), []byte("altered")) {
		t.Fatal("CopyTo did not produce deep copy of key")
	}
}

func TestCookieAttributesAndSanitization(t *testing.T) {
	c := AcquireCookie()
	defer ReleaseCookie(c)

	// Key and Value with semicolons and newlines
	c.SetKey("key;injected\r\npart")
	if bytes.Contains(c.Key(), []byte(";")) || bytes.Contains(c.Key(), []byte("\r")) ||
		bytes.Contains(c.Key(), []byte("\n")) {
		t.Fatalf("SetKey failed to sanitize: %q", c.Key())
	}
	c.SetKeyBytes([]byte("key_bytes;more"))
	if bytes.Contains(c.Key(), []byte(";")) {
		t.Fatalf("SetKeyBytes failed to sanitize: %q", c.Key())
	}

	c.SetValue("val;injected\r\npart")
	if bytes.Contains(c.Value(), []byte(";")) || bytes.Contains(c.Value(), []byte("\r")) ||
		bytes.Contains(c.Value(), []byte("\n")) {
		t.Fatalf("SetValue failed to sanitize: %q", c.Value())
	}
	c.SetValueBytes([]byte("val_bytes;more"))
	if bytes.Contains(c.Value(), []byte(";")) {
		t.Fatalf("SetValueBytes failed to sanitize: %q", c.Value())
	}

	// Domain
	c.SetDomain("example.com;evil.com\r\n")
	if bytes.Contains(c.Domain(), []byte(";")) || bytes.Contains(c.Domain(), []byte("\r")) {
		t.Fatalf("SetDomain failed to sanitize: %q", c.Domain())
	}
	c.SetDomainBytes([]byte("sub.example.com;evil.com"))
	if bytes.Contains(c.Domain(), []byte(";")) {
		t.Fatalf("SetDomainBytes failed to sanitize: %q", c.Domain())
	}

	// Path
	c.SetPath("/a/b/../c;injected\r\n")
	if bytes.Contains(c.Path(), []byte(";")) || bytes.Contains(c.Path(), []byte("\r")) {
		t.Fatalf("SetPath failed to sanitize: %q", c.Path())
	}
	c.SetPathBytes([]byte("/foo/bar;more"))
	if bytes.Contains(c.Path(), []byte(";")) {
		t.Fatalf("SetPathBytes failed to sanitize: %q", c.Path())
	}

	// Flags
	c.SetHTTPOnly(true)
	if !c.HTTPOnly() {
		t.Fatal("HTTPOnly was not true")
	}
	c.SetHTTPOnly(false)
	if c.HTTPOnly() {
		t.Fatal("HTTPOnly was not false")
	}

	c.SetSecure(true)
	if !c.Secure() {
		t.Fatal("Secure was not true")
	}
	c.SetSecure(false)
	if c.Secure() {
		t.Fatal("Secure was not false")
	}

	// SameSite modes
	c.SetSameSite(CookieSameSiteDefaultMode)
	if c.SameSite() != CookieSameSiteDefaultMode {
		t.Fatal("expected DefaultMode")
	}
	c.SetSameSite(CookieSameSiteLaxMode)
	if c.SameSite() != CookieSameSiteLaxMode {
		t.Fatal("expected LaxMode")
	}
	c.SetSameSite(CookieSameSiteStrictMode)
	if c.SameSite() != CookieSameSiteStrictMode {
		t.Fatal("expected StrictMode")
	}

	// Setting SameSiteNoneMode should automatically enable Secure
	c.SetSecure(false)
	c.SetSameSite(CookieSameSiteNoneMode)
	if c.SameSite() != CookieSameSiteNoneMode || !c.Secure() {
		t.Fatal("SameSiteNoneMode did not enforce Secure = true")
	}

	// Partitioned flag auto-enables Secure and sets Path to "/"
	c.SetSecure(false)
	c.SetPath("/custom")
	c.SetPartitioned(true)
	if !c.Partitioned() || !c.Secure() || !bytes.Equal(c.Path(), []byte("/")) {
		t.Fatalf(
			"SetPartitioned(true) failed: partitioned=%v, secure=%v, path=%s",
			c.Partitioned(),
			c.Secure(),
			c.Path(),
		)
	}
	c.SetPartitioned(false)
	if c.Partitioned() {
		t.Fatal("Partitioned should be false")
	}

	// MaxAge and Expire
	c.SetMaxAge(60)
	if c.MaxAge() != 60 {
		t.Fatalf("expected MaxAge 60, got %d", c.MaxAge())
	}

	// Expire default
	c.SetExpire(zeroTime)
	if c.Expire() != CookieExpireUnlimited {
		t.Fatal("expected CookieExpireUnlimited when expire is zeroTime")
	}
	c.SetExpire(CookieExpireDelete)
	if c.Expire() != CookieExpireDelete {
		t.Fatal("expected CookieExpireDelete")
	}
}

func TestCookieSerialization(t *testing.T) {
	c := AcquireCookie()
	defer ReleaseCookie(c)

	// 1. Simple key=value
	c.SetKey("token")
	c.SetValue("12345")
	if string(c.Cookie()) != "token=12345" {
		t.Fatalf("unexpected cookie string: %s", c.Cookie())
	}
	if c.String() != "token=12345" {
		t.Fatalf("String() != token=12345: %s", c.String())
	}

	// WriteTo
	var buf bytes.Buffer
	n, err := c.WriteTo(&buf)
	if err != nil || n != int64(len("token=12345")) || buf.String() != "token=12345" {
		t.Fatalf("WriteTo failed: %d, %v, %s", n, err, buf.String())
	}

	// 2. Cookie with value only (no key)
	c.Reset()
	c.SetValue("only_value")
	if string(c.Cookie()) != "only_value" {
		t.Fatalf("expected 'only_value', got %s", c.Cookie())
	}

	// 3. MaxAge positive and negative
	c.Reset()
	c.SetKey("a")
	c.SetValue("b")
	c.SetMaxAge(100)
	if !bytes.Contains(c.Cookie(), []byte("max-age=100")) {
		t.Fatalf("expected max-age=100, got %s", c.Cookie())
	}

	c.SetMaxAge(-1)
	if !bytes.Contains(c.Cookie(), []byte("max-age=0")) {
		t.Fatalf("expected max-age=0 for negative max-age, got %s", c.Cookie())
	}

	// 4. Expire (when maxAge == 0)
	c.SetMaxAge(0)
	exp := time.Date(2028, time.November, 10, 23, 0, 0, 0, time.UTC)
	c.SetExpire(exp)
	if !bytes.Contains(c.Cookie(), []byte("expires=Fri, 10 Nov 2028 23:00:00 GMT")) {
		t.Fatalf("expected formatted expires date, got %s", c.Cookie())
	}

	// 5. Domain, Path, HttpOnly, Secure, SameSite, Partitioned
	c.SetDomain("test.com")
	c.SetPath("/api")
	c.SetHTTPOnly(true)
	c.SetSecure(true)
	c.SetPartitioned(true)

	// Test each sameSite in serialized output
	c.SetSameSite(CookieSameSiteDefaultMode)
	res := string(c.Cookie())
	if !bytes.Contains([]byte(res), []byte("; SameSite")) || bytes.Contains([]byte(res), []byte("; SameSite=")) {
		t.Fatalf("expected '; SameSite', got %s", res)
	}

	c.SetSameSite(CookieSameSiteLaxMode)
	if !bytes.Contains(c.Cookie(), []byte("; SameSite=Lax")) {
		t.Fatalf("expected SameSite=Lax, got %s", c.Cookie())
	}

	c.SetSameSite(CookieSameSiteStrictMode)
	if !bytes.Contains(c.Cookie(), []byte("; SameSite=Strict")) {
		t.Fatalf("expected SameSite=Strict, got %s", c.Cookie())
	}

	c.SetSameSite(CookieSameSiteNoneMode)
	if !bytes.Contains(c.Cookie(), []byte("; SameSite=None")) {
		t.Fatalf("expected SameSite=None, got %s", c.Cookie())
	}

	if !bytes.Contains(c.Cookie(), []byte("; HttpOnly")) ||
		!bytes.Contains(c.Cookie(), []byte("; secure")) ||
		!bytes.Contains(c.Cookie(), []byte("; domain=test.com")) ||
		!bytes.Contains(c.Cookie(), []byte("; path=/")) ||
		!bytes.Contains(c.Cookie(), []byte("; Partitioned")) {
		t.Fatalf("missing attributes in cookie: %s", c.Cookie())
	}
}

func TestCookieParseAndParseBytes(t *testing.T) {
	c := AcquireCookie()
	defer ReleaseCookie(c)

	// Empty string
	if err := c.Parse(""); !errors.Is(err, ErrNoCookies) {
		t.Fatalf("expected ErrNoCookies on empty, got %v", err)
	}

	// Invalid cookie value with illegal chars
	if err := c.Parse("key=val\"quote"); !errors.Is(err, ErrInvalidCookieValue) {
		t.Fatalf("expected ErrInvalidCookieValue, got %v", err)
	}
	if err := c.Parse("key=val\\backslash"); !errors.Is(err, ErrInvalidCookieValue) {
		t.Fatalf("expected ErrInvalidCookieValue, got %v", err)
	}

	// Full parsing test with all RFC 6265 attributes (mixed case)
	raw := "id=a3fWa; MAX-AGE=3600; EXPIRES=Wed, 21 Oct 2026 07:28:00 GMT; DOMAIN=example.com; PATH=/web; SAMESITE=STRICT; HTTPONLY; SECURE; PARTITIONED"
	if err := c.Parse(raw); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !bytes.Equal(c.Key(), []byte("id")) || !bytes.Equal(c.Value(), []byte("a3fWa")) {
		t.Fatalf("unexpected key/val: %s=%s", c.Key(), c.Value())
	}
	if c.MaxAge() != 3600 {
		t.Fatalf("expected MaxAge 3600, got %d", c.MaxAge())
	}
	if c.Expire().IsZero() {
		t.Fatal("expected non-zero expire time")
	}
	if !bytes.Equal(c.Domain(), []byte("example.com")) {
		t.Fatalf("expected domain example.com, got %s", c.Domain())
	}
	if !bytes.Equal(c.Path(), []byte("/web")) {
		t.Fatalf("expected path /web, got %s", c.Path())
	}
	if c.SameSite() != CookieSameSiteStrictMode {
		t.Fatalf("expected SameSite Strict, got %v", c.SameSite())
	}
	if !c.HTTPOnly() || !c.Secure() || !c.Partitioned() {
		t.Fatal("expected HTTPOnly, Secure, and Partitioned to be true")
	}

	// Test SameSite Lax and None
	if err := c.Parse("foo=bar; SameSite=Lax"); err != nil || c.SameSite() != CookieSameSiteLaxMode {
		t.Fatalf("expected SameSite Lax, got %v, err=%v", c.SameSite(), err)
	}
	if err := c.Parse("foo=bar; SameSite=None"); err != nil || c.SameSite() != CookieSameSiteNoneMode {
		t.Fatalf("expected SameSite None, got %v, err=%v", c.SameSite(), err)
	}
	// SameSite without value in flag position
	if err := c.Parse("foo=bar; SameSite"); err != nil || c.SameSite() != CookieSameSiteDefaultMode {
		t.Fatalf("expected SameSite DefaultMode, got %v, err=%v", c.SameSite(), err)
	}

	// Quoted value
	if err := c.Parse(`name="hello world"`); err != nil {
		t.Fatalf("Parse quoted value failed: %v", err)
	}
	if !bytes.Equal(c.Value(), []byte("hello world")) {
		t.Fatalf("expected unquoted 'hello world', got %q", c.Value())
	}

	// Error cases in parse attributes
	if err := c.Parse("foo=bar; max-age=invalid"); err == nil {
		t.Fatal("expected error on invalid max-age")
	}
	if err := c.Parse("foo=bar; expires=not-a-date"); err == nil {
		t.Fatal("expected error on invalid expires")
	}
	if err := c.Parse("foo=bar; domain=invalid\"char"); !errors.Is(err, ErrInvalidCookieValue) {
		t.Fatalf("expected ErrInvalidCookieValue on invalid domain, got %v", err)
	}
	if err := c.Parse("foo=bar; path=invalid\x01byte"); !errors.Is(err, ErrInvalidCookieValue) {
		t.Fatalf("expected ErrInvalidCookieValue on invalid path, got %v", err)
	}

	// Parse with empty attributes / trailing semicolons
	if err := c.Parse("k=v; ; ;"); err != nil {
		t.Fatalf("Parse with stray semicolons failed: %v", err)
	}
	if !bytes.Equal(c.Key(), []byte("k")) || !bytes.Equal(c.Value(), []byte("v")) {
		t.Fatal("Parse with stray semicolons gave wrong result")
	}
}

func TestCookieRequestAndResponseCookies(t *testing.T) {
	// AppendCookiePart
	part := AppendCookiePart(nil, []byte("Domain"), []byte("example.com"))
	if string(part) != "; Domain=example.com" {
		t.Fatalf("AppendCookiePart failed: %s", part)
	}

	// GetCookieKey
	k1 := GetCookieKey(nil, []byte("session_id=abcdef; other"))
	if string(k1) != "session_id" {
		t.Fatalf("GetCookieKey failed: %s", k1)
	}
	k2 := GetCookieKey(nil, []byte("key_only"))
	if string(k2) != "key_only" {
		t.Fatalf("GetCookieKey for key_only failed: %s", k2)
	}

	// AppendRequestCookieBytes and AppendResponseCookieBytes
	cookies := []ArgsKV{
		{Key: []byte("c1"), Value: []byte("v1")},
		{Key: []byte("c2"), Value: []byte("v2")},
		{Key: nil, Value: []byte("v3")},
	}
	reqBytes := AppendRequestCookieBytes(nil, cookies)
	if string(reqBytes) != "c1=v1; c2=v2; v3" {
		t.Fatalf("AppendRequestCookieBytes failed: %s", reqBytes)
	}

	respBytes := AppendResponseCookieBytes(nil, cookies)
	if string(respBytes) != "v1; v2; v3" {
		t.Fatalf("AppendResponseCookieBytes failed: %s", respBytes)
	}

	// ParseRequestCookies
	parsedKVs := ParseRequestCookies(nil, []byte("cookie1=val1; cookie2=\"quoted val\"; invalid=bad\"val; flag"))
	if len(parsedKVs) < 2 {
		t.Fatalf("ParseRequestCookies parsed too few cookies: %d", len(parsedKVs))
	}
	if !bytes.Equal(parsedKVs[0].Key, []byte("cookie1")) || !bytes.Equal(parsedKVs[0].Value, []byte("val1")) {
		t.Fatalf("parsedKVs[0] mismatch: %s=%s", parsedKVs[0].Key, parsedKVs[0].Value)
	}
	if !bytes.Equal(parsedKVs[1].Key, []byte("cookie2")) || !bytes.Equal(parsedKVs[1].Value, []byte("quoted val")) {
		t.Fatalf("parsedKVs[1] mismatch: %s=%s", parsedKVs[1].Key, parsedKVs[1].Value)
	}

	// ParseRequestCookies on empty
	if len(ParseRequestCookies(nil, nil)) != 0 {
		t.Fatal("expected empty result for nil request cookies")
	}
}

func TestCookieDateParsingFallback(t *testing.T) {
	// Single digit day or non-standard RFC1123 matching http.TimeFormat
	s := "Mon, 02 Jan 2006 15:04:05 GMT"
	tm, err := parseCookieExpires([]byte(s))
	if err != nil || tm.Year() != 2006 {
		t.Fatalf("parseCookieExpires GMT failed: %v, %v", tm, err)
	}

	// Legacy cookie date format: "Mon, 02-Jan-2006 15:04:05 MST"
	legacyStr := "Mon, 02-Jan-2006 15:04:05 GMT"
	tm2, err := parseCookieExpires([]byte(legacyStr))
	if err != nil || tm2.Year() != 2006 {
		t.Fatalf("parseCookieExpires legacy format failed: %v, %v", tm2, err)
	}

	// Garbage date
	_, err = parseCookieExpires([]byte("totally-invalid-date-string"))
	if err == nil {
		t.Fatal("expected error on garbage date")
	}
}

func TestCookieScopedBorrowing(t *testing.T) {
	c := AcquireCookie()
	defer ReleaseCookie(c)

	s := borrow.NewScope()
	defer s.Release()

	// Empty cookie borrowing
	if len(c.KeyScoped(s).Bytes()) != 0 ||
		len(c.ValueScoped(s).Bytes()) != 0 ||
		len(c.DomainScoped(s).Bytes()) != 0 ||
		len(c.PathScoped(s).Bytes()) != 0 ||
		len(c.CookieScoped(s).Bytes()) != 0 {
		t.Fatal("expected empty borrow.Bytes for empty cookie")
	}

	// Populated cookie borrowing
	c.SetKey("session")
	c.SetValue("val123")
	c.SetDomain("example.org")
	c.SetPath("/api")

	if !bytes.Equal(c.KeyScoped(s).Bytes(), []byte("session")) {
		t.Fatal("KeyScoped failed")
	}
	if !bytes.Equal(c.ValueScoped(s).Bytes(), []byte("val123")) {
		t.Fatal("ValueScoped failed")
	}
	if !bytes.Equal(c.DomainScoped(s).Bytes(), []byte("example.org")) {
		t.Fatal("DomainScoped failed")
	}
	if !bytes.Equal(c.PathScoped(s).Bytes(), []byte("/api")) {
		t.Fatal("PathScoped failed")
	}
	if !bytes.Contains(c.CookieScoped(s).Bytes(), []byte("session=val123")) {
		t.Fatal("CookieScoped failed")
	}
}

func TestCookieValidPathValue(t *testing.T) {
	// Legal path chars including quotes and backslashes and CR/LF
	validPath := []byte("/api/v1/\"test\"\\path\r\n")
	if !validCookiePathValue(validPath) {
		t.Fatal("expected validCookiePathValue to accept path with quotes and backslashes")
	}

	// Semicolon is forbidden in path value
	if validCookiePathValue([]byte("/path;semicolon")) {
		t.Fatal("expected validCookiePathValue to reject semicolon")
	}

	// Control character < 0x20 (other than CR/LF) is forbidden
	if validCookiePathValue([]byte("/path/\x05")) {
		t.Fatal("expected validCookiePathValue to reject control chars")
	}

	// Non-ASCII >= 0x7f is forbidden
	if validCookiePathValue([]byte("/path/\x80")) {
		t.Fatal("expected validCookiePathValue to reject non-ascii")
	}
}

func TestTrimCookieArgAndDecodeArg(t *testing.T) {
	// Fast path: already trimmed, not quoted
	res1 := decodeCookieArg(nil, []byte("simple"), true)
	if string(res1) != "simple" {
		t.Fatalf("expected simple, got %s", res1)
	}

	// Leading/trailing spaces
	res2 := decodeCookieArg(nil, []byte("   spaced   "), true)
	if string(res2) != "spaced" {
		t.Fatalf("expected spaced, got %s", res2)
	}

	// Quoted string
	res3 := decodeCookieArg(nil, []byte(`"quoted"`), true)
	if string(res3) != "quoted" {
		t.Fatalf("expected quoted, got %s", res3)
	}

	// Quoted string with skipQuotes = false
	res4 := decodeCookieArg(nil, []byte(`"quoted"`), false)
	if string(res4) != `"quoted"` {
		t.Fatalf("expected \"quoted\", got %s", res4)
	}

	// trimCookieArgNoCopy
	tc1 := trimCookieArgNoCopy([]byte("  val  "), false)
	if string(tc1) != "val" {
		t.Fatalf("trimCookieArgNoCopy failed: %s", tc1)
	}
	tc2 := trimCookieArgNoCopy([]byte(`  "inner"  `), true)
	if string(tc2) != "inner" {
		t.Fatalf("trimCookieArgNoCopy with quotes failed: %s", tc2)
	}
}
