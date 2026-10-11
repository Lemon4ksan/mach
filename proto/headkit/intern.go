// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit

import (
	"bytes"
	"encoding/binary"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Precompiled HTTP/1.1, HTTP/2, and HTTP/3 static pseudo-header keys as byte slices.
var (
	PseudoMethodBytes    = []byte(PseudoMethod)
	PseudoAuthorityBytes = []byte(PseudoAuthority)
	PseudoSchemeBytes    = []byte(PseudoScheme)
	PseudoPathBytes      = []byte(PseudoPath)
	PseudoStatusBytes    = []byte(PseudoStatus)
	PseudoProtocolBytes  = []byte(PseudoProtocol)
)

// Precompiled HTTP header canonical & lower-case keys as byte slices.
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

// Precompiled common HTTP header values as byte slices.
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
// It is case-insensitive for standard ASCII header keys, supporting both string and []byte inputs.
// Accelerated via O(1) length-dispatch and 64-bit SWAR case-insensitive vector matching.
func InternKey[S ~string | ~[]byte](key S) []byte {
	b := stringOrBytesToBytes(key)
	n := len(b)
	if n < 4 || n > 25 {
		return nil
	}

	switch n {
	case 4:
		switch b[0] | 0x20 {
		case 'h':
			if equalFoldSWAR(b, KeyHost) {
				return KeyHost
			}
		case 'd':
			if equalFoldSWAR(b, KeyDate) {
				return KeyDate
			}
		}
	case 5:
		if bytes.Equal(b, PseudoPathBytes) {
			return PseudoPathBytes
		}
	case 6:
		switch b[0] | 0x20 {
		case 'a':
			if equalFoldSWAR(b, KeyAccept) {
				return KeyAccept
			}
		case 'c':
			if equalFoldSWAR(b, KeyCookie) {
				return KeyCookie
			}
		case 's':
			if equalFoldSWAR(b, KeyServer) {
				return KeyServer
			}
		}
	case 7:
		if b[0] == ':' {
			switch b[1] {
			case 'm':
				if bytes.Equal(b, PseudoMethodBytes) {
					return PseudoMethodBytes
				}
			case 's':
				switch b[2] {
				case 'c':
					if bytes.Equal(b, PseudoSchemeBytes) {
						return PseudoSchemeBytes
					}
				case 't':
					if bytes.Equal(b, PseudoStatusBytes) {
						return PseudoStatusBytes
					}
				}
			}
		} else if (b[0] | 0x20) == 'r' {
			if equalFoldSWAR(b, KeyReferer) {
				return KeyReferer
			}
		}
	case 8:
		if (b[0] | 0x20) == 'p' {
			if equalFoldSWAR(b, KeyPriority) {
				return KeyPriority
			}
		}
	case 9:
		switch b[0] | 0x20 {
		case 's':
			if equalFoldSWAR(b, KeySecChUa) {
				return KeySecChUa
			}
		case ':':
			if bytes.Equal(b, PseudoProtocolBytes) {
				return PseudoProtocolBytes
			}
		}
	case 10:
		switch b[0] {
		case ':':
			if bytes.Equal(b, PseudoAuthorityBytes) {
				return PseudoAuthorityBytes
			}
		case 'c', 'C':
			if equalFoldSWAR(b, KeyConnection) {
				return KeyConnection
			}
		case 's', 'S':
			if equalFoldSWAR(b, KeySetCookie) {
				return KeySetCookie
			}
		case 'u', 'U':
			if equalFoldSWAR(b, KeyUserAgent) {
				return KeyUserAgent
			}
		}
	case 12:
		if (b[0] | 0x20) == 'c' {
			if equalFoldSWAR(b, KeyContentType) {
				return KeyContentType
			}
		}
	case 13:
		if (b[0] | 0x20) == 'c' {
			if equalFoldSWAR(b, KeyCacheControl) {
				return KeyCacheControl
			}
		}
	case 14:
		switch b[0] | 0x20 {
		case 'c':
			if equalFoldSWAR(b, KeyContentLength) {
				return KeyContentLength
			}
		case 's':
			switch b[10] | 0x20 {
			case 'd':
				if equalFoldSWAR(b, KeySecFetchDest) {
					return KeySecFetchDest
				}
			case 'm':
				if equalFoldSWAR(b, KeySecFetchMode) {
					return KeySecFetchMode
				}
			case 's':
				if equalFoldSWAR(b, KeySecFetchSite) {
					return KeySecFetchSite
				}
			case 'u':
				if equalFoldSWAR(b, KeySecFetchUser) {
					return KeySecFetchUser
				}
			}
		}
	case 15:
		if (b[0] | 0x20) == 'a' {
			switch b[7] | 0x20 {
			case 'e':
				if equalFoldSWAR(b, KeyAcceptEncoding) {
					return KeyAcceptEncoding
				}
			case 'l':
				if equalFoldSWAR(b, KeyAcceptLanguage) {
					return KeyAcceptLanguage
				}
			}
		}
	case 16:
		if (b[0] | 0x20) == 's' {
			if equalFoldSWAR(b, KeySecChUaMobile) {
				return KeySecChUaMobile
			}
		}
	case 18:
		if (b[0] | 0x20) == 's' {
			if equalFoldSWAR(b, KeySecChUaPlatform) {
				return KeySecChUaPlatform
			}
		}
	case 25:
		if (b[0] | 0x20) == 'u' {
			if equalFoldSWAR(b, KeyUpgradeInsecureRequests) {
				return KeyUpgradeInsecureRequests
			}
		}
	}

	return nil
}


// InternValue returns a static .rodata byte slice for val if recognized, or nil if not matched.
// Accelerated via O(1) length-dispatch and zero-allocation slice comparisons.
func InternValue[S ~string | ~[]byte](val S) []byte {
	b := stringOrBytesToBytes(val)
	switch len(b) {
	case 2:
		if b[0] == '?' {
			switch b[1] {
			case '1':
				return ValSecFetchUserQuestion
			case '0':
				return ValSecChUaMobileFalse
			}
		}
	case 4:
		if bytes.Equal(b, ValSecFetchSiteNone) {
			return ValSecFetchSiteNone
		}
	case 8:
		if bytes.Equal(b, ValSecFetchDestDoc) {
			return ValSecFetchDestDoc
		}
		if bytes.Equal(b, ValSecFetchModeNav) {
			return ValSecFetchModeNav
		}
	case 10:
		if bytes.Equal(b, ValConnectionKeepAlive) {
			return ValConnectionKeepAlive
		}
		if bytes.Equal(b, ValSecFetchSiteCross) {
			return ValSecFetchSiteCross
		}
	case 11:
		if bytes.Equal(b, ValSecFetchSiteSame) {
			return ValSecFetchSiteSame
		}
	case 16:
		if bytes.Equal(b, ValApplicationJSON) {
			return ValApplicationJSON
		}
	case 23:
		if bytes.Equal(b, ValAcceptEncodingGzip) {
			return ValAcceptEncodingGzip
		}
	case 33:
		if bytes.Equal(b, ValApplicationForm) {
			return ValApplicationForm
		}
	}

	return nil
}


func stringOrBytesToBytes[S ~string | ~[]byte](s S) []byte {
	switch v := any(s).(type) {
	case []byte:
		return v
	case string:
		return bytesconv.S2B(v)
	default:
		return []byte(s)
	}
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
