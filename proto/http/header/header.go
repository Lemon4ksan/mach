// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package header provides official, canonical HTTP header names, pseudo-headers,
// MIME media types, HTTP methods, and common header values conforming strictly to
// IANA registry, RFC 9110, RFC 9111, RFC 9112, RFC 7540, RFC 9113, RFC 9114,
// RFC 8441, RFC 9220, RFC 9449, W3C Fetch Metadata, and W3C Trace Context.
package header

// ============================================================================
// 1. HTTP/2 & HTTP/3 Pseudo-Headers (RFC 7540, RFC 9113, RFC 9114, RFC 8441)
// ============================================================================

const (
	// PseudoMethod represents the HTTP request method in HTTP/2 (RFC 9113 §8.3.1) and HTTP/3 (RFC 9114 §4.3).
	PseudoMethod = ":method"

	// PseudoScheme represents the target URI scheme ("http" or "https") in HTTP/2 (RFC 9113 §8.3.1) and HTTP/3 (RFC 9114 §4.3).
	PseudoScheme = ":scheme"

	// PseudoAuthority represents the target internet host and port in HTTP/2 (RFC 9113 §8.3.1) and HTTP/3 (RFC 9114 §4.3).
	PseudoAuthority = ":authority"

	// PseudoPath represents the target request path and query string in HTTP/2 (RFC 9113 §8.3.1) and HTTP/3 (RFC 9114 §4.3).
	PseudoPath = ":path"

	// PseudoStatus represents the 3-digit HTTP response status code in HTTP/2 (RFC 9113 §8.3.2) and HTTP/3 (RFC 9114 §4.3).
	PseudoStatus = ":status"

	// PseudoProtocol specifies the negotiated sub-protocol in Extended CONNECT handshakes (RFC 8441 / RFC 9220).
	PseudoProtocol = ":protocol"
)

// ============================================================================
// 2. Standard Request Headers (RFC 9110 / RFC 9112)
// ============================================================================

const (
	// Accept specifies media types that are acceptable for the response (RFC 9110 §12.5.1).
	Accept = "Accept"

	// AcceptCharset specifies character sets that are acceptable for the response (RFC 9110 §12.5.2).
	AcceptCharset = "Accept-Charset"

	// AcceptEncoding specifies content coding algorithms (e.g. gzip, br, zstd) acceptable for the response (RFC 9110 §12.5.3).
	AcceptEncoding = "Accept-Encoding"

	// AcceptLanguage specifies preferred natural languages for the response (RFC 9110 §12.5.4).
	AcceptLanguage = "Accept-Language"

	// Authorization contains client credentials for authenticating with the origin server (RFC 9110 §11.6.2).
	Authorization = "Authorization"

	// Connection manages connection-specific options (hop-by-hop) such as keep-alive or close (RFC 9110 §7.6.1).
	Connection = "Connection"

	// ContentLength specifies the size of the payload body in decimal octets (RFC 9110 §8.6).
	ContentLength = "Content-Length"

	// ContentType indicates the media type of the underlying request payload body (RFC 9110 §8.3).
	ContentType = "Content-Type"

	// Cookie contains stored HTTP state / session cookies previously sent by the server via Set-Cookie (RFC 6265bis).
	Cookie = "Cookie"

	// Expect indicates client expectations that must be met by the server (e.g. "100-continue") (RFC 9110 §10.1.1).
	Expect = "Expect"

	// From contains an Internet email address for the human user who controls the requesting agent (RFC 9110 §10.1.2).
	From = "From"

	// Host specifies the internet host and port number of the target resource (RFC 9112 §3.2 / RFC 9110 §7.2).
	Host = "Host"

	// MaxForwards limits the number of times a request can be forwarded by proxies (RFC 9110 §7.6.2).
	MaxForwards = "Max-Forwards"

	// ProxyAuthorization contains client credentials for authenticating with an intermediary proxy (RFC 9110 §11.7.2).
	ProxyAuthorization = "Proxy-Authorization"

	// Range requests one or more partial ranges of the resource payload (RFC 9110 §14.2).
	Range = "Range"

	// Referer identifies the address of the resource from which the current URI was obtained (RFC 9110 §10.1.3).
	Referer = "Referer"

	// TE specifies the transfer encodings the client is willing to accept and if trailers are supported (RFC 9110 §10.1.4).
	TE = "TE"

	// UserAgent contains information about the user agent originating the request (RFC 9110 §10.1.5).
	UserAgent = "User-Agent"
)

// ============================================================================
// 3. Standard Response Headers (RFC 9110 / RFC 9112)
// ============================================================================

const (
	// AcceptRanges indicates server support for range requests (e.g. "bytes" or "none") (RFC 9110 §14.1).
	AcceptRanges = "Accept-Ranges"

	// Allow lists the set of HTTP methods supported by the target resource (RFC 9110 §10.2.1).
	Allow = "Allow"

	// ContentDisposition conveys presentation instructions (inline vs attachment) and default filename (RFC 6266).
	ContentDisposition = "Content-Disposition"

	// ContentEncoding indicates what compression codings have been applied to the payload (RFC 9110 §8.4).
	ContentEncoding = "Content-Encoding"

	// ContentLanguage describes the natural language(s) of the intended audience for the payload (RFC 9110 §8.5).
	ContentLanguage = "Content-Language"

	// ContentLocation indicates an alternate URI location for the returned payload (RFC 9110 §8.7).
	ContentLocation = "Content-Location"

	// ContentRange indicates where in a full representation a partial payload belongs (RFC 9110 §14.4).
	ContentRange = "Content-Range"

	// Date represents the date and time at which the message was generated (RFC 9110 §6.6.1).
	Date = "Date"

	// LastModified indicates the date and time at which the origin server believes the selected representation was last modified (RFC 9110 §8.8.2).
	LastModified = "Last-Modified"

	// Location provides a target URI for redirection or newly created resource identification (RFC 9110 §10.2.2).
	Location = "Location"

	// ProxyAuthenticate defines the authentication scheme and parameters required by a proxy (RFC 9110 §11.7.1).
	ProxyAuthenticate = "Proxy-Authenticate"

	// RetryAfter indicates how long the client should wait before making a follow-up request (RFC 9110 §10.2.3).
	RetryAfter = "Retry-After"

	// Server contains information about the software used by the origin server (RFC 9110 §10.2.4).
	Server = "Server"

	// SetCookie transmits session cookies and state from origin server to client (RFC 6265bis).
	SetCookie = "Set-Cookie"

	// Trailer indicates that the listed header fields are present in the chunked trailer section (RFC 9112 §7.1.2).
	Trailer = "Trailer"

	// TransferEncoding indicates what transfer codings have been applied to the message body (RFC 9112 §6.1).
	TransferEncoding = "Transfer-Encoding"

	// Upgrade specifies additional communication protocols the client supports and server invites (RFC 9110 §7.8).
	Upgrade = "Upgrade"

	// Vary indicates the set of request headers that determined whether a cached response is valid (RFC 9110 §12.5.5).
	Vary = "Vary"

	// Via tracks intermediary proxies and protocols between client and server (RFC 9110 §7.6.3).
	Via = "Via"

	// Warning carries additional information about the status or transformation of a message (RFC 9111 §5.5).
	Warning = "Warning"

	// WWWAuthenticate defines the authentication scheme and parameters required to access the resource (RFC 9110 §11.6.1).
	WWWAuthenticate = "WWW-Authenticate"
)

// ============================================================================
// 4. Conditional Request Headers (RFC 9110 §13)
// ============================================================================

const (
	// IfMatch makes the request conditional: execute only if the current ETag matches (RFC 9110 §13.1.1).
	IfMatch = "If-Match"

	// IfNoneMatch makes the request conditional: execute only if no ETags match (304 Not Modified check) (RFC 9110 §13.1.2).
	IfNoneMatch = "If-None-Match"

	// IfModifiedSince makes the request conditional: execute only if modified since date (RFC 9110 §13.1.3).
	IfModifiedSince = "If-Modified-Since"

	// IfUnmodifiedSince makes the request conditional: execute only if not modified since date (RFC 9110 §13.1.4).
	IfUnmodifiedSince = "If-Unmodified-Since"

	// IfRange allows a client to conditionally request a sub-range if the entity is unchanged (RFC 9110 §13.1.5).
	IfRange = "If-Range"
)

// ============================================================================
// 5. Caching & Stored Response Headers (RFC 9111 & Extensions)
// ============================================================================

const (
	// Age conveys the sender's estimate of the time elapsed since the response was generated (RFC 9111 §5.1).
	Age = "Age"

	// CacheControl specifies directives for caching mechanisms along the request/response chain (RFC 9111 §5.2).
	CacheControl = "Cache-Control"

	// Expires gives the date/time after which the response is considered stale (RFC 9111 §5.3).
	Expires = "Expires"

	// ETag provides an entity tag (opaque identifier) for cache validation (RFC 9110 §8.8.3).
	ETag = "ETag"

	// Pragma specifies legacy backward-compatible caching directives (e.g. "no-cache") (RFC 9111 §5.4).
	Pragma = "Pragma"

	// ClearSiteData instructs the user agent to clear browser data (cookies, storage, cache) (W3C).
	ClearSiteData = "Clear-Site-Data"

	// CDNCacheControl defines caching directives intended exclusively for CDN intermediaries (RFC 9213).
	CDNCacheControl = "CDN-Cache-Control"

	// SurrogateControl defines caching directives for downstream surrogate caches and reverse proxies.
	SurrogateControl = "Surrogate-Control"

	// CacheStatus provides diagnostic information about how proxy caches handled the request (RFC 9211).
	CacheStatus = "Cache-Status"

	// CDNCacheStatus provides diagnostic information about how CDN edge caches handled the request (RFC 9211).
	CDNCacheStatus = "CDN-Cache-Status"
)

// ============================================================================
// 6. CORS (Cross-Origin Resource Sharing — W3C / Fetch)
// ============================================================================

const (
	// Origin indicates the origin (scheme, host, port) of the cross-origin request (RFC 6454 / Fetch).
	Origin = "Origin"

	// AccessControlAllowOrigin indicates whether the response can be shared with requesting origin (Fetch).
	AccessControlAllowOrigin = "Access-Control-Allow-Origin"

	// AccessControlAllowMethods specifies the allowed HTTP methods when accessing the resource in CORS (Fetch).
	AccessControlAllowMethods = "Access-Control-Allow-Methods"

	// AccessControlAllowHeaders specifies the allowed HTTP headers when accessing the resource in CORS (Fetch).
	AccessControlAllowHeaders = "Access-Control-Allow-Headers"

	// AccessControlAllowCredentials indicates whether credentials (cookies, auth) can be exposed in CORS (Fetch).
	AccessControlAllowCredentials = "Access-Control-Allow-Credentials"

	// AccessControlExposeHeaders lists the response headers safe to expose to client-side scripts (Fetch).
	AccessControlExposeHeaders = "Access-Control-Expose-Headers"

	// AccessControlMaxAge indicates how long the results of a CORS preflight request can be cached (Fetch).
	AccessControlMaxAge = "Access-Control-Max-Age"

	// AccessControlRequestMethod indicates which HTTP method will be used in the actual CORS request (Fetch).
	AccessControlRequestMethod = "Access-Control-Request-Method"

	// AccessControlRequestHeaders indicates which HTTP headers will be used in the actual CORS request (Fetch).
	AccessControlRequestHeaders = "Access-Control-Request-Headers"
)

// ============================================================================
// 7. Security & Context Isolation Headers (W3C / RFC)
// ============================================================================

const (
	// ContentSecurityPolicy defines approved sources of content that browser may load (CSP Level 3).
	ContentSecurityPolicy = "Content-Security-Policy"

	// ContentSecurityPolicyReportOnly monitors and reports CSP violations without enforcement (CSP Level 3).
	ContentSecurityPolicyReportOnly = "Content-Security-Policy-Report-Only"

	// StrictTransportSecurity enforces secure (HTTPS) connections and prevents SSL stripping (HSTS - RFC 6797).
	StrictTransportSecurity = "Strict-Transport-Security"

	// XContentTypeOptions prevents browsers from MIME-sniffing a response away from declared Content-Type (RFC 9110).
	XContentTypeOptions = "X-Content-Type-Options"

	// XFrameOptions indicates whether a browser should be allowed to render a page in a frame/iframe (RFC 7034).
	XFrameOptions = "X-Frame-Options"

	// XXSSProtection configures the legacy browser Cross-Site Scripting (XSS) filter.
	XXSSProtection = "X-XSS-Protection"

	// ReferrerPolicy controls how much referrer information is included with requests (W3C).
	ReferrerPolicy = "Referrer-Policy"

	// PermissionsPolicy allows site developers to restrict browser features and APIs (W3C).
	PermissionsPolicy = "Permissions-Policy"

	// CrossOriginOpenerPolicy isolates top-level browsing contexts to prevent cross-origin attacks (COOP - W3C).
	CrossOriginOpenerPolicy = "Cross-Origin-Opener-Policy"

	// CrossOriginEmbedderPolicy prevents a document from loading cross-origin resources without permission (COEP - W3C).
	CrossOriginEmbedderPolicy = "Cross-Origin-Embedder-Policy"

	// CrossOriginResourcePolicy protects resources from being embedded by cross-origin documents (CORP - W3C).
	CrossOriginResourcePolicy = "Cross-Origin-Resource-Policy"

	// PublicKeyPins associates a specific cryptographic public key with a certain web server (HPKP - RFC 7469).
	PublicKeyPins = "Public-Key-Pins"

	// PublicKeyPinsReportOnly sends reports of HPKP pinning failures without enforcement (RFC 7469).
	PublicKeyPinsReportOnly = "Public-Key-Pins-Report-Only"

	// XPermittedCrossDomainPolicies restricts Adobe Flash / PDF cross-domain policy files.
	XPermittedCrossDomainPolicies = "X-Permitted-Cross-Domain-Policies"
)

// ============================================================================
// 8. Fetch Metadata & Client Hints (W3C)
// ============================================================================

const (
	// SecFetchDest indicates the initiator's target destination for the requested data (e.g. "document", "image") (W3C).
	SecFetchDest = "Sec-Fetch-Dest"

	// SecFetchMode indicates the mode of the request (e.g. "navigate", "cors", "no-cors") (W3C).
	SecFetchMode = "Sec-Fetch-Mode"

	// SecFetchSite indicates the relationship between request initiator's origin and target origin (W3C).
	SecFetchSite = "Sec-Fetch-Site"

	// SecFetchUser indicates whether the navigation request was triggered by explicit user gesture (W3C).
	SecFetchUser = "Sec-Fetch-User"

	// SecCHUA provides user agent's branding and major version list (Client Hints - W3C).
	SecCHUA = "Sec-CH-UA"

	// SecCHUAArch provides target device architecture (e.g. "x86", "arm") (Client Hints - W3C).
	SecCHUAArch = "Sec-CH-UA-Arch"

	// SecCHUABitness provides CPU architecture bitness (e.g. "64") (Client Hints - W3C).
	SecCHUABitness = "Sec-CH-UA-Bitness"

	// SecCHUAFullVersion provides full user agent version string (Client Hints - W3C).
	SecCHUAFullVersion = "Sec-CH-UA-Full-Version"

	// SecCHUAFullVersionList provides full brand and version list (Client Hints - W3C).
	SecCHUAFullVersionList = "Sec-CH-UA-Full-Version-List"

	// SecCHUAMobile indicates whether the browser is running on a mobile device ("?1" or "?0") (Client Hints - W3C).
	SecCHUAMobile = "Sec-CH-UA-Mobile"

	// SecCHUAModel provides the device model on which user agent is running (Client Hints - W3C).
	SecCHUAModel = "Sec-CH-UA-Model"

	// SecCHUAPlatform provides the operating system platform (e.g. "Windows", "Linux", "Android") (Client Hints - W3C).
	SecCHUAPlatform = "Sec-CH-UA-Platform"

	// SecCHUAPlatformVersion provides the operating system platform version (Client Hints - W3C).
	SecCHUAPlatformVersion = "Sec-CH-UA-Platform-Version"

	// SecCHUAFormFactors provides the form factors for the device (Client Hints - W3C).
	SecCHUAFormFactors = "Sec-CH-UA-Form-Factors"

	// SecCHWidth provides the physical pixel width of the target display (Client Hints - W3C).
	SecCHWidth = "Sec-CH-Width"

	// SecCHDPR provides the device pixel ratio (Client Hints - W3C).
	SecCHDPR = "Sec-CH-DPR"

	// SecCHViewportWidth provides the viewport width in CSS pixels (Client Hints - W3C).
	SecCHViewportWidth = "Sec-CH-Viewport-Width"

	// SecCHPrefersColorScheme indicates user's dark/light color scheme preference (Client Hints - W3C).
	SecCHPrefersColorScheme = "Sec-CH-Prefers-Color-Scheme"

	// SecCHPrefersReducedMotion indicates user's motion animation reduction preference (Client Hints - W3C).
	SecCHPrefersReducedMotion = "Sec-CH-Prefers-Reduced-Motion"

	// AcceptCH informs the client which Client Hints headers the server is requesting (W3C).
	AcceptCH = "Accept-CH"

	// CriticalCH informs the client which Client Hints are critical for page rendering (W3C).
	CriticalCH = "Critical-CH"
)

// ============================================================================
// 9. Modern Transport, Protocol Upgrades & WebSockets (RFC 6455, 7838, 9218)
// ============================================================================

const (
	// AltSvc advertises alternative services and protocol endpoints (e.g. HTTP/3 over QUIC) (RFC 7838).
	AltSvc = "Alt-Svc"

	// AltUsed indicates the alternative service host that was actually used (RFC 7838).
	AltUsed = "Alt-Used"

	// SecWebSocketKey provides a random client nonce for the WebSocket opening handshake (RFC 6455).
	SecWebSocketKey = "Sec-WebSocket-Key"

	// SecWebSocketAccept provides the server's cryptographic confirmation token in WebSocket handshake (RFC 6455).
	SecWebSocketAccept = "Sec-WebSocket-Accept"

	// SecWebSocketVersion indicates the WebSocket protocol version requested by the client (RFC 6455).
	SecWebSocketVersion = "Sec-WebSocket-Version"

	// SecWebSocketProtocol indicates the client-requested or server-selected WebSocket sub-protocol (RFC 6455).
	SecWebSocketProtocol = "Sec-WebSocket-Protocol"

	// SecWebSocketExtensions specifies WebSocket protocol level extensions (e.g. permessage-deflate) (RFC 6455).
	SecWebSocketExtensions = "Sec-WebSocket-Extensions"

	// Priority carries end-to-end stream prioritization parameters (RFC 9218).
	Priority = "Priority"

	// UpgradeInsecureRequests signals server that client supports automatic redirection to HTTPS (W3C).
	UpgradeInsecureRequests = "Upgrade-Insecure-Requests"
)

// ============================================================================
// 10. Distributed Tracing & Telemetry (W3C Trace Context, OpenTelemetry, Zipkin)
// ============================================================================

const (
	// Traceparent identifies the incoming request in a tracing system conforming to W3C Trace Context.
	Traceparent = "traceparent"

	// Tracestate carries vendor-specific state routing information conforming to W3C Trace Context.
	Tracestate = "tracestate"

	// Baggage carries cross-cutting contextual key-value properties conforming to W3C Baggage.
	Baggage = "baggage"

	// XRequestID carries a unique correlation token identifying an HTTP request across services.
	XRequestID = "X-Request-ID"

	// XCorrelationID carries a distributed transaction correlation identifier.
	XCorrelationID = "X-Correlation-ID"

	// XTraceID carries a trace identifier for distributed request tracing.
	XTraceID = "X-Trace-ID"

	// XSpanID carries a span identifier within a distributed execution trace.
	XSpanID = "X-Span-ID"

	// XB3TraceID carries the Zipkin B3 trace identifier.
	XB3TraceID = "X-B3-TraceId"

	// XB3SpanID carries the Zipkin B3 span identifier.
	XB3SpanID = "X-B3-SpanId"

	// XB3Sampled carries the Zipkin B3 sampling decision flag ("1" or "0").
	XB3Sampled = "X-B3-Sampled"

	// XB3Flags carries the Zipkin B3 debug flag.
	XB3Flags = "X-B3-Flags"

	// XB3ParentSpanID carries the Zipkin B3 parent span identifier.
	XB3ParentSpanID = "X-B3-ParentSpanId"

	// XAmznTraceID carries AWS Application Load Balancer / X-Ray trace context data.
	XAmznTraceID = "X-Amzn-Trace-Id"
)

// ============================================================================
// 11. Reverse Proxy, Load Balancer & CDN Headers
// ============================================================================

const (
	// Forwarded contains standardized proxy forwarding information (RFC 7239).
	Forwarded = "Forwarded"

	// XForwardedFor carries the de-facto client IP address through HTTP proxies and load balancers.
	XForwardedFor = "X-Forwarded-For"

	// XForwardedProto carries the de-facto original request protocol scheme ("http" or "https").
	XForwardedProto = "X-Forwarded-Proto"

	// XForwardedHost carries the de-facto original host requested by the client.
	XForwardedHost = "X-Forwarded-Host"

	// XForwardedPort carries the de-facto original destination port requested by the client.
	XForwardedPort = "X-Forwarded-Port"

	// XForwardedPrefix carries the URL prefix path stripped by a reverse proxy.
	XForwardedPrefix = "X-Forwarded-Prefix"

	// XForwardedServer carries the reverse proxy hostname.
	XForwardedServer = "X-Forwarded-Server"

	// XRealIP carries the immediate remote IP address forwarded by Nginx or reverse proxies.
	XRealIP = "X-Real-IP"

	// CFConnectingIP provides the original client IP address connecting to Cloudflare CDN.
	CFConnectingIP = "CF-Connecting-IP"

	// CFConnectingIPv6 provides the original client IPv6 address connecting to Cloudflare CDN.
	CFConnectingIPv6 = "CF-Connecting-IPv6"

	// CFRay provides the unique Cloudflare Ray ID request tracking hash.
	CFRay = "CF-Ray"

	// CFIPCountry provides the two-letter ISO-3166-1 country code of the client IP connecting to Cloudflare.
	CFIPCountry = "CF-IPCountry"

	// CFVisitor provides Cloudflare visitor scheme metadata (JSON object).
	CFVisitor = "CF-Visitor"

	// TrueClientIP provides the original client IP address on Akamai and Cloudflare Enterprise networks.
	TrueClientIP = "True-Client-IP"

	// FastlyClientIP provides the original client IP address connecting to Fastly CDN.
	FastlyClientIP = "Fastly-Client-IP"

	// XAccelBuffering controls internal response buffering in Nginx ("yes" or "no").
	XAccelBuffering = "X-Accel-Buffering"

	// XAccelRedirect initiates an internal redirection to a protected URI in Nginx.
	XAccelRedirect = "X-Accel-Redirect"

	// DNT indicates the user's tracking preference (Do Not Track, W3C).
	DNT = "DNT"

	// EarlyData indicates that the request was conveyed in TLS early data (RFC 8470).
	EarlyData = "Early-Data"

	// ExpectCT enables Certificate Transparency monitoring and enforcement (RFC 9163).
	ExpectCT = "Expect-CT"

	// KeepAlive communicates connection persistence parameters (RFC 2068 §19.7.1).
	KeepAlive = "Keep-Alive"

	// LastEventID contains the event ID of the last event received by the client (W3C EventSource).
	LastEventID = "Last-Event-ID"

	// Link specifies relationship between current document and external resources (RFC 8288).
	Link = "Link"

	// ServerTiming communicates performance metrics about the request-response cycle (W3C Server Timing).
	ServerTiming = "Server-Timing"

	// XDNSPrefetchControl controls DNS prefetching in modern browsers (Chromium / W3C).
	XDNSPrefetchControl = "X-DNS-Prefetch-Control"
)

// ============================================================================
// 12. Authentication, Authorization & DPoP (RFC 9449, RFC 9421)
// ============================================================================

const (
	// DPoP contains an RFC 9449 Demonstrating Proof-of-Possession JWT proof header.
	DPoP = "DPoP"

	// DPoPNonce contains a server-provided nonce for DPoP replay protection (RFC 9449 §4.3).
	DPoPNonce = "DPoP-Nonce"

	// Signature carries an RFC 9421 HTTP message cryptographic signature.
	Signature = "Signature"

	// SignatureInput carries metadata and component parameters for RFC 9421 HTTP message signatures.
	SignatureInput = "Signature-Input"

	// AcceptSignature advertises acceptable signature parameters and algorithms (RFC 9421).
	AcceptSignature = "Accept-Signature"

	// Digest provides an instance digest of the payload representation (RFC 3230).
	Digest = "Digest"

	// WantDigest requests instance digest calculation for the response (RFC 3230).
	WantDigest = "Want-Digest"
)

// ============================================================================
// 13. gRPC & gRPC-Web (Official gRPC Wire Protocol)
// ============================================================================

const (
	// GRPCStatus conveys the canonical gRPC status code in response trailers.
	GRPCStatus = "grpc-status"

	// GRPCMessage conveys an error description message in response trailers.
	GRPCMessage = "grpc-message"

	// GRPCTimeout specifies the client request deadline timeout duration.
	GRPCTimeout = "grpc-timeout"

	// GRPCEncoding specifies message compression coding applied to the gRPC frame stream.
	GRPCEncoding = "grpc-encoding"

	// GRPCAcceptEncoding specifies supported message compression codings for gRPC frames.
	GRPCAcceptEncoding = "grpc-accept-encoding"

	// GRPCStatusDetailsBin carries binary Base64-encoded google.rpc.Status protobuf details.
	GRPCStatusDetailsBin = "grpc-status-details-bin"

	// GRPCPreviousRPCAttempts indicates how many times the RPC call was previously attempted.
	GRPCPreviousRPCAttempts = "grpc-previous-rpc-attempts"

	// GRPCRetryPushbackMS indicates server backoff pushback duration in milliseconds.
	GRPCRetryPushbackMS = "grpc-retry-pushback-ms"

	// XGRPCWeb indicates gRPC-Web transport framing protocol.
	XGRPCWeb = "x-grpc-web"

	// ContentTransferEncoding indicates MIME encoding in text-mode gRPC-Web (e.g. "base64").
	ContentTransferEncoding = "Content-Transfer-Encoding"
)

// ============================================================================
// 14. Standard MIME Media Types (Content-Type Constants)
// ============================================================================

const (
	// MIMEApplicationJSON represents standard JSON data (RFC 8259).
	MIMEApplicationJSON = "application/json"

	// MIMEApplicationJSONCharsetUTF8 represents standard JSON data explicitly with UTF-8 charset.
	MIMEApplicationJSONCharsetUTF8 = "application/json; charset=utf-8"

	// MIMEApplicationProblemJSON represents standard RFC 7807 problem details JSON.
	MIMEApplicationProblemJSON = "application/problem+json"

	// MIMEApplicationForm represents standard URL-encoded form data.
	MIMEApplicationForm = "application/x-www-form-urlencoded"

	// MIMEMultipartFormData represents multipart form data payload (RFC 7578).
	MIMEMultipartFormData = "multipart/form-data"

	// MIMEMultipartByteRanges represents multipart partial byte ranges (RFC 9110 §14.5).
	MIMEMultipartByteRanges = "multipart/byteranges"

	// MIMEApplicationProtobuf represents binary Protocol Buffer payloads.
	MIMEApplicationProtobuf = "application/x-protobuf"

	// MIMEApplicationGRPC represents standard binary gRPC stream frames.
	MIMEApplicationGRPC = "application/grpc"

	// MIMEApplicationGRPCWeb represents binary gRPC-Web frames over HTTP/1.1 and HTTP/2.
	MIMEApplicationGRPCWeb = "application/grpc-web"

	// MIMEApplicationGRPCWebProto represents Protobuf-encoded gRPC-Web frames.
	MIMEApplicationGRPCWebProto = "application/grpc-web+proto"

	// MIMEApplicationGRPCWebText represents Base64 text-encoded gRPC-Web frames.
	MIMEApplicationGRPCWebText = "application/grpc-web-text"

	// MIMEApplicationXML represents standard XML data (RFC 7303).
	MIMEApplicationXML = "application/xml"

	// MIMEApplicationYAML represents YAML data.
	MIMEApplicationYAML = "application/yaml"

	// MIMEApplicationOctetStream represents arbitrary binary data (RFC 2046).
	MIMEApplicationOctetStream = "application/octet-stream"

	// MIMEApplicationPDF represents Portable Document Format (PDF) files.
	MIMEApplicationPDF = "application/pdf"

	// MIMEApplicationZIP represents standard ZIP archives.
	MIMEApplicationZIP = "application/zip"

	// MIMEApplicationGzip represents Gzip-compressed archive streams.
	MIMEApplicationGzip = "application/gzip"

	// MIMEApplicationZstd represents Zstandard-compressed data streams.
	MIMEApplicationZstd = "application/zstd"

	// MIMEApplicationJWT represents JSON Web Token (JWT) data (RFC 7519).
	MIMEApplicationJWT = "application/jwt"

	// MIMEApplicationCBOR represents Concise Binary Object Representation (CBOR) data (RFC 8949).
	MIMEApplicationCBOR = "application/cbor"

	// MIMETextPlain represents plain text without formatting (RFC 2046).
	MIMETextPlain = "text/plain"

	// MIMETextPlainCharsetUTF8 represents plain text explicitly encoded in UTF-8.
	MIMETextPlainCharsetUTF8 = "text/plain; charset=utf-8"

	// MIMETextHTML represents HTML document data (W3C HTML5).
	MIMETextHTML = "text/html"

	// MIMETextHTMLCharsetUTF8 represents HTML document data explicitly encoded in UTF-8.
	MIMETextHTMLCharsetUTF8 = "text/html; charset=utf-8"

	// MIMETextCSS represents Cascading Style Sheets (CSS).
	MIMETextCSS = "text/css"

	// MIMETextJavaScript represents JavaScript source code (RFC 9239).
	MIMETextJavaScript = "text/javascript"

	// MIMETextEventStream represents Server-Sent Events (SSE) stream (W3C).
	MIMETextEventStream = "text/event-stream"

	// MIMEImagePNG represents Portable Network Graphics (PNG) image.
	MIMEImagePNG = "image/png"

	// MIMEImageJPEG represents JPEG image data.
	MIMEImageJPEG = "image/jpeg"

	// MIMEImageGIF represents Graphics Interchange Format (GIF) image.
	MIMEImageGIF = "image/gif"

	// MIMEImageWebP represents WebP image data.
	MIMEImageWebP = "image/webp"

	// MIMEImageSVG represents Scalable Vector Graphics (SVG) data.
	MIMEImageSVG = "image/svg+xml"

	// MIMEImageAVIF represents AV1 Image File Format (AVIF) image.
	MIMEImageAVIF = "image/avif"

	// MIMEImageICO represents Microsoft Icon (ICO) format.
	MIMEImageICO = "image/x-icon"
)

// ============================================================================
// 15. Standard HTTP Methods (RFC 9110 §9)
// ============================================================================

const (
	// MethodGet requests transfer of a current representation of the target resource (RFC 9110 §9.3.1).
	MethodGet = "GET"

	// MethodHead is identical to GET except that the server must not return a message body (RFC 9110 §9.3.2).
	MethodHead = "HEAD"

	// MethodPost requests that target resource process representation enclosed in payload (RFC 9110 §9.3.3).
	MethodPost = "POST"

	// MethodPut requests that state of target resource be created or replaced with payload (RFC 9110 §9.3.4).
	MethodPut = "PUT"

	// MethodPatch requests that a set of changes described in payload be applied to target (RFC 5789).
	MethodPatch = "PATCH"

	// MethodDelete requests that target resource be deleted (RFC 9110 §9.3.5).
	MethodDelete = "DELETE"

	// MethodConnect establishes a tunnel to the server identified by target URI (RFC 9110 §9.3.6).
	MethodConnect = "CONNECT"

	// MethodOptions requests information about communication options for target URI (RFC 9110 §9.3.7).
	MethodOptions = "OPTIONS"

	// MethodTrace performs a message loop-back test along path to target resource (RFC 9110 §9.3.8).
	MethodTrace = "TRACE"
)

// ============================================================================
// 16. Standard Common Header Values
// ============================================================================

const (
	// ValueKeepAlive indicates that the connection should remain open for subsequent requests.
	ValueKeepAlive = "keep-alive"

	// ValueClose indicates that the connection will be closed after completion of the response.
	ValueClose = "close"

	// ValueChunked indicates chunked transfer encoding framing.
	ValueChunked = "chunked"

	// ValueNoCache instructs caches to submit request to origin server for validation before serving stored response.
	ValueNoCache = "no-cache"

	// ValueNoStore instructs caches not to store any part of the request or response.
	ValueNoStore = "no-store"

	// ValueMustRevalidate instructs caches that once response is stale, they must revalidate with origin server.
	ValueMustRevalidate = "must-revalidate"

	// ValueNoTransform instructs intermediaries not to transform or modify the payload.
	ValueNoTransform = "no-transform"

	// ValuePublic marks response as cacheable by any cache (including shared/proxy caches).
	ValuePublic = "public"

	// ValuePrivate marks response as intended for a single user (not to be stored by shared caches).
	ValuePrivate = "private"

	// ValueImmutable indicates that response body will not change over time.
	ValueImmutable = "immutable"

	// ValueMaxAge0 indicates zero cache validity duration (immediate expiration).
	ValueMaxAge0 = "max-age=0"

	// ValueGzip indicates Gzip content coding algorithm.
	ValueGzip = "gzip"

	// ValueDeflate indicates Deflate content coding algorithm.
	ValueDeflate = "deflate"

	// ValueBrotli indicates Brotli ("br") content coding algorithm.
	ValueBrotli = "br"

	// ValueZstd indicates Zstandard ("zstd") content coding algorithm.
	ValueZstd = "zstd"

	// ValueTrailers indicates that trailing headers will be sent at end of chunked payload.
	ValueTrailers = "trailers"

	// ValueIdentity indicates absence of content coding (unmodified raw data).
	ValueIdentity = "identity"

	// ValueWildcard represents the wildcard character "*".
	ValueWildcard = "*"

	// ValueNone indicates absence of feature or capability (e.g. Accept-Ranges: none).
	ValueNone = "none"

	// ValueSameOrigin represents the "same-origin" policy / context value.
	ValueSameOrigin = "same-origin"

	// ValueCrossOrigin represents the "cross-origin" policy / context value.
	ValueCrossOrigin = "cross-origin"

	// Value100Continue represents the "100-continue" Expect header value.
	Value100Continue = "100-continue"

	// ValueBearer represents the "Bearer" HTTP authorization scheme prefix.
	ValueBearer = "Bearer"

	// ValueBasic represents the "Basic" HTTP authorization scheme prefix.
	ValueBasic = "Basic"

	// ValueDPoP represents the "DPoP" HTTP authorization scheme prefix.
	ValueDPoP = "DPoP"

	// ValueWebSocket represents the "websocket" Upgrade token.
	ValueWebSocket = "websocket"

	// ValueUpgrade represents the "Upgrade" Connection token.
	ValueUpgrade = "Upgrade"

	// ValueBytes represents the "bytes" Accept-Ranges token.
	ValueBytes = "bytes"
)
