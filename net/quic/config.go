// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quic

import (
	"time"
)

// Config contains configuration options for QUIC endpoints.
type Config struct {
	GetConfigForClient               func(info *ClientInfo) (*Config, error)
	Versions                         []Version
	HandshakeIdleTimeout             time.Duration
	MaxIdleTimeout                   time.Duration
	KeepAlivePeriod                  time.Duration
	InitialStreamReceiveWindow       uint64
	MaxStreamReceiveWindow           uint64
	InitialConnectionReceiveWindow   uint64
	MaxConnectionReceiveWindow       uint64
	MaxIncomingStreams               int64
	MaxIncomingUniStreams            int64
	EnableDatagrams                  bool
	InitialPacketSize                uint16
	DisablePathMTUDiscovery          bool
	Allow0RTT                        bool
	EnableStreamResetPartialDelivery bool
}

// Clone returns a shallow copy of c.
func (c *Config) Clone() *Config {
	if c == nil {
		return &Config{}
	}

	cpy := *c

	return &cpy
}

// Option configures a Config instance.
type Option func(*Config)

// WithDatagrams enables or disables unreliable datagram frames (RFC 9221).
func WithDatagrams(enable bool) Option {
	return func(c *Config) { c.EnableDatagrams = enable }
}

// WithKeepAlivePeriod sets the interval between keep-alive probes.
func WithKeepAlivePeriod(p time.Duration) Option {
	return func(c *Config) { c.KeepAlivePeriod = p }
}
