// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/mach"
	h1client "github.com/lemon4ksan/mach/client/h1"
	h2client "github.com/lemon4ksan/mach/client/h2"
	h3client "github.com/lemon4ksan/mach/client/h3"
	"github.com/lemon4ksan/mach/net/quic"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	"github.com/lemon4ksan/mach/proto/http"
)

var _ mach.Engine = (*Engine)(nil)

// EngineOption defines functional options for configuring Engine instances.
type EngineOption func(*Engine)

// Engine is the intelligent universal multi-protocol client transport engine.
// It implements mach.Engine, providing automatic ALPN protocol negotiation,
// connection reuse via PoolManager, and transparent Alt-Svc upgrade to HTTP/3.
type Engine struct {
	// TLSConfig is the base TLS configuration for TLS handshakes and ALPN negotiation.
	TLSConfig *tls.Config

	// Dialer is the network dialer used for TCP connection establishment.
	Dialer net.Dialer

	// IdleTimeout specifies the maximum idle duration before a pooled connection is evicted.
	IdleTimeout time.Duration

	// MaxConnsPerHost bounds the number of concurrent connections per destination host.
	MaxConnsPerHost int

	// PoolManager manages connection reuse and idle eviction across hosts.
	PoolManager *PoolManager[Conn]

	// AltSvcCache tracks alternative services for transparent HTTP/3 upgrades (RFC 7838).
	AltSvcCache *AltSvcCache

	// DisableAltSvc disables transparent Alt-Svc upgrade to HTTP/3.
	DisableAltSvc bool

	// H2Opts configures client HTTP/2 connection options.
	H2Opts h2client.ConnOpts

	// H3Settings configures client HTTP/3 settings.
	H3Settings *coreh3.Settings

	// H3ClientTLS configures TLS parameters for QUIC/HTTP/3 connections.
	H3ClientTLS *tls.Config

	mu       sync.RWMutex
	closed   atomic.Bool
	h2Active map[string]*h2Conn
	h3Active map[string]*h3Conn
}

// NewEngine creates and initializes a universal multi-protocol client Engine.
func NewEngine(opts ...EngineOption) *Engine {
	e := &Engine{
		IdleTimeout:     90 * time.Second,
		MaxConnsPerHost: 100,
		AltSvcCache:     NewAltSvcCache(),
		h2Active:        make(map[string]*h2Conn),
		h3Active:        make(map[string]*h3Conn),
	}

	for _, opt := range opts {
		opt(e)
	}

	e.PoolManager = NewPoolManager[Conn]()
	e.PoolManager.IdleTimeout = generic.From(e.IdleTimeout, e.IdleTimeout > 0).ValueOr(90 * time.Second)
	e.PoolManager.MaxConnsPerHost = generic.From(e.MaxConnsPerHost, e.MaxConnsPerHost > 0).ValueOr(100)
	e.PoolManager.Dial = func(ctx context.Context, addr string) (Conn, error) {
		return e.dialOrigin(ctx, addr)
	}
	e.PoolManager.IsHealthy = func(c Conn) bool {
		return c != nil && !c.IsClosed()
	}
	e.PoolManager.CloseConn = func(c Conn) error {
		if c == nil {
			return nil
		}

		return c.CloseRaw()
	}

	return e
}

// WithTLSConfig sets the base TLS configuration for Engine.
func WithTLSConfig(conf *tls.Config) EngineOption {
	return func(e *Engine) {
		e.TLSConfig = conf
	}
}

// WithDialer sets the net.Dialer for Engine.
func WithDialer(dialer net.Dialer) EngineOption {
	return func(e *Engine) {
		e.Dialer = dialer
	}
}

// WithIdleTimeout configures the connection idle eviction timeout.
func WithIdleTimeout(d time.Duration) EngineOption {
	return func(e *Engine) {
		e.IdleTimeout = d
	}
}

// WithMaxConnsPerHost configures the maximum concurrent connections per host.
func WithMaxConnsPerHost(n int) EngineOption {
	return func(e *Engine) {
		e.MaxConnsPerHost = n
	}
}

// WithDisableAltSvc disables Alt-Svc HTTP/3 upgrade.
func WithDisableAltSvc(disable bool) EngineOption {
	return func(e *Engine) {
		e.DisableAltSvc = disable
	}
}

// WithAltSvcCache sets a custom AltSvcCache.
func WithAltSvcCache(cache *AltSvcCache) EngineOption {
	return func(e *Engine) {
		e.AltSvcCache = cache
	}
}

// WithH2Opts sets HTTP/2 connection options.
func WithH2Opts(opts h2client.ConnOpts) EngineOption {
	return func(e *Engine) {
		e.H2Opts = opts
	}
}

// WithH3Settings sets HTTP/3 settings.
func WithH3Settings(settings *coreh3.Settings) EngineOption {
	return func(e *Engine) {
		e.H3Settings = settings
	}
}

// WithH3ClientTLS sets client TLS parameters for HTTP/3.
func WithH3ClientTLS(conf *tls.Config) EngineOption {
	return func(e *Engine) {
		e.H3ClientTLS = conf
	}
}

// Dial establishes an intelligent protocol connection to addr, satisfying mach.Engine.
// It prioritizes HTTP/3 if an unexpired Alt-Svc entry exists for the destination,
// falling back to ALPN negotiation ("h2", "http/1.1") and pooling via PoolManager.
func (e *Engine) Dial(ctx context.Context, addr string) (mach.Conn, error) {
	if e.closed.Load() {
		return nil, errors.New("client: engine is closed")
	}

	origin, host := parseOriginAndHost(addr, e.TLSConfig != nil)

	// 1. Transparent Alt-Svc Upgrade to HTTP/3 if cached
	if !e.DisableAltSvc && e.AltSvcCache != nil {
		if h3Opt := e.AltSvcCache.GetH3Optional(origin); h3Opt.IsPresent() {
			h3Addr := h3Opt.MustValue()
			h3c, err := e.getOrDialH3(ctx, h3Addr, host)
			if err == nil {
				return newPooledConn(e, origin, h3c), nil
			}

			e.AltSvcCache.MarkFailed(origin, 5*time.Minute)
		}
	}

	// 2. Origin ALPN negotiation & PoolManager reuse
	c, err := e.getOrDialOrigin(ctx, origin)
	if err != nil {
		return nil, err
	}

	return newPooledConn(e, origin, c), nil
}

// Do executes an HTTP request with automatic ALPN negotiation, PoolManager reuse,
// and transparent Alt-Svc upgrade to HTTP/3 (RFC 7838).
func (e *Engine) Do(ctx context.Context, req *http.Request, res *http.Response) error {
	if e.closed.Load() {
		return errors.New("client: engine is closed")
	}

	origin, host := extractRequestOrigin(req, e.TLSConfig != nil)
	if origin == "" {
		return errors.New("client: request host is empty")
	}

	// EXP-1208: Reset response buffer before dispatching to prevent duplicated/stale payloads
	res.ResetBody()
	res.Header.Reset()

	// 1. Transparent Alt-Svc Upgrade to HTTP/3
	if !e.DisableAltSvc && e.AltSvcCache != nil {
		if h3Opt := e.AltSvcCache.GetH3Optional(origin); h3Opt.IsPresent() {
			h3Addr := h3Opt.MustValue()
			h3c, err := e.getOrDialH3(ctx, h3Addr, host)
			if err == nil {
				err = h3c.Do(ctx, req, res)
				if err == nil {
					if altHdr := res.Header.Peek("Alt-Svc"); len(altHdr) > 0 {
						e.AltSvcCache.UpdateFromHeader(origin, altHdr)
					}

					return nil
				}

				// EXP-1202B: evict broken H3 connection from active table
				e.evictH3(h3Addr, h3c)
			}

			// EXP-1208: Reset response buffer again before origin fallback
			res.ResetBody()
			res.Header.Reset()

			// RFC 7838 §3.4: "A client MUST be prepared to fall back to the origin if an alternative service fails."
			e.AltSvcCache.MarkFailed(origin, 5*time.Minute)
		}
	}

	// 2. Origin ALPN Request (H2 or H1 with PoolManager reuse)
	// EXP-1209: Transparently retry once on fresh connection if pooled socket died with EOF (RFC 9112 §9.3)
	var (
		c   Conn
		err error
	)
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			res.ResetBody()
			res.Header.Reset()
		}

		c, err = e.getOrDialOrigin(ctx, origin)
		if err != nil {
			return err
		}

		err = c.Do(ctx, req, res)

		if attempt == 0 && isStaleConnErr(err) {
			e.releaseOrigin(origin, c, res, err)
			continue
		}

		break
	}

	// 3. Inspect Alt-Svc header for future transparent upgrades
	if altHdr := res.Header.Peek("Alt-Svc"); len(altHdr) > 0 {
		if !e.DisableAltSvc && e.AltSvcCache != nil {
			e.AltSvcCache.UpdateFromHeader(origin, altHdr)
		}
	}

	// 4. Return connection to pool or handle cleanup (EXP-1202: no count leaks)
	e.releaseOrigin(origin, c, res, err)

	return err
}

// DoResult executes an HTTP request returning a Result wrapping the response or error.
func (e *Engine) DoResult(ctx context.Context, req *http.Request, res *http.Response) generic.Result[*http.Response] {
	err := e.Do(ctx, req, res)
	return generic.ToResult(res, err)
}

func (e *Engine) getOrDialOrigin(ctx context.Context, origin string) (Conn, error) {
	e.mu.RLock()
	h2c := e.h2Active[origin]
	e.mu.RUnlock()

	if h2c != nil && !h2c.IsClosed() && h2c.CanOpenStream() {
		return h2c, nil
	}

	c, err := e.PoolManager.Get(ctx, origin)
	if err != nil {
		return nil, err
	}

	if c.Protocol() == "h2" {
		if h2Wrapped, ok := c.(*h2Conn); ok {
			e.mu.Lock()
			e.h2Active[origin] = h2Wrapped
			e.mu.Unlock()
		}
	}

	return c, nil
}

func (e *Engine) releaseOrigin(origin string, c Conn, res *http.Response, doErr error) {
	if c.Protocol() == "http/1.1" {
		if doErr == nil && !c.IsClosed() && (res == nil || !res.Header.ConnectionClose()) {
			e.PoolManager.Put(origin, c)
		} else {
			e.PoolManager.Discard(origin, c)
		}

		return
	}

	// EXP-1202 & EXP-1209: When H2 connection terminates or encounters error, evict and discard
	if c.IsClosed() || doErr != nil {
		e.mu.Lock()
		delete(e.h2Active, origin)
		e.mu.Unlock()

		e.PoolManager.Discard(origin, c)

		if doErr != nil {
			_ = c.CloseRaw()
		}
	}
}

func (e *Engine) evictH2(origin string, c *h2client.Conn) {
	e.mu.Lock()
	if active, ok := e.h2Active[origin]; ok && active.c == c {
		delete(e.h2Active, origin)
	}

	e.mu.Unlock()

	// Decrement count in PoolManager to prevent host bricking (EXP-1202)
	e.PoolManager.Discard(origin, nil)
}

func (e *Engine) evictH3(h3Addr string, c *h3Conn) {
	e.mu.Lock()
	if active, ok := e.h3Active[h3Addr]; ok && active == c {
		delete(e.h3Active, h3Addr)
	}

	e.mu.Unlock()

	if c != nil {
		_ = c.CloseRaw()
	}
}

func (e *Engine) dialOrigin(ctx context.Context, origin string) (Conn, error) {
	isTLS := strings.HasPrefix(origin, "https://")
	cleanAddr := strings.TrimPrefix(origin, "https://")
	cleanAddr = strings.TrimPrefix(cleanAddr, "http://")

	host, _, err := net.SplitHostPort(cleanAddr)
	if err != nil {
		host = cleanAddr
	}

	rawConn, err := e.Dialer.DialContext(ctx, "tcp", cleanAddr)
	if err != nil {
		return nil, err
	}

	if !isTLS {
		h1c := h1client.NewClientConn(rawConn)

		return newH1Conn(h1c, origin), nil
	}

	tlsConf := e.TLSConfig
	if tlsConf == nil {
		tlsConf = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	} else {
		tlsConf = tlsConf.Clone()
	}

	if tlsConf.ServerName == "" {
		tlsConf.ServerName = host
	}

	// Automatic ALPN negotiation: advertise both h2 and http/1.1
	tlsConf.NextProtos = []string{"h2", "http/1.1"}

	tlsConn := tls.Client(rawConn, tlsConf)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()

		return nil, err
	}

	negotiated := tlsConn.ConnectionState().NegotiatedProtocol
	if negotiated == "h2" {
		opts := e.H2Opts
		userDisconnect := opts.OnDisconnect
		opts.OnDisconnect = func(dCtx context.Context, c *h2client.Conn) {
			e.evictH2(origin, c)

			if userDisconnect != nil {
				userDisconnect(dCtx, c)
			}
		}

		h2c := h2client.NewConn(tlsConn, opts)
		if err := h2c.Handshake(); err != nil {
			_ = tlsConn.Close()

			return nil, err
		}

		return newH2Conn(h2c, origin), nil
	}

	h1c := h1client.NewClientConn(tlsConn)

	return newH1Conn(h1c, origin), nil
}

func (e *Engine) getOrDialH3(ctx context.Context, h3Addr, originHost string) (*h3Conn, error) {
	e.mu.RLock()
	h3c := e.h3Active[h3Addr]
	e.mu.RUnlock()

	if h3c != nil && !h3c.IsClosed() {
		return h3c, nil
	}

	h3c, err := e.dialH3(ctx, h3Addr, originHost)
	if err != nil {
		return nil, err
	}

	e.mu.Lock()
	e.h3Active[h3Addr] = h3c
	e.mu.Unlock()

	return h3c, nil
}

func (e *Engine) dialH3(ctx context.Context, h3Addr, originHost string) (*h3Conn, error) {
	probeTimeout := 1 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		remaining := time.Until(dl)
		if remaining < probeTimeout {
			probeTimeout = remaining / 2
			if probeTimeout <= 0 {
				probeTimeout = 50 * time.Millisecond
			}
		}
	}

	h3Ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	clientTLS := e.H3ClientTLS
	switch {
	case clientTLS == nil && e.TLSConfig != nil:
		clientTLS = e.TLSConfig.Clone()
	case clientTLS == nil:
		clientTLS = &tls.Config{
			MinVersion: tls.VersionTLS13,
		}
	default:
		clientTLS = clientTLS.Clone()
	}

	clientTLS.NextProtos = []string{"h3"}
	if clientTLS.ServerName == "" {
		host, _, err := net.SplitHostPort(h3Addr)
		if err != nil {
			host = originHost
		}

		clientTLS.ServerName = host
	}

	quicConn, err := quic.DialAddr(h3Ctx, h3Addr, clientTLS, quic.WithDatagrams(true))
	if err != nil {
		return nil, err
	}

	cc, err := h3client.NewClientConn(quicConn, e.H3Settings)
	if err != nil {
		_ = quicConn.CloseWithError(0, "")

		return nil, err
	}

	return newH3Conn(cc, h3Addr), nil
}

// Close gracefully terminates all idle connections, active multiplexers, and clears the Alt-Svc cache.
func (e *Engine) Close() error {
	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}

	if e.PoolManager != nil {
		_ = e.PoolManager.Close()
	}

	// Snapshot active multiplexers before invoking CloseRaw to avoid reentrant deadlock
	// on e.mu when OnDisconnect triggers evictH2 (EXP-1206).
	e.mu.Lock()

	h2Conns := generic.Values(e.h2Active)
	clear(e.h2Active)

	h3Conns := generic.Values(e.h3Active)
	clear(e.h3Active)
	e.mu.Unlock()

	for _, h2c := range h2Conns {
		_ = h2c.CloseRaw()
	}

	for _, h3c := range h3Conns {
		_ = h3c.CloseRaw()
	}

	if e.AltSvcCache != nil {
		e.AltSvcCache.ClearAll()
	}

	return nil
}

func isStaleConnErr(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return true
	}

	msg := err.Error()

	return strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "closed") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset")
}

func parseOriginAndHost(target string, isTLS bool) (origin, host string) {
	raw := target
	hasHTTPS := strings.HasPrefix(raw, "https://")
	hasHTTP := strings.HasPrefix(raw, "http://")

	if hasHTTPS {
		raw = strings.TrimPrefix(raw, "https://")
		isTLS = true
	} else if hasHTTP {
		raw = strings.TrimPrefix(raw, "http://")
		isTLS = false
	}

	h, p, err := net.SplitHostPort(raw)
	if err != nil {
		h = raw

		if isTLS {
			p = "443"
		} else {
			p = "80"
		}
	} else if p == "443" {
		isTLS = true
	}

	host = h

	cleanAddr := net.JoinHostPort(h, p)
	if isTLS {
		origin = "https://" + cleanAddr
	} else {
		origin = "http://" + cleanAddr
	}

	return origin, host
}

func extractRequestOrigin(req *http.Request, isTLS bool) (origin, host string) {
	var target string

	rawURI := string(req.Header.RequestURI())

	switch {
	case strings.HasPrefix(rawURI, "http://"):
		rest := strings.TrimPrefix(rawURI, "http://")

		idx := strings.IndexByte(rest, '/')
		if idx != -1 {
			target = rest[:idx]
			req.SetRequestURI(rest[idx:])
		} else {
			target = rest

			req.SetRequestURI("/")
		}

		req.SetHost(target)
		req.Header.SetHost(target)
		req.UseHostHeader = true
		req.URI().SetScheme("http")

		isTLS = false

	case strings.HasPrefix(rawURI, "https://"):
		rest := strings.TrimPrefix(rawURI, "https://")

		idx := strings.IndexByte(rest, '/')
		if idx != -1 {
			target = rest[:idx]
			req.SetRequestURI(rest[idx:])
		} else {
			target = rest

			req.SetRequestURI("/")
		}

		req.SetHost(target)
		req.Header.SetHost(target)
		req.UseHostHeader = true
		req.URI().SetScheme("https")

		isTLS = true

	default:
		if h := req.Header.Host(); len(h) > 0 {
			target = string(h)
		} else if uh := req.URI().Host(); len(uh) > 0 {
			target = string(uh)
		}

		if req.URI().Scheme() != nil && len(req.URI().Scheme()) > 0 {
			scheme := string(req.URI().Scheme())
			if strings.EqualFold(scheme, "https") {
				isTLS = true
			} else if strings.EqualFold(scheme, "http") {
				isTLS = false
			}
		}
	}

	if target == "" {
		return "", ""
	}

	return parseOriginAndHost(target, isTLS)
}
