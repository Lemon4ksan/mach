// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rodata provides static read-only byte slices stored in the binary's .rodata segment
// for zero-allocation HTTP header framing, parsing, and interning.
package rodata

import (
	"bytes"
	"encoding/binary"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Precompiled HTTP/1.1 and HTTP/2 static pseudo-header keys.
var (
	PseudoMethod    = []byte(":method")
	PseudoAuthority = []byte(":authority")
	PseudoScheme    = []byte(":scheme")
	PseudoPath      = []byte(":path")
	PseudoStatus    = []byte(":status")
)

// Precompiled HTTP header canonical & lower-case keys.
var (
	KeyContentType             = []byte("content-type")
	KeyAcceptEncoding          = []byte("accept-encoding")
	KeyAcceptLanguage          = []byte("accept-language")
	KeyAccept                  = []byte("accept")
	KeyUserAgent               = []byte("user-agent")
	KeyCookie                  = []byte("cookie")
	KeySetCookie               = []byte("set-cookie")
	KeyConnection              = []byte("connection")
	KeyPriority                = []byte("priority")
	KeyHost                    = []byte("host")
	KeyReferer                 = []byte("referer")
	KeyUpgradeInsecureRequests = []byte("upgrade-insecure-requests")
	KeySecChUa                 = []byte("sec-ch-ua")
	KeySecChUaMobile           = []byte("sec-ch-ua-mobile")
	KeySecChUaPlatform         = []byte("sec-ch-ua-platform")
	KeySecFetchDest            = []byte("sec-fetch-dest")
	KeySecFetchMode            = []byte("sec-fetch-mode")
	KeySecFetchSite            = []byte("sec-fetch-site")
	KeySecFetchUser            = []byte("sec-fetch-user")
	KeyContentLength           = []byte("content-length")
	KeyServer                  = []byte("server")
	KeyDate                    = []byte("date")
	KeyCacheControl            = []byte("cache-control")
)

// Precompiled common HTTP header values.
var (
	ValApplicationJSON      = []byte("application/json")
	ValApplicationForm      = []byte("application/x-www-form-urlencoded")
	ValAcceptEncodingGzip   = []byte("gzip, deflate, br, zstd")
	ValConnectionKeepAlive  = []byte("keep-alive")
	ValSecFetchDestDoc      = []byte("document")
	ValSecFetchModeNav      = []byte("navigate")
	ValSecFetchSiteSame     = []byte("same-origin")
	ValSecFetchSiteNone     = []byte("none")
	ValSecFetchSiteCross    = []byte("cross-site")
	ValSecFetchUserQuestion = []byte("?1")
	ValSecChUaMobileFalse   = []byte("?0")
)

// InternKey returns a static .rodata byte slice for key if recognized, or nil if not matched.
// It is case-insensitive for standard ASCII header keys.
func InternKey(key string) []byte {
	return InternKeyBytes(bytesconv.S2B(key))
}

// InternKeyBytes returns a static .rodata byte slice for key if recognized, or nil if not matched.
// Accelerated via O(1) length-dispatch and 64-bit SWAR case-insensitive vector matching.
func InternKeyBytes(key []byte) []byte {
	n := len(key)
	if n < 4 || n > 25 {
		return nil
	}

	switch n {
	case 4:
		switch key[0] | 0x20 {
		case 'h':
			if equalFoldSWAR(key, KeyHost) {
				return KeyHost
			}
		case 'd':
			if equalFoldSWAR(key, KeyDate) {
				return KeyDate
			}
		}
	case 5:
		if bytes.Equal(key, PseudoPath) {
			return PseudoPath
		}
	case 6:
		switch key[0] | 0x20 {
		case 'a':
			if equalFoldSWAR(key, KeyAccept) {
				return KeyAccept
			}
		case 'c':
			if equalFoldSWAR(key, KeyCookie) {
				return KeyCookie
			}
		case 's':
			if equalFoldSWAR(key, KeyServer) {
				return KeyServer
			}
		}
	case 7:
		if key[0] == ':' {
			switch key[1] {
			case 'm':
				if bytes.Equal(key, PseudoMethod) {
					return PseudoMethod
				}
			case 's':
				switch key[2] {
				case 'c':
					if bytes.Equal(key, PseudoScheme) {
						return PseudoScheme
					}
				case 't':
					if bytes.Equal(key, PseudoStatus) {
						return PseudoStatus
					}
				}
			}
		} else if (key[0] | 0x20) == 'r' {
			if equalFoldSWAR(key, KeyReferer) {
				return KeyReferer
			}
		}
	case 8:
		if (key[0] | 0x20) == 'p' {
			if equalFoldSWAR(key, KeyPriority) {
				return KeyPriority
			}
		}
	case 9:
		if (key[0] | 0x20) == 's' {
			if equalFoldSWAR(key, KeySecChUa) {
				return KeySecChUa
			}
		}
	case 10:
		switch key[0] {
		case ':':
			if bytes.Equal(key, PseudoAuthority) {
				return PseudoAuthority
			}
		case 'c', 'C':
			if equalFoldSWAR(key, KeyConnection) {
				return KeyConnection
			}
		case 's', 'S':
			if equalFoldSWAR(key, KeySetCookie) {
				return KeySetCookie
			}
		case 'u', 'U':
			if equalFoldSWAR(key, KeyUserAgent) {
				return KeyUserAgent
			}
		}
	case 12:
		if (key[0] | 0x20) == 'c' {
			if equalFoldSWAR(key, KeyContentType) {
				return KeyContentType
			}
		}
	case 13:
		if (key[0] | 0x20) == 'c' {
			if equalFoldSWAR(key, KeyCacheControl) {
				return KeyCacheControl
			}
		}
	case 14:
		switch key[0] | 0x20 {
		case 'c':
			if equalFoldSWAR(key, KeyContentLength) {
				return KeyContentLength
			}
		case 's':
			switch key[10] | 0x20 {
			case 'd':
				if equalFoldSWAR(key, KeySecFetchDest) {
					return KeySecFetchDest
				}
			case 'm':
				if equalFoldSWAR(key, KeySecFetchMode) {
					return KeySecFetchMode
				}
			case 's':
				if equalFoldSWAR(key, KeySecFetchSite) {
					return KeySecFetchSite
				}
			case 'u':
				if equalFoldSWAR(key, KeySecFetchUser) {
					return KeySecFetchUser
				}
			}
		}
	case 15:
		if (key[0] | 0x20) == 'a' {
			switch key[7] | 0x20 {
			case 'e':
				if equalFoldSWAR(key, KeyAcceptEncoding) {
					return KeyAcceptEncoding
				}
			case 'l':
				if equalFoldSWAR(key, KeyAcceptLanguage) {
					return KeyAcceptLanguage
				}
			}
		}
	case 16:
		if (key[0] | 0x20) == 's' {
			if equalFoldSWAR(key, KeySecChUaMobile) {
				return KeySecChUaMobile
			}
		}
	case 18:
		if (key[0] | 0x20) == 's' {
			if equalFoldSWAR(key, KeySecChUaPlatform) {
				return KeySecChUaPlatform
			}
		}
	case 25:
		if (key[0] | 0x20) == 'u' {
			if equalFoldSWAR(key, KeyUpgradeInsecureRequests) {
				return KeyUpgradeInsecureRequests
			}
		}
	}

	return nil
}

// InternValue returns a static .rodata byte slice for val if recognized, or nil if not matched.
func InternValue(val string) []byte {
	return InternValueBytes(bytesconv.S2B(val))
}

// InternValueBytes returns a static .rodata byte slice for val if recognized, or nil if not matched.
// Accelerated via O(1) length-dispatch and zero-allocation slice comparisons.
func InternValueBytes(val []byte) []byte {
	switch len(val) {
	case 2:
		if val[0] == '?' {
			switch val[1] {
			case '1':
				return ValSecFetchUserQuestion
			case '0':
				return ValSecChUaMobileFalse
			}
		}
	case 4:
		if bytes.Equal(val, ValSecFetchSiteNone) {
			return ValSecFetchSiteNone
		}
	case 8:
		if bytes.Equal(val, ValSecFetchDestDoc) {
			return ValSecFetchDestDoc
		}
		if bytes.Equal(val, ValSecFetchModeNav) {
			return ValSecFetchModeNav
		}
	case 10:
		if bytes.Equal(val, ValConnectionKeepAlive) {
			return ValConnectionKeepAlive
		}
		if bytes.Equal(val, ValSecFetchSiteCross) {
			return ValSecFetchSiteCross
		}
	case 11:
		if bytes.Equal(val, ValSecFetchSiteSame) {
			return ValSecFetchSiteSame
		}
	case 16:
		if bytes.Equal(val, ValApplicationJSON) {
			return ValApplicationJSON
		}
	case 23:
		if bytes.Equal(val, ValAcceptEncodingGzip) {
			return ValAcceptEncodingGzip
		}
	case 33:
		if bytes.Equal(val, ValApplicationForm) {
			return ValApplicationForm
		}
	}

	return nil
}

// equalFoldSWAR compares a and b case-insensitively using 64-bit SWAR vector processing.
// Target b is assumed to be a lowercase ASCII slice.
func equalFoldSWAR(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	n := len(a)
	i := 0

	// 8-byte SWAR blocks
	for i+8 <= n {
		va := binary.LittleEndian.Uint64(a[i:])
		vb := binary.LittleEndian.Uint64(b[i:])
		if toLower64(va) != vb {
			return false
		}
		i += 8
	}

	// 4-byte remainder
	if i+4 <= n {
		va := binary.LittleEndian.Uint32(a[i:])
		vb := binary.LittleEndian.Uint32(b[i:])
		if toLower32(va) != vb {
			return false
		}
		i += 4
	}

	// Scalar tail
	for ; i < n; i++ {
		ca := a[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if ca != b[i] {
			return false
		}
	}

	return true
}

func toLower64(v uint64) uint64 {
	mask := v + 0x7f7f7f7f7f7f7f7f
	mask = (mask ^ v) & 0x8080808080808080
	mask = ((v + 0x3f3f3f3f3f3f3f3f) ^ v) & mask
	mask = (mask >> 2) & 0x2020202020202020
	return v | mask
}

func toLower32(v uint32) uint32 {
	mask := v + 0x7f7f7f7f
	mask = (mask ^ v) & 0x80808080
	mask = ((v + 0x3f3f3f3f) ^ v) & mask
	mask = (mask >> 2) & 0x20202020
	return v | mask
}
