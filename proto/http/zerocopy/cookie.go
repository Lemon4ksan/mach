// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"time"

	"github.com/lemon4ksan/mach/proto/semantic/cookie"
)

var zeroTime time.Time

// Cookie represents an HTTP response cookie (RFC 6265).
type Cookie = cookie.Cookie

// CookieSameSite specifies the SameSite policy mode for a cookie (RFC 6265bis).
type CookieSameSite = cookie.CookieSameSite

const (
	// CookieSameSiteDisabled turns off the SameSite attribute on the cookie.
	CookieSameSiteDisabled = cookie.CookieSameSiteDisabled
	// CookieSameSiteDefaultMode sets the SameSite attribute to the default mode.
	CookieSameSiteDefaultMode = cookie.CookieSameSiteDefaultMode
	// CookieSameSiteLaxMode sets the SameSite attribute to Lax.
	CookieSameSiteLaxMode = cookie.CookieSameSiteLaxMode
	// CookieSameSiteStrictMode sets the SameSite attribute to Strict.
	CookieSameSiteStrictMode = cookie.CookieSameSiteStrictMode
	// CookieSameSiteNoneMode sets the SameSite attribute to None.
	CookieSameSiteNoneMode = cookie.CookieSameSiteNoneMode
)

var (
	// CookieExpireDelete specifies a past expiration time used to delete a cookie on the client (RFC 6265).
	CookieExpireDelete = cookie.CookieExpireDelete

	// CookieExpireUnlimited indicates that the cookie does not have an expiration time (RFC 6265).
	CookieExpireUnlimited = cookie.CookieExpireUnlimited

	// ErrNoCookies is returned when no cookies are found.
	ErrNoCookies = cookie.ErrNoCookies

	// ErrInvalidCookieValue indicates that a cookie contains invalid characters (RFC 6265 §4.1.1).
	ErrInvalidCookieValue = cookie.ErrInvalidCookieValue

	// AcquireCookie returns an empty [Cookie] instance from the pool.
	AcquireCookie = cookie.AcquireCookie

	// ReleaseCookie resets c and returns it to the pool.
	ReleaseCookie = cookie.ReleaseCookie

	// AppendCookiePart formats and appends an attribute "; key=value" pair to dst.
	AppendCookiePart = cookie.AppendCookiePart

	// GetCookieKey extracts the cookie name from src and appends it to dst.
	GetCookieKey = cookie.GetCookieKey

	// AppendRequestCookieBytes formats cookies as a Cookie request header and appends them to dst (RFC 6265 §4.2).
	AppendRequestCookieBytes = cookie.AppendRequestCookieBytes

	// AppendResponseCookieBytes formats cookies as Set-Cookie response headers and appends them to dst (RFC 6265 §4.1).
	AppendResponseCookieBytes = cookie.AppendResponseCookieBytes

	// ParseRequestCookies parses cookie pairs from a Cookie header byte slice into cookies (RFC 6265 §5.4).
	ParseRequestCookies = cookie.ParseRequestCookies

	parseCookieExpires   = cookie.ParseCookieExpires
	validCookiePathValue = cookie.ValidCookiePathValue
	validCookieValue     = cookie.ValidCookieValue
	trimCookieArgNoCopy  = cookie.TrimCookieArgNoCopy
	decodeCookieArg      = cookie.DecodeCookieArg
)
