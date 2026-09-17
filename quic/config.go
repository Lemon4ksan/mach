// Copyright (c) 2016 the quic-go authors. All rights reserved.
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quic

import (
	"fmt"
	"time"

	"github.com/lemon4ksan/foundation/encoding/varint"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
)

// Clone clones a conf.
func (c *Config) Clone() *Config {
	copy := *c
	return &copy
}

func (c *Config) handshakeTimeout() time.Duration {
	return 2 * c.HandshakeIdleTimeout
}

func validateConfig(conf *Config) error {
	if conf == nil {
		return nil
	}

	const maxStreams = 1 << 60
	if conf.MaxIncomingStreams > maxStreams {
		conf.MaxIncomingStreams = maxStreams
	}

	if conf.MaxIncomingUniStreams > maxStreams {
		conf.MaxIncomingUniStreams = maxStreams
	}

	if conf.MaxStreamReceiveWindow > varint.Max {
		conf.MaxStreamReceiveWindow = varint.Max
	}

	if conf.MaxConnectionReceiveWindow > varint.Max {
		conf.MaxConnectionReceiveWindow = varint.Max
	}

	if conf.InitialPacketSize > 0 && conf.InitialPacketSize < protocol.MinInitialPacketSize {
		conf.InitialPacketSize = protocol.MinInitialPacketSize
	}

	if conf.InitialPacketSize > protocol.MaxPacketBufferSize {
		conf.InitialPacketSize = protocol.MaxPacketBufferSize
	}

	// check that all QUIC versions are actually supported
	for _, v := range conf.Versions {
		if !protocol.IsValidVersion(v) {
			return fmt.Errorf("invalid QUIC version: %s", v)
		}
	}

	return nil
}

// populateConfig populates fields in the quic.Config with their default values, if none are set
// it may be called with nil
func populateConfig(conf *Config) *Config {
	if conf == nil {
		conf = &Config{}
	}

	versions := conf.Versions
	if len(versions) == 0 {
		versions = protocol.SupportedVersions
	}

	handshakeIdleTimeout := protocol.DefaultHandshakeIdleTimeout
	if conf.HandshakeIdleTimeout != 0 {
		handshakeIdleTimeout = conf.HandshakeIdleTimeout
	}

	idleTimeout := protocol.DefaultIdleTimeout
	if conf.MaxIdleTimeout != 0 {
		idleTimeout = conf.MaxIdleTimeout
	}

	initialStreamReceiveWindow := conf.InitialStreamReceiveWindow
	if initialStreamReceiveWindow == 0 {
		initialStreamReceiveWindow = protocol.DefaultInitialMaxStreamData
	}

	maxStreamReceiveWindow := conf.MaxStreamReceiveWindow
	if maxStreamReceiveWindow == 0 {
		maxStreamReceiveWindow = protocol.DefaultMaxReceiveStreamFlowControlWindow
	}

	initialConnectionReceiveWindow := conf.InitialConnectionReceiveWindow
	if initialConnectionReceiveWindow == 0 {
		initialConnectionReceiveWindow = protocol.DefaultInitialMaxData
	}

	maxConnectionReceiveWindow := conf.MaxConnectionReceiveWindow
	if maxConnectionReceiveWindow == 0 {
		maxConnectionReceiveWindow = protocol.DefaultMaxReceiveConnectionFlowControlWindow
	}

	maxIncomingStreams := conf.MaxIncomingStreams
	if maxIncomingStreams == 0 {
		maxIncomingStreams = protocol.DefaultMaxIncomingStreams
	} else if maxIncomingStreams < 0 {
		maxIncomingStreams = 0
	}

	maxIncomingUniStreams := conf.MaxIncomingUniStreams
	if maxIncomingUniStreams == 0 {
		maxIncomingUniStreams = protocol.DefaultMaxIncomingUniStreams
	} else if maxIncomingUniStreams < 0 {
		maxIncomingUniStreams = 0
	}

	initialPacketSize := conf.InitialPacketSize
	if initialPacketSize == 0 {
		initialPacketSize = protocol.InitialPacketSize
	}

	return &Config{
		GetConfigForClient:               conf.GetConfigForClient,
		Versions:                         versions,
		HandshakeIdleTimeout:             handshakeIdleTimeout,
		MaxIdleTimeout:                   idleTimeout,
		KeepAlivePeriod:                  conf.KeepAlivePeriod,
		InitialStreamReceiveWindow:       initialStreamReceiveWindow,
		MaxStreamReceiveWindow:           maxStreamReceiveWindow,
		InitialConnectionReceiveWindow:   initialConnectionReceiveWindow,
		MaxConnectionReceiveWindow:       maxConnectionReceiveWindow,
		AllowConnectionWindowIncrease:    conf.AllowConnectionWindowIncrease,
		MaxIncomingStreams:               maxIncomingStreams,
		MaxIncomingUniStreams:            maxIncomingUniStreams,
		TokenStore:                       conf.TokenStore,
		EnableDatagrams:                  conf.EnableDatagrams,
		InitialPacketSize:                initialPacketSize,
		DisablePathMTUDiscovery:          conf.DisablePathMTUDiscovery,
		EnableStreamResetPartialDelivery: conf.EnableStreamResetPartialDelivery,
		Allow0RTT:                        conf.Allow0RTT,
	}
}

// Option is a functional option for configuring a QUIC connection or server.
type Option func(*Config)

func WithDatagrams(enable bool) Option { return func(c *Config) { c.EnableDatagrams = enable } }

func WithKeepAlivePeriod(p time.Duration) Option { return func(c *Config) { c.KeepAlivePeriod = p } }
