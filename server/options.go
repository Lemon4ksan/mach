// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"crypto/tls"
	"time"

	"github.com/lemon4ksan/foundation/generic"
)

// Option configures Server instances.
type Option func(*Server)

// WithHandler sets the universal request handler.
func WithHandler(h Handler) Option {
	return func(s *Server) {
		s.Handler = h
	}
}

// WithHandlerFunc sets the universal request handler function.
func WithHandlerFunc(fn HandlerFunc) Option {
	return func(s *Server) {
		s.HandlerFunc = fn
	}
}

// WithTLSConfig sets the TLS configuration for secure listeners.
func WithTLSConfig(cfg *tls.Config) Option {
	return func(s *Server) {
		s.TLSConfig = cfg
	}
}

// WithAltSvcPort sets the UDP port advertised in Alt-Svc headers for HTTP/3 over QUIC.
func WithAltSvcPort(port int) Option {
	return func(s *Server) {
		s.SetAltSvcPort(port)
	}
}

// WithAltSvcMaxAge sets the freshness lifetime (ma) for Alt-Svc headers.
func WithAltSvcMaxAge(dur time.Duration) Option {
	return func(s *Server) {
		s.AltSvcMaxAge = dur
		if s.AltSvcPort > 0 {
			s.SetAltSvcPort(s.AltSvcPort)
		}
	}
}

// WithAltSvcDisabled enables or disables automatic Alt-Svc emission.
func WithAltSvcDisabled(disabled bool) Option {
	return func(s *Server) {
		s.DisableAltSvc = disabled
	}
}

// WithCustomAltSvc sets an explicit raw Alt-Svc header value to emit.
func WithCustomAltSvc(val string) Option {
	return func(s *Server) {
		s.CustomAltSvc = val
		s.altSvcHeader.Store(generic.Ptr(val))
	}
}

// WithReadTimeout sets the maximum duration for reading the entire request.
func WithReadTimeout(dur time.Duration) Option {
	return func(s *Server) {
		s.ReadTimeout = dur
	}
}

// WithWriteTimeout sets the maximum duration for writing the complete response.
func WithWriteTimeout(dur time.Duration) Option {
	return func(s *Server) {
		s.WriteTimeout = dur
	}
}

// WithIdleTimeout sets the maximum duration to wait for the next request on keep-alive connections.
func WithIdleTimeout(dur time.Duration) Option {
	return func(s *Server) {
		s.IdleTimeout = dur
	}
}

// WithMaxBodySize sets the maximum permitted request body size in bytes.
func WithMaxBodySize(size int64) Option {
	return func(s *Server) {
		s.MaxBodySize = size
	}
}
