// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	fheader "github.com/lemon4ksan/mach/proto/http/header"
)

const (
	// HeaderAccept indicates the content types acceptable for the response (RFC 9110 Section 12.5.1).
	HeaderAccept = fheader.Accept

	// HeaderAcceptCH lists the Client Hints headers a server wishes to receive (Client Hints Infrastructure §2.2.1).
	HeaderAcceptCH = "Accept-CH"

	// HeaderAcceptCharset indicates character sets acceptable for the response (RFC 9110 Section 12.5.2).
	HeaderAcceptCharset = fheader.AcceptCharset

	// HeaderAcceptCHLifetime specifies persistence duration for Client Hints preferences.
	HeaderAcceptCHLifetime = "Accept-CH-Lifetime"

	// HeaderAcceptEncoding indicates acceptable content codings in the response (RFC 9110 Section 12.5.3).
	HeaderAcceptEncoding = fheader.AcceptEncoding

	// HeaderAcceptLanguage indicates natural languages preferred for the response (RFC 9110 Section 12.5.4).
	HeaderAcceptLanguage = fheader.AcceptLanguage

	// HeaderAcceptPatch specifies patch document media types accepted by the server (RFC 5789 Section 3.1).
	HeaderAcceptPatch = "Accept-Patch"

	// HeaderAcceptPushPolicy specifies client preference for HTTP/2 server push.
	HeaderAcceptPushPolicy = "Accept-Push-Policy"

	// HeaderAcceptRanges indicates server support for range requests (RFC 9110 Section 14.3).
	HeaderAcceptRanges = fheader.AcceptRanges

	// HeaderAcceptSignature indicates signature algorithms supported by the client (IETF HTTP Message Signatures).
	HeaderAcceptSignature = "Accept-Signature"

	// HeaderAccessControlAllowCredentials indicates whether credentials may be exposed (W3C CORS / Fetch §3.2.1).
	HeaderAccessControlAllowCredentials = fheader.AccessControlAllowCredentials

	// HeaderAccessControlAllowHeaders indicates allowed request headers during CORS (W3C CORS / Fetch §3.2.2).
	HeaderAccessControlAllowHeaders = fheader.AccessControlAllowHeaders

	// HeaderAccessControlAllowMethods specifies methods allowed when accessing the resource (W3C CORS / Fetch §3.2.3).
	HeaderAccessControlAllowMethods = fheader.AccessControlAllowMethods

	// HeaderAccessControlAllowOrigin specifies origins allowed to access the resource (W3C CORS / Fetch §3.2.4).
	HeaderAccessControlAllowOrigin = fheader.AccessControlAllowOrigin

	// HeaderAccessControlExposeHeaders specifies response headers exposed to browser scripts (W3C CORS / Fetch §3.2.5).
	HeaderAccessControlExposeHeaders = fheader.AccessControlExposeHeaders

	// HeaderAccessControlMaxAge specifies preflight request caching duration (W3C CORS / Fetch §3.2.6).
	HeaderAccessControlMaxAge = fheader.AccessControlMaxAge

	// HeaderAccessControlRequestHeaders indicates headers used in actual request during CORS preflight (W3C CORS / Fetch §3.2.7).
	HeaderAccessControlRequestHeaders = fheader.AccessControlRequestHeaders

	// HeaderAccessControlRequestMethod indicates method used in actual request during CORS preflight (W3C CORS / Fetch §3.2.8).
	HeaderAccessControlRequestMethod = fheader.AccessControlRequestMethod

	// HeaderAge conveys sender's estimate of time since response generation (RFC 9111 Section 5.1).
	HeaderAge = fheader.Age

	// HeaderAllow lists target resource allowed methods (RFC 9110 Section 10.2.1).
	HeaderAllow = fheader.Allow

	// HeaderAltSvc advertises alternative services available for reaching origin (RFC 7838 Section 3).
	HeaderAltSvc = fheader.AltSvc

	// HeaderAuthorization contains client authentication credentials (RFC 9110 Section 11.6.2).
	HeaderAuthorization = fheader.Authorization

	// HeaderCacheControl conveys caching directives across request/response chain (RFC 9111 Section 5.2).
	HeaderCacheControl = fheader.CacheControl

	// HeaderClearSiteData requests browser to clear browsing data associated with origin (W3C Clear Site Data §2).
	HeaderClearSiteData = fheader.ClearSiteData

	// HeaderConnection controls whether network connection stays open after current transaction (RFC 9110 Section 7.6.1).
	HeaderConnection = fheader.Connection

	// HeaderContentDisposition specifies presentation style and filename for downloaded payload (RFC 6266 Section 4).
	HeaderContentDisposition = fheader.ContentDisposition

	// HeaderContentDPR indicates device pixel ratio for targeted content representation.
	HeaderContentDPR = "Content-DPR"

	// HeaderContentEncoding indicates encodings applied to payload representation (RFC 9110 Section 8.4).
	HeaderContentEncoding = fheader.ContentEncoding

	// HeaderContentLanguage describes natural languages of intended audience for representation (RFC 9110 Section 8.5).
	HeaderContentLanguage = fheader.ContentLanguage

	// HeaderContentLength specifies payload body size in decimal number of octets (RFC 9110 Section 8.6, RFC 9112 Section 6.2).
	HeaderContentLength = fheader.ContentLength

	// HeaderContentLocation indicates alternate direct URL for representation (RFC 9110 Section 8.7).
	HeaderContentLocation = fheader.ContentLocation

	// HeaderContentRange indicates where in full body a partial message belongs (RFC 9110 Section 14.4).
	HeaderContentRange = fheader.ContentRange

	// HeaderContentSecurityPolicy controls resources the user agent is allowed to load (W3C CSP Level 3).
	HeaderContentSecurityPolicy = fheader.ContentSecurityPolicy

	// HeaderContentSecurityPolicyReportOnly monitors policy violations without enforcing restrictions (W3C CSP Level 3).
	HeaderContentSecurityPolicyReportOnly = fheader.ContentSecurityPolicyReportOnly

	// HeaderContentType indicates media type of payload representation (RFC 9110 Section 8.3).
	HeaderContentType = fheader.ContentType

	// HeaderCookie contains stored HTTP cookies previously sent by server (RFC 6265 Section 4.2).
	HeaderCookie = fheader.Cookie

	// HeaderCookie2 contains historic state management information (RFC 2965).
	HeaderCookie2 = "Cookie2"

	// HeaderCrossOriginResourcePolicy prevents resource from being loaded cross-origin (W3C Fetch §7.4).
	HeaderCrossOriginResourcePolicy = fheader.CrossOriginResourcePolicy

	// HeaderDate conveys origination timestamp of HTTP message (RFC 9110 Section 6.6.1).
	HeaderDate = fheader.Date

	// HeaderDNT expresses user tracking preference (W3C Tracking Preference Expression).
	HeaderDNT = "DNT"

	// HeaderDPR requests content scaled to client device pixel ratio.
	HeaderDPR = "DPR"

	// HeaderEarlyData conveys that request was conveyed in TLS early data (RFC 8470 Section 5.1).
	HeaderEarlyData = "Early-Data"

	// HeaderETag provides entity tag representing specific resource state (RFC 9110 Section 8.8.3).
	HeaderETag = fheader.ETag

	// HeaderExpect indicates server behaviors required by client (RFC 9110 Section 10.1.1).
	HeaderExpect = fheader.Expect

	// HeaderExpectCT allows certificate transparency compliance enforcement (RFC 9163 Section 2.1).
	HeaderExpectCT = "Expect-CT"

	// HeaderExpires conveys date/time after which response is considered stale (RFC 9111 Section 5.3).
	HeaderExpires = fheader.Expires

	// HeaderFeaturePolicy controls browser features available to document (W3C Feature Policy).
	HeaderFeaturePolicy = "Feature-Policy"

	// HeaderForwarded discloses proxy information obscured during forwarding (RFC 7239 Section 4).
	HeaderForwarded = fheader.Forwarded

	// HeaderFrom contains email address for user controlling requesting agent (RFC 9110 Section 10.1.2).
	HeaderFrom = fheader.From

	// HeaderHost specifies target URI host and port number (RFC 9112 Section 7.2, RFC 9110 Section 7.2).
	HeaderHost = fheader.Host

	// HeaderIfMatch enables conditional execution based on matching ETag (RFC 9110 Section 13.1.1).
	HeaderIfMatch = fheader.IfMatch

	// HeaderIfModifiedSince enables conditional execution if modified after date (RFC 9110 Section 13.1.3).
	HeaderIfModifiedSince = fheader.IfModifiedSince

	// HeaderIfNoneMatch enables conditional execution if no ETags match (RFC 9110 Section 13.1.2).
	HeaderIfNoneMatch = fheader.IfNoneMatch

	// HeaderIfRange conditionally retrieves entire entity or requested range (RFC 9110 Section 13.1.5).
	HeaderIfRange = fheader.IfRange

	// HeaderIfUnmodifiedSince enables conditional execution if unmodified since date (RFC 9110 Section 13.1.4).
	HeaderIfUnmodifiedSince = fheader.IfUnmodifiedSince

	// HeaderIndex provides catalog index hints for directory navigation.
	HeaderIndex = "Index"

	// HeaderKeepAlive configures persistent connection parameters (RFC 2068 Section 19.7.1.1).
	HeaderKeepAlive = "Keep-Alive"

	// HeaderLargeAllocation requests larger memory heap allocation for web pages.
	HeaderLargeAllocation = "Large-Allocation"

	// HeaderLastEventID conveys last received Server-Sent Events event identifier (W3C SSE).
	HeaderLastEventID = "Last-Event-ID"

	// HeaderLastModified conveys date/time when resource was last altered (RFC 9110 Section 8.8.2).
	HeaderLastModified = fheader.LastModified

	// HeaderLink conveys typed relationship with another resource (RFC 8288 Section 3).
	HeaderLink = "Link"

	// HeaderLocation specifies target URL for redirect or new resource (RFC 9110 Section 10.2.2).
	HeaderLocation = fheader.Location

	// HeaderMaxForwards limits forwarding hops for TRACE and OPTIONS (RFC 9110 Section 7.6.2).
	HeaderMaxForwards = fheader.MaxForwards

	// HeaderNEL configures Network Error Logging policy (W3C NEL).
	HeaderNEL = "NEL"

	// HeaderOrigin indicates origin of request initiator (RFC 6454 Section 7).
	HeaderOrigin = fheader.Origin

	// HeaderPingFrom identifies source URL of hyperlinked ping attribute (W3C HTML).
	HeaderPingFrom = "Ping-From"

	// HeaderPingTo identifies target URL of hyperlinked ping attribute (W3C HTML).
	HeaderPingTo = "Ping-To"

	// HeaderPragma contains legacy implementation-specific directives (RFC 9111 Section 5.4).
	HeaderPragma = fheader.Pragma

	// HeaderProxyAuthenticate defines authentication challenge for proxy access (RFC 9110 Section 11.7.1).
	HeaderProxyAuthenticate = fheader.ProxyAuthenticate

	// HeaderProxyAuthorization conveys client credentials for proxy authentication (RFC 9110 Section 11.7.2).
	HeaderProxyAuthorization = fheader.ProxyAuthorization

	// HeaderProxyConnection specifies hop-by-hop connection directives to intermediary proxies.
	HeaderProxyConnection = "Proxy-Connection"

	// HeaderPublicKeyPins configures HTTP Public Key Pinning (RFC 7469 Section 2.1).
	HeaderPublicKeyPins = "Public-Key-Pins"

	// HeaderPublicKeyPinsReportOnly reports HPKP certificate pin violations without blocking (RFC 7469 Section 2.1).
	HeaderPublicKeyPinsReportOnly = "Public-Key-Pins-Report-Only"

	// HeaderPushPolicy configures server push preferences.
	HeaderPushPolicy = "Push-Policy"

	// HeaderRange requests partial representation octet ranges (RFC 9110 Section 14.1.2).
	HeaderRange = fheader.Range

	// HeaderReferer indicates address of resource from which request URI was obtained (RFC 9110 Section 10.1.3).
	HeaderReferer = fheader.Referer

	// HeaderReferrerPolicy controls how much referrer information is included with requests (W3C Referrer Policy §8).
	HeaderReferrerPolicy = fheader.ReferrerPolicy

	// HeaderReportTo configures reporting endpoints for browser security violation reports (W3C Reporting API).
	HeaderReportTo = "Report-To"

	// HeaderRetryAfter indicates when client should retry subsequent request (RFC 9110 Section 10.2.3).
	HeaderRetryAfter = fheader.RetryAfter

	// HeaderSaveData communicates client preference for reduced data usage (W3C Save-Data).
	HeaderSaveData = "Save-Data"

	// HeaderSecWebSocketAccept verifies server willingness to accept WebSocket upgrade (RFC 6455 Section 4.2.2).
	HeaderSecWebSocketAccept = fheader.SecWebSocketAccept

	// HeaderSecWebSocketExtensions negotiates WebSocket protocol extensions (RFC 6455 Section 4.2.2).
	HeaderSecWebSocketExtensions = fheader.SecWebSocketExtensions // #nosec G101

	// HeaderSecWebSocketKey provides challenge key for WebSocket handshake (RFC 6455 Section 4.2.1).
	HeaderSecWebSocketKey = fheader.SecWebSocketKey

	// HeaderSecWebSocketProtocol specifies requested WebSocket subprotocols (RFC 6455 Section 4.2.2).
	HeaderSecWebSocketProtocol = fheader.SecWebSocketProtocol

	// HeaderSecWebSocketVersion specifies WebSocket protocol version (RFC 6455 Section 4.2.1).
	HeaderSecWebSocketVersion = fheader.SecWebSocketVersion

	// HeaderServer conveys software information about origin server (RFC 9110 Section 10.2.4).
	HeaderServer = fheader.Server

	// HeaderServerTiming communicates performance metrics regarding server operations (W3C Server Timing).
	HeaderServerTiming = "Server-Timing"

	// HeaderSetCookie delivers HTTP state cookie to client (RFC 6265 Section 4.1).
	HeaderSetCookie = fheader.SetCookie

	// HeaderSignature contains digital signature covering HTTP message components (IETF HTTP Message Signatures).
	HeaderSignature = "Signature"

	// HeaderSignedHeaders lists header names included in message signature verification.
	HeaderSignedHeaders = "Signed-Headers"

	// HeaderSourceMap links generated code to its original source map file.
	HeaderSourceMap = "SourceMap"

	// HeaderStrictTransportSecurity declares security policy enforcing HTTPS only (RFC 6797 Section 6.1).
	HeaderStrictTransportSecurity = fheader.StrictTransportSecurity

	// HeaderTE indicates transfer codings client is willing to accept (RFC 9110 Section 10.1.4).
	HeaderTE = fheader.TE

	// HeaderTimingAllowOrigin specifies origins allowed to see detailed resource timing (W3C Resource Timing).
	HeaderTimingAllowOrigin = "Timing-Allow-Origin"

	// HeaderTk indicates tracking status for tracking preference responses (W3C Tracking Protection).
	HeaderTk = "Tk"

	// HeaderTrailer indicates header fields present in message chunked trailer section (RFC 9112 Section 7.1.2).
	HeaderTrailer = fheader.Trailer

	// HeaderTransferEncoding specifies coding transformations applied to payload (RFC 9112 Section 6.1).
	HeaderTransferEncoding = fheader.TransferEncoding

	// HeaderUpgrade requests transition to different protocol on current connection (RFC 9110 Section 7.8).
	HeaderUpgrade = fheader.Upgrade

	// HeaderUpgradeInsecureRequests requests client preferences for encrypted connections (W3C Upgrade Insecure Requests).
	HeaderUpgradeInsecureRequests = fheader.UpgradeInsecureRequests

	// HeaderUserAgent conveys client software application and platform identity (RFC 9110 Section 10.1.5).
	HeaderUserAgent = fheader.UserAgent

	// HeaderVary specifies headers determining response representation cacheability (RFC 9110 Section 12.5.5).
	HeaderVary = fheader.Vary

	// HeaderVia tracks intermediaries and proxies forwarding message (RFC 9110 Section 7.6.3).
	HeaderVia = fheader.Via

	// HeaderViewportWidth specifies layout viewport width in CSS pixels (Client Hints).
	HeaderViewportWidth = "Viewport-Width"

	// HeaderWarning conveys additional information about response status or caching (RFC 9111 Section 5.5).
	HeaderWarning = fheader.Warning

	// HeaderWidth conveys intended display width of image resource in CSS pixels (Client Hints).
	HeaderWidth = "Width"

	// HeaderWWWAuthenticate defines authentication challenge issued by origin server (RFC 9110 Section 11.6.1).
	HeaderWWWAuthenticate = fheader.WWWAuthenticate

	// HeaderXContentTypeOptions prevents MIME type sniffing by client (W3C Fetch §3.1).
	HeaderXContentTypeOptions = fheader.XContentTypeOptions

	// HeaderXDNSPrefetchControl controls browser DNS prefetching behavior.
	HeaderXDNSPrefetchControl = "X-DNS-Prefetch-Control"

	// HeaderXDownloadOptions controls file download and execution behavior in Internet Explorer.
	HeaderXDownloadOptions = "X-Download-Options"

	// HeaderXForwardedFor identifies originating IP address of client connecting through proxy.
	HeaderXForwardedFor = "X-Forwarded-For"

	// HeaderXForwardedHost identifies original host requested by client connecting through proxy.
	HeaderXForwardedHost = "X-Forwarded-Host"

	// HeaderXForwardedProto identifies protocol scheme (HTTP/HTTPS) requested by client connecting through proxy.
	HeaderXForwardedProto = "X-Forwarded-Proto"

	// HeaderXFrameOptions controls whether browser may render page inside frame or iframe (RFC 7034 Section 2).
	HeaderXFrameOptions = fheader.XFrameOptions

	// HeaderXPermittedCrossDomainPolicies controls cross-domain policy files for Adobe products.
	HeaderXPermittedCrossDomainPolicies = "X-Permitted-Cross-Domain-Policies"

	// HeaderXPingback advertises Pingback XML-RPC server URL (Pingback 1.0).
	HeaderXPingback = "X-Pingback"

	// HeaderXPoweredBy discloses backend framework or execution platform.
	HeaderXPoweredBy = "X-Powered-By"

	// HeaderXRequestedWith identifies Ajax / XMLHttpRequest asynchronous requests.
	HeaderXRequestedWith = "X-Requested-With"

	// HeaderXRobotsTag instructs search engine crawlers regarding indexing and snippet generation.
	HeaderXRobotsTag = "X-Robots-Tag"

	// HeaderXUACompatible specifies legacy Internet Explorer rendering engine mode.
	HeaderXUACompatible = "X-UA-Compatible"

	// HeaderXXSSProtection enables cross-site scripting filter in legacy user agents.
	HeaderXXSSProtection = fheader.XXSSProtection
)
