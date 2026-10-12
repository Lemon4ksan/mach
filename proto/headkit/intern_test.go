// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/mach/proto/headkit"
)

func TestInternKey_AllPrecompiledKeys(t *testing.T) {
	tests := []struct {
		input    string
		expected []byte
	}{
		// Pseudo headers
		{":method", headkit.PseudoMethodBytes},
		{":authority", headkit.PseudoAuthorityBytes},
		{":scheme", headkit.PseudoSchemeBytes},
		{":path", headkit.PseudoPathBytes},
		{":status", headkit.PseudoStatusBytes},
		{":protocol", headkit.PseudoProtocolBytes},

		// 'a' keys
		{"accept-encoding", headkit.KeyAcceptEncoding},
		{"ACCEPT-ENCODING", headkit.KeyAcceptEncoding},
		{"accept-language", headkit.KeyAcceptLanguage},
		{"Accept-Language", headkit.KeyAcceptLanguage},
		{"accept", headkit.KeyAccept},
		{"ACCEPT", headkit.KeyAccept},

		// 'c' keys
		{"content-type", headkit.KeyContentType},
		{"Content-Type", headkit.KeyContentType},
		{"cookie", headkit.KeyCookie},
		{"Cookie", headkit.KeyCookie},
		{"connection", headkit.KeyConnection},
		{"Connection", headkit.KeyConnection},
		{"content-length", headkit.KeyContentLength},
		{"Content-Length", headkit.KeyContentLength},
		{"cache-control", headkit.KeyCacheControl},
		{"Cache-Control", headkit.KeyCacheControl},

		// 'd' keys
		{"date", headkit.KeyDate},
		{"Date", headkit.KeyDate},
		{"DATE", headkit.KeyDate},

		// 'h' keys
		{"host", headkit.KeyHost},
		{"HOST", headkit.KeyHost},

		// 'p' keys
		{"priority", headkit.KeyPriority},
		{"Priority", headkit.KeyPriority},

		// 'r' keys
		{"referer", headkit.KeyReferer},
		{"Referer", headkit.KeyReferer},

		// 's' keys
		{"set-cookie", headkit.KeySetCookie},
		{"Set-Cookie", headkit.KeySetCookie},
		{"server", headkit.KeyServer},
		{"Server", headkit.KeyServer},
		{"sec-ch-ua", headkit.KeySecChUa},
		{"Sec-Ch-Ua", headkit.KeySecChUa},
		{"sec-ch-ua-mobile", headkit.KeySecChUaMobile},
		{"Sec-Ch-Ua-Mobile", headkit.KeySecChUaMobile},
		{"sec-ch-ua-platform", headkit.KeySecChUaPlatform},
		{"Sec-Ch-Ua-Platform", headkit.KeySecChUaPlatform},
		{"sec-fetch-dest", headkit.KeySecFetchDest},
		{"Sec-Fetch-Dest", headkit.KeySecFetchDest},
		{"sec-fetch-mode", headkit.KeySecFetchMode},
		{"Sec-Fetch-Mode", headkit.KeySecFetchMode},
		{"sec-fetch-site", headkit.KeySecFetchSite},
		{"Sec-Fetch-Site", headkit.KeySecFetchSite},
		{"sec-fetch-user", headkit.KeySecFetchUser},
		{"Sec-Fetch-User", headkit.KeySecFetchUser},

		// 'u' keys
		{"user-agent", headkit.KeyUserAgent},
		{"User-Agent", headkit.KeyUserAgent},
		{"upgrade-insecure-requests", headkit.KeyUpgradeInsecureRequests},
		{"Upgrade-Insecure-Requests", headkit.KeyUpgradeInsecureRequests},

		// Unknown / empty / length mismatches
		{"", nil},
		{":unknown", nil},
		{"unknown-key", nil},
		{"a", nil},
		{"c", nil},
		{"s", nil},
		{"u", nil},
	}

	for _, tt := range tests {
		got := headkit.InternKey(tt.input)
		gotBytes := headkit.InternKey([]byte(tt.input))

		if !bytes.Equal(got, tt.expected) {
			t.Errorf("InternKey(%q) = %q; want %q", tt.input, got, tt.expected)
		}
		if !bytes.Equal(gotBytes, tt.expected) {
			t.Errorf("InternKey([]byte(%q)) = %q; want %q", tt.input, gotBytes, tt.expected)
		}
	}
}

func TestInternValue_AllPrecompiledValues(t *testing.T) {
	tests := []struct {
		input    string
		expected []byte
	}{
		{"application/json", headkit.ValApplicationJSON},
		{"application/x-www-form-urlencoded", headkit.ValApplicationForm},
		{"gzip, deflate, br, zstd", headkit.ValAcceptEncodingGzip},
		{"keep-alive", headkit.ValConnectionKeepAlive},
		{"document", headkit.ValSecFetchDestDoc},
		{"navigate", headkit.ValSecFetchModeNav},
		{"same-origin", headkit.ValSecFetchSiteSame},
		{"none", headkit.ValSecFetchSiteNone},
		{"cross-site", headkit.ValSecFetchSiteCross},
		{"?1", headkit.ValSecFetchUserQuestion},
		{"?0", headkit.ValSecChUaMobileFalse},

		// Mismatch
		{"", nil},
		{"unknown-val", nil},
		{"application/xml", nil},
		{"?", nil},
		{"?2", nil},
	}

	for _, tt := range tests {
		got := headkit.InternValue(tt.input)
		gotBytes := headkit.InternValue([]byte(tt.input))

		if !bytes.Equal(got, tt.expected) {
			t.Errorf("InternValue(%q) = %q; want %q", tt.input, got, tt.expected)
		}
		if !bytes.Equal(gotBytes, tt.expected) {
			t.Errorf("InternValue([]byte(%q)) = %q; want %q", tt.input, gotBytes, tt.expected)
		}
	}
}

func TestInternCanonicalKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"host", headkit.Host},
		{"Host", headkit.Host},
		{"HOST", headkit.Host},
		{"content-type", headkit.ContentType},
		{"Content-Type", headkit.ContentType},
		{"content-length", headkit.ContentLength},
		{"transfer-encoding", headkit.TransferEncoding},
		{"connection", headkit.Connection},
		{"user-agent", headkit.UserAgent},
		{"accept-encoding", headkit.AcceptEncoding},
		{"accept", headkit.Accept},
		{"vary", headkit.Vary},
		{"authorization", headkit.Authorization},
		{"cookie", headkit.Cookie},
		{"set-cookie", headkit.SetCookie},
		{"trailer", headkit.Trailer},
		{"upgrade", headkit.Upgrade},
		{"unknown-header-xyz", ""},
	}

	for _, tt := range tests {
		got := headkit.InternCanonicalKey(tt.input)
		gotBytes := headkit.InternCanonicalKey([]byte(tt.input))

		if got != tt.expected {
			t.Errorf("InternCanonicalKey(%q) = %q; want %q", tt.input, got, tt.expected)
		}
		if gotBytes != tt.expected {
			t.Errorf("InternCanonicalKey([]byte(%q)) = %q; want %q", tt.input, gotBytes, tt.expected)
		}
	}
}

func TestInternValueString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"chunked", "chunked"},
		{"close", "close"},
		{"keep-alive", "keep-alive"},
		{"gzip", "gzip"},
		{"application/json", "application/json"},
		{"unknown-value-xyz", ""},
	}

	for _, tt := range tests {
		got := headkit.InternValueString(tt.input)
		gotBytes := headkit.InternValueString([]byte(tt.input))

		if got != tt.expected {
			t.Errorf("InternValueString(%q) = %q; want %q", tt.input, got, tt.expected)
		}
		if gotBytes != tt.expected {
			t.Errorf("InternValueString([]byte(%q)) = %q; want %q", tt.input, gotBytes, tt.expected)
		}
	}
}
