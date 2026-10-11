// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"github.com/lemon4ksan/mach/proto/headkit"
)

// Legacy Header constant aliases retained for internal proto/http compatibility.
// New code should import and reference "github.com/lemon4ksan/mach/proto/headkit" directly.
const (
	// HeaderConnection controls whether network connection stays open after current transaction (RFC 9110 Section 7.6.1).
	HeaderConnection = headkit.Connection

	// HeaderContentEncoding indicates encodings applied to payload representation (RFC 9110 Section 8.4).
	HeaderContentEncoding = headkit.ContentEncoding

	// HeaderContentLength specifies payload body size in decimal number of octets (RFC 9110 Section 8.6, RFC 9112 Section 6.2).
	HeaderContentLength = headkit.ContentLength

	// HeaderContentType indicates media type of payload representation (RFC 9110 Section 8.3).
	HeaderContentType = headkit.ContentType

	// HeaderCookie contains stored HTTP cookies previously sent by server (RFC 6265 Section 4.2).
	HeaderCookie = headkit.Cookie

	// HeaderHost specifies target URI host and port number (RFC 9112 Section 7.2, RFC 9110 Section 7.2).
	HeaderHost = headkit.Host

	// HeaderProxyConnection specifies hop-by-hop connection directives to intermediary proxies.
	HeaderProxyConnection = headkit.ProxyConnection

	// HeaderServer conveys software information about origin server (RFC 9110 Section 10.2.4).
	HeaderServer = headkit.Server

	// HeaderSetCookie delivers HTTP state cookie to client (RFC 6265 Section 4.1).
	HeaderSetCookie = headkit.SetCookie

	// HeaderTrailer indicates header fields present in message chunked trailer section (RFC 9112 Section 7.1.2).
	HeaderTrailer = headkit.Trailer

	// HeaderUserAgent conveys client software application and platform identity (RFC 9110 Section 10.1.5).
	HeaderUserAgent = headkit.UserAgent

	// HeaderVary specifies headers determining response representation cacheability (RFC 9110 Section 12.5.5).
	HeaderVary = headkit.Vary
)
