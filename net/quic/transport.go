// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quic

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/mach/net/quic/core/handshake"
	"github.com/lemon4ksan/mach/net/quic/core/wire"
)

// Transport manages UDP socket communication and QUIC connection demultiplexing.
type Transport struct {
	Conn net.PacketConn

	mu          sync.Mutex
	closed      bool
	createdConn bool
	listener    *Listener
}

// Close closes the underlying packet connection and any active sessions.
func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	t.closed = true
	if t.listener != nil {
		_ = t.listener.Close()
	}

	if t.Conn != nil && t.createdConn {
		return t.Conn.Close()
	}

	return nil
}

// Listen starts listening for incoming QUIC connections on this Transport.
func (t *Transport) Listen(tlsConf *tls.Config, opts ...Option) (*Listener, error) {
	if tlsConf == nil {
		return nil, errors.New("quic: nil tls.Config")
	}

	conf := &Config{}
	for _, opt := range opts {
		if opt != nil {
			opt(conf)
		}
	}

	tlsCopy := tlsConf.Clone()
	if len(tlsCopy.NextProtos) == 0 {
		tlsCopy.NextProtos = []string{"h3"}
	}

	ln := &Listener{
		tr:        t,
		conn:      t.Conn,
		tlsConf:   tlsCopy,
		config:    conf,
		connChan:  make(chan *Conn, 64),
		errChan:   make(chan error, 1),
		closeChan: make(chan struct{}),
		sessions:  make(map[string]*CoreSessionBridge),
	}

	t.mu.Lock()
	t.listener = ln
	t.mu.Unlock()

	go ln.readLoop()

	return ln, nil
}

// Dial establishes a new client QUIC connection using this Transport.
func (t *Transport) Dial(ctx context.Context, addr net.Addr, tlsConf *tls.Config, opts ...Option) (*Conn, error) {
	conf := &Config{}
	for _, opt := range opts {
		if opt != nil {
			opt(conf)
		}
	}

	return dialInternal(ctx, t.Conn, addr, tlsConf, conf, false)
}

// Listener listens for incoming QUIC connections on a UDP socket.
type Listener struct {
	tr        *Transport
	conn      net.PacketConn
	tlsConf   *tls.Config
	config    *Config
	connChan  chan *Conn
	errChan   chan error
	closeChan chan struct{}
	closed    atomic.Bool

	mu       sync.Mutex
	sessions map[string]*CoreSessionBridge
}

// Accept blocks until an incoming QUIC connection is established.
func (l *Listener) Accept(ctx context.Context) (*Conn, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-l.closeChan:
		return nil, ErrServerClosed
	case err := <-l.errChan:
		return nil, err
	case conn := <-l.connChan:
		return conn, nil
	}
}

// Close terminates the listener and all associated sessions.
func (l *Listener) Close() error {
	if l.closed.CompareAndSwap(false, true) {
		close(l.closeChan)
		l.mu.Lock()
		for _, s := range l.sessions {
			_ = s.Close()
		}

		l.mu.Unlock()

		if l.tr != nil {
			_ = l.tr.Close()
		}
	}

	return nil
}

// Addr returns the local network address of the listener socket.
func (l *Listener) Addr() net.Addr {
	if l.conn != nil {
		return l.conn.LocalAddr()
	}

	return nil
}

func (l *Listener) readLoop() {
	buf := make([]byte, 65536)
	for {
		n, raddr, err := l.conn.ReadFrom(buf)
		if err != nil {
			select {
			case <-l.closeChan:
			default:
				select {
				case l.errChan <- err:
				default:
				}
			}

			return
		}

		if n < 5 {
			continue
		}

		packetData := buf[:n]
		isLong := (packetData[0] & 0x80) != 0

		if isLong {
			if len(packetData) < 6 {
				continue
			}

			dcil := int(packetData[5])
			if len(packetData) < 6+dcil+1 {
				continue
			}

			dcid := packetData[6 : 6+dcil]

			scil := int(packetData[6+dcil])
			if len(packetData) < 7+dcil+scil {
				continue
			}

			scid := packetData[7+dcil : 7+dcil+scil]

			l.mu.Lock()
			session, exists := l.sessions[string(dcid)]
			l.mu.Unlock()

			if exists {
				_ = session.HandleIncomingPacket(packetData)
				continue
			}

			// New session: only Initial packets can create sessions
			pktType := (packetData[0] & 0x30) >> 4
			if pktType != 0 {
				continue
			}

			var serverCIDBytes [8]byte

			_, _ = rand.Read(serverCIDBytes[:])
			serverCID := handshake.NewConnectionID(serverCIDBytes[:])
			clientDCID := handshake.NewConnectionID(dcid)
			clientSCID := handshake.NewConnectionID(scid)

			serverParams := handshake.DefaultTransportParameters()
			serverParams.OriginalDestinationConnectionID = clientDCID
			serverParams.HasOriginalDestinationConnectionID = true
			serverParams.InitialSourceConnectionID = serverCID
			serverParams.HasInitialSourceConnectionID = true
			serverParams.MaxIdleTimeout = 30 * time.Second
			serverParams.HasMaxIdleTimeout = true
			serverParams.InitialMaxData = 64 * 1024 * 1024
			serverParams.InitialMaxStreamDataBidiLocal = 64 * 1024 * 1024
			serverParams.InitialMaxStreamDataBidiRemote = 64 * 1024 * 1024
			serverParams.InitialMaxStreamDataUni = 64 * 1024 * 1024
			serverParams.InitialMaxStreamsBidi = 1000
			serverParams.InitialMaxStreamsUni = 1000

			session, err := NewCoreSessionBridge(CoreBridgeConfig{
				Perspective:        handshake.PerspectiveServer,
				TLSConfig:          l.tlsConf,
				LocalParams:        serverParams,
				OriginalDestConnID: clientDCID,
				DestConnID:         clientSCID,
				SrcConnID:          serverCID,
				Version:            wire.Version1,
				LocalAddr:          l.conn.LocalAddr(),
				RemoteAddr:         raddr,
				PacketConn:         l.conn,
				OwnsConn:           false,
			})
			if err != nil {
				continue
			}

			l.mu.Lock()
			l.sessions[string(serverCID.Bytes())] = session
			l.sessions[string(clientDCID.Bytes())] = session
			l.mu.Unlock()

			if err := session.Start(context.Background()); err != nil {
				continue
			}

			_ = session.HandleIncomingPacket(packetData)

			select {
			case l.connChan <- session:
			case <-l.closeChan:
				return
			}
		} else {
			if len(packetData) < 9 {
				continue
			}

			dcid := packetData[1:9]

			l.mu.Lock()
			session, exists := l.sessions[string(dcid)]
			l.mu.Unlock()

			if exists {
				_ = session.HandleIncomingPacket(packetData)
			}
		}
	}
}

// ListenAddr creates a new QUIC listener on the specified UDP address.
func ListenAddr(addr string, tlsConf *tls.Config, opts ...Option) (*Listener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	_ = conn.SetReadBuffer(16 * 1024 * 1024)
	_ = conn.SetWriteBuffer(16 * 1024 * 1024)
	tr := &Transport{
		Conn:        conn,
		createdConn: true,
	}

	return tr.Listen(tlsConf, opts...)
}

// Listen creates a new QUIC listener on an existing PacketConn.
func Listen(conn net.PacketConn, tlsConf *tls.Config, opts ...Option) (*Listener, error) {
	tr := &Transport{
		Conn:        conn,
		createdConn: false,
	}

	return tr.Listen(tlsConf, opts...)
}

// DialAddr establishes a new client QUIC connection to the specified address.
func DialAddr(ctx context.Context, addr string, tlsConf *tls.Config, opts ...Option) (*Conn, error) {
	conf := &Config{}
	for _, opt := range opts {
		if opt != nil {
			opt(conf)
		}
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}

	_ = udpConn.SetReadBuffer(16 * 1024 * 1024)
	_ = udpConn.SetWriteBuffer(16 * 1024 * 1024)

	return dialInternal(ctx, udpConn, udpAddr, tlsConf, conf, true)
}

// Dial establishes a client QUIC connection on an existing PacketConn.
func Dial(ctx context.Context, c net.PacketConn, addr net.Addr, tlsConf *tls.Config, opts ...Option) (*Conn, error) {
	conf := &Config{}
	for _, opt := range opts {
		if opt != nil {
			opt(conf)
		}
	}

	return dialInternal(ctx, c, addr, tlsConf, conf, false)
}

func dialInternal(
	ctx context.Context,
	packetConn net.PacketConn,
	remoteAddr net.Addr,
	tlsConf *tls.Config,
	conf *Config,
	ownsConn bool,
) (*Conn, error) {
	if tlsConf == nil {
		if ownsConn {
			_ = packetConn.Close()
		}

		return nil, errors.New("quic: nil tls.Config")
	}

	tlsCopy := tlsConf.Clone()
	if len(tlsCopy.NextProtos) == 0 {
		tlsCopy.NextProtos = []string{"h3"}
	}

	var clientDestCIDBytes, clientSrcCIDBytes [8]byte

	_, _ = rand.Read(clientDestCIDBytes[:])
	_, _ = rand.Read(clientSrcCIDBytes[:])
	destCID := handshake.NewConnectionID(clientDestCIDBytes[:])
	srcCID := handshake.NewConnectionID(clientSrcCIDBytes[:])

	clientParams := handshake.DefaultTransportParameters()
	clientParams.InitialSourceConnectionID = srcCID
	clientParams.HasInitialSourceConnectionID = true
	clientParams.MaxIdleTimeout = 30 * time.Second
	clientParams.HasMaxIdleTimeout = true
	clientParams.InitialMaxData = 64 * 1024 * 1024
	clientParams.InitialMaxStreamDataBidiLocal = 64 * 1024 * 1024
	clientParams.InitialMaxStreamDataBidiRemote = 64 * 1024 * 1024
	clientParams.InitialMaxStreamDataUni = 64 * 1024 * 1024
	clientParams.InitialMaxStreamsBidi = 1000
	clientParams.InitialMaxStreamsUni = 1000

	session, err := NewCoreSessionBridge(CoreBridgeConfig{
		Perspective: handshake.PerspectiveClient,
		TLSConfig:   tlsCopy,
		LocalParams: clientParams,
		DestConnID:  destCID,
		SrcConnID:   srcCID,
		Version:     wire.Version1,
		LocalAddr:   packetConn.LocalAddr(),
		RemoteAddr:  remoteAddr,
		PacketConn:  packetConn,
		OwnsConn:    ownsConn,
	})
	if err != nil {
		if ownsConn {
			_ = packetConn.Close()
		}

		return nil, err
	}

	if err := session.Start(ctx); err != nil {
		if ownsConn {
			_ = packetConn.Close()
		}

		return nil, err
	}

	go func() {
		buf := make([]byte, 65536)
		for {
			n, _, err := packetConn.ReadFrom(buf)
			if err != nil {
				return
			}

			if n > 0 {
				_ = session.HandleIncomingPacket(buf[:n])
			}
		}
	}()

	if err := session.FlushPendingPackets(); err != nil {
		_ = session.Close()
		return nil, err
	}

	select {
	case <-ctx.Done():
		_ = session.Close()
		return nil, ctx.Err()
	case <-session.HandshakeDoneWait():
		return session, nil
	}
}
