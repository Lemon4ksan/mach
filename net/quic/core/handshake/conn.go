// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"slices"
	"sync"
)

// Config configures a QUIC TLS 1.3 handshake session driver.
type Config struct {
	// TLSConfig is the crypto/tls configuration. Must be non-nil.
	// MinVersion will be set to tls.VersionTLS13.
	TLSConfig *tls.Config

	// QUICConfig is an optional custom crypto/tls.QUICConfig.
	QUICConfig *tls.QUICConfig

	// LocalParams is the local transport parameters to send to the peer.
	LocalParams TransportParameters

	// DestConnID is the destination connection ID used for Initial key derivation.
	DestConnID ConnectionID

	// Version is the QUIC version (default Version1).
	Version Version

	// InitialSalt is an optional override for Initial key derivation salt.
	InitialSalt []byte

	// Enable0RTT enables 0-RTT early data.
	Enable0RTT bool

	// Event hooks (optional)
	OnHandshakeDone        func()
	OnPeerTransportParams  func(tp *TransportParameters)
	OnHandshakeReadSecret  func(level EncryptionLevel, secret []byte, suite uint16)
	OnHandshakeWriteSecret func(level EncryptionLevel, secret []byte, suite uint16)
	OnEarlyDataRejected    func()
}

// HandshakeDriver drives a native Go stdlib crypto/tls.QUICConn handshake session.
type HandshakeDriver struct {
	mu sync.Mutex

	perspective Perspective
	version     Version
	quicConn    *tls.QUICConn
	tlsConf     *tls.Config

	localParams TransportParameters
	peerParams  *TransportParameters

	initialKeys *InitialKeys

	readSecrets  [4][]byte
	writeSecrets [4][]byte

	readKeys  [4]*TrafficKeys
	writeKeys [4]*TrafficKeys

	outgoingCrypto [4][]byte

	handshakeDone     bool
	earlyDataRejected bool
	closed            bool
	err               error

	// Hooks
	onHandshakeDone       func()
	onPeerTransportParams func(tp *TransportParameters)
	onReadSecret          func(level EncryptionLevel, secret []byte, suite uint16)
	onWriteSecret         func(level EncryptionLevel, secret []byte, suite uint16)
	onEarlyDataRejected   func()
}

// NewClientHandshake creates a new HandshakeDriver for a QUIC client.
func NewClientHandshake(cfg *Config) (*HandshakeDriver, error) {
	if cfg == nil || cfg.TLSConfig == nil {
		return nil, errors.New("quic: TLSConfig must not be nil for client")
	}
	tlsConf := cfg.TLSConfig.Clone()
	tlsConf.MinVersion = tls.VersionTLS13

	// RFC 9001 requires ALPN
	if len(tlsConf.NextProtos) == 0 {
		return nil, errors.New("quic: TLSConfig.NextProtos must contain at least one protocol")
	}

	quicCfg := cfg.QUICConfig
	if quicCfg == nil {
		quicCfg = &tls.QUICConfig{
			TLSConfig:           tlsConf,
			EnableSessionEvents: true,
		}
	} else if quicCfg.TLSConfig == nil {
		quicCfg.TLSConfig = tlsConf
	}

	quicConn := tls.QUICClient(quicCfg)
	return newHandshakeDriver(quicConn, tlsConf, cfg, PerspectiveClient)
}

// NewServerHandshake creates a new HandshakeDriver for a QUIC server.
func NewServerHandshake(cfg *Config) (*HandshakeDriver, error) {
	if cfg == nil || cfg.TLSConfig == nil {
		return nil, errors.New("quic: TLSConfig must not be nil for server")
	}
	tlsConf := cfg.TLSConfig.Clone()
	tlsConf.MinVersion = tls.VersionTLS13

	if len(tlsConf.Certificates) == 0 && tlsConf.GetCertificate == nil && tlsConf.GetConfigForClient == nil {
		return nil, errors.New("quic: TLSConfig must have at least one certificate or GetCertificate")
	}

	quicCfg := cfg.QUICConfig
	if quicCfg == nil {
		quicCfg = &tls.QUICConfig{
			TLSConfig:           tlsConf,
			EnableSessionEvents: true,
		}
	} else if quicCfg.TLSConfig == nil {
		quicCfg.TLSConfig = tlsConf
	}

	quicConn := tls.QUICServer(quicCfg)
	return newHandshakeDriver(quicConn, tlsConf, cfg, PerspectiveServer)
}

func newHandshakeDriver(quicConn *tls.QUICConn, tlsConf *tls.Config, cfg *Config, pers Perspective) (*HandshakeDriver, error) {
	v := cfg.Version
	if v == 0 {
		v = Version1
	}

	// Derive Initial keys
	salt := cfg.InitialSalt
	if salt == nil {
		if v == Version2 {
			salt = SaltV2
		} else {
			salt = SaltV1
		}
	}
	initKeys := NewInitialKeysWithSalt(cfg.DestConnID.Bytes(), salt, v)

	d := &HandshakeDriver{
		perspective:           pers,
		version:               v,
		quicConn:              quicConn,
		tlsConf:               tlsConf,
		localParams:           cfg.LocalParams,
		initialKeys:           initKeys,
		onHandshakeDone:       cfg.OnHandshakeDone,
		onPeerTransportParams: cfg.OnPeerTransportParams,
		onReadSecret:          cfg.OnHandshakeReadSecret,
		onWriteSecret:         cfg.OnHandshakeWriteSecret,
		onEarlyDataRejected:   cfg.OnEarlyDataRejected,
	}

	// Pre-load Initial secrets into table
	if pers == PerspectiveClient {
		d.readSecrets[EncryptionLevelInitial] = initKeys.ServerSecret
		d.writeSecrets[EncryptionLevelInitial] = initKeys.ClientSecret
	} else {
		d.readSecrets[EncryptionLevelInitial] = initKeys.ClientSecret
		d.writeSecrets[EncryptionLevelInitial] = initKeys.ServerSecret
	}

	// Set transport parameters upfront so QUICTransportParametersRequired is not needed
	marshaled := d.localParams.Marshal(d.perspective)
	d.quicConn.SetTransportParameters(marshaled)

	return d, nil
}

// Start begins the handshake.
func (d *HandshakeDriver) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return errors.New("quic: handshake driver is closed")
	}

	if err := d.quicConn.Start(ctx); err != nil {
		d.err = err
		return fmt.Errorf("quic: tls.QUICConn.Start failed: %w", err)
	}

	return d.processEventsLocked()
}

// HandleCryptoData supplies received CRYPTO frame data to the TLS state machine.
func (d *HandshakeDriver) HandleCryptoData(level EncryptionLevel, data []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return errors.New("quic: handshake driver is closed")
	}
	if d.err != nil {
		return d.err
	}

	tlsLevel := level.ToTLS()
	if err := d.quicConn.HandleData(tlsLevel, data); err != nil {
		d.err = err
		return fmt.Errorf("quic: tls.QUICConn.HandleData failed at %s: %w", level, err)
	}

	return d.processEventsLocked()
}

func (d *HandshakeDriver) processEventsLocked() error {
	for {
		ev := d.quicConn.NextEvent()
		if ev.Kind == tls.QUICNoEvent {
			break
		}

		if err := d.handleEventLocked(ev); err != nil {
			d.err = err
			return err
		}
	}
	return nil
}

func (d *HandshakeDriver) handleEventLocked(ev tls.QUICEvent) error {
	switch ev.Kind {
	case tls.QUICNoEvent:
		return nil

	case tls.QUICSetReadSecret:
		level := FromTLSEncryptionLevel(ev.Level)
		secret := slices.Clone(ev.Data)
		d.readSecrets[level] = secret

		tKeys, err := DeriveTrafficKeys(secret, ev.Suite, d.version)
		if err != nil {
			return fmt.Errorf("quic: failed to derive read traffic keys at %s: %w", level, err)
		}
		d.readKeys[level] = tKeys

		if d.onReadSecret != nil {
			d.onReadSecret(level, secret, ev.Suite)
		}
		return nil

	case tls.QUICSetWriteSecret:
		level := FromTLSEncryptionLevel(ev.Level)
		secret := slices.Clone(ev.Data)
		d.writeSecrets[level] = secret

		tKeys, err := DeriveTrafficKeys(secret, ev.Suite, d.version)
		if err != nil {
			return fmt.Errorf("quic: failed to derive write traffic keys at %s: %w", level, err)
		}
		d.writeKeys[level] = tKeys

		if d.onWriteSecret != nil {
			d.onWriteSecret(level, secret, ev.Suite)
		}
		return nil

	case tls.QUICWriteData:
		level := FromTLSEncryptionLevel(ev.Level)
		// Important: ev.Data is owned by crypto/tls and overwritten on subsequent NextEvent calls.
		d.outgoingCrypto[level] = append(d.outgoingCrypto[level], ev.Data...)
		return nil

	case tls.QUICTransportParameters:
		var tp TransportParameters
		if err := tp.Unmarshal(ev.Data, d.perspective.Opposite()); err != nil {
			return fmt.Errorf("quic: failed to unmarshal peer transport parameters: %w", err)
		}
		d.peerParams = &tp
		if d.onPeerTransportParams != nil {
			d.onPeerTransportParams(&tp)
		}
		return nil

	case tls.QUICTransportParametersRequired:
		marshaled := d.localParams.Marshal(d.perspective)
		d.quicConn.SetTransportParameters(marshaled)
		return nil

	case tls.QUICHandshakeDone:
		d.handshakeDone = true
		if d.onHandshakeDone != nil {
			d.onHandshakeDone()
		}
		return nil

	case tls.QUICRejectedEarlyData:
		d.earlyDataRejected = true
		if d.onEarlyDataRejected != nil {
			d.onEarlyDataRejected()
		}
		return nil

	case tls.QUICResumeSession:
		// Accept session resumption
		return nil

	case tls.QUICStoreSession:
		if d.perspective == PerspectiveClient && ev.SessionState != nil {
			_ = d.quicConn.StoreSession(ev.SessionState)
		}
		return nil

	case tls.QUICErrorEvent:
		return fmt.Errorf("quic: tls alert/error: %w", ev.Err)

	default:
		// Unknown events are ignored per crypto/tls contract
		return nil
	}
}

// HasCryptoData reports whether pending crypto data exists for the given encryption level.
func (d *HandshakeDriver) HasCryptoData(level EncryptionLevel) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.outgoingCrypto[level]) > 0
}

// DrainCryptoData returns and clears all pending crypto data for the given encryption level.
func (d *HandshakeDriver) DrainCryptoData(level EncryptionLevel) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()

	data := d.outgoingCrypto[level]
	d.outgoingCrypto[level] = nil
	return data
}

// PeekCryptoData returns pending crypto data without clearing it.
func (d *HandshakeDriver) PeekCryptoData(level EncryptionLevel) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.outgoingCrypto[level]
}

// IsHandshakeDone reports whether the TLS handshake has reached 1-RTT completion.
func (d *HandshakeDriver) IsHandshakeDone() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.handshakeDone
}

// PeerTransportParameters returns the peer's validated transport parameters, or nil if not yet received.
func (d *HandshakeDriver) PeerTransportParameters() *TransportParameters {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.peerParams
}

// LocalTransportParameters returns the local transport parameters.
func (d *HandshakeDriver) LocalTransportParameters() TransportParameters {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.localParams
}

// InitialKeys returns the derived RFC 9001 Initial keys.
func (d *HandshakeDriver) InitialKeys() *InitialKeys {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.initialKeys
}

// ReadSecret returns the traffic secret for the given encryption level.
func (d *HandshakeDriver) ReadSecret(level EncryptionLevel) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.readSecrets[level]
}

// WriteSecret returns the traffic secret for the given encryption level.
func (d *HandshakeDriver) WriteSecret(level EncryptionLevel) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.writeSecrets[level]
}

// ReadKeys returns the derived TrafficKeys for the given encryption level.
func (d *HandshakeDriver) ReadKeys(level EncryptionLevel) *TrafficKeys {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.readKeys[level]
}

// WriteKeys returns the derived TrafficKeys for the given encryption level.
func (d *HandshakeDriver) WriteKeys(level EncryptionLevel) *TrafficKeys {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.writeKeys[level]
}

// ConnectionState returns the underlying TLS connection state.
func (d *HandshakeDriver) ConnectionState() tls.ConnectionState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.quicConn.ConnectionState()
}

// Close closes the TLS handshake driver and stops any running handshake goroutines.
func (d *HandshakeDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true
	return d.quicConn.Close()
}
