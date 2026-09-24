// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"bytes"
	"errors"
	"testing"

	"github.com/lemon4ksan/foundation/borrow"
)

func TestURIPoolAndReset(t *testing.T) {
	u := AcquireURI()
	if u == nil {
		t.Fatal("AcquireURI returned nil")
	}

	u.SetScheme("https")
	u.SetHost("example.com:8443")
	u.SetPath("/api/v1")
	u.SetQueryString("k=v")
	u.SetHash("sec")
	u.SetUsername("admin")
	u.SetPassword("pass")
	u.DisablePathNormalizing = true

	ReleaseURI(u)

	u2 := AcquireURI()
	if len(u2.Host()) != 0 || len(u2.QueryString()) != 0 || len(u2.Hash()) != 0 ||
		len(u2.Username()) != 0 || len(u2.Password()) != 0 || u2.DisablePathNormalizing {
		t.Fatal("reacquired URI was not properly reset")
	}
	if string(u2.Scheme()) != "http" || string(u2.Path()) != "/" {
		t.Fatalf("unexpected defaults on reset URI: scheme=%s, path=%s", u2.Scheme(), u2.Path())
	}
	ReleaseURI(u2)
}

func TestURICopyTo(t *testing.T) {
	src := AcquireURI()
	defer ReleaseURI(src)

	raw := []byte("https://user:secret@example.com:8443/api/v1/resource?id=123&action=update#details")
	if err := src.Parse(nil, raw); err != nil {
		t.Fatalf("src.Parse failed: %v", err)
	}
	src.DisablePathNormalizing = true

	dst := AcquireURI()
	defer ReleaseURI(dst)

	src.CopyTo(dst)

	if !bytes.Equal(dst.Scheme(), src.Scheme()) ||
		!bytes.Equal(dst.Host(), src.Host()) ||
		!bytes.Equal(dst.Path(), src.Path()) ||
		!bytes.Equal(dst.PathOriginal(), src.PathOriginal()) ||
		!bytes.Equal(dst.QueryString(), src.QueryString()) ||
		!bytes.Equal(dst.Hash(), src.Hash()) ||
		!bytes.Equal(dst.Username(), src.Username()) ||
		!bytes.Equal(dst.Password(), src.Password()) ||
		dst.DisablePathNormalizing != src.DisablePathNormalizing {
		t.Fatal("CopyTo failed to copy all fields accurately")
	}

	// Mutate src and verify dst remains unaffected
	src.SetHost("mutated.com")
	if bytes.Equal(dst.Host(), []byte("mutated.com")) {
		t.Fatal("CopyTo did not produce deep copy of host")
	}
}

func TestURIAccessorsAndMutators(t *testing.T) {
	u := AcquireURI()
	defer ReleaseURI(u)

	// Hash
	u.SetHash("sec1")
	if string(u.Hash()) != "sec1" {
		t.Fatalf("SetHash failed: %s", u.Hash())
	}
	u.SetHashBytes([]byte("sec2"))
	if string(u.Hash()) != "sec2" {
		t.Fatalf("SetHashBytes failed: %s", u.Hash())
	}

	// Username
	u.SetUsername("user1")
	if string(u.Username()) != "user1" {
		t.Fatalf("SetUsername failed: %s", u.Username())
	}
	u.SetUsernameBytes([]byte("user2"))
	if string(u.Username()) != "user2" {
		t.Fatalf("SetUsernameBytes failed: %s", u.Username())
	}

	// Password
	u.SetPassword("pwd1")
	if string(u.Password()) != "pwd1" {
		t.Fatalf("SetPassword failed: %s", u.Password())
	}
	u.SetPasswordBytes([]byte("pwd2"))
	if string(u.Password()) != "pwd2" {
		t.Fatalf("SetPasswordBytes failed: %s", u.Password())
	}

	// QueryString
	u.SetQueryString("a=1&b=2")
	if string(u.QueryString()) != "a=1&b=2" {
		t.Fatalf("SetQueryString failed: %s", u.QueryString())
	}
	u.SetQueryStringBytes([]byte("x=10"))
	if string(u.QueryString()) != "x=10" {
		t.Fatalf("SetQueryStringBytes failed: %s", u.QueryString())
	}

	// Path and PathOriginal
	u.SetPath("/a/b/../c")
	if string(u.Path()) != "/a/c" {
		t.Fatalf("SetPath failed: %s", u.Path())
	}
	if string(u.PathOriginal()) != "/a/b/../c" {
		t.Fatalf("PathOriginal failed: %s", u.PathOriginal())
	}
	u.SetPathBytes([]byte("/x//y/./z"))
	if string(u.Path()) != "/x/y/z" {
		t.Fatalf("SetPathBytes failed: %s", u.Path())
	}

	// Scheme and HTTP/HTTPS predicates
	u.SetScheme("HTTP")
	if string(u.Scheme()) != "http" || !u.IsHTTP() || u.IsHTTPS() {
		t.Fatalf("SetScheme HTTP failed: %s", u.Scheme())
	}
	u.SetSchemeBytes([]byte("HTTPS"))
	if string(u.Scheme()) != "https" || !u.IsHTTPS() || u.IsHTTP() {
		t.Fatalf("SetSchemeBytes HTTPS failed: %s", u.Scheme())
	}

	// Host
	u.SetHost("EXAMPLE.COM:8080")
	if string(u.Host()) != "example.com:8080" {
		t.Fatalf("SetHost failed: %s", u.Host())
	}
	u.SetHostBytes([]byte("SUB.EXAMPLE.COM:9000"))
	if string(u.Host()) != "sub.example.com:9000" {
		t.Fatalf("SetHostBytes failed: %s", u.Host())
	}
}

func TestURIParseMatrix(t *testing.T) {
	u := AcquireURI()
	defer ReleaseURI(u)

	// 1. Control character rejection
	if err := u.Parse(nil, []byte("http://example.com/bad\x00path")); !errors.Is(err, ErrorInvalidURI) {
		t.Fatalf("expected ErrorInvalidURI for control char, got %v", err)
	}

	// 2. Full absolute URL with user, pass, port, query, fragment
	raw := []byte("https://john:doe@api.example.com:8443/v1/users?page=2&limit=50#section1")
	if err := u.Parse(nil, raw); err != nil {
		t.Fatalf("Parse absolute failed: %v", err)
	}
	if string(u.Scheme()) != "https" ||
		string(u.Username()) != "john" ||
		string(u.Password()) != "doe" ||
		string(u.Host()) != "api.example.com:8443" ||
		string(u.Path()) != "/v1/users" ||
		string(u.QueryString()) != "page=2&limit=50" ||
		string(u.Hash()) != "section1" {
		t.Fatalf("unexpected parsed absolute URI: %s", u.String())
	}

	// 3. User without password
	if err := u.Parse(nil, []byte("http://user@example.com/")); err != nil {
		t.Fatalf("Parse user only failed: %v", err)
	}
	if string(u.Username()) != "user" || len(u.Password()) != 0 {
		t.Fatalf("expected user with empty pass, got %s:%s", u.Username(), u.Password())
	}

	// 4. Invalid userinfo
	if err := u.Parse(nil, []byte("http://user^bad@example.com/")); !errors.Is(err, ErrorInvalidURI) {
		t.Fatalf("expected ErrorInvalidURI on invalid userinfo, got %v", err)
	}

	// 5. Host passed separately, URI is requestURI
	if err := u.Parse([]byte("separate.com:80"), []byte("/test?k=v")); err != nil {
		t.Fatalf("Parse with separate host failed: %v", err)
	}
	if string(u.Host()) != "separate.com:80" || string(u.Path()) != "/test" || string(u.QueryString()) != "k=v" {
		t.Fatalf("unexpected separate host parse: %s", u.String())
	}

	// 6. Schemeless authority //host/path
	if err := u.Parse(nil, []byte("//schemeless.com/path")); err != nil {
		t.Fatalf("Parse schemeless failed: %v", err)
	}
	if string(u.Scheme()) != "http" || string(u.Host()) != "schemeless.com" || string(u.Path()) != "/path" {
		t.Fatalf("unexpected schemeless parse: %s", u.String())
	}

	// 7. Invalid scheme (starting with digit or containing illegal chars)
	if err := u.Parse(nil, []byte("123bad://example.com/")); err == nil {
		t.Fatal("expected error on invalid scheme starting with digit")
	}

	// 8. TLS flag
	if err := u.ParseInternal(nil, []byte("http://example.com/"), true); err != nil {
		t.Fatalf("ParseInternal TLS failed: %v", err)
	}
	if !u.IsHTTPS() {
		t.Fatal("expected HTTPS when isTLS=true")
	}

	// 9. Path with only fragment (no query)
	if err := u.Parse(nil, []byte("http://example.com/path#fragmentOnly")); err != nil {
		t.Fatalf("Parse fragment only failed: %v", err)
	}
	if string(u.Path()) != "/path" || string(u.Hash()) != "fragmentOnly" || len(u.QueryString()) != 0 {
		t.Fatalf("unexpected fragment-only parse: %s", u.String())
	}

	// 10. Query inside fragment must be ignored
	if err := u.Parse(nil, []byte("http://example.com/path#frag?notquery=1")); err != nil {
		t.Fatalf("Parse frag with query failed: %v", err)
	}
	if len(u.QueryString()) != 0 || string(u.Hash()) != "frag?notquery=1" {
		t.Fatalf("expected empty query and hash with query, got q=%s h=%s", u.QueryString(), u.Hash())
	}

	// 11. IPv6 host with port
	if err := u.Parse(nil, []byte("http://[::1]:8080/v1")); err != nil {
		t.Fatalf("Parse IPv6 failed: %v", err)
	}
	if string(u.Host()) != "[::1]:8080" {
		t.Fatalf("expected host [::1]:8080, got %s", u.Host())
	}

	// 12. IPv6 scoped address with zone identifier %25
	if err := u.Parse(nil, []byte("http://[fe80::1%25eth0]:80/")); err != nil {
		t.Fatalf("Parse scoped IPv6 failed: %v", err)
	}
	if string(u.Host()) != "[fe80::1%eth0]:80" {
		t.Fatalf("expected unescaped scoped host [fe80::1%%eth0]:80, got %s", u.Host())
	}

	// 13. IPv6 missing ']' bracket
	if err := u.Parse(nil, []byte("http://[fe80::1/")); err == nil {
		t.Fatal("expected error for missing ']' in IPv6 host")
	}

	// 14. IPv6 invalid port
	if err := u.Parse(nil, []byte("http://[::1]:badport/")); err == nil {
		t.Fatal("expected error for invalid port after IPv6")
	}

	// 15. Invalid non-bracket host containing brackets
	if err := u.Parse(nil, []byte("http://example[dot]com/")); err == nil {
		t.Fatal("expected error on host with stray brackets")
	}

	// 16. Host with multiple port delimiters
	if err := u.Parse(nil, []byte("http://example.com:80:90/")); err == nil {
		t.Fatal("expected error on host with multiple port delimiters")
	}

	// 17. Host with invalid port
	if err := u.Parse(nil, []byte("http://example.com:xyz/")); err == nil {
		t.Fatal("expected error on host with non-numeric port")
	}
}

func TestURINormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "/"},
		{"foo", "/foo"},
		{"/foo/bar", "/foo/bar"},
		{"///foo///bar//", "/foo/bar/"},
		{"/foo/./bar", "/foo/bar"},
		{"/foo/bar/../baz", "/foo/baz"},
		{"/foo/bar/..", "/foo/"},
		{"/../../foo", "/foo"},
		{"/a/b/c/../../d", "/a/d"},
		{"/foo%20bar/test", "/foo bar/test"},
	}

	for _, tc := range tests {
		got := string(NormalizePath(nil, []byte(tc.input)))
		if got != tc.expected {
			t.Fatalf("NormalizePath(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestURIRequestURI(t *testing.T) {
	u := AcquireURI()
	defer ReleaseURI(u)

	// Normalizing enabled
	_ = u.Parse(nil, []byte("http://example.com/foo/./bar?a=1&b=2"))
	reqURI := string(u.RequestURI())
	if reqURI != "/foo/bar?a=1&b=2" {
		t.Fatalf("RequestURI = %s, want /foo/bar?a=1&b=2", reqURI)
	}

	// Normalizing disabled
	u.Reset()
	_ = u.Parse(nil, []byte("http://example.com/foo/./bar?a=1"))
	u.DisablePathNormalizing = true
	reqURI2 := string(u.RequestURI())
	if reqURI2 != "/foo/./bar?a=1" {
		t.Fatalf("RequestURI with DisablePathNormalizing = %s, want /foo/./bar?a=1", reqURI2)
	}

	// RequestURI using QueryArgs
	u.Reset()
	_ = u.Parse(nil, []byte("http://example.com/test?initial=1"))
	u.QueryArgs().Set("dynamic", "true")
	reqURI3 := string(u.RequestURI())
	if !bytes.Contains([]byte(reqURI3), []byte("dynamic=true")) {
		t.Fatalf("RequestURI after QueryArgs modification = %s", reqURI3)
	}
}

func TestURILastPathSegment(t *testing.T) {
	u := AcquireURI()
	defer ReleaseURI(u)

	u.SetPath("/foo/bar/baz.html")
	if string(u.LastPathSegment()) != "baz.html" {
		t.Fatalf("LastPathSegment failed: %s", u.LastPathSegment())
	}

	u.SetPath("/foo/bar/")
	if string(u.LastPathSegment()) != "" {
		t.Fatalf("LastPathSegment on trailing slash failed: %s", u.LastPathSegment())
	}

	u.SetPath("/foobar.js")
	if string(u.LastPathSegment()) != "foobar.js" {
		t.Fatalf("LastPathSegment failed: %s", u.LastPathSegment())
	}
}

func TestURIUpdate(t *testing.T) {
	u := AcquireURI()
	defer ReleaseURI(u)

	_ = u.Parse(nil, []byte("http://example.com/v1/users?active=1#intro"))

	// Empty update returns current buffer
	u.Update("")
	if string(u.Host()) != "example.com" {
		t.Fatal("empty Update changed URI")
	}

	// Absolute URI replacement
	u.Update("https://newsite.org:9000/v2/items?page=1#top")
	if string(u.Scheme()) != "https" || string(u.Host()) != "newsite.org:9000" ||
		string(u.Path()) != "/v2/items" || string(u.QueryString()) != "page=1" || string(u.Hash()) != "top" {
		t.Fatalf("Update absolute failed: %s", u.String())
	}

	// Absolute without scheme (preserves https scheme)
	u.Update("//schemeless.net/catalog")
	if string(u.Scheme()) != "https" || string(u.Host()) != "schemeless.net" || string(u.Path()) != "/catalog" {
		t.Fatalf("Update schemeless failed: %s", u.String())
	}

	// Path only replacement
	u.Update("/newpath?limit=10")
	if string(u.Host()) != "schemeless.net" || string(u.Path()) != "/newpath" || string(u.QueryString()) != "limit=10" {
		t.Fatalf("Update path-only failed: %s", u.String())
	}

	// Query-only replacement
	u.Update("?newquery=100")
	if string(u.Path()) != "/newpath" || string(u.QueryString()) != "newquery=100" {
		t.Fatalf("Update query-only failed: %s", u.String())
	}

	// Hash-only replacement
	u.Update("#newhash")
	if string(u.Hash()) != "newhash" {
		t.Fatalf("Update hash-only failed: %s", u.String())
	}

	// Relative path replacement
	u.Update("details.html")
	if string(u.Path()) != "/details.html" {
		t.Fatalf("Update relative path failed: %s", u.String())
	}
}

func TestURISerializationAndWriting(t *testing.T) {
	u := AcquireURI()
	defer ReleaseURI(u)

	raw := []byte("http://example.com/foo/bar?baz=1#hash")
	_ = u.Parse(nil, raw)

	full := u.FullURI()
	if !bytes.Equal(full, raw) {
		t.Fatalf("FullURI = %s, want %s", full, raw)
	}
	if u.String() != string(raw) {
		t.Fatalf("String() = %s, want %s", u.String(), raw)
	}

	var buf bytes.Buffer
	n, err := u.WriteTo(&buf)
	if err != nil || n != int64(len(raw)) || !bytes.Equal(buf.Bytes(), raw) {
		t.Fatalf("WriteTo failed: %d, %v, %s", n, err, buf.String())
	}
}

func TestURISplitHostURI(t *testing.T) {
	// Scheme with //
	scheme, host, uri := SplitHostURI(nil, []byte("https://example.com/path"))
	if string(scheme) != "https" || string(host) != "example.com" || string(uri) != "/path" {
		t.Fatalf("SplitHostURI https failed: %s, %s, %s", scheme, host, uri)
	}

	// No // in URI
	scheme2, host2, uri2 := SplitHostURI([]byte("host.com"), []byte("/onlypath"))
	if string(scheme2) != "http" || string(host2) != "host.com" || string(uri2) != "/onlypath" {
		t.Fatalf("SplitHostURI no // failed: %s, %s, %s", scheme2, host2, uri2)
	}

	// Query hack foobar.com?a=b/xyz
	_, host3, uri3 := SplitHostURI(nil, []byte("http://foobar.com?a=b/xyz"))
	if string(host3) != "foobar.com" || string(uri3) != "?a=b/xyz" {
		t.Fatalf("SplitHostURI query hack failed: %s, %s", host3, uri3)
	}

	// Hash hack foobar.com#abc.com
	_, host4, uri4 := SplitHostURI(nil, []byte("http://foobar.com#abc"))
	if string(host4) != "foobar.com" || string(uri4) != "#abc" {
		t.Fatalf("SplitHostURI hash hack failed: %s, %s", host4, uri4)
	}

	// Host without path
	scheme5, host5, uri5 := SplitHostURI(nil, []byte("http://nopath.com"))
	if string(scheme5) != "http" || string(host5) != "nopath.com" || string(uri5) != "/" {
		t.Fatalf("SplitHostURI no path failed: %s, %s, %s", scheme5, host5, uri5)
	}
}

func TestURIHelpersAndEdgeCases(t *testing.T) {
	// StringContainsCTLByte
	if !StringContainsCTLByte([]byte("hello\x01world")) {
		t.Fatal("expected true for CTL byte \\x01")
	}
	if !StringContainsCTLByte([]byte("hello\x7fworld")) {
		t.Fatal("expected true for CTL byte \\x7f")
	}
	if StringContainsCTLByte([]byte("clean ascii string 123 !@#")) {
		t.Fatal("expected false for clean string")
	}

	// IsValidScheme
	validSchemes := []string{"http", "https", "ftp", "custom-scheme+v1.0"}
	for _, s := range validSchemes {
		if !IsValidScheme([]byte(s)) {
			t.Fatalf("expected IsValidScheme=true for %q", s)
		}
	}
	invalidSchemes := []string{"", "1http", "http$", "ht tp"}
	for _, s := range invalidSchemes {
		if IsValidScheme([]byte(s)) {
			t.Fatalf("expected IsValidScheme=false for %q", s)
		}
	}

	// isAuthorityDelimiter
	if !isAuthorityDelimiter([]byte("//host"), 0) {
		t.Fatal("expected isAuthorityDelimiter=true at 0")
	}
	if !isAuthorityDelimiter([]byte("http://host"), 5) {
		t.Fatal("expected isAuthorityDelimiter=true for http://")
	}
	if isAuthorityDelimiter([]byte("path//sub"), 4) {
		t.Fatal("expected isAuthorityDelimiter=false for path//sub")
	}

	// validOptionalPort
	if !validOptionalPort(nil) || !validOptionalPort([]byte(":8080")) || !validOptionalPort([]byte("")) {
		t.Fatal("validOptionalPort rejected valid ports")
	}
	if validOptionalPort([]byte("8080")) || validOptionalPort([]byte(":80a")) {
		t.Fatal("validOptionalPort accepted invalid ports")
	}

	// Error types
	escErr := EscapeError("%ZZ")
	if escErr.Error() == "" {
		t.Fatal("EscapeError empty string")
	}
	invHostErr := InvalidHostError("^")
	if invHostErr.Error() == "" {
		t.Fatal("InvalidHostError empty string")
	}

	// ishex and unhex
	if !ishex('0') || !ishex('a') || !ishex('F') || ishex('g') || ishex('Z') {
		t.Fatal("ishex failed")
	}
	if unhex('a') != 10 || unhex('F') != 15 || unhex('9') != 9 {
		t.Fatal("unhex failed")
	}

	// unescape errors
	if _, err := unescape([]byte("%2"), encodeHost); err == nil {
		t.Fatal("expected error on truncated % escape")
	}
	if _, err := unescape([]byte("%2g"), encodeHost); err == nil {
		t.Fatal("expected error on non-hex % escape")
	}
	if _, err := unescape([]byte("%20"), encodeHost); err == nil {
		t.Fatal("expected error on host percent escape with ASCII < 8")
	}
	if _, err := unescape([]byte("^"), encodeHost); err == nil {
		t.Fatal("expected error on invalid host character")
	}
}

func TestURIScopedBorrowing(t *testing.T) {
	u := AcquireURI()
	defer ReleaseURI(u)

	s := borrow.NewScope()
	defer s.Release()

	// Empty URI borrowing
	u.Reset()
	if len(u.UsernameScoped(s).Bytes()) != 0 ||
		len(u.PasswordScoped(s).Bytes()) != 0 ||
		len(u.HashScoped(s).Bytes()) != 0 {
		t.Fatal("expected empty borrow for unset fields")
	}

	// Populated URI borrowing
	raw := []byte("https://alice:pass123@cluster.internal:9000/metrics?format=json#stats")
	_ = u.Parse(nil, raw)

	if !bytes.Equal(u.PathScoped(s).Bytes(), []byte("/metrics")) {
		t.Fatal("PathScoped failed")
	}
	if !bytes.Equal(u.QueryScoped(s).Bytes(), []byte("format=json")) {
		t.Fatal("QueryScoped failed")
	}
	if !bytes.Equal(u.HostScoped(s).Bytes(), []byte("cluster.internal:9000")) {
		t.Fatal("HostScoped failed")
	}
	if !bytes.Equal(u.SchemeScoped(s).Bytes(), []byte("https")) {
		t.Fatal("SchemeScoped failed")
	}
	if !bytes.Equal(u.UsernameScoped(s).Bytes(), []byte("alice")) {
		t.Fatal("UsernameScoped failed")
	}
	if !bytes.Equal(u.PasswordScoped(s).Bytes(), []byte("pass123")) {
		t.Fatal("PasswordScoped failed")
	}
	if !bytes.Equal(u.HashScoped(s).Bytes(), []byte("stats")) {
		t.Fatal("HashScoped failed")
	}
	if !bytes.Equal(u.RequestURIScoped(s).Bytes(), []byte("/metrics?format=json")) {
		t.Fatal("RequestURIScoped failed")
	}
	if !bytes.Equal(u.FullURIScoped(s).Bytes(), u.FullURI()) {
		t.Fatal("FullURIScoped failed")
	}
}
