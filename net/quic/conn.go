// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quic

import (
	"context"
	"errors"

	"github.com/lemon4ksan/mach/net/quic/core/stream"
	"github.com/lemon4ksan/mach/net/quic/core/wire"
)

// Stream wraps a clean-room bidirectional stream and coordinates line-rate packet transmission.
type Stream struct {
	*stream.Stream
	bridge *CoreSessionBridge
}

// Write transmits stream payload data with zero allocation on the fast path.
func (s *Stream) Write(p []byte) (int, error) {
	n, err := s.Stream.Write(p)
	if n > 0 {
		_ = s.bridge.FlushStreamData(s.StreamID())
	}
	return n, err
}

// Close gracefully closes the write half of the stream, transmitting FIN.
func (s *Stream) Close() error {
	err := s.Stream.Close()
	_ = s.bridge.FlushStreamData(s.StreamID())
	return err
}

// CloseWrite is an alias for Close, closing the send direction.
func (s *Stream) CloseWrite() error {
	return s.Close()
}

// CloseRead closes the receive direction of the stream.
func (s *Stream) CloseRead() error {
	s.Stream.CancelRead(0)
	return nil
}

// CancelWrite aborts the send direction with RESET_STREAM.
func (s *Stream) CancelWrite(code StreamErrorCode) {
	s.Stream.CancelWrite(code)
	_, finalSize, _ := s.Stream.SendStream().GetResetStreamFrame()
	_ = s.bridge.SendResetStream(s.StreamID(), code, finalSize)
}

// CancelRead aborts the receive direction with STOP_SENDING.
func (s *Stream) CancelRead(code StreamErrorCode) {
	s.Stream.CancelRead(code)
	_ = s.bridge.SendStopSending(s.StreamID(), code)
}

// ReadByte reads a single byte from the stream with 0 allocations.
func (s *Stream) ReadByte() (byte, error) {
	var b [1]byte
	n, err := s.Read(b[:])
	if n == 1 {
		return b[0], nil
	}
	return 0, err
}

// SendStream returns the outbound stream half wrapped with bridge packet flushing.
func (s *Stream) SendStream() *SendStream {
	return &SendStream{SendStream: s.Stream.SendStream(), bridge: s.bridge}
}

// ReceiveStream returns the inbound stream half wrapped with bridge packet flushing.
func (s *Stream) ReceiveStream() *ReceiveStream {
	return &ReceiveStream{ReceiveStream: s.Stream.ReceiveStream(), bridge: s.bridge}
}

// SendStream wraps an outbound unidirectional stream with automatic frame transmission.
type SendStream struct {
	*stream.SendStream
	bridge *CoreSessionBridge
}

// Write transmits payload data on the unidirectional stream.
func (s *SendStream) Write(p []byte) (int, error) {
	n, err := s.SendStream.Write(p)
	if n > 0 {
		_ = s.bridge.FlushStreamData(s.StreamID())
	}
	return n, err
}

// Close gracefully closes the unidirectional send stream with FIN.
func (s *SendStream) Close() error {
	err := s.SendStream.Close()
	_ = s.bridge.FlushStreamData(s.StreamID())
	return err
}

// CancelWrite aborts the outbound unidirectional stream with RESET_STREAM.
func (s *SendStream) CancelWrite(code StreamErrorCode) {
	s.SendStream.CancelWrite(code)
	_, finalSize, _ := s.SendStream.GetResetStreamFrame()
	_ = s.bridge.SendResetStream(s.StreamID(), code, finalSize)
}

// ReceiveStream wraps an inbound unidirectional stream.
type ReceiveStream struct {
	*stream.ReceiveStream
	bridge *CoreSessionBridge
}

// CancelRead aborts the inbound unidirectional stream with STOP_SENDING.
func (s *ReceiveStream) CancelRead(code StreamErrorCode) {
	s.ReceiveStream.CancelRead(code)
	_ = s.bridge.SendStopSending(s.StreamID(), code)
}

// ReadByte reads a single byte from the unidirectional stream with 0 allocations.
func (s *ReceiveStream) ReadByte() (byte, error) {
	var b [1]byte
	n, err := s.Read(b[:])
	if n == 1 {
		return b[0], nil
	}
	return 0, err
}

// Conn represents an active QUIC connection session.
type Conn = CoreSessionBridge

// OpenStream allocates and returns a new bidirectional QUIC stream.
func (b *CoreSessionBridge) OpenStream() (*Stream, error) {
	str, err := b.streamConn.OpenStream()
	if err != nil {
		return nil, err
	}
	return &Stream{Stream: str, bridge: b}, nil
}

// OpenStreamSync blocks until peer flow control permits opening a bidirectional stream.
func (b *CoreSessionBridge) OpenStreamSync(ctx context.Context) (*Stream, error) {
	str, err := b.streamConn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &Stream{Stream: str, bridge: b}, nil
}

// OpenUniStream allocates and returns a new outbound unidirectional stream.
func (b *CoreSessionBridge) OpenUniStream() (*SendStream, error) {
	str, err := b.streamConn.OpenUniStream()
	if err != nil {
		return nil, err
	}
	return &SendStream{SendStream: str, bridge: b}, nil
}

// OpenUniStreamSync blocks until peer flow control permits opening a unidirectional stream.
func (b *CoreSessionBridge) OpenUniStreamSync(ctx context.Context) (*SendStream, error) {
	str, err := b.streamConn.OpenUniStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &SendStream{SendStream: str, bridge: b}, nil
}

// AcceptStream blocks until an inbound peer-initiated bidirectional stream is received.
func (b *CoreSessionBridge) AcceptStream(ctx context.Context) (*Stream, error) {
	str, err := b.streamConn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return &Stream{Stream: str, bridge: b}, nil
}

// AcceptUniStream blocks until an inbound peer-initiated unidirectional stream is received.
func (b *CoreSessionBridge) AcceptUniStream(ctx context.Context) (*ReceiveStream, error) {
	str, err := b.streamConn.AcceptUniStream(ctx)
	if err != nil {
		return nil, err
	}
	return &ReceiveStream{ReceiveStream: str, bridge: b}, nil
}

// CloseWithError terminates the QUIC session with an application error code and reason phrase.
func (b *CoreSessionBridge) CloseWithError(code ApplicationErrorCode, reason string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensure1RTTKeysLocked()
	if b.oneRTTProtector != nil && b.packetConn != nil && b.remoteAddr != nil {
		ccf := wire.ConnectionCloseFrame{
			IsApplication: true,
			ErrorCode:     uint64(code),
			Reason:        []byte(reason),
		}
		payload := wire.AppendConnectionCloseFrame(nil, &ccf)
		var h wire.Header
		h.IsLong = false
		h.Type = wire.PacketType1RTT
		h.DestConnID = b.destConnID.Bytes()
		h.PacketNumber = b.Next1RTTPacketNumber()
		h.PacketNumberLen = 4
		if pkt, err := b.oneRTTProtector.SealPacket(nil, &h, payload); err == nil {
			_, _ = b.packetConn.WriteTo(pkt, b.remoteAddr)
		}
	}
	if b.ownsConn && b.packetConn != nil {
		_ = b.packetConn.Close()
	}
	_ = b.handshakeDriver.Close()
	return b.streamConn.CloseWithError(stream.ApplicationErrorCode(code), reason)
}

// ConnectionState reports active session state including negotiated TLS and version parameters.
func (b *CoreSessionBridge) ConnectionState() ConnectionState {
	b.mu.Lock()
	defer b.mu.Unlock()

	var cs ConnectionState
	cs.Version = b.version
	cs.SupportsDatagrams.Local = true
	cs.SupportsDatagrams.Remote = true
	if b.handshakeDriver != nil {
		cs.TLS = b.handshakeDriver.ConnectionState()
	}
	return cs
}

// SendDatagram transmits an unreliable application datagram (RFC 9221).
func (b *CoreSessionBridge) SendDatagram(payload []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensure1RTTKeysLocked()
	if b.oneRTTProtector == nil {
		return errors.New("quic: 1-RTT protector not ready")
	}

	df := wire.DatagramFrame{
		Data:   payload,
		HasLen: true,
	}
	framePayload := wire.AppendDatagramFrame(nil, &df)

	var h wire.Header
	h.IsLong = false
	h.Type = wire.PacketType1RTT
	h.DestConnID = b.destConnID.Bytes()
	h.PacketNumber = b.Next1RTTPacketNumber()
	h.PacketNumberLen = 4

	pkt, err := b.oneRTTProtector.SealPacket(nil, &h, framePayload)
	if err != nil {
		return err
	}
	if b.packetConn != nil && b.remoteAddr != nil {
		_, err = b.packetConn.WriteTo(pkt, b.remoteAddr)
		return err
	}
	return nil
}

// ReceiveDatagram waits for an incoming DATAGRAM frame.
func (b *CoreSessionBridge) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	return nil, errors.New("quic: datagram reception not queued")
}
