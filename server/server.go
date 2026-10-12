// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/silicon/pool"

	"github.com/lemon4ksan/mach"
	"github.com/lemon4ksan/mach/net/quic"
	"github.com/lemon4ksan/mach/proto/headkit"
	"github.com/lemon4ksan/mach/proto/semantic/altsvc"
	"github.com/lemon4ksan/mach/proto/semantic/status"
	"github.com/lemon4ksan/mach/server/h1"
	"github.com/lemon4ksan/mach/server/h2"
	"github.com/lemon4ksan/mach/server/h3"
)

var (
	// ErrServerClosed is returned by Serve calls when the server is closed.
	ErrServerClosed = errors.New("server: server closed")

	defaultContextPool = pool.NewPerCorePool(func() *Context {
		return &Context{
			userValues: make(map[any]any, 4),
		}
	})
)

// Handler processes incoming requests across HTTP/1.1, HTTP/2, and HTTP/3.
type Handler interface {
	Handle(ctx *Context) error
}

// HandlerFunc is an adapter to allow the use of ordinary functions as universal Handlers.
type HandlerFunc func(ctx *Context) error

// Handle calls f(ctx).
func (f HandlerFunc) Handle(ctx *Context) error {
	return f(ctx)
}

// Serve calls f(ctx) for compatibility with other Mach abstractions.
func (f HandlerFunc) Serve(ctx *Context) error {
	return f(ctx)
}

// ServeHTTP calls f(ctx) for compatibility with standard Go naming.
func (f HandlerFunc) ServeHTTP(ctx *Context) error {
	return f(ctx)
}

// Server is the unified multi-protocol L7 server that binds HTTP/1.1, HTTP/2, and HTTP/3.
type Server struct {
	// Handler is the universal request dispatcher.
	Handler Handler

	// HandlerFunc is an optional convenience handler callback.
	HandlerFunc HandlerFunc

	// StreamHandler is an optional mach.Handler callback for raw framing stream access.
	StreamHandler mach.Handler

	// TLSConfig specifies the TLS configuration for HTTPS listeners.
	// NextProtos will be automatically configured with ALPN protocols ("h2", "http/1.1") if omitted.
	TLSConfig *tls.Config

	// AltSvcPort specifies the UDP port advertised in Alt-Svc headers for HTTP/3 over QUIC.
	// If > 0 or if QUIC listener is active, Alt-Svc headers will be emitted automatically.
	AltSvcPort int

	// AltSvcMaxAge specifies the validity duration of the Alt-Svc advertisement.
	// Defaults to 24 hours (86400s) if 0.
	AltSvcMaxAge time.Duration

	// DisableAltSvc disables automatic emission of Alt-Svc headers.
	DisableAltSvc bool

	// CustomAltSvc sets an explicit raw Alt-Svc header value to emit.
	CustomAltSvc string

	// ReadTimeout specifies the maximum duration for reading request headers/body.
	ReadTimeout time.Duration

	// WriteTimeout specifies the maximum duration for writing response headers/body.
	WriteTimeout time.Duration

	// IdleTimeout specifies the maximum duration to wait for keep-alive requests.
	IdleTimeout time.Duration

	// MaxBodySize sets the maximum permitted request body size in bytes.
	MaxBodySize int64

	mu           sync.Mutex
	isClosed     atomic.Bool
	activeConns  map[net.Conn]struct{}
	activeTCP    []net.Listener
	activeQUIC   []*quic.Listener
	wg           sync.WaitGroup
	ctxPool      *pool.PerCorePool[*Context]
	altSvcHeader atomic.Pointer[string]
	preparedTLS  *generic.Lazy[*tls.Config]
}

// New creates a new unified Server configured with the provided options.
func New(opts ...Option) *Server {
	s := &Server{
		activeConns: make(map[net.Conn]struct{}),
		ctxPool:     defaultContextPool,
	}
	for _, opt := range opts {
		opt(s)
	}

	s.preparedTLS = generic.NewLazyValue(func() *tls.Config {
		return s.ensureTLSConfig(s.TLSConfig)
	})

	if s.CustomAltSvc != "" {
		s.altSvcHeader.Store(&s.CustomAltSvc)
	} else if s.AltSvcPort > 0 {
		s.updateAltSvcHeader(s.AltSvcPort)
	}

	return s
}

// NewServer creates a new unified Server with handler and optional configurations.
func NewServer(h Handler, opts ...Option) *Server {
	s := New(opts...)
	s.Handler = h
	return s
}

// NewWithHandler creates a new unified Server with handler and optional configurations.
func NewWithHandler(h Handler, opts ...Option) *Server {
	s := New(opts...)
	s.Handler = h
	return s
}

// NewWithHandlerFunc creates a new unified Server with handler func and optional configurations.
func NewWithHandlerFunc(fn HandlerFunc, opts ...Option) *Server {
	s := New(opts...)
	s.HandlerFunc = fn
	return s
}

func (s *Server) acquireContext() *Context {
	poolInstance := s.ctxPool
	if poolInstance == nil {
		poolInstance = defaultContextPool
	}

	return poolInstance.Get()
}

func (s *Server) releaseContext(ctx *Context) {
	ctx.Reset()

	poolInstance := s.ctxPool
	if poolInstance == nil {
		poolInstance = defaultContextPool
	}

	poolInstance.Put(ctx)
}

func (s *Server) dispatch(ctx *Context) error {
	if s.Handler != nil {
		return s.Handler.Handle(ctx)
	}

	if s.HandlerFunc != nil {
		return s.HandlerFunc(ctx)
	}

	ctx.SetStatusCode(status.NotImplemented)
	ctx.SetBodyString("No handler configured")

	return nil
}

// SetAltSvcPort updates the advertised HTTP/3 UDP port and regenerates the Alt-Svc header.
func (s *Server) SetAltSvcPort(port int) {
	s.AltSvcPort = port
	s.updateAltSvcHeader(port)
}

func (s *Server) updateAltSvcHeader(port int) {
	if s.DisableAltSvc || port <= 0 {
		return
	}

	if s.CustomAltSvc != "" {
		s.altSvcHeader.Store(&s.CustomAltSvc)
		return
	}

	maxAge := s.AltSvcMaxAge
	if maxAge <= 0 {
		maxAge = altsvc.DefaultMaxAge
	}

	svc := altsvc.Service{
		ProtocolID: "h3",
		Port:       port,
		MaxAge:     maxAge,
	}
	hdr := altsvc.Format(svc)
	s.altSvcHeader.Store(&hdr)
}

func (s *Server) getAltSvcHeader() string {
	ptr := s.altSvcHeader.Load()
	if ptr != nil {
		return *ptr
	}

	return ""
}

func (s *Server) shouldEmitAltSvc(ctx *Context) bool {
	if s.DisableAltSvc || ctx.IsAltSvcDisabled() {
		return false
	}

	return s.getAltSvcHeader() != ""
}

// AltSvcHeader returns the currently configured Alt-Svc header string representation.
func (s *Server) AltSvcHeader() string {
	return s.getAltSvcHeader()
}

func (s *Server) ensureTLSConfig(cfg *tls.Config) *tls.Config {
	var cloned *tls.Config
	if cfg != nil {
		cloned = cfg.Clone()
	} else {
		cloned = &tls.Config{}
	}

	if len(cloned.NextProtos) == 0 {
		cloned.NextProtos = []string{"h2", "http/1.1"}
	}

	return cloned
}

func (s *Server) getPreparedTLS() *tls.Config {
	if s.preparedTLS == nil {
		s.mu.Lock()
		if s.preparedTLS == nil {
			s.preparedTLS = generic.NewLazyValue(func() *tls.Config {
				return s.ensureTLSConfig(s.TLSConfig)
			})
		}

		s.mu.Unlock()
	}

	return s.preparedTLS.Value()
}

func (s *Server) trackConn(c net.Conn, add bool) {
	s.mu.Lock()
	if s.activeConns == nil {
		s.activeConns = make(map[net.Conn]struct{})
	}

	if add {
		if !s.isClosed.Load() {
			s.activeConns[c] = struct{}{}
		}
	} else {
		delete(s.activeConns, c)
	}

	s.mu.Unlock()
}

// Serve accepts incoming connections on ln, performing transparent TLS ALPN negotiation
// across HTTP/2 and HTTP/1.1 on a single TCP listener, or cleartext framing detection.
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	if s.isClosed.Load() {
		s.mu.Unlock()
		return ErrServerClosed
	}

	if s.activeConns == nil {
		s.activeConns = make(map[net.Conn]struct{})
	}

	if s.ctxPool == nil {
		s.ctxPool = defaultContextPool
	}

	if s.AltSvcPort > 0 && s.altSvcHeader.Load() == nil {
		s.updateAltSvcHeader(s.AltSvcPort)
	}

	s.activeTCP = append(s.activeTCP, ln)
	s.mu.Unlock()

	defer func() {
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.isClosed.Load() {
				return nil
			}

			return err
		}

		s.trackConn(conn, true)

		s.wg.Add(1)
		go func(c net.Conn) {
			defer func() {
				s.trackConn(c, false)
				s.wg.Done()
			}()

			s.serveTCPConn(c)
		}(conn)
	}
}

func (s *Server) serveTCPConn(conn net.Conn) {
	// 1. Check if conn is already a *tls.Conn (e.g. from tls.NewListener)
	if tlsConn, ok := conn.(*tls.Conn); ok {
		if s.ReadTimeout > 0 {
			_ = conn.SetDeadline(time.Now().Add(s.ReadTimeout))
		}

		if err := tlsConn.HandshakeContext(context.Background()); err != nil {
			_ = conn.Close()
			return
		}

		_ = conn.SetDeadline(time.Time{})

		proto := tlsConn.ConnectionState().NegotiatedProtocol
		switch proto {
		case "h2":
			s.serveH2(tlsConn)
		case "http/1.1", "":
			s.serveH1(tlsConn)
		default:
			s.serveH1(tlsConn)
		}

		return
	}

	// 2. If Server.TLSConfig is configured, wrap cleartext socket with tls.Server
	if s.TLSConfig != nil {
		cfg := s.getPreparedTLS()

		tlsConn := tls.Server(conn, cfg)
		if s.ReadTimeout > 0 {
			_ = conn.SetDeadline(time.Now().Add(s.ReadTimeout))
		}

		if err := tlsConn.HandshakeContext(context.Background()); err != nil {
			_ = conn.Close()
			return
		}

		_ = conn.SetDeadline(time.Time{})

		proto := tlsConn.ConnectionState().NegotiatedProtocol
		switch proto {
		case "h2":
			s.serveH2(tlsConn)
		case "http/1.1", "":
			s.serveH1(tlsConn)
		default:
			s.serveH1(tlsConn)
		}

		return
	}

	// 3. Cleartext TCP: peek for HTTP/2 client preface (H2C) or route to HTTP/1.1
	s.serveCleartextConn(conn)
}

const h2Preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

var h2PrefaceBytes = []byte(h2Preface)

func (s *Server) serveCleartextConn(conn net.Conn) {
	bConn := &bufferConn{Conn: conn}

	if s.ReadTimeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(s.ReadTimeout))
	}

	n, err := conn.Read(bConn.buf[:])
	if s.ReadTimeout > 0 {
		_ = conn.SetReadDeadline(time.Time{})
	}

	if err != nil {
		if n > 0 {
			bConn.n = n
			s.serveH1(bConn)
			return
		}

		_ = conn.Close()

		return
	}

	bConn.n = n

	// EXP-1201 & EXP-1210 fix: Check if request starts with HTTP/2 preface without blocking on short HTTP/1 requests.
	// As long as the buffered prefix matches the start of h2Preface, continue reading until 24 bytes.
	// If the buffered bytes ever diverge from h2Preface (e.g. 'P' followed by 'O' in "POST", or 'G' in "GET"),
	// break immediately and dispatch to HTTP/1.
	for bConn.n < len(h2PrefaceBytes) && bytes.HasPrefix(h2PrefaceBytes, bConn.buf[:bConn.n]) {
		if s.ReadTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(s.ReadTimeout))
		}

		nr, rErr := conn.Read(bConn.buf[bConn.n:len(bConn.buf)])
		if s.ReadTimeout > 0 {
			_ = conn.SetReadDeadline(time.Time{})
		}

		if nr > 0 {
			bConn.n += nr
		}

		if rErr != nil {
			break
		}
	}

	isH2 := bConn.n >= len(h2PrefaceBytes) && bytes.Equal(bConn.buf[:len(h2PrefaceBytes)], h2PrefaceBytes)
	if isH2 {
		s.serveH2(bConn)
	} else {
		s.serveH1(bConn)
	}
}

func (s *Server) serveH1(conn net.Conn) {
	ch := &h1.ConnHandler{
		ReadTimeout:   s.ReadTimeout,
		WriteTimeout:  s.WriteTimeout,
		IdleTimeout:   s.IdleTimeout,
		MaxBodySize:   s.MaxBodySize,
		StreamHandler: s.StreamHandler,
		Handler: func(req *h1.Request, res *h1.Response) error {
			ctx := s.acquireContext()

			ctx.initH1(s, req, res)
			defer func() {
				s.releaseContext(ctx)
			}()

			err := s.dispatch(ctx)
			if err != nil && res.StatusCode < 400 {
				res.StatusCode = status.InternalServerError
				res.Body = []byte(`{"error":"INTERNAL_ERROR","message":"Internal Server Error"}`)
			}

			// Automated Alt-Svc Emission
			if s.shouldEmitAltSvc(ctx) {
				hdr := s.getAltSvcHeader()
				if hdr != "" && !res.Headers.Has(headkit.AltSvc) {
					res.Headers.Set(headkit.AltSvc, hdr)
				}
			}

			return nil
		},
	}

	_ = ch.ServeConn(conn)
}

func (s *Server) serveH2(conn net.Conn) {
	h2Handler := func(req *h2.ServerRequest, res *h2.ServerResponse) error {
		ctx := s.acquireContext()

		ctx.initH2(s, req, res)
		defer func() {
			s.releaseContext(ctx)
		}()

		err := s.dispatch(ctx)
		if err != nil && res.StatusCode < 400 {
			res.StatusCode = status.InternalServerError
			res.Body = []byte(`{"error":"INTERNAL_ERROR","message":"Internal Server Error"}`)
		}

		// Automated Alt-Svc Emission
		if s.shouldEmitAltSvc(ctx) {
			hdr := s.getAltSvcHeader()
			if hdr != "" && res.Headers.Get("Alt-Svc") == "" && res.Headers.Get("alt-svc") == "" {
				res.Headers.Set("Alt-Svc", hdr)
			}
		}

		return nil
	}

	sc := h2.NewServerConn(conn, h2Handler)
	defer sc.Release()

	_ = sc.Serve()
}

// ServeQUIC accepts incoming QUIC sessions on qln and serves HTTP/3 request streams.
func (s *Server) ServeQUIC(qln *quic.Listener) error {
	s.mu.Lock()
	if s.isClosed.Load() {
		s.mu.Unlock()
		return ErrServerClosed
	}

	if s.ctxPool == nil {
		s.ctxPool = defaultContextPool
	}

	s.activeQUIC = append(s.activeQUIC, qln)

	// Automatically extract UDP port if AltSvcPort is not explicitly set
	if s.AltSvcPort == 0 && !s.DisableAltSvc {
		if udpAddr, ok := qln.Addr().(*net.UDPAddr); ok && udpAddr.Port > 0 {
			s.updateAltSvcHeader(udpAddr.Port)
		}
	}

	s.mu.Unlock()

	defer func() {
		_ = qln.Close()
	}()

	for {
		if s.isClosed.Load() {
			return nil
		}

		conn, err := qln.Accept(context.Background())
		if err != nil {
			if s.isClosed.Load() {
				return nil
			}

			return err
		}

		s.wg.Add(1)
		go func(qc *quic.Conn) {
			defer s.wg.Done()

			s.serveH3(qc)
		}(conn)
	}
}

func (s *Server) serveH3(qc *quic.Conn) {
	defer func() { _ = qc.CloseWithError(0, "server shutdown") }()

	h3Handler := func(req *h3.ServerRequest, res *h3.ServerResponse) error {
		ctx := s.acquireContext()

		ctx.initH3(s, req, res)
		defer func() {
			s.releaseContext(ctx)
		}()

		err := s.dispatch(ctx)
		if err != nil && res.StatusCode < 400 {
			res.StatusCode = status.InternalServerError
			res.Body = []byte(`{"error":"INTERNAL_ERROR","message":"Internal Server Error"}`)
		}

		return nil
	}

	sc := h3.NewServerConn(qc, h3Handler)
	_ = sc.Serve()
}

// ServeMultiProtocol concurrently serves both a TCP listener (HTTP/1.1 + HTTP/2) and a QUIC listener (HTTP/3).
func (s *Server) ServeMultiProtocol(tcpLn net.Listener, quicLn *quic.Listener) error {
	var wg sync.WaitGroup

	errChan := make(chan error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()

		if err := s.Serve(tcpLn); err != nil && !s.isClosed.Load() {
			errChan <- err
		}
	}()

	go func() {
		defer wg.Done()

		if err := s.ServeQUIC(quicLn); err != nil && !s.isClosed.Load() {
			errChan <- err
		}
	}()

	doneChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	select {
	case err := <-errChan:
		_ = s.Close()
		return err
	case <-doneChan:
		return nil
	}
}

// ServeTLS serves HTTPS on ln using certFile and keyFile with transparent ALPN negotiation.
func (s *Server) ServeTLS(ln net.Listener, certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	cfg := s.ensureTLSConfig(s.TLSConfig)
	cfg.Certificates = []tls.Certificate{cert}
	tlsLn := tls.NewListener(ln, cfg)

	return s.Serve(tlsLn)
}

// Listen listens on TCP address addr and serves incoming HTTP requests.
func (s *Server) Listen(addr string) error {
	var lc net.ListenConfig

	ln, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return err
	}

	return s.Serve(ln)
}

// ListenTLS listens on TCP address addr with TLS and serves incoming HTTPS requests.
func (s *Server) ListenTLS(addr, certFile, keyFile string) error {
	var lc net.ListenConfig

	ln, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return err
	}

	return s.ServeTLS(ln, certFile, keyFile)
}

// ListenQUIC listens for QUIC sessions on UDP address addr with tlsConf and serves HTTP/3.
func (s *Server) ListenQUIC(addr string, tlsConf *tls.Config) error {
	cfg := tlsConf.Clone()
	cfg.NextProtos = []string{"h3"}

	qln, err := quic.ListenAddr(addr, cfg, quic.WithDatagrams(true))
	if err != nil {
		return err
	}

	return s.ServeQUIC(qln)
}

// ListenAndServe listens on TCP address addr and serves HTTP requests.
func (s *Server) ListenAndServe(addr string) error {
	return s.Listen(addr)
}

// ListenAndServeTLS listens on TCP address addr with TLS and serves HTTPS requests.
func (s *Server) ListenAndServeTLS(addr, certFile, keyFile string) error {
	return s.ListenTLS(addr, certFile, keyFile)
}

// ListenAndServeMultiProtocol starts unified multi-protocol listeners on tcpAddr (H1+H2) and udpAddr (H3).
func (s *Server) ListenAndServeMultiProtocol(tcpAddr, udpAddr string, tlsConf *tls.Config) error {
	if tlsConf == nil {
		return errors.New("server: TLS configuration required for multi-protocol serving")
	}

	tcpTLS := s.ensureTLSConfig(tlsConf)

	tcpLn, err := tls.Listen("tcp", tcpAddr, tcpTLS)
	if err != nil {
		return fmt.Errorf("server: tcp listen failed: %w", err)
	}

	quicTLS := tlsConf.Clone()
	quicTLS.NextProtos = []string{"h3"}

	quicLn, err := quic.ListenAddr(udpAddr, quicTLS, quic.WithDatagrams(true))
	if err != nil {
		_ = tcpLn.Close()
		return fmt.Errorf("server: quic listen failed: %w", err)
	}

	return s.ServeMultiProtocol(tcpLn, quicLn)
}

// Close terminates all active listeners and closes all open connections immediately.
func (s *Server) Close() error {
	if s.isClosed.Swap(true) {
		return nil
	}

	s.mu.Lock()
	for _, ln := range s.activeTCP {
		_ = ln.Close()
	}

	s.activeTCP = nil

	for _, qln := range s.activeQUIC {
		_ = qln.Close()
	}

	s.activeQUIC = nil

	for conn := range s.activeConns {
		_ = conn.Close()
	}

	clear(s.activeConns)
	s.mu.Unlock()

	return nil
}

// Shutdown gracefully terminates the server, stopping new connections and waiting for in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	s.isClosed.Store(true)

	s.mu.Lock()
	for _, ln := range s.activeTCP {
		_ = ln.Close()
	}

	s.activeTCP = nil

	for _, qln := range s.activeQUIC {
		_ = qln.Close()
	}

	s.activeQUIC = nil
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		for conn := range s.activeConns {
			_ = conn.Close()
		}

		clear(s.activeConns)
		s.mu.Unlock()

		return ctx.Err()
	}
}

type bufferConn struct {
	net.Conn
	buf [1024]byte
	n   int
	off int
}

func (c *bufferConn) Read(b []byte) (int, error) {
	if c.off < c.n {
		n := copy(b, c.buf[c.off:c.n])
		c.off += n
		return n, nil
	}

	return c.Conn.Read(b)
}
