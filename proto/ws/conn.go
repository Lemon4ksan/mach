package ws

import (
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

// Conn represents a WebSocket connection implementing the RFC 6455 state machine,
// control frame interleaving, automatic heartbeats, and close handshakes.
type Conn struct {
	rw       io.ReadWriter
	closer   io.Closer
	netConn  net.Conn
	isServer bool

	// Lifecycle state: StateConnecting, StateOpen, StateClosing, StateClosed
	state atomic.Uint32

	// Mutexes: readMu and writeMu are independent.
	// readMu is held during ReadMessage. writeMu is held during frame writes.
	// When ReadMessage replies with an automatic Pong, it acquires writeMu briefly.
	// writeMu NEVER acquires readMu, preventing deadlocks.
	readMu  sync.Mutex
	writeMu sync.Mutex

	// Frame header scratch buffers (MaxHeaderSize = 14 bytes)
	readHdrBuf  [MaxHeaderSize]byte
	writeHdrBuf [MaxHeaderSize]byte

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

	// Random masking source
	maskRand *randv2.Rand

	// Buffer pool for masked writes
	writeBufPool sync.Pool
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
	c.maskRand = randv2.New(randv2.NewChaCha8(seed))

	if closer, ok := rw.(io.Closer); ok {
		c.closer = closer
	}
	if nc, ok := rw.(net.Conn); ok {
		c.netConn = nc
	}

	// Initial state is StateOpen
	c.state.Store(uint32(StateOpen))

	// Default ping handler: automatically reply with Pong carrying identical payload (RFC 6455 §5.5.3)
	c.pingHandler = func(appData []byte) error {
		return c.writeControlLocked(OpPong, appData)
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
// Messages exceeding this limit will trigger ErrMessageTooBig and close the connection.
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
			if errors.Is(err, ErrUnknownOpcode) || errors.Is(err, ErrProtocolViolation) ||
				errors.Is(err, ErrControlFrameFragmented) || errors.Is(err, ErrControlFrameTooLarge) {
				c.writeProtocolErrorAndClose(StatusProtocolError, err.Error())
				return 0, nil, err
			}
			return 0, nil, err
		}

		// RFC 6455 §5.5.1: If a Close frame was already received, receiving data frames is forbidden
		if c.seenClose && !hdr.Opcode.IsControl() {
			c.writeProtocolErrorAndClose(StatusProtocolError, "Data frame received after close")
			return 0, nil, ErrDataAfterClose
		}

		// Verify masking rules (RFC 6455 §5.1)
		if !c.skipMaskCheck {
			if c.isServer && !hdr.Masked {
				c.writeProtocolErrorAndClose(StatusProtocolError, "Server requires masked frames")
				return 0, nil, ErrMaskRequired
			}
			if !c.isServer && hdr.Masked {
				c.writeProtocolErrorAndClose(StatusProtocolError, "Client requires unmasked frames")
				return 0, nil, ErrMaskForbidden
			}
		}

		// Verify reserved bits (RSV1, RSV2, RSV3)
		if hdr.Rsv2 || hdr.Rsv3 {
			c.writeProtocolErrorAndClose(StatusProtocolError, "Reserved bits RSV2/RSV3 set")
			return 0, nil, ErrRSVSet
		}
		if hdr.Rsv1 {
			if !c.enableDeflate {
				c.writeProtocolErrorAndClose(StatusProtocolError, "RSV1 set without deflate extension")
				return 0, nil, ErrRSVSet
			}
			if hdr.Opcode.IsControl() || hdr.Opcode == OpContinuation {
				c.writeProtocolErrorAndClose(StatusProtocolError, "RSV1 not allowed on control or continuation frame")
				return 0, nil, ErrRSVSet
			}
		}

		// Read payload
		var framePayload []byte
		if hdr.Length > 0 {
			framePayload = make([]byte, hdr.Length)
			if _, err := io.ReadFull(c.rw, framePayload); err != nil {
				c.state.Store(uint32(StateClosed))
				return 0, nil, err
			}
			if hdr.Masked {
				MaskBytes(framePayload, hdr.MaskKey)
			}
		}

		// Dispatch Control Frames
		if hdr.Opcode.IsControl() {
			switch hdr.Opcode {
			case OpPing:
				if c.pingHandler != nil {
					if err := c.pingHandler(framePayload); err != nil {
						return 0, nil, err
					}
				}
				// Seamlessly continue reassembling message!
				continue

			case OpPong:
				if c.pongHandler != nil {
					if err := c.pongHandler(framePayload); err != nil {
						return 0, nil, err
					}
				}
				// Seamlessly continue reassembling message!
				continue

			case OpClose:
				c.seenClose = true
				code, reason, parseErr := ParseClosePayload(framePayload)
				if parseErr != nil {
					if errors.Is(parseErr, ErrInvalidPayload) {
						c.writeProtocolErrorAndClose(StatusInvalidPayloadData, "Invalid UTF-8 in Close frame")
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
					if code == StatusNoStatusRcvd {
						// Chromium test Code1005IsNotEchoed: echo empty close payload
						_ = c.writeControlFrame(OpClose, nil)
					} else {
						payload, _ := AppendClosePayload(code, reason)
						_ = c.writeControlFrame(OpClose, payload)
					}
					c.state.Store(uint32(StateClosed))
					c.closeTransport()
					return OpClose, nil, &CloseError{Code: code, Reason: reason}
				}

				// If we were already in StateClosing (we sent close earlier, now received echo),
				// or simultaneous close occurred:
				c.state.Store(uint32(StateClosed))
				c.closeTransport()
				return OpClose, nil, &CloseError{Code: code, Reason: reason}
			}
		}

		// Handle Data Frames (OpText, OpBinary, OpContinuation)
		if hdr.Opcode == OpContinuation {
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

		// Enforce ReadLimit
		totalLen := int64(len(c.fragmentBuf) + len(framePayload))
		if c.readLimit > 0 && totalLen > c.readLimit {
			c.writeProtocolErrorAndClose(StatusMessageTooBig, "Message exceeds read limit")
			return 0, nil, ErrMessageTooBig
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
			if msgType == OpText && !utf8.Valid(msgPayload) {
				c.writeProtocolErrorAndClose(StatusInvalidPayloadData, "Invalid UTF-8 in text message")
				return 0, nil, ErrInvalidPayload
			}

			return msgType, msgPayload, nil
		}
	}
}

// WriteMessage writes a complete unfragmented data message of type OpText or OpBinary.
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
// Useful for streaming or testing control frame interleaving.
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
			op = OpContinuation
		}
		if err := c.writeDataFrame(fin, false, op, frameData); err != nil {
			return err
		}
	}
	return nil
}

func (c *Conn) writeDataFrame(fin bool, rsv1 bool, op Opcode, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	st := c.State()
	if st == StateClosing || st == StateClosed {
		return ErrClosed
	}

	return c.writeFrameLocked(fin, rsv1, op, data)
}

// WriteControl writes a control frame (Ping, Pong, Close).
// It can be called concurrently with ReadMessage and other writes.
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
	return c.WriteControl(OpPing, data)
}

// WritePong sends a Pong control frame with payload data (len <= 125).
func (c *Conn) WritePong(data []byte) error {
	return c.WriteControl(OpPong, data)
}

// WriteClose initiates a graceful closing handshake by sending a Close frame.
// Validates status code (RFC 6455 §7.4.2) and reason string.
func (c *Conn) WriteClose(statusCode CloseCode, reason string) error {
	if !IsValidCloseCode(statusCode) {
		return ErrInvalidCloseCode
	}
	if len(reason) > 123 {
		return ErrCloseReasonTooLong
	}
	if !utf8.ValidString(reason) {
		return ErrInvalidPayload
	}

	payload, err := AppendClosePayload(statusCode, reason)
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// Chromium test: CloseOnlySentOnce
	if !c.state.CompareAndSwap(uint32(StateOpen), uint32(StateClosing)) {
		return ErrClosed
	}

	return c.writeFrameLocked(true, false, OpClose, payload)
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

func (c *Conn) writeProtocolErrorAndClose(code CloseCode, reason string) {
	c.state.Store(uint32(StateClosing))
	payload, _ := AppendClosePayload(code, reason)
	_ = c.writeControlFrame(OpClose, payload)
	c.state.Store(uint32(StateClosed))
	_ = c.closeTransport()
}

// writeFrameLocked performs the low-level frame serialization and transmission.
// Must be called with writeMu locked.
func (c *Conn) writeFrameLocked(fin bool, rsv1 bool, op Opcode, data []byte) error {
	h := Header{
		Fin:    fin,
		Rsv1:   rsv1,
		Opcode: op,
		Length: int64(len(data)),
	}

	if !c.isServer {
		h.Masked = true
		maskUint32 := c.maskRand.Uint32()
		binary.LittleEndian.PutUint32(h.MaskKey[:], maskUint32)
	}

	hdrBytes := AppendHeader(c.writeHdrBuf[:0], h)
	if _, err := c.rw.Write(hdrBytes); err != nil {
		return err
	}

	if len(data) == 0 {
		return nil
	}

	if !h.Masked {
		_, err := c.rw.Write(data)
		return err
	}

	// For client masked writes, mask without mutating caller's data
	var buf []byte
	bp := c.writeBufPool.Get().(*[]byte)
	if cap(*bp) >= len(data) {
		buf = (*bp)[:len(data)]
	} else {
		buf = make([]byte, len(data))
	}

	copy(buf, data)
	MaskBytes(buf, h.MaskKey)
	_, err := c.rw.Write(buf)

	if cap(buf) == cap(*bp) {
		c.writeBufPool.Put(bp)
	}
	return err
}
