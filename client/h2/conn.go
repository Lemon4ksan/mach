// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/silicon/ringbuf"
	"github.com/lemon4ksan/foundation/sync/spinlock"
	"golang.org/x/sys/cpu"

	"github.com/lemon4ksan/mach/hpack"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

// ConnOpts defines connection configuration options for client-side HTTP/2 sessions (RFC 9113 §6.5).
type ConnOpts struct {
	// PingInterval specifies the frequency of periodic PING keepalive probes (RFC 9113 §6.7).
	PingInterval time.Duration

	// DisablePingChecking disables connection termination when peer PING acks stall.
	DisablePingChecking bool

	// OnDisconnect is an optional callback invoked when the connection terminates.
	OnDisconnect func(ctx context.Context, c *Conn)

	// OnRTT is an optional callback reporting measured round-trip time from PING frames.
	OnRTT func(time.Duration)

	// OnPushPromise is an optional callback invoked upon receiving server push streams (RFC 9113 §8.4).
	OnPushPromise func(pushReq *h1.Request, pushResp *h1.Response)

	// Settings specifies initial local HTTP/2 SETTINGS parameters advertised to the server (RFC 9113 §6.5).
	Settings *coreh2.Settings
}

// Conn manages a multiplexed HTTP/2 client connection over an underlying net.Conn socket
// in compliance with RFC 9113 §3 (Starting HTTP/2), §4 (HTTP Frames), §5 (Streams and Multiplexing),
// and §6 (Frame Definitions).
//
// Concurrency:
// Conn is safe for concurrent use across multiple goroutines. Egress frames are serialized
// via an internal lock-free SPSC ring buffer and write mutex, while stream states are mapped
// using lock-free open addressing with partitioned overflow buckets.
//
// Silicon Invariants:
// Hot atomic counters are isolated on a dedicated 64-byte cacheline (_ cpu.CacheLinePad)
// to eliminate false sharing across CPU cores during high-concurrency request pipelining.
type Conn struct {
	c             net.Conn
	br            *bufio.Reader
	bw            *bufio.Writer
	enc           *hpack.HPACK
	dec           *hpack.HPACK
	onDisconnect  func(ctx context.Context, c *Conn)
	onRTT         func(time.Duration)
	onPushPromise func(pushReq *h1.Request, pushResp *h1.Response)
	lastErr       error
	orderedKeys   []string
	windowCond    *sync.Cond

	writeMu  sync.Mutex
	inMu     sync.Mutex
	windowMu spinlock.SpinLock

	// Hot atomic counters isolated on their own 64-byte cache lines
	serverWindow             atomic.Int32
	serverStreamWindow       uint32
	maxWindow                int32
	currentWindow            int32
	openStreams              atomic.Int32
	pingUnacks               int32
	consecutiveControlFrames int32
	nextID                   atomic.Uint32

	_ cpu.CacheLinePad

	current    coreh2.Settings
	serverS    coreh2.Settings
	reqStreams [streamTableSize]atomic.Pointer[Context]
	reqShards  [streamNumShards]streamShard

	in           chan *Context
	out          chan *coreh2.FrameHeader
	outRing      *ringbuf.SPSCRingBuffer[coreh2.FrameHeader]
	pingInterval time.Duration
	closed       atomic.Uint64
	inClosed     bool

	disableAcks bool
}

// NewConn instantiates a new HTTP/2 client connection wrapping socket c.
//
// Lifecycle:
// NewConn initializes internal SPSC ring buffers, flow control windows (initial send window
// set to 65,535 octets per RFC 9113 §5.2.1), and HPACK compression tables. Handshake() must
// be invoked prior to executing requests.
func NewConn(c net.Conn, opts ConnOpts) *Conn {
	nc := &Conn{
		c:   c,
		br:  bufio.NewReaderSize(c, 16384),
		bw:  bufio.NewWriterSize(c, 16384),
		enc: hpack.AcquireHPACK(),
		dec: hpack.AcquireHPACK(),

		maxWindow:     15663105,
		currentWindow: 15663105,
		in:            make(chan *Context, 128),
		out:           make(chan *coreh2.FrameHeader, 128),
		outRing:       ringbuf.NewSPSCRingBuffer[coreh2.FrameHeader](512),
		pingInterval:  opts.PingInterval,
		disableAcks:   opts.DisablePingChecking,
		onDisconnect:  opts.OnDisconnect,
		onRTT:         opts.OnRTT,
		onPushPromise: opts.OnPushPromise,
	}
	nc.nextID.Store(1)
	// RFC 9113 §5.2.1: The initial flow-control window is 65,535 octets for the overall connection.
	nc.serverWindow.Store(65535)

	nc.windowCond = sync.NewCond(&nc.windowMu)
	nc.current.Reset()
	nc.serverS.Reset() // Initialize server settings with default maxStreams = 100 (RFC 9113 §6.5.2)

	if opts.Settings != nil {
		opts.Settings.CopyTo(&nc.current)
	} else {
		nc.current.SetMaxWindowSize(6291456)
		nc.current.SetMaxFrameSize(coreh2.DefaultDataFrameSize)
		nc.current.SetPush(false)
	}

	if nc.current.HeaderTableSize() > 0 {
		nc.dec.SetMaxCapacity(nc.current.HeaderTableSize())
	}

	nc.enc.DisableDynamicTable = false

	return nc
}

// SetOrderedHeaders configures custom HPACK header emission order to preserve
// deterministic header sequence matching browser or peer signatures (RFC 9113 §8.2).
func (c *Conn) SetOrderedHeaders(keys []string) {
	c.orderedKeys = keys
}

// CancelStream terminates an active HTTP/2 stream by transmitting an RST_STREAM frame
// with error code CANCEL (0x08) per RFC 9113 §5.1 and §6.4.
func (c *Conn) CancelStream(ctx *Context) {
	if ctx == nil || ctx.StreamID.Load() == 0 {
		return
	}

	if ctx.State() == streamClosed {
		return
	}

	ctx.SetState(streamClosed)
	streamID := ctx.StreamID.Load()
	c.deleteStream(streamID)
	c.openStreams.Add(-1)

	fr := coreh2.AcquireFrameHeader()
	fr.SetStream(streamID)

	rst := coreh2.AcquireFrame(coreh2.FrameResetStream).(*coreh2.RstStream)
	rst.SetCode(coreh2.StreamCanceled)
	fr.SetBody(rst)

	if !c.outRing.Push(fr) {
		select {
		case c.out <- fr:
		default:
		}
	}

	c.broadcastWindowUpdate()
}

// Close gracefully terminates the HTTP/2 connection by transmitting a GOAWAY frame
// with error code NO_ERROR (0x00) per RFC 9113 §6.8 and closing the underlying network socket.
func (c *Conn) Close() error {
	if !c.closed.CompareAndSwap(0, 1) {
		return io.EOF
	}

	c.inMu.Lock()
	if !c.inClosed {
		c.inClosed = true
		close(c.in)
	}

	c.inMu.Unlock()

	fr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(fr)

	ga := coreh2.AcquireFrame(coreh2.FrameGoAway).(*coreh2.GoAway)
	ga.SetStream(0)
	ga.SetCode(coreh2.NoError)
	fr.SetBody(ga)

	c.writeMu.Lock()

	_, err := fr.WriteTo(c.bw)
	if err == nil {
		_ = c.bw.Flush()
	}

	c.writeMu.Unlock()

	_ = c.c.Close()
	c.broadcastWindowUpdate()
	c.broadcastErrorToAllStreams(coreh2.ErrStreamClosed)

	if c.onDisconnect != nil {
		c.onDisconnect(context.Background(), c)
	}

	return nil
}

// Handshake performs the HTTP/2 connection preface exchange and SETTINGS negotiation
// per RFC 9113 §3.4 (Starting HTTP/2 with Prior Knowledge) and §6.5 (SETTINGS Frame).
// It spawns background readLoop and writeLoop goroutines upon successful negotiation.
func (c *Conn) Handshake() error {
	_ = c.c.SetDeadline(time.Now().Add(10 * time.Second))
	defer func() { _ = c.c.SetDeadline(time.Time{}) }()

	c.writeMu.Lock()
	err := coreh2.PerformHandshake(true, c.bw, &c.current, c.maxWindow-65535)
	c.writeMu.Unlock()

	if err != nil {
		_ = c.c.Close()
		return err
	}

	fr, err := coreh2.ReadFrameFrom(c.br)
	if err != nil {
		_ = c.c.Close()
		return err
	}

	if fr.Type() != coreh2.FrameSettings {
		_ = c.c.Close()

		coreh2.ReleaseFrameHeader(fr)

		return fmt.Errorf("h2engine: expected SETTINGS frame, got %s", fr.Type())
	}

	st := fr.Body().(*coreh2.Settings)
	if !st.IsAck() {
		st.CopyTo(&c.serverS)
		c.serverStreamWindow += c.serverS.MaxWindowSize()

		c.enc.SetMaxTableSize(st.HeaderTableSize())

		c.sendSettingsAck()
	}

	coreh2.ReleaseFrameHeader(fr)

	go c.writeLoop()
	go c.readLoop()

	return nil
}

func (c *Conn) sendSettingsAck() {
	fr := coreh2.AcquireFrameHeader()
	stRes := coreh2.AcquireFrame(coreh2.FrameSettings).(*coreh2.Settings)
	stRes.SetAck(true)
	fr.SetBody(stRes)

	c.writeMu.Lock()
	if _, err := fr.WriteTo(c.bw); err == nil {
		_ = c.bw.Flush()
	}

	c.writeMu.Unlock()

	coreh2.ReleaseFrameHeader(fr)
}

// CanOpenStream reports whether the connection can open a new concurrent stream without
// exceeding peer-advertised SETTINGS_MAX_CONCURRENT_STREAMS (RFC 9113 §5.1.2) or exhausting
// the 31-bit stream identifier space (RFC 9113 §5.1.1).
func (c *Conn) CanOpenStream() bool {
	if c.nextID.Load() >= (1<<31 - 1) {
		return false
	}

	return c.openStreams.Load() < int32(c.serverS.MaxStreams) //nolint:gosec
}

// Closed reports whether the connection has been closed or experienced a fatal socket error.
func (c *Conn) Closed() bool {
	return c.closed.Load() == 1
}

// Write enqueues a request context into the connection's egress submission channel.
// Returns coreh2.ErrNoAvailableStreams if the channel is saturated or coreh2.ErrStreamClosed
// if the connection has terminated.
func (c *Conn) Write(r *Context) error {
	c.inMu.Lock()
	defer c.inMu.Unlock()

	if c.inClosed || c.closed.Load() == 1 {
		return coreh2.ErrStreamClosed
	}

	select {
	case c.in <- r:
		return nil
	default:
		return coreh2.ErrNoAvailableStreams
	}
}

// Do executes a single HTTP request/response exchange over a multiplexed HTTP/2 stream
// per RFC 9113 §8.1 (HTTP Message Exchanges).
//
// Lifecycle:
// Blocks until the peer delivers a complete response, an error occurs, or ctx is cancelled.
// If ctx is cancelled before completion, CancelStream is triggered to transmit RST_STREAM
// with CANCEL (RFC 9113 §6.4, §8.1).
func (c *Conn) Do(ctx context.Context, req *h1.Request, res *h1.Response) error {
	errCh := make(chan error, 1)
	reqCtx := &Context{
		Request:  req,
		Response: res,
		Err:      errCh,
	}

	if err := c.Write(reqCtx); err != nil {
		return coreh2.ErrGoAwayRetryable
	}

	select {
	case <-ctx.Done():
		c.CancelStream(reqCtx)
		return ctx.Err()

	case err := <-errCh:
		return err
	}
}
