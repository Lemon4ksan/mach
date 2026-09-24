// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit_test

import (
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/headkit"
)

func TestDirectives(t *testing.T) {
	t.Parallel()

	raw := `public, max-age=3600, must-revalidate, stale-while-revalidate=60, custom="val,ue"`

	parsed := maps.Collect(headkit.Directives(raw))

	assert.Equal(t, "", parsed["public"])
	assert.Equal(t, "3600", parsed["max-age"])
	assert.Equal(t, "", parsed["must-revalidate"])
	assert.Equal(t, "60", parsed["stale-while-revalidate"])
	assert.Equal(t, "val,ue", parsed["custom"])

	dm := headkit.ParseDirectives(raw)
	assert.True(t, dm.Has("public"))
	assert.True(t, dm.Bool("public"))
	assert.Equal(t, 3600, dm.Int("max-age", 0))
	assert.Equal(t, 3600*time.Second, dm.Duration("max-age", 0))
	assert.Equal(t, 60*time.Second, dm.Duration("stale-while-revalidate", 0))
	assert.Equal(t, 10, dm.Int("missing", 10))
	assert.Equal(t, 5, dm.Len())

	// Bytes test
	bytesParsed := make(map[string]string)
	for k, v := range headkit.DirectivesBytes([]byte(raw)) {
		bytesParsed[string(k)] = string(v)
	}
	assert.Equal(t, parsed, bytesParsed)
}

func TestParamDirectives(t *testing.T) {
	t.Parallel()

	raw := `h3=":443"; ma=86400; persist=1; custom="quoted;semi"`
	dm := headkit.ParseParamDirectives(raw)

	assert.Equal(t, ":443", dm.Get("h3"))
	assert.Equal(t, 86400, dm.Int("ma", 0))
	assert.True(t, dm.Bool("persist"))
	assert.Equal(t, "quoted;semi", dm.Get("custom"))
}

func TestAcceptAndNegotiate(t *testing.T) {
	t.Parallel()

	acceptHeader := "text/html, application/xhtml+xml, application/xml;q=0.9, image/webp, */*;q=0.8"
	sorted := headkit.SortedAccepts(acceptHeader)

	require.NotEmpty(t, sorted)
	assert.Equal(t, "text/html", sorted[0].Value)
	assert.Equal(t, float32(1.0), sorted[0].Q)

	// Content negotiation
	offers := []string{"application/json", "text/html", "application/xml"}
	chosen := headkit.Negotiate(acceptHeader, offers)
	assert.Equal(t, "text/html", chosen)

	// Fallback negotiation
	offers2 := []string{"application/json", "application/xml"}
	chosen2 := headkit.Negotiate(acceptHeader, offers2)
	assert.Equal(t, "application/xml", chosen2)

	// Wildcard negotiation
	offers3 := []string{"video/mp4", "image/png"}
	chosen3 := headkit.Negotiate(acceptHeader, offers3)
	assert.Equal(t, "video/mp4", chosen3)
}

func TestMediaType(t *testing.T) {
	t.Parallel()

	ct := `multipart/form-data; charset=utf-8; boundary="----WebKitFormBoundary7MA4YWxkTrZu0gW"`
	mt, params, err := headkit.ParseMediaType(ct)
	require.NoError(t, err)

	assert.Equal(t, "multipart/form-data", mt)
	assert.Equal(t, "utf-8", params.Get("charset"))
	assert.Equal(t, "----WebKitFormBoundary7MA4YWxkTrZu0gW", params.Get("boundary"))

	assert.Equal(t, "----WebKitFormBoundary7MA4YWxkTrZu0gW", headkit.ExtractBoundary(ct))
	assert.Equal(t, "utf-8", headkit.ExtractCharset(ct))

	assert.True(t, headkit.IsMultipart(ct))
	assert.False(t, headkit.IsJSON(ct))

	jsonCT := "application/problem+json; charset=utf-8"
	assert.True(t, headkit.IsJSON(jsonCT))
	assert.False(t, headkit.IsXML(jsonCT))

	xmlCT := "application/soap+xml"
	assert.True(t, headkit.IsXML(xmlCT))

	formCT := "application/x-www-form-urlencoded"
	assert.True(t, headkit.IsForm(formCT))

	// Format media type
	formatted := headkit.FormatMediaType("application/json", map[string]string{"charset": "utf-8"})
	assert.Equal(t, "application/json; charset=utf-8", formatted)
}

func TestSensitiveRedaction(t *testing.T) {
	t.Parallel()

	assert.True(t, headkit.IsSensitive("Authorization"))
	assert.True(t, headkit.IsSensitive("authorization"))
	assert.True(t, headkit.IsSensitive("Cookie"))
	assert.True(t, headkit.IsSensitive("Set-Cookie"))
	assert.True(t, headkit.IsSensitive("X-Api-Key"))
	assert.True(t, headkit.IsSensitive("Proxy-Authorization"))
	assert.False(t, headkit.IsSensitive("Content-Type"))
	assert.False(t, headkit.IsSensitive("User-Agent"))

	assert.True(t, headkit.IsSensitiveBytes([]byte("Authorization")))
	assert.False(t, headkit.IsSensitiveBytes([]byte("Content-Type")))

	assert.Equal(t, "Bearer [REDACTED]", headkit.RedactValue("Authorization", "Bearer eyJhbGciOi..."))
	assert.Equal(t, "Basic [REDACTED]", headkit.RedactValue("Authorization", "Basic dXNlcjpwYXNz"))
	assert.Equal(t, "[REDACTED]", headkit.RedactValue("Cookie", "session=secret123"))

	h := make(http.Header)
	h.Set("Authorization", "Bearer my-secret-token")
	h.Set("User-Agent", "aoni/1.0")

	redacted := headkit.RedactHeader(h)
	assert.Equal(t, "Bearer [REDACTED]", redacted.Get("Authorization"))
	assert.Equal(t, "aoni/1.0", redacted.Get("User-Agent"))
}

func TestCanonicalKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Content-Type", headkit.CanonicalKey("content-type"))
	assert.Equal(t, "Content-Type", headkit.CanonicalKey("Content-Type"))
	assert.Equal(t, "Accept-Encoding", headkit.CanonicalKey("accept-encoding"))
	assert.True(t, headkit.IsCanonical("User-Agent"))
	assert.False(t, headkit.IsCanonical("user-agent"))

	b := []byte("x-custom-header")
	headkit.CanonicalKeyBytes(b)
	assert.Equal(t, "X-Custom-Header", string(b))
}

func TestHeaderIterators(t *testing.T) {
	t.Parallel()

	h := make(http.Header)
	h.Add("Accept", "text/html")
	h.Add("Accept", "application/json")
	h.Set("Cache-Control", "no-cache, no-store, must-revalidate")

	var flattenedKeys []string
	for k, v := range headkit.Flatten(h) {
		flattenedKeys = append(flattenedKeys, k+"="+v)
	}
	assert.Len(t, flattenedKeys, 3)

	var ccTokens []string
	for tok := range headkit.SplitCommaValues(h, "Cache-Control") {
		ccTokens = append(ccTokens, tok)
	}
	assert.Equal(t, []string{"no-cache", "no-store", "must-revalidate"}, ccTokens)
}

func BenchmarkDirectives(b *testing.B) {
	raw := "public, max-age=3600, must-revalidate, stale-while-revalidate=60"
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		count := 0
		for k, v := range headkit.Directives(raw) {
			if k != "" || v != "" {
				count++
			}
		}
	}
}

func BenchmarkCanonicalKey_FastPath(b *testing.B) {
	key := "Content-Type"
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = headkit.CanonicalKey(key)
	}
}
