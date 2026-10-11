// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"github.com/lemon4ksan/mach/proto/headkit"
)

const (
	// HeaderAccept indicates the content types acceptable for the response (RFC 9110 Section 12.5.1).
	HeaderAccept = headkit.Accept

	// HeaderAcceptCH lists the Client Hints headers a server wishes to receive (Client Hints Infrastructure §2.2.1).
	HeaderAcceptCH = headkit.AcceptCH

	// HeaderAcceptCharset indicates character sets acceptable for the response (RFC 9110 Section 12.5.2).
	HeaderAcceptCharset = headkit.AcceptCharset

	// HeaderAcceptCHLifetime specifies persistence duration for Client Hints preferences.
	HeaderAcceptCHLifetime = headkit.AcceptCHLifetime

	// HeaderAcceptEncoding indicates acceptable content codings in the response (RFC 9110 Section 12.5.3).
	HeaderAcceptEncoding = headkit.AcceptEncoding

	// HeaderAcceptLanguage indicates natural languages preferred for the response (RFC 9110 Section 12.5.4).
	HeaderAcceptLanguage = headkit.AcceptLanguage

	// HeaderAcceptPatch specifies patch document media types accepted by the server (RFC 5789 Section 3.1).
	HeaderAcceptPatch = headkit.AcceptPatch

	// HeaderAcceptPushPolicy specifies client preference for HTTP/2 server push.
	HeaderAcceptPushPolicy = headkit.AcceptPushPolicy

	// HeaderAcceptRanges indicates server support for range requests (RFC 9110 Section 14.3).
	HeaderAcceptRanges = headkit.AcceptRanges

	// HeaderAcceptSignature indicates signature algorithms supported by the client (IETF HTTP Message Signatures).
	HeaderAcceptSignature = headkit.AcceptSignature

	// HeaderAccessControlAllowCredentials indicates whether credentials may be exposed (W3C CORS / Fetch §3.2.1).
	HeaderAccessControlAllowCredentials = headkit.AccessControlAllowCredentials

	// HeaderAccessControlAllowHeaders indicates allowed request headers during CORS (W3C CORS / Fetch §3.2.2).
	HeaderAccessControlAllowHeaders = headkit.AccessControlAllowHeaders

	// HeaderAccessControlAllowMethods specifies methods allowed when accessing the resource (W3C CORS / Fetch §3.2.3).
	HeaderAccessControlAllowMethods = headkit.AccessControlAllowMethods

	// HeaderAccessControlAllowOrigin specifies origins allowed to access the resource (W3C CORS / Fetch §3.2.4).
	HeaderAccessControlAllowOrigin = headkit.AccessControlAllowOrigin

	// HeaderAccessControlExposeHeaders specifies response headers exposed to browser scripts (W3C CORS / Fetch §3.2.5).
	HeaderAccessControlExposeHeaders = headkit.AccessControlExposeHeaders

	// HeaderAccessControlMaxAge specifies preflight request caching duration (W3C CORS / Fetch §3.2.6).
	HeaderAccessControlMaxAge = headkit.AccessControlMaxAge

	// HeaderAccessControlRequestHeaders indicates headers used in actual request during CORS preflight (W3C CORS / Fetch §3.2.7).
	HeaderAccessControlRequestHeaders = headkit.AccessControlRequestHeaders

	// HeaderAccessControlRequestMethod indicates method used in actual request during CORS preflight (W3C CORS / Fetch §3.2.8).
	HeaderAccessControlRequestMethod = headkit.AccessControlRequestMethod

	// HeaderAge conveys sender's estimate of time since response generation (RFC 9111 Section 5.1).
	HeaderAge = headkit.Age

	// HeaderAllow lists target resource allowed methods (RFC 9110 Section 10.2.1).
	HeaderAllow = headkit.Allow

	// HeaderAltSvc advertises alternative services available for reaching origin (RFC 7838 Section 3).
	HeaderAltSvc = headkit.AltSvc

	// HeaderAuthorization contains client authentication credentials (RFC 9110 Section 11.6.2).
	HeaderAuthorization = headkit.Authorization

	// HeaderCacheControl conveys caching directives across request/response chain (RFC 9111 Section 5.2).
	HeaderCacheControl = headkit.CacheControl

	// HeaderClearSiteData requests browser to clear browsing data associated with origin (W3C Clear Site Data §2).
	HeaderClearSiteData = headkit.ClearSiteData

	// HeaderConnection controls whether network connection stays open after current transaction (RFC 9110 Section 7.6.1).
	HeaderConnection = headkit.Connection

	// HeaderContentDisposition specifies presentation style and filename for downloaded payload (RFC 6266 Section 4).
	HeaderContentDisposition = headkit.ContentDisposition

	// HeaderContentDPR indicates device pixel ratio for targeted content representation.
	HeaderContentDPR = headkit.ContentDPR

	// HeaderContentEncoding indicates encodings applied to payload representation (RFC 9110 Section 8.4).
	HeaderContentEncoding = headkit.ContentEncoding

	// HeaderContentLanguage describes natural languages of intended audience for representation (RFC 9110 Section 8.5).
	HeaderContentLanguage = headkit.ContentLanguage

	// HeaderContentLength specifies payload body size in decimal number of octets (RFC 9110 Section 8.6, RFC 9112 Section 6.2).
	HeaderContentLength = headkit.ContentLength

	// HeaderContentLocation indicates alternate direct URL for representation (RFC 9110 Section 8.7).
	HeaderContentLocation = headkit.ContentLocation

	// HeaderContentRange indicates where in full body a partial message belongs (RFC 9110 Section 14.4).
	HeaderContentRange = headkit.ContentRange

	// HeaderContentSecurityPolicy controls resources the user agent is allowed to load (W3C CSP Level 3).
	HeaderContentSecurityPolicy = headkit.ContentSecurityPolicy

	// HeaderContentSecurityPolicyReportOnly monitors policy violations without enforcing restrictions (W3C CSP Level 3).
	HeaderContentSecurityPolicyReportOnly = headkit.ContentSecurityPolicyReportOnly

	// HeaderContentType indicates media type of payload representation (RFC 9110 Section 8.3).
	HeaderContentType = headkit.ContentType

	// HeaderCookie contains stored HTTP cookies previously sent by server (RFC 6265 Section 4.2).
	HeaderCookie = headkit.Cookie

	// HeaderCookie2 contains historic state management information (RFC 2965).
	HeaderCookie2 = headkit.Cookie2

	// HeaderCrossOriginResourcePolicy prevents resource from being loaded cross-origin (W3C Fetch §7.4).
	HeaderCrossOriginResourcePolicy = headkit.CrossOriginResourcePolicy

	// HeaderDate conveys origination timestamp of HTTP message (RFC 9110 Section 6.6.1).
	HeaderDate = headkit.Date

	// HeaderDNT expresses user tracking preference (W3C Tracking Preference Expression).
	HeaderDNT = headkit.DNT

	// HeaderDPR requests content scaled to client device pixel ratio.
	HeaderDPR = headkit.DPR

	// HeaderEarlyData conveys that request was conveyed in TLS early data (RFC 8470 Section 5.1).
	HeaderEarlyData = headkit.EarlyData

	// HeaderETag provides entity tag representing specific resource state (RFC 9110 Section 8.8.3).
	HeaderETag = headkit.ETag

	// HeaderExpect indicates server behaviors required by client (RFC 9110 Section 10.1.1).
	HeaderExpect = headkit.Expect

	// HeaderExpectCT allows certificate transparency compliance enforcement (RFC 9163 Section 2.1).
	HeaderExpectCT = headkit.ExpectCT

	// HeaderExpires conveys date/time after which response is considered stale (RFC 9111 Section 5.3).
	HeaderExpires = headkit.Expires

	// HeaderFeaturePolicy controls browser features available to document (W3C Feature Policy).
	HeaderFeaturePolicy = headkit.FeaturePolicy

	// HeaderForwarded discloses proxy information obscured during forwarding (RFC 7239 Section 4).
	HeaderForwarded = headkit.Forwarded

	// HeaderFrom contains email address for user controlling requesting agent (RFC 9110 Section 10.1.2).
	HeaderFrom = headkit.From

	// HeaderHost specifies target URI host and port number (RFC 9112 Section 7.2, RFC 9110 Section 7.2).
	HeaderHost = headkit.Host

	// HeaderIfMatch enables conditional execution based on matching ETag (RFC 9110 Section 13.1.1).
	HeaderIfMatch = headkit.IfMatch

	// HeaderIfModifiedSince enables conditional execution if modified after date (RFC 9110 Section 13.1.3).
	HeaderIfModifiedSince = headkit.IfModifiedSince

	// HeaderIfNoneMatch enables conditional execution if no ETags match (RFC 9110 Section 13.1.2).
	HeaderIfNoneMatch = headkit.IfNoneMatch

	// HeaderIfRange conditionally retrieves entire entity or requested range (RFC 9110 Section 13.1.5).
	HeaderIfRange = headkit.IfRange

	// HeaderIfUnmodifiedSince enables conditional execution if unmodified since date (RFC 9110 Section 13.1.4).
	HeaderIfUnmodifiedSince = headkit.IfUnmodifiedSince

	// HeaderIndex provides catalog index hints for directory navigation.
	HeaderIndex = headkit.Index

	// HeaderKeepAlive configures persistent connection parameters (RFC 2068 Section 19.7.1.1).
	HeaderKeepAlive = headkit.KeepAlive

	// HeaderLargeAllocation requests larger memory heap allocation for web pages.
	HeaderLargeAllocation = headkit.LargeAllocation

	// HeaderLastEventID conveys last received Server-Sent Events event identifier (W3C SSE).
	HeaderLastEventID = headkit.LastEventID

	// HeaderLastModified conveys date/time when resource was last altered (RFC 9110 Section 8.8.2).
	HeaderLastModified = headkit.LastModified

	// HeaderLink conveys typed relationship with another resource (RFC 8288 Section 3).
	HeaderLink = headkit.Link

	// HeaderLocation specifies target URL for redirect or new resource (RFC 9110 Section 10.2.2).
	HeaderLocation = headkit.Location

	// HeaderMaxForwards limits forwarding hops for TRACE and OPTIONS (RFC 9110 Section 7.6.2).
	HeaderMaxForwards = headkit.MaxForwards

	// HeaderNEL configures Network Error Logging policy (W3C NEL).
	HeaderNEL = headkit.NEL

	// HeaderOrigin indicates origin of request initiator (RFC 6454 Section 7).
	HeaderOrigin = headkit.Origin

	// HeaderPingFrom identifies source URL of hyperlinked ping attribute (W3C HTML).
	HeaderPingFrom = headkit.PingFrom

	// HeaderPingTo identifies target URL of hyperlinked ping attribute (W3C HTML).
	HeaderPingTo = headkit.PingTo

	// HeaderPragma contains legacy implementation-specific directives (RFC 9111 Section 5.4).
	HeaderPragma = headkit.Pragma

	// HeaderProxyAuthenticate defines authentication challenge for proxy access (RFC 9110 Section 11.7.1).
	HeaderProxyAuthenticate = headkit.ProxyAuthenticate

	// HeaderProxyAuthorization conveys client credentials for proxy authentication (RFC 9110 Section 11.7.2).
	HeaderProxyAuthorization = headkit.ProxyAuthorization

	// HeaderProxyConnection specifies hop-by-hop connection directives to intermediary proxies.
	HeaderProxyConnection = headkit.ProxyConnection

	// HeaderPublicKeyPins configures HTTP Public Key Pinning (RFC 7469 Section 2.1).
	HeaderPublicKeyPins = headkit.PublicKeyPins

	// HeaderPublicKeyPinsReportOnly reports HPKP certificate pin violations without blocking (RFC 7469 Section 2.1).
	HeaderPublicKeyPinsReportOnly = headkit.PublicKeyPinsReportOnly

	// HeaderPushPolicy configures server push preferences.
	HeaderPushPolicy = headkit.PushPolicy

	// HeaderRange requests partial representation octet ranges (RFC 9110 Section 14.1.2).
	HeaderRange = headkit.Range

	// HeaderReferer indicates address of resource from which request URI was obtained (RFC 9110 Section 10.1.3).
	HeaderReferer = headkit.Referer

	// HeaderReferrerPolicy controls how much referrer information is included with requests (W3C Referrer Policy §8).
	HeaderReferrerPolicy = headkit.ReferrerPolicy

	// HeaderReportTo configures reporting endpoints for browser security violation reports (W3C Reporting API).
	HeaderReportTo = headkit.ReportTo

	// HeaderRetryAfter indicates when client should retry subsequent request (RFC 9110 Section 10.2.3).
	HeaderRetryAfter = headkit.RetryAfter

	// HeaderSaveData communicates client preference for reduced data usage (W3C Save-Data).
	HeaderSaveData = headkit.SaveData

	// HeaderSecWebSocketAccept verifies server willingness to accept WebSocket upgrade (RFC 6455 Section 4.2.2).
	HeaderSecWebSocketAccept = headkit.SecWebSocketAccept

	// HeaderSecWebSocketExtensions negotiates WebSocket protocol extensions (RFC 6455 Section 4.2.2).
	HeaderSecWebSocketExtensions = headkit.SecWebSocketExtensions // #nosec G101

	// HeaderSecWebSocketKey provides challenge key for WebSocket handshake (RFC 6455 Section 4.2.1).
	HeaderSecWebSocketKey = headkit.SecWebSocketKey

	// HeaderSecWebSocketProtocol specifies requested WebSocket subprotocols (RFC 6455 Section 4.2.2).
	HeaderSecWebSocketProtocol = headkit.SecWebSocketProtocol

	// HeaderSecWebSocketVersion specifies WebSocket protocol version (RFC 6455 Section 4.2.1).
	HeaderSecWebSocketVersion = headkit.SecWebSocketVersion

	// HeaderServer conveys software information about origin server (RFC 9110 Section 10.2.4).
	HeaderServer = headkit.Server

	// HeaderServerTiming communicates performance metrics regarding server operations (W3C Server Timing).
	HeaderServerTiming = headkit.ServerTiming

	// HeaderSetCookie delivers HTTP state cookie to client (RFC 6265 Section 4.1).
	HeaderSetCookie = headkit.SetCookie

	// HeaderSignature contains digital signature covering HTTP message components (IETF HTTP Message Signatures).
	HeaderSignature = headkit.Signature

	// HeaderSignedHeaders lists header names included in message signature verification.
	HeaderSignedHeaders = headkit.SignedHeaders

	// HeaderSourceMap links generated code to its original source map file.
	HeaderSourceMap = headkit.SourceMap

	// HeaderStrictTransportSecurity declares security policy enforcing HTTPS only (RFC 6797 Section 6.1).
	HeaderStrictTransportSecurity = headkit.StrictTransportSecurity

	// HeaderTE indicates transfer codings client is willing to accept (RFC 9110 Section 10.1.4).
	HeaderTE = headkit.TE

	// HeaderTimingAllowOrigin specifies origins allowed to see detailed resource timing (W3C Resource Timing).
	HeaderTimingAllowOrigin = headkit.TimingAllowOrigin

	// HeaderTk indicates tracking status for tracking preference responses (W3C Tracking Protection).
	HeaderTk = headkit.Tk

	// HeaderTrailer indicates header fields present in message chunked trailer section (RFC 9112 Section 7.1.2).
	HeaderTrailer = headkit.Trailer

	// HeaderTransferEncoding specifies coding transformations applied to payload (RFC 9112 Section 6.1).
	HeaderTransferEncoding = headkit.TransferEncoding

	// HeaderUpgrade requests transition to different protocol on current connection (RFC 9110 Section 7.8).
	HeaderUpgrade = headkit.Upgrade

	// HeaderUpgradeInsecureRequests requests client preferences for encrypted connections (W3C Upgrade Insecure Requests).
	HeaderUpgradeInsecureRequests = headkit.UpgradeInsecureRequests

	// HeaderUserAgent conveys client software application and platform identity (RFC 9110 Section 10.1.5).
	HeaderUserAgent = headkit.UserAgent

	// HeaderVary specifies headers determining response representation cacheability (RFC 9110 Section 12.5.5).
	HeaderVary = headkit.Vary

	// HeaderVia tracks intermediaries and proxies forwarding message (RFC 9110 Section 7.6.3).
	HeaderVia = headkit.Via

	// HeaderViewportWidth specifies layout viewport width in CSS pixels (Client Hints).
	HeaderViewportWidth = headkit.ViewportWidth

	// HeaderWarning conveys additional information about response status or caching (RFC 9111 Section 5.5).
	HeaderWarning = headkit.Warning

	// HeaderWidth conveys intended display width of image resource in CSS pixels (Client Hints).
	HeaderWidth = headkit.Width

	// HeaderWWWAuthenticate defines authentication challenge issued by origin server (RFC 9110 Section 11.6.1).
	HeaderWWWAuthenticate = headkit.WWWAuthenticate

	// HeaderXContentTypeOptions prevents MIME type sniffing by client (W3C Fetch §3.1).
	HeaderXContentTypeOptions = headkit.XContentTypeOptions

	// HeaderXDNSPrefetchControl controls browser DNS prefetching behavior.
	HeaderXDNSPrefetchControl = headkit.XDNSPrefetchControl

	// HeaderXDownloadOptions controls file download and execution behavior in Internet Explorer.
	HeaderXDownloadOptions = headkit.XDownloadOptions

	// HeaderXForwardedFor identifies originating IP address of client connecting through proxy.
	HeaderXForwardedFor = headkit.XForwardedFor

	// HeaderXForwardedHost identifies original host requested by client connecting through proxy.
	HeaderXForwardedHost = headkit.XForwardedHost

	// HeaderXForwardedProto identifies protocol scheme (HTTP/HTTPS) requested by client connecting through proxy.
	HeaderXForwardedProto = headkit.XForwardedProto

	// HeaderXFrameOptions controls whether browser may render page inside frame or iframe (RFC 7034 Section 2).
	HeaderXFrameOptions = headkit.XFrameOptions

	// HeaderXPermittedCrossDomainPolicies controls cross-domain policy files for Adobe products.
	HeaderXPermittedCrossDomainPolicies = headkit.XPermittedCrossDomainPolicies

	// HeaderXPingback advertises Pingback XML-RPC server URL (Pingback 1.0).
	HeaderXPingback = headkit.XPingback

	// HeaderXPoweredBy discloses backend framework or execution platform.
	HeaderXPoweredBy = headkit.XPoweredBy

	// HeaderXRequestedWith identifies Ajax / XMLHttpRequest asynchronous requests.
	HeaderXRequestedWith = headkit.XRequestedWith

	// HeaderXRobotsTag instructs search engine crawlers regarding indexing and snippet generation.
	HeaderXRobotsTag = headkit.XRobotsTag

	// HeaderXUACompatible specifies legacy Internet Explorer rendering engine mode.
	HeaderXUACompatible = headkit.XUACompatible

	// HeaderXXSSProtection enables cross-site scripting filter in legacy user agents.
	HeaderXXSSProtection = headkit.XXSSProtection
)
