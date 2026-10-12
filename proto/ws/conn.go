// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"bufio"
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	randv2 "math/rand/v2"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// Conn represents an RFC 6455 compliant WebSocket connection with thread-safe
// concurrent read/write support, automatic heartbeats, control frame interleaving,
// and robust adversarial defenses.
type Conn struct {
	rw       io.ReadWriter
	closer   io.Closer
	netConn  net.Conn
	isServer bool

	// Lifecycle state: StateConnecting, StateOpen, StateClosing, StateClosed
	state atomic.Uint32

	// Mutexes: readMu and writeMu are independent.
	readMu  sync.Mutex
	writeMu sync.Mutex

	// Frame header scratch buffers (MaxHeaderSize = 14 bytes)
	readHdrBuf [14]byte

	// Configuration
	readLimit     int64
	skipMaskCheck bool
	enableDeflate bool

	// Handlers
	pingHandler  func(appData []byte) error
	pongHandler  func(appData []byte) error
	closeHandler func(code CloseCode, reason string) error

	// State tracked within readMu
	seenClose      bool
	fragmentOpcode Opcode
	fragmentBuf    []byte
	fragmentRsv1   bool

	// Random masking source (ChaCha8 PRNG seeded with crypto/rand)
	maskRand *randv2.ChaCha8

	// Buffer pool for masked writes
	writeBufPool sync.Pool

	// Preallocated frame buffer protected by writeMu for 0-allocation writes
	writeFrameBuf []byte

	// Negotiated subprotocol
	subprotocol string
}

// NewConn initializes a new WebSocket connection wrapping rw.
// isServer indicates whether this endpoint is a server (true) or client (false).
func NewConn(rw io.ReadWriter, isServer bool) *Conn {
	c := &Conn{
		rw:        rw,
		isServer:  isServer,
		readLimit: 32 << 20, // 32MB default read limit
		writeBufPool: sync.Pool{
			New: func() any {
				b := make([]byte, 32*1024)
				return &b
			},
		},
	}

	// Seed client masking PRNG with cryptographic randomness
	var seed [32]byte

	_, _ = crand.Read(seed[:])
	c.maskRand = randv2.NewChaCha8(seed)

	if closer, ok := rw.(io.Closer); ok {
		c.closer = closer
	}

	if nc, ok := rw.(net.Conn); ok {
		c.netConn = nc
	}

	c.state.Store(uint32(StateOpen))

	// Default ping handler: automatically reply with Pong carrying identical payload (RFC 6455 §5.5.3)
	c.pingHandler = func(appData []byte) error {
		return c.writeControlLocked(OpcodePong, appData)
	}

	// Default pong handler: accept unsolicited or replied pong frames
	c.pongHandler = func(appData []byte) error {
		return nil
	}

	return c
}

// NewClientConn creates a client-side WebSocket connection.
func NewClientConn(rw io.ReadWriter) *Conn {
	return NewConn(rw, false)
}

// NewServerConn creates a server-side WebSocket connection.
func NewServerConn(rw io.ReadWriter) *Conn {
	return NewConn(rw, true)
}

// State returns the current connection state.
func (c *Conn) State() ConnState {
	return ConnState(c.state.Load())
}

// SetState transitions the connection to the specified state.
func (c *Conn) SetState(s ConnState) {
	c.state.Store(uint32(s))
}

// IsServer reports whether this connection is operating as a server.
func (c *Conn) IsServer() bool {
	return c.isServer
}

// SetReadLimit sets the maximum message size allowed in bytes.
func (c *Conn) SetReadLimit(limit int64) {
	c.readLimit = limit
}

// SetSkipMaskCheck toggles strict masking verification.
func (c *Conn) SetSkipMaskCheck(skip bool) {
	c.skipMaskCheck = skip
}

// EnableCompression enables or disables RFC 7692 permessage-deflate compression.
func (c *Conn) EnableCompression(enable bool) {
	c.enableDeflate = enable
}

// SetPingHandler sets the callback for incoming Ping control frames.
func (c *Conn) SetPingHandler(h func(appData []byte) error) {
	c.pingHandler = h
}

// SetPongHandler sets the callback for incoming Pong control frames.
func (c *Conn) SetPongHandler(h func(appData []byte) error) {
	c.pongHandler = h
}

// SetCloseHandler sets the callback for incoming Close control frames.
func (c *Conn) SetCloseHandler(h func(code CloseCode, reason string) error) {
	c.closeHandler = h
}

// Subprotocol returns the negotiated subprotocol.
func (c *Conn) Subprotocol() string {
	return c.subprotocol
}

// SetSubprotocol sets the negotiated subprotocol.
func (c *Conn) SetSubprotocol(sub string) {
	c.subprotocol = sub
}

// WriteText writes an unfragmented Text message (OpcodeText).
func (c *Conn) WriteText(text string) error {
	return c.WriteMessage(OpcodeText, []byte(text))
}

// WriteBinary writes an unfragmented Binary message (OpcodeBinary).
func (c *Conn) WriteBinary(data []byte) error {
	return c.WriteMessage(OpcodeBinary, data)
}

type connReaderWriter struct {
	io.Reader
	io.Writer
}

func (rw *connReaderWriter) Read(p []byte) (int, error) {
	return rw.Reader.Read(p)
}

func (rw *connReaderWriter) Write(p []byte) (int, error) {
	return rw.Writer.Write(p)
}

// NewConnWithReader initializes a WebSocket connection using an existing bufio.Reader.
func NewConnWithReader(rwc io.ReadWriteCloser, br *bufio.Reader, isServer bool) *Conn {
	var rw io.ReadWriter
	if br != nil {
		rw = &connReaderWriter{Reader: br, Writer: rwc}
	} else {
		rw = rwc
	}

	c := NewConn(rw, isServer)

	c.closer = rwc
	if nc, ok := rwc.(net.Conn); ok {
		c.netConn = nc
	}

	return c
}

// Underlying returns the underlying io.ReadWriter.
func (c *Conn) Underlying() io.ReadWriter {
	return c.rw
}

// NetConn returns the underlying net.Conn if available.
func (c *Conn) NetConn() net.Conn {
	return c.netConn
}

// LocalAddr returns the local network address if available.
func (c *Conn) LocalAddr() net.Addr {
	if c.netConn != nil {
		return c.netConn.LocalAddr()
	}

	return nil
}

// RemoteAddr returns the remote network address if available.
func (c *Conn) RemoteAddr() net.Addr {
	if c.netConn != nil {
		return c.netConn.RemoteAddr()
	}

	return nil
}

// SetDeadline sets the read and write deadlines if underlying connection supports it.
func (c *Conn) SetDeadline(t time.Time) error {
	if c.netConn != nil {
		return c.netConn.SetDeadline(t)
	}

	return nil
}

// SetReadDeadline sets the read deadline if underlying connection supports it.
func (c *Conn) SetReadDeadline(t time.Time) error {
	if c.netConn != nil {
		return c.netConn.SetReadDeadline(t)
	}

	return nil
}

// SetWriteDeadline sets the write deadline if underlying connection supports it.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	if c.netConn != nil {
		return c.netConn.SetWriteDeadline(t)
	}

	return nil
}

// ReadMessage reads the next complete message, reassembling fragmented frames,
// processing and replying to interleaved control frames (Ping/Pong/Close) seamlessly.
//
// Hardened against RFC 6455 framing violations, DoS allocation attacks, and invalid payloads.
func (c *Conn) ReadMessage() (messageType Opcode, payload []byte, err error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	for {
		// Verify connection state
		st := c.State()
		if st == StateClosed {
			return 0, nil, ErrClosed
		}

		hdr, err := ReadHeader(c.rw, c.readHdrBuf[:])
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				c.state.Store(uint32(StateClosed))
				return 0, nil, io.EOF
			}

			if errors.Is(err, ErrInvalidOpcode) || errors.Is(err, ErrProtocolViolation) ||
				errors.Is(err, ErrControlFrameFragmented) || errors.Is(err, ErrControlFrameTooLarge) ||
				errors.Is(err, ErrInvalidLength) || errors.Is(err, ErrPayloadTooLarge) {
				c.writeProtocolErrorAndClose(StatusProtocolError, err.Error())
				return 0, nil, err
			}

			return 0, nil, err
		}

		// RFC 6455 §5.2: Opcode validation (check reserved and valid opcodes)
		if !hdr.Opcode.IsValid() || hdr.Opcode.IsReserved() {
			c.drainPayload(hdr.PayloadLen)
			c.writeProtocolErrorAndClose(StatusProtocolError, "Unknown or reserved opcode")
			return 0, nil, ErrInvalidOpcode
		}

		// RFC 6455 §5.5.1: If a Close frame was already received, receiving data frames is forbidden
		if c.seenClose && !hdr.Opcode.IsControl() {
			c.drainPayload(hdr.PayloadLen)
			c.writeProtocolErrorAndClose(StatusProtocolError, "Data frame received after close")
			return 0, nil, ErrDataAfterClose
		}

		// Verify masking rules (RFC 6455 §5.1)
		if !c.skipMaskCheck {
			if c.isServer && !hdr.Masked {
				c.drainPayload(hdr.PayloadLen)
				c.writeProtocolErrorAndClose(StatusProtocolError, "Server requires masked frames")
				return 0, nil, ErrMaskRequired
			}

			if !c.isServer && hdr.Masked {
				c.drainPayload(hdr.PayloadLen)
				c.writeProtocolErrorAndClose(StatusProtocolError, "Client requires unmasked frames")
				return 0, nil, ErrMaskForbidden
			}
		}

		// Verify reserved bits (RSV1, RSV2, RSV3)
		if hdr.Rsv2 || hdr.Rsv3 {
			c.drainPayload(hdr.PayloadLen)
			c.writeProtocolErrorAndClose(StatusProtocolError, "Reserved bits RSV2/RSV3 set")
			return 0, nil, ErrRSVSet
		}

		if hdr.Rsv1 {
			if !c.enableDeflate {
				c.drainPayload(hdr.PayloadLen)
				c.writeProtocolErrorAndClose(StatusProtocolError, "RSV1 set without deflate extension")
				return 0, nil, ErrRSVSet
			}

			if hdr.Opcode.IsControl() || hdr.Opcode == OpcodeContinuation {
				c.drainPayload(hdr.PayloadLen)
				c.writeProtocolErrorAndClose(StatusProtocolError, "RSV1 not allowed on control or continuation frame")
				return 0, nil, ErrRSVSet
			}
		}

		// Control frame constraints (RFC 6455 §5.5)
		if hdr.Opcode.IsControl() {
			if !hdr.Fin {
				c.drainPayload(hdr.PayloadLen)
				c.writeProtocolErrorAndClose(StatusProtocolError, "Control frame must not be fragmented")
				return 0, nil, ErrControlFrameFragmented
			}

			if hdr.PayloadLen > 125 {
				c.drainPayload(hdr.PayloadLen)
				c.writeProtocolErrorAndClose(StatusProtocolError, "Control frame payload exceeds 125 bytes")
				return 0, nil, ErrControlFrameTooLarge
			}
		}

		// DoS / Memory Exhaustion Defense (BUG #1 Fix):
		// Validate against ReadLimit BEFORE allocating payload slice!
		if c.readLimit > 0 && hdr.PayloadLen > c.readLimit {
			c.drainPayload(hdr.PayloadLen)
			c.writeProtocolErrorAndClose(StatusMessageTooBig, "Frame payload exceeds read limit")
			return 0, nil, ErrMessageTooBig
		}

		if !hdr.Opcode.IsControl() && c.readLimit > 0 && (int64(len(c.fragmentBuf))+hdr.PayloadLen) > c.readLimit {
			c.drainPayload(hdr.PayloadLen)
			c.writeProtocolErrorAndClose(StatusMessageTooBig, "Message exceeds read limit")
			return 0, nil, ErrMessageTooBig
		}

		// Read payload safely
		var framePayload []byte
		if hdr.PayloadLen > 0 {
			framePayload = make([]byte, hdr.PayloadLen)
			if _, err := io.ReadFull(c.rw, framePayload); err != nil {
				c.state.Store(uint32(StateClosed))
				return 0, nil, err
			}

			if hdr.Masked {
				Mask(framePayload, hdr.MaskKey)
			}
		}

		// Dispatch Control Frames (Ping / Pong / Close)
		if hdr.Opcode.IsControl() {
			switch hdr.Opcode {
			case OpcodePing:
				if c.pingHandler != nil {
					if err := c.pingHandler(framePayload); err != nil {
						return 0, nil, err
					}
				}

				continue

			case OpcodePong:
				if c.pongHandler != nil {
					if err := c.pongHandler(framePayload); err != nil {
						return 0, nil, err
					}
				}

				continue

			case OpcodeClose:
				c.seenClose = true

				code, reason, parseErr := ParseClosePayload(framePayload)
				if parseErr != nil {
					if errors.Is(parseErr, ErrInvalidUTF8) {
						c.writeProtocolErrorAndClose(StatusInvalidFramePayloadData, "Invalid UTF-8 in Close frame")
					} else {
						c.writeProtocolErrorAndClose(StatusProtocolError, parseErr.Error())
					}

					return 0, nil, parseErr
				}

				if c.closeHandler != nil {
					_ = c.closeHandler(code, reason)
				}

				// RFC 6455 §5.5.1 / §7.1.1: If we haven't sent a Close yet, echo Close back
				oldState := c.State()
				if oldState == StateOpen || oldState == StateConnecting {
					c.state.Store(uint32(StateClosing))

					if code == StatusNoStatusReceived {
						_ = c.writeControlFrame(OpcodeClose, nil)
					} else {
						var buf [128]byte

						n, _ := FormatClosePayload(buf[:], code, reason)
						_ = c.writeControlFrame(OpcodeClose, buf[:n])
					}

					c.state.Store(uint32(StateClosed))
					_ = c.closeTransport()

					return OpcodeClose, nil, &CloseError{Code: code, Reason: reason}
				}

				// Already in StateClosing or simultaneous close occurred
				c.state.Store(uint32(StateClosed))
				_ = c.closeTransport()

				return OpcodeClose, nil, &CloseError{Code: code, Reason: reason}
			}
		}

		// Handle Data Frames (Continuation, Text, Binary)
		if hdr.Opcode == OpcodeContinuation {
			if c.fragmentOpcode == 0 {
				c.writeProtocolErrorAndClose(StatusProtocolError, "Unexpected continuation frame")
				return 0, nil, ErrUnexpectedContinuation
			}
		} else {
			if c.fragmentOpcode != 0 {
				c.writeProtocolErrorAndClose(StatusProtocolError, "Expected continuation frame")
				return 0, nil, ErrExpectedContinuation
			}

			c.fragmentOpcode = hdr.Opcode
			c.fragmentRsv1 = hdr.Rsv1
		}

		c.fragmentBuf = append(c.fragmentBuf, framePayload...)

		// If this is the final fragment, complete message reassembly
		if hdr.Fin {
			msgType := c.fragmentOpcode
			msgPayload := c.fragmentBuf
			isCompressed := c.fragmentRsv1

			// Reset fragmentation state
			c.fragmentOpcode = 0
			c.fragmentBuf = nil
			c.fragmentRsv1 = false

			if isCompressed {
				decompressed, err := DecompressDeflate(msgPayload, c.readLimit)
				if err != nil {
					c.writeProtocolErrorAndClose(StatusProtocolError, "Deflate decompression error")
					return 0, nil, err
				}

				msgPayload = decompressed
			}

			// Validate UTF-8 for Text messages (RFC 6455 §5.6, §8.1)
			if msgType == OpcodeText && !utf8.Valid(msgPayload) {
				c.writeProtocolErrorAndClose(StatusInvalidFramePayloadData, "Invalid UTF-8 in text message")
				return 0, nil, ErrInvalidUTF8
			}

			return msgType, msgPayload, nil
		}
	}
}

// WriteMessage writes a complete unfragmented data message of type OpcodeText or OpcodeBinary.
func (c *Conn) WriteMessage(messageType Opcode, data []byte) error {
	if !messageType.IsData() {
		return ErrProtocolViolation
	}

	st := c.State()
	if st == StateClosing || st == StateClosed {
		return ErrClosed
	}

	rsv1 := false

	payload := data
	if c.enableDeflate {
		compressed, err := CompressDeflate(data, 0)
		if err == nil && len(compressed) < len(data) {
			rsv1 = true
			payload = compressed
		}
	}

	return c.writeDataFrame(true, rsv1, messageType, payload)
}

// WriteFragmentedMessage writes a data message split into multiple frames.
func (c *Conn) WriteFragmentedMessage(messageType Opcode, frames [][]byte) error {
	if !messageType.IsData() {
		return ErrProtocolViolation
	}

	if len(frames) == 0 {
		return c.WriteMessage(messageType, nil)
	}

	for i, frameData := range frames {
		fin := (i == len(frames)-1)

		var op Opcode
		if i == 0 {
			op = messageType
		} else {
			op = OpcodeContinuation
		}

		if err := c.writeDataFrame(fin, false, op, frameData); err != nil {
			return err
		}
	}

	return nil
}

func (c *Conn) writeDataFrame(fin, rsv1 bool, op Opcode, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	st := c.State()
	if st == StateClosing || st == StateClosed {
		return ErrClosed
	}

	return c.writeFrameLocked(fin, rsv1, op, data)
}

// WriteControl writes a control frame (Ping, Pong, Close).
func (c *Conn) WriteControl(messageType Opcode, data []byte) error {
	if !messageType.IsControl() {
		return ErrProtocolViolation
	}

	if len(data) > 125 {
		return ErrControlFrameTooLarge
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	st := c.State()
	if st == StateClosed {
		return ErrClosed
	}

	return c.writeFrameLocked(true, false, messageType, data)
}

// WritePing sends a Ping control frame with payload data (len <= 125).
func (c *Conn) WritePing(data []byte) error {
	return c.WriteControl(OpcodePing, data)
}

// WritePong sends a Pong control frame with payload data (len <= 125).
func (c *Conn) WritePong(data []byte) error {
	return c.WriteControl(OpcodePong, data)
}

// WriteClose initiates a graceful closing handshake by sending a Close frame.
func (c *Conn) WriteClose(statusCode CloseCode, reason string) error {
	if !statusCode.IsValidForWire() {
		return ErrInvalidCloseCode
	}

	if len(reason) > 123 {
		return ErrCloseReasonTooLong
	}

	if !utf8.ValidString(reason) {
		return ErrInvalidUTF8
	}

	var buf [128]byte

	n, err := FormatClosePayload(buf[:], statusCode, reason)
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if !c.state.CompareAndSwap(uint32(StateOpen), uint32(StateClosing)) {
		return ErrClosed
	}

	return c.writeFrameLocked(true, false, OpcodeClose, buf[:n])
}

// Close closes the underlying transport immediately and sets state to StateClosed.
func (c *Conn) Close() error {
	c.state.Store(uint32(StateClosed))
	return c.closeTransport()
}

func (c *Conn) closeTransport() error {
	if c.closer != nil {
		return c.closer.Close()
	}

	return nil
}

func (c *Conn) writeControlLocked(op Opcode, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.writeFrameLocked(true, false, op, data)
}

func (c *Conn) writeControlFrame(op Opcode, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.writeFrameLocked(true, false, op, data)
}

// drainPayload reads and discards up to n unread bytes of an invalid frame payload.
// This prevents Windows TCP sockets (Winsock) from transmitting a hard RST upon closeTransport()
// when unread payload data remains in the socket receive buffer.
func (c *Conn) drainPayload(n int64) {
	if n <= 0 || n > 65536 {
		return
	}

	var discardBuf [512]byte
	for n > 0 {
		toRead := int64(len(discardBuf))
		if n < toRead {
			toRead = n
		}

		readN, err := c.rw.Read(discardBuf[:toRead])
		if err != nil {
			return
		}

		n -= int64(readN)
	}
}

// writeProtocolErrorAndClose sends a Close frame with status code and safely truncated reason,
// then transitions to StateClosed and terminates the transport.
//
// Hardened (BUG #2 Fix): Guarantees reason is truncated to <= 123 bytes at valid UTF-8 boundary
// so the status code is NEVER dropped or replaced by an empty close frame on the wire.
func (c *Conn) writeProtocolErrorAndClose(code CloseCode, reason string) {
	c.state.Store(uint32(StateClosing))

	if len(reason) > 123 {
		// Truncate to at most 123 bytes on valid UTF-8 rune boundary
		cut := 123
		for cut > 0 && !utf8.RuneStart(reason[cut]) {
			cut--
		}

		reason = reason[:cut]
	}

	var buf [128]byte

	n, err := FormatClosePayload(buf[:], code, reason)
	if err != nil {
		binary.BigEndian.PutUint16(buf[:2], uint16(code))

		n = 2
	}

	_ = c.writeControlFrame(OpcodeClose, buf[:n])
	c.state.Store(uint32(StateClosed))
	_ = c.closeTransport()
}

// writeFrameLocked performs the low-level frame serialization and transmission.
// writeFrameLocked performs the low-level frame serialization and transmission.
// Must be called with writeMu locked.
// Transmits the entire frame (header + payload) in a single atomic Write call
// to prevent packet fragmentation, Nagle delays, and pipe reader deadlocks.
func (c *Conn) writeFrameLocked(fin, rsv1 bool, op Opcode, data []byte) error {
	h := Header{
		Fin:        fin,
		Rsv1:       rsv1,
		Opcode:     op,
		PayloadLen: int64(len(data)),
	}

	if !c.isServer {
		h.Masked = true
		maskUint32 := uint32(c.maskRand.Uint64())
		binary.LittleEndian.PutUint32(h.MaskKey[:], maskUint32)
	}

	hdrLen := HeaderLen(h)
	totalLen := hdrLen + len(data)

	if cap(c.writeFrameBuf) < totalLen {
		c.writeFrameBuf = make([]byte, totalLen+1024)
	}

	buf := c.writeFrameBuf[:totalLen]

	_, err := WriteHeader(buf[:hdrLen], h)
	if err != nil {
		return err
	}

	if len(data) > 0 {
		copy(buf[hdrLen:totalLen], data)

		if h.Masked {
			Mask(buf[hdrLen:totalLen], h.MaskKey)
		}
	}

	_, err = c.rw.Write(buf)

	return err
}
