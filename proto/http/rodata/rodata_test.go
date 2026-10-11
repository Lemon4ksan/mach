// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rodata_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/mach/proto/http/rodata"
)

func TestInternKey_AllPrecompiledKeys(t *testing.T) {
	tests := []struct {
		input    string
		expected []byte
	}{
		// Pseudo headers
		{":method", rodata.PseudoMethod},
		{":authority", rodata.PseudoAuthority},
		{":scheme", rodata.PseudoScheme},
		{":path", rodata.PseudoPath},
		{":status", rodata.PseudoStatus},

		// 'a' keys
		{"accept-encoding", rodata.KeyAcceptEncoding},
		{"ACCEPT-ENCODING", rodata.KeyAcceptEncoding},
		{"accept-language", rodata.KeyAcceptLanguage},
		{"Accept-Language", rodata.KeyAcceptLanguage},
		{"accept", rodata.KeyAccept},
		{"ACCEPT", rodata.KeyAccept},

		// 'c' keys
		{"content-type", rodata.KeyContentType},
		{"Content-Type", rodata.KeyContentType},
		{"cookie", rodata.KeyCookie},
		{"Cookie", rodata.KeyCookie},
		{"connection", rodata.KeyConnection},
		{"Connection", rodata.KeyConnection},
		{"content-length", rodata.KeyContentLength},
		{"Content-Length", rodata.KeyContentLength},
		{"cache-control", rodata.KeyCacheControl},
		{"Cache-Control", rodata.KeyCacheControl},

		// 'd' keys
		{"date", rodata.KeyDate},
		{"Date", rodata.KeyDate},
		{"DATE", rodata.KeyDate},

		// 'h' keys
		{"host", rodata.KeyHost},
		{"HOST", rodata.KeyHost},

		// 'p' keys
		{"priority", rodata.KeyPriority},
		{"Priority", rodata.KeyPriority},

		// 'r' keys
		{"referer", rodata.KeyReferer},
		{"Referer", rodata.KeyReferer},

		// 's' keys
		{"set-cookie", rodata.KeySetCookie},
		{"Set-Cookie", rodata.KeySetCookie},
		{"server", rodata.KeyServer},
		{"Server", rodata.KeyServer},
		{"sec-ch-ua", rodata.KeySecChUa},
		{"Sec-Ch-Ua", rodata.KeySecChUa},
		{"sec-ch-ua-mobile", rodata.KeySecChUaMobile},
		{"Sec-Ch-Ua-Mobile", rodata.KeySecChUaMobile},
		{"sec-ch-ua-platform", rodata.KeySecChUaPlatform},
		{"Sec-Ch-Ua-Platform", rodata.KeySecChUaPlatform},
		{"sec-fetch-dest", rodata.KeySecFetchDest},
		{"Sec-Fetch-Dest", rodata.KeySecFetchDest},
		{"sec-fetch-mode", rodata.KeySecFetchMode},
		{"Sec-Fetch-Mode", rodata.KeySecFetchMode},
		{"sec-fetch-site", rodata.KeySecFetchSite},
		{"Sec-Fetch-Site", rodata.KeySecFetchSite},
		{"sec-fetch-user", rodata.KeySecFetchUser},
		{"Sec-Fetch-User", rodata.KeySecFetchUser},

		// 'u' keys
		{"user-agent", rodata.KeyUserAgent},
		{"User-Agent", rodata.KeyUserAgent},
		{"upgrade-insecure-requests", rodata.KeyUpgradeInsecureRequests},
		{"Upgrade-Insecure-Requests", rodata.KeyUpgradeInsecureRequests},

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
		got := rodata.InternKey(tt.input)
		if tt.expected == nil {
			if got != nil {
				t.Errorf("InternKey(%q) = %s, want nil", tt.input, got)
			}
		} else {
			if !bytes.Equal(got, tt.expected) {
				t.Errorf("InternKey(%q) = %s, want %s", tt.input, got, tt.expected)
			}
		}

		gotBytes := rodata.InternKeyBytes([]byte(tt.input))
		if tt.expected == nil {
			if gotBytes != nil {
				t.Errorf("InternKeyBytes(%q) = %s, want nil", tt.input, gotBytes)
			}
		} else {
			if !bytes.Equal(gotBytes, tt.expected) {
				t.Errorf("InternKeyBytes(%q) = %s, want %s", tt.input, gotBytes, tt.expected)
			}
		}
	}
}

func TestInternValue_AllPrecompiledValues(t *testing.T) {
	tests := []struct {
		input    string
		expected []byte
	}{
		{"application/json", rodata.ValApplicationJSON},
		{"application/x-www-form-urlencoded", rodata.ValApplicationForm},
		{"gzip, deflate, br, zstd", rodata.ValAcceptEncodingGzip},
		{"keep-alive", rodata.ValConnectionKeepAlive},
		{"document", rodata.ValSecFetchDestDoc},
		{"navigate", rodata.ValSecFetchModeNav},
		{"same-origin", rodata.ValSecFetchSiteSame},
		{"none", rodata.ValSecFetchSiteNone},
		{"cross-site", rodata.ValSecFetchSiteCross},
		{"?1", rodata.ValSecFetchUserQuestion},
		{"?0", rodata.ValSecChUaMobileFalse},
		{"unknown-val", nil},
		{"", nil},
	}

	for _, tt := range tests {
		got := rodata.InternValue(tt.input)
		if tt.expected == nil {
			if got != nil {
				t.Errorf("InternValue(%q) = %s, want nil", tt.input, got)
			}
		} else {
			if !bytes.Equal(got, tt.expected) {
				t.Errorf("InternValue(%q) = %s, want %s", tt.input, got, tt.expected)
			}
		}

		gotBytes := rodata.InternValueBytes([]byte(tt.input))
		if tt.expected == nil {
			if gotBytes != nil {
				t.Errorf("InternValueBytes(%q) = %s, want nil", tt.input, gotBytes)
			}
		} else {
			if !bytes.Equal(gotBytes, tt.expected) {
				t.Errorf("InternValueBytes(%q) = %s, want %s", tt.input, gotBytes, tt.expected)
			}
		}
	}
}

func BenchmarkInternKeyBytes(b *testing.B) {
	keys := [][]byte{
		[]byte("content-type"),
		[]byte("Content-Type"),
		[]byte("CONTENT-LENGTH"),
		[]byte("host"),
		[]byte("user-agent"),
		[]byte("sec-fetch-dest"),
		[]byte("upgrade-insecure-requests"),
		[]byte("unknown-header"),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, k := range keys {
			_ = rodata.InternKeyBytes(k)
		}
	}
}

func BenchmarkInternValueBytes(b *testing.B) {
	values := [][]byte{
		[]byte("application/json"),
		[]byte("keep-alive"),
		[]byte("gzip, deflate, br, zstd"),
		[]byte("?1"),
		[]byte("unknown-val"),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, v := range values {
			_ = rodata.InternValueBytes(v)
		}
	}
}
