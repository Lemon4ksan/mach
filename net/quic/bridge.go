// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/lemon4ksan/mach/net/quic/core/handshake"
	"github.com/lemon4ksan/mach/net/quic/core/stream"
	"github.com/lemon4ksan/mach/net/quic/core/wire"
)

// Clean-Room Core Type Aliases:
// These type aliases provide direct access to the clean-room RFC 9000/9001/9114
// core stream and error types defined in net/quic/core/stream.
type (
	// CoreStream is the clean-room bidirectional stream engine.
	CoreStream = stream.Stream
	// CoreSendStream is the clean-room outbound unidirectional stream engine.
	CoreSendStream = stream.SendStream
	// CoreReceiveStream is the clean-room inbound unidirectional stream engine.
	CoreReceiveStream = stream.ReceiveStream
	// CoreStreamID is the 62-bit stream identifier.
	CoreStreamID = stream.StreamID
	// CoreStreamErrorCode is the application-specified stream error code.
	CoreStreamErrorCode = stream.StreamErrorCode
	// CoreApplicationErrorCode is the application-level closure code.
	CoreApplicationErrorCode = stream.ApplicationErrorCode
	// CoreTransportErrorCode is the transport error code.
	CoreTransportErrorCode = stream.TransportErrorCode
	// CoreConn is the clean-room connection session multiplexer.
	CoreConn = stream.Conn
	// CoreConnConfig configures the stream multiplexer and flow controller.
	CoreConnConfig = stream.ConnConfig
	// CoreStreamI defines the bidirectional stream interface.
	CoreStreamI = stream.StreamI
	// CoreSendStreamI defines the unidirectional send stream interface.
	CoreSendStreamI = stream.SendStreamI
	// CoreReceiveStreamI defines the unidirectional receive stream interface.
	CoreReceiveStreamI = stream.ReceiveStreamI
	// CoreConnI defines the connection session interface required by HTTP/3.
	CoreConnI = stream.ConnI
)

// CoreSessionBridge connects the clean-room core engines (core/wire, core/handshake, core/stream)
// with the transport layer, implementing CID demultiplexing, TLS 1.3 key derivation,
// packet wire protection, and stream frame routing with asymmetric L7 isolation.
type CoreSessionBridge struct {
	mu sync.Mutex

	perspective handshake.Perspective
	version     wire.Version

	destConnID handshake.ConnectionID
	srcConnID  handshake.ConnectionID

	handshakeDriver *handshake.HandshakeDriver
	streamConn      *stream.Conn

	initialProtector   *wire.PacketProtector
	initialOpener      *wire.PacketOpener
	handshakeProtector *wire.PacketProtector
	handshakeOpener    *wire.PacketOpener
	oneRTTProtector    *wire.PacketProtector
	oneRTTOpener       *wire.PacketOpener

	nextPNInitial atomic.Uint64
	nextPNHs      atomic.Uint64
	nextPN1RTT    atomic.Uint64

	largestRecvPNInitial atomic.Uint64
	largestRecvPNHs      atomic.Uint64
	largestRecvPN1RTT    atomic.Uint64

	earlyHandshakePackets [][]byte
	early1RTTPackets      [][]byte

	handshakeDoneCh chan struct{}
	closedCh        chan struct{}

	localAddr  net.Addr
	remoteAddr net.Addr
	packetConn net.PacketConn
	ownsConn   bool

	// Outgoing datagram channel for network transmission
	outgoingPackets chan []byte
}

// CoreBridgeConfig configures a CoreSessionBridge session.
type CoreBridgeConfig struct {
	Perspective        handshake.Perspective
	TLSConfig          *tls.Config
	LocalParams        handshake.TransportParameters
	DestConnID         handshake.ConnectionID
	OriginalDestConnID handshake.ConnectionID
	SrcConnID          handshake.ConnectionID
	Version            wire.Version
	LocalAddr          net.Addr
	RemoteAddr         net.Addr
	PacketConn         net.PacketConn
	OwnsConn           bool
}

// NewCoreSessionBridge initializes a clean-room QUIC session bridging core wire,
// handshake, and stream engines into a unified transport session driver.
func NewCoreSessionBridge(cfg CoreBridgeConfig) (*CoreSessionBridge, error) {
	if cfg.TLSConfig == nil {
		return nil, errors.New("quic: bridge requires non-nil TLSConfig")
	}
	if cfg.Version == 0 {
		cfg.Version = wire.Version1
	}

	persStream := stream.PerspectiveClient
	if cfg.Perspective == handshake.PerspectiveServer {
		persStream = stream.PerspectiveServer
	}

	streamConn := stream.NewConn(stream.ConnConfig{
		Perspective:                    persStream,
		LocalAddr:                      cfg.LocalAddr,
		RemoteAddr:                     cfg.RemoteAddr,
		InitialStreamReceiveWindow:     64 * 1024 * 1024,
		MaxStreamReceiveWindow:         64 * 1024 * 1024,
		InitialConnectionReceiveWindow: 64 * 1024 * 1024,
		MaxConnectionReceiveWindow:     64 * 1024 * 1024,
		InitialMaxStreamsBidi:          1000,
		InitialMaxStreamsUni:           1000,
		InitialPeerMaxStreamsBidi:      1000,
		InitialPeerMaxStreamsUni:       1000,
		InitialPeerMaxData:             64 * 1024 * 1024,
	})

	initialCID := cfg.DestConnID
	if cfg.Perspective == handshake.PerspectiveServer && cfg.OriginalDestConnID.Len() > 0 {
		initialCID = cfg.OriginalDestConnID
	}

	handshakeCfg := &handshake.Config{
		TLSConfig:   cfg.TLSConfig,
		LocalParams: cfg.LocalParams,
		DestConnID:  initialCID,
		Version:     handshake.Version(cfg.Version),
	}

	var driver *handshake.HandshakeDriver
	var err error
	if cfg.Perspective == handshake.PerspectiveClient {
		driver, err = handshake.NewClientHandshake(handshakeCfg)
	} else {
		driver, err = handshake.NewServerHandshake(handshakeCfg)
	}
	if err != nil {
		return nil, fmt.Errorf("quic: failed to create handshake driver: %w", err)
	}

	b := &CoreSessionBridge{
		perspective:     cfg.Perspective,
		version:         cfg.Version,
		destConnID:      cfg.DestConnID,
		srcConnID:       cfg.SrcConnID,
		handshakeDriver: driver,
		streamConn:      streamConn,
		handshakeDoneCh: make(chan struct{}),
		closedCh:        make(chan struct{}),
		localAddr:       cfg.LocalAddr,
		remoteAddr:      cfg.RemoteAddr,
		packetConn:      cfg.PacketConn,
		ownsConn:        cfg.OwnsConn,
		outgoingPackets: make(chan []byte, 64),
	}
	b.largestRecvPNInitial.Store(wire.InvalidPacketNumber)
	b.largestRecvPNHs.Store(wire.InvalidPacketNumber)
	b.largestRecvPN1RTT.Store(wire.InvalidPacketNumber)

	// Initialize Initial encryption keys
	initKeys := driver.InitialKeys()
	if cfg.Perspective == handshake.PerspectiveClient {
		b.initialProtector, err = wire.NewPacketProtector(
			wire.TLS_AES_128_GCM_SHA256,
			initKeys.ClientKey,
			initKeys.ClientIV,
			initKeys.ClientHPKey,
		)
		if err != nil {
			return nil, err
		}
		b.initialOpener, err = wire.NewPacketOpener(
			wire.TLS_AES_128_GCM_SHA256,
			initKeys.ServerKey,
			initKeys.ServerIV,
			initKeys.ServerHPKey,
		)
		if err != nil {
			return nil, err
		}
	} else {
		b.initialProtector, err = wire.NewPacketProtector(
			wire.TLS_AES_128_GCM_SHA256,
			initKeys.ServerKey,
			initKeys.ServerIV,
			initKeys.ServerHPKey,
		)
		if err != nil {
			return nil, err
		}
		b.initialOpener, err = wire.NewPacketOpener(
			wire.TLS_AES_128_GCM_SHA256,
			initKeys.ClientKey,
			initKeys.ClientIV,
			initKeys.ClientHPKey,
		)
		if err != nil {
			return nil, err
		}
	}

	return b, nil
}

// Start begins the TLS 1.3 handshake session.
func (b *CoreSessionBridge) Start(ctx context.Context) error {
	if err := b.handshakeDriver.Start(ctx); err != nil {
		return err
	}
	b.checkHandshakeProgressLocked()
	return nil
}

// StreamConn returns the underlying clean-room stream multiplexer.
func (b *CoreSessionBridge) StreamConn() *stream.Conn {
	return b.streamConn
}

// HandshakeDriver returns the underlying native TLS 1.3 handshake driver.
func (b *CoreSessionBridge) HandshakeDriver() *handshake.HandshakeDriver {
	return b.handshakeDriver
}

// Context returns the connection context.
func (b *CoreSessionBridge) Context() context.Context {
	return b.streamConn.Context()
}

// LocalAddr returns the local address.
func (b *CoreSessionBridge) LocalAddr() net.Addr {
	return b.localAddr
}

// RemoteAddr returns the remote address.
func (b *CoreSessionBridge) RemoteAddr() net.Addr {
	return b.remoteAddr
}

// IsHandshakeDone reports whether the 1-RTT handshake is complete.
func (b *CoreSessionBridge) IsHandshakeDone() bool {
	select {
	case <-b.handshakeDoneCh:
		return true
	default:
		return false
	}
}

// HandshakeDoneWait returns a channel closed when the handshake completes.
func (b *CoreSessionBridge) HandshakeDoneWait() <-chan struct{} {
	return b.handshakeDoneCh
}

// SetDestConnID updates the peer destination connection ID used for outgoing packets.
func (b *CoreSessionBridge) SetDestConnID(cid handshake.ConnectionID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.destConnID = cid
}

// HandleIncomingPacket processes an incoming raw UDP datagram with zero-allocation
// header parsing, AEAD decryption, frame dispatch, and strict asymmetric isolation.
func (b *CoreSessionBridge) HandleIncomingPacket(packetData []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.handleSinglePacketLocked(packetData)
}

func (b *CoreSessionBridge) handleSinglePacketLocked(packetData []byte) error {
	if len(packetData) == 0 {
		return nil
	}

	isLong := (packetData[0] & 0x80) != 0
	var pktType wire.PacketType
	if isLong {
		typeBits := (packetData[0] & 0x30) >> 4
		switch typeBits {
		case 0x0:
			pktType = wire.PacketTypeInitial
		case 0x2:
			pktType = wire.PacketTypeHandshake
		default:
			return errors.New("quic: unsupported or invalid long header packet type")
		}
	} else {
		if (packetData[0] & 0x40) == 0 {
			return errors.New("quic: invalid fixed bit in short header")
		}
		pktType = wire.PacketType1RTT
	}

	// OpenPacket modifies packet in-place to remove header protection, so work on a copy
	pktCopy := append([]byte(nil), packetData...)
	connIDLen := b.srcConnID.Len()

	var hdr wire.Header
	var decryptedPayload []byte
	var err error

	switch pktType {
	case wire.PacketTypeInitial:
		if b.initialOpener == nil {
			return errors.New("quic: initial opener not available")
		}
		largestAcked := b.largestRecvPNInitial.Load()
		hdr, decryptedPayload, err = b.initialOpener.OpenPacket(nil, pktCopy, connIDLen, largestAcked)
		if err != nil {
			return fmt.Errorf("quic: initial open error: %w", err)
		}
		if largestAcked == wire.InvalidPacketNumber || hdr.PacketNumber > largestAcked {
			b.largestRecvPNInitial.Store(hdr.PacketNumber)
		}
		if err := b.processDecryptedFramesLocked(handshake.EncryptionLevelInitial, decryptedPayload); err != nil {
			return err
		}
		if b.perspective == handshake.PerspectiveClient && len(hdr.SrcConnID) > 0 {
			b.destConnID = handshake.NewConnectionID(hdr.SrcConnID)
		}

	case wire.PacketTypeHandshake:
		b.ensureHandshakeKeysLocked()
		if b.handshakeOpener == nil {
			// Early packet reordering: buffer handshake packet until handshake keys are derived
			if len(b.earlyHandshakePackets) < 32 {
				b.earlyHandshakePackets = append(b.earlyHandshakePackets, pktCopy)
			}
			return nil
		}
		largestAcked := b.largestRecvPNHs.Load()
		hdr, decryptedPayload, err = b.handshakeOpener.OpenPacket(nil, pktCopy, connIDLen, largestAcked)
		if err != nil {
			return fmt.Errorf("quic: handshake open error: %w", err)
		}
		if largestAcked == wire.InvalidPacketNumber || hdr.PacketNumber > largestAcked {
			b.largestRecvPNHs.Store(hdr.PacketNumber)
		}
		if err := b.processDecryptedFramesLocked(handshake.EncryptionLevelHandshake, decryptedPayload); err != nil {
			return err
		}

	case wire.PacketType1RTT:
		b.ensure1RTTKeysLocked()
		if b.oneRTTOpener == nil {
			// Early packet reordering: buffer 1-RTT packet until 1-RTT keys are derived
			if len(b.early1RTTPackets) < 32 {
				b.early1RTTPackets = append(b.early1RTTPackets, pktCopy)
			}
			return nil
		}
		largestAcked := b.largestRecvPN1RTT.Load()
		hdr, decryptedPayload, err = b.oneRTTOpener.OpenPacket(nil, pktCopy, connIDLen, largestAcked)
		if err != nil {
			return fmt.Errorf("quic: 1-RTT open error: %w", err)
		}
		if largestAcked == wire.InvalidPacketNumber || hdr.PacketNumber > largestAcked {
			b.largestRecvPN1RTT.Store(hdr.PacketNumber)
		}
		if err := b.processDecryptedFramesLocked(handshake.EncryptionLevel1RTT, decryptedPayload); err != nil {
			return err
		}

	default:
		return nil
	}

	b.checkHandshakeProgressLocked()
	if b.packetConn != nil && b.remoteAddr != nil {
		packets := b.drainCryptoPacketsLocked()
		for _, pkt := range packets {
			_, _ = b.packetConn.WriteTo(pkt, b.remoteAddr)
		}
	}
	return nil
}

func (b *CoreSessionBridge) processDecryptedFramesLocked(level handshake.EncryptionLevel, payload []byte) error {
	// Parse frames with zero heap allocation using push iterator
	for f, err := range wire.Frames(payload) {
		if err != nil {
			// Asymmetric isolation: Corrupted frames are rejected without corrupting active streams
			return fmt.Errorf("quic: malformed frame in %s packet: %w", level, err)
		}

		switch {
		case f.Type == wire.FrameTypePadding:
			// No-op

		case f.Type == wire.FrameTypePing:
			// Ping eliciting ACK

		case f.Type == wire.FrameTypeCrypto:
			if err := b.handshakeDriver.HandleCryptoData(level, f.Crypto.Data); err != nil {
				return fmt.Errorf("quic: crypto data error at %s: %w", level, err)
			}

		case f.Type >= 0x08 && f.Type <= 0x0f:
			// Stream frame routed directly into clean-room stream engine
			err := b.streamConn.HandleStreamFrame(
				stream.StreamID(f.Stream.StreamID),
				f.Stream.Offset,
				f.Stream.Data,
				f.Stream.Fin,
			)
			if err != nil {
				return err
			}

		case f.Type == wire.FrameTypeResetStream:
			err := b.streamConn.HandleResetStreamFrame(
				stream.StreamID(f.ResetStream.StreamID),
				stream.StreamErrorCode(f.ResetStream.ErrorCode),
				f.ResetStream.FinalSize,
			)
			if err != nil {
				return err
			}

		case f.Type == wire.FrameTypeStopSending:
			b.streamConn.HandleStopSendingFrame(
				stream.StreamID(f.StopSending.StreamID),
				stream.StreamErrorCode(f.StopSending.ErrorCode),
			)

		case f.Type == wire.FrameTypeMaxData:
			b.streamConn.HandleMaxDataFrame(f.MaxData.MaximumData)

		case f.Type == wire.FrameTypeMaxStreamData:
			b.streamConn.HandleMaxStreamDataFrame(
				stream.StreamID(f.MaxStreamData.StreamID),
				f.MaxStreamData.MaximumStreamData,
			)

		case f.Type == wire.FrameTypeMaxStreamsBidi:
			b.streamConn.HandleMaxStreamsFrame(stream.StreamTypeClientBidi, f.MaxStreams.MaxStreams)

		case f.Type == wire.FrameTypeMaxStreamsUni:
			b.streamConn.HandleMaxStreamsFrame(stream.StreamTypeClientUni, f.MaxStreams.MaxStreams)

		case f.Type == wire.FrameTypeConnectionCloseTransport || f.Type == wire.FrameTypeConnectionCloseApp:
			_ = b.streamConn.CloseWithError(
				stream.ApplicationErrorCode(f.ConnectionClose.ErrorCode),
				string(f.ConnectionClose.Reason),
			)

		default:
			// Other frames ignored or handled cleanly
		}
	}
	return nil
}

func (b *CoreSessionBridge) checkHandshakeProgressLocked() {
	b.ensureHandshakeKeysLocked()
	b.ensure1RTTKeysLocked()

	// Drain early Handshake packets if opener became available
	if b.handshakeOpener != nil && len(b.earlyHandshakePackets) > 0 {
		pkts := b.earlyHandshakePackets
		b.earlyHandshakePackets = nil
		for _, pkt := range pkts {
			_ = b.handleSinglePacketLocked(pkt)
		}
	}

	// Drain early 1-RTT packets if opener became available
	if b.oneRTTOpener != nil && len(b.early1RTTPackets) > 0 {
		pkts := b.early1RTTPackets
		b.early1RTTPackets = nil
		for _, pkt := range pkts {
			_ = b.handleSinglePacketLocked(pkt)
		}
	}

	if b.handshakeDriver.IsHandshakeDone() {
		select {
		case <-b.handshakeDoneCh:
		default:
			close(b.handshakeDoneCh)
		}
	}
}

func (b *CoreSessionBridge) ensureHandshakeKeysLocked() {
	if b.handshakeProtector == nil {
		wKeys := b.handshakeDriver.WriteKeys(handshake.EncryptionLevelHandshake)
		if wKeys != nil {
			p, err := wire.NewPacketProtector(wire.CipherSuite(wKeys.Suite), wKeys.Key, wKeys.IV, wKeys.HPKey)
			if err == nil {
				b.handshakeProtector = p
			}
		}
	}
	if b.handshakeOpener == nil {
		rKeys := b.handshakeDriver.ReadKeys(handshake.EncryptionLevelHandshake)
		if rKeys != nil {
			o, err := wire.NewPacketOpener(wire.CipherSuite(rKeys.Suite), rKeys.Key, rKeys.IV, rKeys.HPKey)
			if err == nil {
				b.handshakeOpener = o
			}
		}
	}
}

func (b *CoreSessionBridge) ensure1RTTKeysLocked() {
	if b.oneRTTProtector == nil {
		wKeys := b.handshakeDriver.WriteKeys(handshake.EncryptionLevel1RTT)
		if wKeys != nil {
			p, err := wire.NewPacketProtector(wire.CipherSuite(wKeys.Suite), wKeys.Key, wKeys.IV, wKeys.HPKey)
			if err == nil {
				b.oneRTTProtector = p
			}
		}
	}
	if b.oneRTTOpener == nil {
		rKeys := b.handshakeDriver.ReadKeys(handshake.EncryptionLevel1RTT)
		if rKeys != nil {
			o, err := wire.NewPacketOpener(wire.CipherSuite(rKeys.Suite), rKeys.Key, rKeys.IV, rKeys.HPKey)
			if err == nil {
				b.oneRTTOpener = o
			}
		}
	}
}

func (b *CoreSessionBridge) drainCryptoPacketsLocked() [][]byte {
	var packets [][]byte
	levels := []handshake.EncryptionLevel{
		handshake.EncryptionLevelInitial,
		handshake.EncryptionLevelHandshake,
	}

	for _, lvl := range levels {
		if b.handshakeDriver.HasCryptoData(lvl) {
			data := b.handshakeDriver.DrainCryptoData(lvl)
			pkt, err := b.packCryptoPacketLocked(lvl, data)
			if err == nil && len(pkt) > 0 {
				packets = append(packets, pkt)
			}
		}
	}
	return packets
}

// DrainOutgoingPackets extracts all pending packets ready for transmission.
func (b *CoreSessionBridge) DrainOutgoingPackets() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.drainCryptoPacketsLocked()
}

// FlushPendingPackets sends all pending crypto/handshake packets to remoteAddr over packetConn.
func (b *CoreSessionBridge) FlushPendingPackets() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	packets := b.drainCryptoPacketsLocked()
	for _, pkt := range packets {
		if b.packetConn != nil && b.remoteAddr != nil {
			if _, err := b.packetConn.WriteTo(pkt, b.remoteAddr); err != nil {
				return err
			}
		} else {
			select {
			case b.outgoingPackets <- pkt:
			default:
			}
		}
	}
	return nil
}

func (b *CoreSessionBridge) packCryptoPacketLocked(lvl handshake.EncryptionLevel, cryptoData []byte) ([]byte, error) {
	var pn uint64
	switch lvl {
	case handshake.EncryptionLevelInitial:
		pn = b.nextPNInitial.Add(1) - 1
	case handshake.EncryptionLevelHandshake:
		pn = b.nextPNHs.Add(1) - 1
	default:
		return nil, errors.New("quic: unhandled encryption level for crypto packet")
	}

	var cf wire.CryptoFrame
	cf.Offset = 0
	cf.Data = cryptoData

	payload := wire.AppendCryptoFrame(nil, &cf)

	// RFC 9000 §14.1: Client Initial packets must be padded to at least 1200 bytes
	if lvl == handshake.EncryptionLevelInitial && b.perspective == handshake.PerspectiveClient {
		const minInitialLen = 1200
		curLen := len(payload) + 30 // Approximate header + AEAD overhead
		if curLen < minInitialLen {
			payload = wire.AppendPadding(payload, minInitialLen-curLen)
		}
	}

	var h wire.Header
	h.IsLong = true
	h.Version = b.version
	h.DestConnID = b.destConnID.Bytes()
	h.SrcConnID = b.srcConnID.Bytes()
	h.PacketNumber = pn
	h.PacketNumberLen = 4

	var protector *wire.PacketProtector
	switch lvl {
	case handshake.EncryptionLevelInitial:
		h.Type = wire.PacketTypeInitial
		protector = b.initialProtector
	case handshake.EncryptionLevelHandshake:
		h.Type = wire.PacketTypeHandshake
		b.ensureHandshakeKeysLocked()
		protector = b.handshakeProtector
	default:
		return nil, errors.New("quic: unhandled encryption level for crypto packet")
	}

	if protector == nil {
		return nil, errors.New("quic: protector not ready")
	}

	return protector.SealPacket(nil, &h, payload)
}

func (b *CoreSessionBridge) packStreamPacketLocked(id stream.StreamID, offset uint64, data []byte, fin bool) ([]byte, error) {
	b.ensure1RTTKeysLocked()
	if b.oneRTTProtector == nil {
		return nil, errors.New("quic: 1-RTT protector not ready")
	}

	pn := b.nextPN1RTT.Add(1) - 1

	sf := wire.StreamFrame{
		StreamID: uint64(id),
		Offset:   offset,
		Fin:      fin,
		HasLen:   true,
		Data:     data,
	}

	payload := wire.AppendStreamFrame(nil, &sf)

	var h wire.Header
	h.IsLong = false
	h.Type = wire.PacketType1RTT
	h.DestConnID = b.destConnID.Bytes()
	h.PacketNumber = pn
	h.PacketNumberLen = 4

	return b.oneRTTProtector.SealPacket(nil, &h, payload)
}

// Next1RTTPacketNumber allocates and returns the next 1-RTT packet number.
func (b *CoreSessionBridge) Next1RTTPacketNumber() uint64 {
	return b.nextPN1RTT.Add(1) - 1
}

// PackStreamPacket serializes stream data into a protected 1-RTT short header packet.
func (b *CoreSessionBridge) PackStreamPacket(id stream.StreamID, offset uint64, data []byte, fin bool) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.packStreamPacketLocked(id, offset, data, fin)
}

// FlushStreamData pops pending stream frames and transmits them as 1-RTT packets.
func (b *CoreSessionBridge) FlushStreamData(id stream.StreamID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensure1RTTKeysLocked()
	if b.oneRTTProtector == nil {
		return nil
	}

	str, ok := b.streamConn.GetSendStream(id)
	if !ok {
		return nil
	}

	for {
		data, offset, fin, ok := str.PopStreamFrame(1150)
		if !ok {
			break
		}
		pkt, err := b.packStreamPacketLocked(id, offset, data, fin)
		if err != nil {
			return err
		}
		if b.packetConn != nil && b.remoteAddr != nil {
			if _, err := b.packetConn.WriteTo(pkt, b.remoteAddr); err != nil {
				return err
			}
		} else {
			select {
			case b.outgoingPackets <- pkt:
			default:
			}
		}
	}
	return nil
}

// SendResetStream transmits a RESET_STREAM frame over the network.
func (b *CoreSessionBridge) SendResetStream(id stream.StreamID, code stream.StreamErrorCode, finalSize uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensure1RTTKeysLocked()
	if b.oneRTTProtector == nil || b.packetConn == nil || b.remoteAddr == nil {
		return nil
	}

	rsf := wire.ResetStreamFrame{
		StreamID:  uint64(id),
		ErrorCode: uint64(code),
		FinalSize: finalSize,
	}
	payload := wire.AppendResetStreamFrame(nil, &rsf)

	var h wire.Header
	h.IsLong = false
	h.Type = wire.PacketType1RTT
	h.DestConnID = b.destConnID.Bytes()
	h.PacketNumber = b.nextPN1RTT.Add(1) - 1
	h.PacketNumberLen = 4

	pkt, err := b.oneRTTProtector.SealPacket(nil, &h, payload)
	if err != nil {
		return err
	}
	_, err = b.packetConn.WriteTo(pkt, b.remoteAddr)
	return err
}

// SendStopSending transmits a STOP_SENDING frame over the network.
func (b *CoreSessionBridge) SendStopSending(id stream.StreamID, code stream.StreamErrorCode) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensure1RTTKeysLocked()
	if b.oneRTTProtector == nil || b.packetConn == nil || b.remoteAddr == nil {
		return nil
	}

	ssf := wire.StopSendingFrame{
		StreamID:  uint64(id),
		ErrorCode: uint64(code),
	}
	payload := wire.AppendStopSendingFrame(nil, &ssf)

	var h wire.Header
	h.IsLong = false
	h.Type = wire.PacketType1RTT
	h.DestConnID = b.destConnID.Bytes()
	h.PacketNumber = b.nextPN1RTT.Add(1) - 1
	h.PacketNumberLen = 4

	pkt, err := b.oneRTTProtector.SealPacket(nil, &h, payload)
	if err != nil {
		return err
	}
	_, err = b.packetConn.WriteTo(pkt, b.remoteAddr)
	return err
}

// Close gracefully closes the session and underlying clean-room stream multiplexer.
func (b *CoreSessionBridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	select {
	case <-b.closedCh:
		return nil
	default:
		close(b.closedCh)
	}

	if b.ownsConn && b.packetConn != nil {
		_ = b.packetConn.Close()
	}

	_ = b.handshakeDriver.Close()
	return b.streamConn.Close()
}
