// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package header provides backward-compatible forwarders to canonical HTTP header names,
// pseudo-headers, MIME media types, HTTP methods, and common header values defined in proto/headkit.
package header

import (
	"github.com/lemon4ksan/mach/proto/headkit"
)

// ============================================================================
// 1. HTTP/2 & HTTP/3 Pseudo-Headers (RFC 7540, RFC 9113, RFC 9114, RFC 8441)
// ============================================================================

const (
	// PseudoMethod represents the HTTP request method in HTTP/2 (RFC 9113 §8.3.1) and HTTP/3 (RFC 9114 §4.3).
	PseudoMethod = headkit.PseudoMethod

	// PseudoScheme represents the target URI scheme ("http" or "https") in HTTP/2 (RFC 9113 §8.3.1) and HTTP/3 (RFC 9114 §4.3).
	PseudoScheme = headkit.PseudoScheme

	// PseudoAuthority represents the target internet host and port in HTTP/2 (RFC 9113 §8.3.1) and HTTP/3 (RFC 9114 §4.3).
	PseudoAuthority = headkit.PseudoAuthority

	// PseudoPath represents the target request path and query string in HTTP/2 (RFC 9113 §8.3.1) and HTTP/3 (RFC 9114 §4.3).
	PseudoPath = headkit.PseudoPath

	// PseudoStatus represents the 3-digit HTTP response status code in HTTP/2 (RFC 9113 §8.3.2) and HTTP/3 (RFC 9114 §4.3).
	PseudoStatus = headkit.PseudoStatus

	// PseudoProtocol specifies the negotiated sub-protocol in Extended CONNECT handshakes (RFC 8441 / RFC 9220).
	PseudoProtocol = headkit.PseudoProtocol
)

// ============================================================================
// 2. Standard Request Headers (RFC 9110 / RFC 9112)
// ============================================================================

const (
	// Accept specifies media types that are acceptable for the response (RFC 9110 §12.5.1).
	Accept = headkit.Accept

	// AcceptCharset specifies character sets that are acceptable for the response (RFC 9110 §12.5.2).
	AcceptCharset = headkit.AcceptCharset

	// AcceptEncoding specifies content coding algorithms (e.g. gzip, br, zstd) acceptable for the response (RFC 9110 §12.5.3).
	AcceptEncoding = headkit.AcceptEncoding

	// AcceptLanguage specifies preferred natural languages for the response (RFC 9110 §12.5.4).
	AcceptLanguage = headkit.AcceptLanguage

	// Authorization contains client credentials for authenticating with the origin server (RFC 9110 §11.6.2).
	Authorization = headkit.Authorization

	// Connection manages connection-specific options (hop-by-hop) such as keep-alive or close (RFC 9110 §7.6.1).
	Connection = headkit.Connection

	// ContentLength specifies the size of the payload body in decimal octets (RFC 9110 §8.6).
	ContentLength = headkit.ContentLength

	// ContentType indicates the media type of the underlying request payload body (RFC 9110 §8.3).
	ContentType = headkit.ContentType

	// Cookie contains stored HTTP state / session cookies previously sent by the server via Set-Cookie (RFC 6265bis).
	Cookie = headkit.Cookie

	// Expect indicates client expectations that must be met by the server (e.g. "100-continue") (RFC 9110 §10.1.1).
	Expect = headkit.Expect

	// From contains an Internet email address for the human user who controls the requesting agent (RFC 9110 §10.1.2).
	From = headkit.From

	// Host specifies the internet host and port number of the target resource (RFC 9112 §3.2 / RFC 9110 §7.2).
	Host = headkit.Host

	// MaxForwards limits the number of times a request can be forwarded by proxies (RFC 9110 §7.6.2).
	MaxForwards = headkit.MaxForwards

	// ProxyAuthorization contains client credentials for authenticating with an intermediary proxy (RFC 9110 §11.7.2).
	ProxyAuthorization = headkit.ProxyAuthorization

	// Range requests one or more partial ranges of the resource payload (RFC 9110 §14.2).
	Range = headkit.Range

	// Referer identifies the address of the resource from which the current URI was obtained (RFC 9110 §10.1.3).
	Referer = headkit.Referer

	// TE specifies the transfer encodings the client is willing to accept and if trailers are supported (RFC 9110 §10.1.4).
	TE = headkit.TE

	// UserAgent contains information about the user agent originating the request (RFC 9110 §10.1.5).
	UserAgent = headkit.UserAgent
)

// ============================================================================
// 3. Standard Response Headers (RFC 9110 / RFC 9112)
// ============================================================================

const (
	// AcceptRanges indicates server support for range requests (e.g. "bytes" or "none") (RFC 9110 §14.1).
	AcceptRanges = headkit.AcceptRanges

	// Allow lists the set of HTTP methods supported by the target resource (RFC 9110 §10.2.1).
	Allow = headkit.Allow

	// ContentDisposition conveys presentation instructions (inline vs attachment) and default filename (RFC 6266).
	ContentDisposition = headkit.ContentDisposition

	// ContentEncoding indicates what compression codings have been applied to the payload (RFC 9110 §8.4).
	ContentEncoding = headkit.ContentEncoding

	// ContentLanguage describes the natural language(s) of the intended audience for the payload (RFC 9110 §8.5).
	ContentLanguage = headkit.ContentLanguage

	// ContentLocation indicates an alternate URI location for the returned payload (RFC 9110 §8.7).
	ContentLocation = headkit.ContentLocation

	// ContentRange indicates where in a full representation a partial payload belongs (RFC 9110 §14.4).
	ContentRange = headkit.ContentRange

	// Date represents the date and time at which the message was generated (RFC 9110 §6.6.1).
	Date = headkit.Date

	// LastModified indicates the date and time at which the origin server believes the selected representation was last modified (RFC 9110 §8.8.2).
	LastModified = headkit.LastModified

	// Location provides a target URI for redirection or newly created resource identification (RFC 9110 §10.2.2).
	Location = headkit.Location

	// ProxyAuthenticate defines the authentication scheme and parameters required by a proxy (RFC 9110 §11.7.1).
	ProxyAuthenticate = headkit.ProxyAuthenticate

	// RetryAfter indicates how long the client should wait before making a follow-up request (RFC 9110 §10.2.3).
	RetryAfter = headkit.RetryAfter

	// Server contains information about the software used by the origin server (RFC 9110 §10.2.4).
	Server = headkit.Server

	// SetCookie transmits session cookies and state from origin server to client (RFC 6265bis).
	SetCookie = headkit.SetCookie

	// Trailer indicates that the listed header fields are present in the chunked trailer section (RFC 9112 §7.1.2).
	Trailer = headkit.Trailer

	// TransferEncoding indicates what transfer codings have been applied to the message body (RFC 9112 §6.1).
	TransferEncoding = headkit.TransferEncoding

	// Upgrade specifies additional communication protocols the client supports and server invites (RFC 9110 §7.8).
	Upgrade = headkit.Upgrade

	// Vary indicates the set of request headers that determined whether a cached response is valid (RFC 9110 §12.5.5).
	Vary = headkit.Vary

	// Via tracks intermediary proxies and protocols between client and server (RFC 9110 §7.6.3).
	Via = headkit.Via

	// Warning carries additional information about the status or transformation of a message (RFC 9111 §5.5).
	Warning = headkit.Warning

	// WWWAuthenticate defines the authentication scheme and parameters required to access the resource (RFC 9110 §11.6.1).
	WWWAuthenticate = headkit.WWWAuthenticate
)

// ============================================================================
// 4. Conditional Request Headers (RFC 9110 §13)
// ============================================================================

const (
	// IfMatch makes the request conditional: execute only if the current ETag matches (RFC 9110 §13.1.1).
	IfMatch = headkit.IfMatch

	// IfNoneMatch makes the request conditional: execute only if no ETags match (304 Not Modified check) (RFC 9110 §13.1.2).
	IfNoneMatch = headkit.IfNoneMatch

	// IfModifiedSince makes the request conditional: execute only if modified since date (RFC 9110 §13.1.3).
	IfModifiedSince = headkit.IfModifiedSince

	// IfUnmodifiedSince makes the request conditional: execute only if not modified since date (RFC 9110 §13.1.4).
	IfUnmodifiedSince = headkit.IfUnmodifiedSince

	// IfRange allows a client to conditionally request a sub-range if the entity is unchanged (RFC 9110 §13.1.5).
	IfRange = headkit.IfRange
)

// ============================================================================
// 5. Caching & Stored Response Headers (RFC 9111 & Extensions)
// ============================================================================

const (
	// Age conveys the sender's estimate of the time elapsed since the response was generated (RFC 9111 §5.1).
	Age = headkit.Age

	// CacheControl specifies directives for caching mechanisms along the request/response chain (RFC 9111 §5.2).
	CacheControl = headkit.CacheControl

	// Expires gives the date/time after which the response is considered stale (RFC 9111 §5.3).
	Expires = headkit.Expires

	// ETag provides an entity tag (opaque identifier) for cache validation (RFC 9110 §8.8.3).
	ETag = headkit.ETag

	// Pragma specifies legacy backward-compatible caching directives (e.g. "no-cache") (RFC 9111 §5.4).
	Pragma = headkit.Pragma

	// ClearSiteData instructs the user agent to clear browser data (cookies, storage, cache) (W3C).
	ClearSiteData = headkit.ClearSiteData

	// CDNCacheControl defines caching directives intended exclusively for CDN intermediaries (RFC 9213).
	CDNCacheControl = headkit.CDNCacheControl

	// SurrogateControl defines caching directives for downstream surrogate caches and reverse proxies.
	SurrogateControl = headkit.SurrogateControl

	// CacheStatus provides diagnostic information about how proxy caches handled the request (RFC 9211).
	CacheStatus = headkit.CacheStatus

	// CDNCacheStatus provides diagnostic information about how CDN edge caches handled the request (RFC 9211).
	CDNCacheStatus = headkit.CDNCacheStatus
)

// ============================================================================
// 6. CORS (Cross-Origin Resource Sharing — W3C / Fetch)
// ============================================================================

const (
	// Origin indicates the origin (scheme, host, port) of the cross-origin request (RFC 6454 / Fetch).
	Origin = headkit.Origin

	// AccessControlAllowOrigin indicates whether the response can be shared with requesting origin (Fetch).
	AccessControlAllowOrigin = headkit.AccessControlAllowOrigin

	// AccessControlAllowMethods specifies the allowed HTTP methods when accessing the resource in CORS (Fetch).
	AccessControlAllowMethods = headkit.AccessControlAllowMethods

	// AccessControlAllowHeaders specifies the allowed HTTP headers when accessing the resource in CORS (Fetch).
	AccessControlAllowHeaders = headkit.AccessControlAllowHeaders

	// AccessControlAllowCredentials indicates whether credentials (cookies, auth) can be exposed in CORS (Fetch).
	AccessControlAllowCredentials = headkit.AccessControlAllowCredentials

	// AccessControlExposeHeaders lists the response headers safe to expose to client-side scripts (Fetch).
	AccessControlExposeHeaders = headkit.AccessControlExposeHeaders

	// AccessControlMaxAge indicates how long the results of a CORS preflight request can be cached (Fetch).
	AccessControlMaxAge = headkit.AccessControlMaxAge

	// AccessControlRequestMethod indicates which HTTP method will be used in the actual CORS request (Fetch).
	AccessControlRequestMethod = headkit.AccessControlRequestMethod

	// AccessControlRequestHeaders indicates which HTTP headers will be used in the actual CORS request (Fetch).
	AccessControlRequestHeaders = headkit.AccessControlRequestHeaders
)

// ============================================================================
// 7. Security & Context Isolation Headers (W3C / RFC)
// ============================================================================

const (
	// ContentSecurityPolicy defines approved sources of content that browser may load (CSP Level 3).
	ContentSecurityPolicy = headkit.ContentSecurityPolicy

	// ContentSecurityPolicyReportOnly monitors and reports CSP violations without enforcement (CSP Level 3).
	ContentSecurityPolicyReportOnly = headkit.ContentSecurityPolicyReportOnly

	// StrictTransportSecurity enforces secure (HTTPS) connections and prevents SSL stripping (HSTS - RFC 6797).
	StrictTransportSecurity = headkit.StrictTransportSecurity

	// XContentTypeOptions prevents browsers from MIME-sniffing a response away from declared Content-Type (RFC 9110).
	XContentTypeOptions = headkit.XContentTypeOptions

	// XFrameOptions indicates whether a browser should be allowed to render a page in a frame/iframe (RFC 7034).
	XFrameOptions = headkit.XFrameOptions

	// XXSSProtection configures the legacy browser Cross-Site Scripting (XSS) filter.
	XXSSProtection = headkit.XXSSProtection

	// ReferrerPolicy controls how much referrer information is included with requests (W3C).
	ReferrerPolicy = headkit.ReferrerPolicy

	// PermissionsPolicy allows site developers to restrict browser features and APIs (W3C).
	PermissionsPolicy = headkit.PermissionsPolicy

	// CrossOriginOpenerPolicy isolates top-level browsing contexts to prevent cross-origin attacks (COOP - W3C).
	CrossOriginOpenerPolicy = headkit.CrossOriginOpenerPolicy

	// CrossOriginEmbedderPolicy prevents a document from loading cross-origin resources without permission (COEP - W3C).
	CrossOriginEmbedderPolicy = headkit.CrossOriginEmbedderPolicy

	// CrossOriginResourcePolicy protects resources from being embedded by cross-origin documents (CORP - W3C).
	CrossOriginResourcePolicy = headkit.CrossOriginResourcePolicy

	// PublicKeyPins associates a specific cryptographic public key with a certain web server (HPKP - RFC 7469).
	PublicKeyPins = headkit.PublicKeyPins

	// PublicKeyPinsReportOnly sends reports of HPKP pinning failures without enforcement (RFC 7469).
	PublicKeyPinsReportOnly = headkit.PublicKeyPinsReportOnly

	// XPermittedCrossDomainPolicies restricts Adobe Flash / PDF cross-domain policy files.
	XPermittedCrossDomainPolicies = headkit.XPermittedCrossDomainPolicies
)

// ============================================================================
// 8. Fetch Metadata & Client Hints (W3C)
// ============================================================================

const (
	// SecFetchDest indicates the initiator's target destination for the requested data (e.g. "document", "image") (W3C).
	SecFetchDest = headkit.SecFetchDest

	// SecFetchMode indicates the mode of the request (e.g. "navigate", "cors", "no-cors") (W3C).
	SecFetchMode = headkit.SecFetchMode

	// SecFetchSite indicates the relationship between request initiator's origin and target origin (W3C).
	SecFetchSite = headkit.SecFetchSite

	// SecFetchUser indicates whether the navigation request was triggered by explicit user gesture (W3C).
	SecFetchUser = headkit.SecFetchUser

	// SecCHUA provides user agent's branding and major version list (Client Hints - W3C).
	SecCHUA = headkit.SecCHUA

	// SecCHUAArch provides target device architecture (e.g. "x86", "arm") (Client Hints - W3C).
	SecCHUAArch = headkit.SecCHUAArch

	// SecCHUABitness provides CPU architecture bitness (e.g. "64") (Client Hints - W3C).
	SecCHUABitness = headkit.SecCHUABitness

	// SecCHUAFullVersion provides full user agent version string (Client Hints - W3C).
	SecCHUAFullVersion = headkit.SecCHUAFullVersion

	// SecCHUAFullVersionList provides full brand and version list (Client Hints - W3C).
	SecCHUAFullVersionList = headkit.SecCHUAFullVersionList

	// SecCHUAMobile indicates whether the browser is running on a mobile device ("?1" or "?0") (Client Hints - W3C).
	SecCHUAMobile = headkit.SecCHUAMobile

	// SecCHUAModel provides the device model on which user agent is running (Client Hints - W3C).
	SecCHUAModel = headkit.SecCHUAModel

	// SecCHUAPlatform provides the operating system platform (e.g. "Windows", "Linux", "Android") (Client Hints - W3C).
	SecCHUAPlatform = headkit.SecCHUAPlatform

	// SecCHUAPlatformVersion provides the operating system platform version (Client Hints - W3C).
	SecCHUAPlatformVersion = headkit.SecCHUAPlatformVersion

	// SecCHUAFormFactors provides the form factors for the device (Client Hints - W3C).
	SecCHUAFormFactors = headkit.SecCHUAFormFactors

	// SecCHWidth provides the physical pixel width of the target display (Client Hints - W3C).
	SecCHWidth = headkit.SecCHWidth

	// SecCHDPR provides the device pixel ratio (Client Hints - W3C).
	SecCHDPR = headkit.SecCHDPR

	// SecCHViewportWidth provides the viewport width in CSS pixels (Client Hints - W3C).
	SecCHViewportWidth = headkit.SecCHViewportWidth

	// SecCHPrefersColorScheme indicates user's dark/light color scheme preference (Client Hints - W3C).
	SecCHPrefersColorScheme = headkit.SecCHPrefersColorScheme

	// SecCHPrefersReducedMotion indicates user's motion animation reduction preference (Client Hints - W3C).
	SecCHPrefersReducedMotion = headkit.SecCHPrefersReducedMotion

	// AcceptCH informs the client which Client Hints headers the server is requesting (W3C).
	AcceptCH = headkit.AcceptCH

	// CriticalCH informs the client which Client Hints are critical for page rendering (W3C).
	CriticalCH = headkit.CriticalCH
)

// ============================================================================
// 9. Modern Transport, Protocol Upgrades & WebSockets (RFC 6455, 7838, 9218)
// ============================================================================

const (
	// AltSvc advertises alternative services and protocol endpoints (e.g. HTTP/3 over QUIC) (RFC 7838).
	AltSvc = headkit.AltSvc

	// AltUsed indicates the alternative service host that was actually used (RFC 7838).
	AltUsed = headkit.AltUsed

	// SecWebSocketKey provides a random client nonce for the WebSocket opening handshake (RFC 6455).
	SecWebSocketKey = headkit.SecWebSocketKey

	// SecWebSocketAccept provides the server's cryptographic confirmation token in WebSocket handshake (RFC 6455).
	SecWebSocketAccept = headkit.SecWebSocketAccept

	// SecWebSocketVersion indicates the WebSocket protocol version requested by the client (RFC 6455).
	SecWebSocketVersion = headkit.SecWebSocketVersion

	// SecWebSocketProtocol indicates the client-requested or server-selected WebSocket sub-protocol (RFC 6455).
	SecWebSocketProtocol = headkit.SecWebSocketProtocol

	// SecWebSocketExtensions specifies WebSocket protocol level extensions (e.g. permessage-deflate) (RFC 6455).
	SecWebSocketExtensions = headkit.SecWebSocketExtensions

	// Priority carries end-to-end stream prioritization parameters (RFC 9218).
	Priority = headkit.Priority

	// UpgradeInsecureRequests signals server that client supports automatic redirection to HTTPS (W3C).
	UpgradeInsecureRequests = headkit.UpgradeInsecureRequests
)

// ============================================================================
// 10. Distributed Tracing & Telemetry (W3C Trace Context, OpenTelemetry, Zipkin)
// ============================================================================

const (
	// Traceparent identifies the incoming request in a tracing system conforming to W3C Trace Context.
	Traceparent = headkit.Traceparent

	// Tracestate carries vendor-specific state routing information conforming to W3C Trace Context.
	Tracestate = headkit.Tracestate

	// Baggage carries cross-cutting contextual key-value properties conforming to W3C Baggage.
	Baggage = headkit.Baggage

	// XRequestID carries a unique correlation token identifying an HTTP request across services.
	XRequestID = headkit.XRequestID

	// XCorrelationID carries a distributed transaction correlation identifier.
	XCorrelationID = headkit.XCorrelationID

	// XTraceID carries a trace identifier for distributed request tracing.
	XTraceID = headkit.XTraceID

	// XSpanID carries a span identifier within a distributed execution trace.
	XSpanID = headkit.XSpanID

	// XB3TraceID carries the Zipkin B3 trace identifier.
	XB3TraceID = headkit.XB3TraceID

	// XB3SpanID carries the Zipkin B3 span identifier.
	XB3SpanID = headkit.XB3SpanID

	// XB3Sampled carries the Zipkin B3 sampling decision flag ("1" or "0").
	XB3Sampled = headkit.XB3Sampled

	// XB3Flags carries the Zipkin B3 debug flag.
	XB3Flags = headkit.XB3Flags

	// XB3ParentSpanID carries the Zipkin B3 parent span identifier.
	XB3ParentSpanID = headkit.XB3ParentSpanID

	// XAmznTraceID carries AWS Application Load Balancer / X-Ray trace context data.
	XAmznTraceID = headkit.XAmznTraceID
)

// ============================================================================
// 11. Reverse Proxy, Load Balancer & CDN Headers
// ============================================================================

const (
	// Forwarded contains standardized proxy forwarding information (RFC 7239).
	Forwarded = headkit.Forwarded

	// XForwardedFor carries the de-facto client IP address through HTTP proxies and load balancers.
	XForwardedFor = headkit.XForwardedFor

	// XForwardedProto carries the de-facto original request protocol scheme ("http" or "https").
	XForwardedProto = headkit.XForwardedProto

	// XForwardedHost carries the de-facto original host requested by the client.
	XForwardedHost = headkit.XForwardedHost

	// XForwardedPort carries the de-facto original destination port requested by the client.
	XForwardedPort = headkit.XForwardedPort

	// XForwardedPrefix carries the URL prefix path stripped by a reverse proxy.
	XForwardedPrefix = headkit.XForwardedPrefix

	// XForwardedServer carries the reverse proxy hostname.
	XForwardedServer = headkit.XForwardedServer

	// XRealIP carries the immediate remote IP address forwarded by Nginx or reverse proxies.
	XRealIP = headkit.XRealIP

	// CFConnectingIP provides the original client IP address connecting to Cloudflare CDN.
	CFConnectingIP = headkit.CFConnectingIP

	// CFConnectingIPv6 provides the original client IPv6 address connecting to Cloudflare CDN.
	CFConnectingIPv6 = headkit.CFConnectingIPv6

	// CFRay provides the unique Cloudflare Ray ID request tracking hash.
	CFRay = headkit.CFRay

	// CFIPCountry provides the two-letter ISO-3166-1 country code of the client IP connecting to Cloudflare.
	CFIPCountry = headkit.CFIPCountry

	// CFVisitor provides Cloudflare visitor scheme metadata (JSON object).
	CFVisitor = headkit.CFVisitor

	// TrueClientIP provides the original client IP address on Akamai and Cloudflare Enterprise networks.
	TrueClientIP = headkit.TrueClientIP

	// FastlyClientIP provides the original client IP address connecting to Fastly CDN.
	FastlyClientIP = headkit.FastlyClientIP

	// XAccelBuffering controls internal response buffering in Nginx ("yes" or "no").
	XAccelBuffering = headkit.XAccelBuffering

	// XAccelRedirect initiates an internal redirection to a protected URI in Nginx.
	XAccelRedirect = headkit.XAccelRedirect

	// DNT indicates the user's tracking preference (Do Not Track, W3C).
	DNT = headkit.DNT

	// EarlyData indicates that the request was conveyed in TLS early data (RFC 8470).
	EarlyData = headkit.EarlyData

	// ExpectCT enables Certificate Transparency monitoring and enforcement (RFC 9163).
	ExpectCT = headkit.ExpectCT

	// KeepAlive communicates connection persistence parameters (RFC 2068 §19.7.1).
	KeepAlive = headkit.KeepAlive

	// LastEventID contains the event ID of the last event received by the client (W3C EventSource).
	LastEventID = headkit.LastEventID

	// Link specifies relationship between current document and external resources (RFC 8288).
	Link = headkit.Link

	// ServerTiming communicates performance metrics about the request-response cycle (W3C Server Timing).
	ServerTiming = headkit.ServerTiming

	// XDNSPrefetchControl controls DNS prefetching in modern browsers (Chromium / W3C).
	XDNSPrefetchControl = headkit.XDNSPrefetchControl
)

// ============================================================================
// 12. Authentication, Authorization & DPoP (RFC 9449, RFC 9421)
// ============================================================================

const (
	// DPoP contains an RFC 9449 Demonstrating Proof-of-Possession JWT proof header.
	DPoP = headkit.DPoP

	// DPoPNonce contains a server-provided nonce for DPoP replay protection (RFC 9449 §4.3).
	DPoPNonce = headkit.DPoPNonce

	// Signature carries an RFC 9421 HTTP message cryptographic signature.
	Signature = headkit.Signature

	// SignatureInput carries metadata and component parameters for RFC 9421 HTTP message signatures.
	SignatureInput = headkit.SignatureInput

	// AcceptSignature advertises acceptable signature parameters and algorithms (RFC 9421).
	AcceptSignature = headkit.AcceptSignature

	// Digest provides an instance digest of the payload representation (RFC 3230).
	Digest = headkit.Digest

	// WantDigest requests instance digest calculation for the response (RFC 3230).
	WantDigest = headkit.WantDigest
)

// ============================================================================
// 13. gRPC & gRPC-Web (Official gRPC Wire Protocol)
// ============================================================================

const (
	// GRPCStatus conveys the canonical gRPC status code in response trailers.
	GRPCStatus = headkit.GRPCStatus

	// GRPCMessage conveys an error description message in response trailers.
	GRPCMessage = headkit.GRPCMessage

	// GRPCTimeout specifies the client request deadline timeout duration.
	GRPCTimeout = headkit.GRPCTimeout

	// GRPCEncoding specifies message compression coding applied to the gRPC frame stream.
	GRPCEncoding = headkit.GRPCEncoding

	// GRPCAcceptEncoding specifies supported message compression codings for gRPC frames.
	GRPCAcceptEncoding = headkit.GRPCAcceptEncoding

	// GRPCStatusDetailsBin carries binary Base64-encoded google.rpc.Status protobuf details.
	GRPCStatusDetailsBin = headkit.GRPCStatusDetailsBin

	// GRPCPreviousRPCAttempts indicates how many times the RPC call was previously attempted.
	GRPCPreviousRPCAttempts = headkit.GRPCPreviousRPCAttempts

	// GRPCRetryPushbackMS indicates server backoff pushback duration in milliseconds.
	GRPCRetryPushbackMS = headkit.GRPCRetryPushbackMS

	// XGRPCWeb indicates gRPC-Web transport framing protocol.
	XGRPCWeb = headkit.XGRPCWeb

	// ContentTransferEncoding indicates MIME encoding in text-mode gRPC-Web (e.g. "base64").
	ContentTransferEncoding = headkit.ContentTransferEncoding
)

// ============================================================================
// 14. Standard MIME Media Types (Content-Type Constants)
// ============================================================================

const (
	// MIMEApplicationJSON represents standard JSON data (RFC 8259).
	MIMEApplicationJSON = headkit.MIMEApplicationJSON

	// MIMEApplicationJSONCharsetUTF8 represents standard JSON data explicitly with UTF-8 charset.
	MIMEApplicationJSONCharsetUTF8 = headkit.MIMEApplicationJSONCharsetUTF8

	// MIMEApplicationProblemJSON represents standard RFC 7807 problem details JSON.
	MIMEApplicationProblemJSON = headkit.MIMEApplicationProblemJSON

	// MIMEApplicationForm represents standard URL-encoded form data.
	MIMEApplicationForm = headkit.MIMEApplicationForm

	// MIMEMultipartFormData represents multipart form data payload (RFC 7578).
	MIMEMultipartFormData = headkit.MIMEMultipartFormData

	// MIMEMultipartByteRanges represents multipart partial byte ranges (RFC 9110 §14.5).
	MIMEMultipartByteRanges = headkit.MIMEMultipartByteRanges

	// MIMEApplicationProtobuf represents binary Protocol Buffer payloads.
	MIMEApplicationProtobuf = headkit.MIMEApplicationProtobuf

	// MIMEApplicationGRPC represents standard binary gRPC stream frames.
	MIMEApplicationGRPC = headkit.MIMEApplicationGRPC

	// MIMEApplicationGRPCWeb represents binary gRPC-Web frames over HTTP/1.1 and HTTP/2.
	MIMEApplicationGRPCWeb = headkit.MIMEApplicationGRPCWeb

	// MIMEApplicationGRPCWebProto represents Protobuf-encoded gRPC-Web frames.
	MIMEApplicationGRPCWebProto = headkit.MIMEApplicationGRPCWebProto

	// MIMEApplicationGRPCWebText represents Base64 text-encoded gRPC-Web frames.
	MIMEApplicationGRPCWebText = headkit.MIMEApplicationGRPCWebText

	// MIMEApplicationXML represents standard XML data (RFC 7303).
	MIMEApplicationXML = headkit.MIMEApplicationXML

	// MIMEApplicationYAML represents YAML data.
	MIMEApplicationYAML = headkit.MIMEApplicationYAML

	// MIMEApplicationOctetStream represents arbitrary binary data (RFC 2046).
	MIMEApplicationOctetStream = headkit.MIMEApplicationOctetStream

	// MIMEApplicationPDF represents Portable Document Format (PDF) files.
	MIMEApplicationPDF = headkit.MIMEApplicationPDF

	// MIMEApplicationZIP represents standard ZIP archives.
	MIMEApplicationZIP = headkit.MIMEApplicationZIP

	// MIMEApplicationGzip represents Gzip-compressed archive streams.
	MIMEApplicationGzip = headkit.MIMEApplicationGzip

	// MIMEApplicationZstd represents Zstandard-compressed data streams.
	MIMEApplicationZstd = headkit.MIMEApplicationZstd

	// MIMEApplicationJWT represents JSON Web Token (JWT) data (RFC 7519).
	MIMEApplicationJWT = headkit.MIMEApplicationJWT

	// MIMEApplicationCBOR represents Concise Binary Object Representation (CBOR) data (RFC 8949).
	MIMEApplicationCBOR = headkit.MIMEApplicationCBOR

	// MIMETextPlain represents plain text without formatting (RFC 2046).
	MIMETextPlain = headkit.MIMETextPlain

	// MIMETextPlainCharsetUTF8 represents plain text explicitly encoded in UTF-8.
	MIMETextPlainCharsetUTF8 = headkit.MIMETextPlainCharsetUTF8

	// MIMETextHTML represents HTML document data (W3C HTML5).
	MIMETextHTML = headkit.MIMETextHTML

	// MIMETextHTMLCharsetUTF8 represents HTML document data explicitly encoded in UTF-8.
	MIMETextHTMLCharsetUTF8 = headkit.MIMETextHTMLCharsetUTF8

	// MIMETextCSS represents Cascading Style Sheets (CSS).
	MIMETextCSS = headkit.MIMETextCSS

	// MIMETextJavaScript represents JavaScript source code (RFC 9239).
	MIMETextJavaScript = headkit.MIMETextJavaScript

	// MIMETextEventStream represents Server-Sent Events (SSE) stream (W3C).
	MIMETextEventStream = headkit.MIMETextEventStream

	// MIMEImagePNG represents Portable Network Graphics (PNG) image.
	MIMEImagePNG = headkit.MIMEImagePNG

	// MIMEImageJPEG represents JPEG image data.
	MIMEImageJPEG = headkit.MIMEImageJPEG

	// MIMEImageGIF represents Graphics Interchange Format (GIF) image.
	MIMEImageGIF = headkit.MIMEImageGIF

	// MIMEImageWebP represents WebP image data.
	MIMEImageWebP = headkit.MIMEImageWebP

	// MIMEImageSVG represents Scalable Vector Graphics (SVG) data.
	MIMEImageSVG = headkit.MIMEImageSVG

	// MIMEImageAVIF represents AV1 Image File Format (AVIF) image.
	MIMEImageAVIF = headkit.MIMEImageAVIF

	// MIMEImageICO represents Microsoft Icon (ICO) format.
	MIMEImageICO = headkit.MIMEImageICO
)

// ============================================================================
// 15. Standard HTTP Methods (RFC 9110 §9)
// ============================================================================

const (
	// MethodGet requests transfer of a current representation of the target resource (RFC 9110 §9.3.1).
	MethodGet = headkit.MethodGet

	// MethodHead is identical to GET except that the server must not return a message body (RFC 9110 §9.3.2).
	MethodHead = headkit.MethodHead

	// MethodPost requests that target resource process representation enclosed in payload (RFC 9110 §9.3.3).
	MethodPost = headkit.MethodPost

	// MethodPut requests that state of target resource be created or replaced with payload (RFC 9110 §9.3.4).
	MethodPut = headkit.MethodPut

	// MethodPatch requests that a set of changes described in payload be applied to target (RFC 5789).
	MethodPatch = headkit.MethodPatch

	// MethodDelete requests that target resource be deleted (RFC 9110 §9.3.5).
	MethodDelete = headkit.MethodDelete

	// MethodConnect establishes a tunnel to the server identified by target URI (RFC 9110 §9.3.6).
	MethodConnect = headkit.MethodConnect

	// MethodOptions requests information about communication options for target URI (RFC 9110 §9.3.7).
	MethodOptions = headkit.MethodOptions

	// MethodTrace performs a message loop-back test along path to target resource (RFC 9110 §9.3.8).
	MethodTrace = headkit.MethodTrace
)

// ============================================================================
// 16. Standard Common Header Values
// ============================================================================

const (
	// ValueKeepAlive indicates that the connection should remain open for subsequent requests.
	ValueKeepAlive = headkit.ValueKeepAlive

	// ValueClose indicates that the connection will be closed after completion of the response.
	ValueClose = headkit.ValueClose

	// ValueChunked indicates chunked transfer encoding framing.
	ValueChunked = headkit.ValueChunked

	// ValueNoCache instructs caches to submit request to origin server for validation before serving stored response.
	ValueNoCache = headkit.ValueNoCache

	// ValueNoStore instructs caches not to store any part of the request or response.
	ValueNoStore = headkit.ValueNoStore

	// ValueMustRevalidate instructs caches that once response is stale, they must revalidate with origin server.
	ValueMustRevalidate = headkit.ValueMustRevalidate

	// ValueNoTransform instructs intermediaries not to transform or modify the payload.
	ValueNoTransform = headkit.ValueNoTransform

	// ValuePublic marks response as cacheable by any cache (including shared/proxy caches).
	ValuePublic = headkit.ValuePublic

	// ValuePrivate marks response as intended for a single user (not to be stored by shared caches).
	ValuePrivate = headkit.ValuePrivate

	// ValueImmutable indicates that response body will not change over time.
	ValueImmutable = headkit.ValueImmutable

	// ValueMaxAge0 indicates zero cache validity duration (immediate expiration).
	ValueMaxAge0 = headkit.ValueMaxAge0

	// ValueGzip indicates Gzip content coding algorithm.
	ValueGzip = headkit.ValueGzip

	// ValueDeflate indicates Deflate content coding algorithm.
	ValueDeflate = headkit.ValueDeflate

	// ValueBrotli indicates Brotli ("br") content coding algorithm.
	ValueBrotli = headkit.ValueBrotli

	// ValueZstd indicates Zstandard ("zstd") content coding algorithm.
	ValueZstd = headkit.ValueZstd

	// ValueTrailers indicates that trailing headers will be sent at end of chunked payload.
	ValueTrailers = headkit.ValueTrailers

	// ValueIdentity indicates absence of content coding (unmodified raw data).
	ValueIdentity = headkit.ValueIdentity

	// ValueWildcard represents the wildcard character "*".
	ValueWildcard = headkit.ValueWildcard

	// ValueNone indicates absence of feature or capability (e.g. Accept-Ranges: none).
	ValueNone = headkit.ValueNone

	// ValueSameOrigin represents the "same-origin" policy / context value.
	ValueSameOrigin = headkit.ValueSameOrigin

	// ValueCrossOrigin represents the "cross-origin" policy / context value.
	ValueCrossOrigin = headkit.ValueCrossOrigin

	// Value100Continue represents the "100-continue" Expect header value.
	Value100Continue = headkit.Value100Continue

	// ValueBearer represents the "Bearer" HTTP authorization scheme prefix.
	ValueBearer = headkit.ValueBearer

	// ValueBasic represents the "Basic" HTTP authorization scheme prefix.
	ValueBasic = headkit.ValueBasic

	// ValueDPoP represents the "DPoP" HTTP authorization scheme prefix.
	ValueDPoP = headkit.ValueDPoP

	// ValueWebSocket represents the "websocket" Upgrade token.
	ValueWebSocket = headkit.ValueWebSocket

	// ValueUpgrade represents the "Upgrade" Connection token.
	ValueUpgrade = headkit.ValueUpgrade

	// ValueBytes represents the "bytes" Accept-Ranges token.
	ValueBytes = headkit.ValueBytes
)
