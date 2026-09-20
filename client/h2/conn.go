// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/net/hpack"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/clock"
	"github.com/lemon4ksan/foundation/silicon/ringbuf"
	"github.com/lemon4ksan/foundation/silicon/sysnet"
	"github.com/lemon4ksan/foundation/sync/spinlock"
	"golang.org/x/sys/cpu"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

// maxConsecutiveControlFrames bounds consecutive control frames to prevent denial of service (RFC 9113 §10.5).
const maxConsecutiveControlFrames = 1000

// FrameWithHeaders defines an interface for HTTP/2 frames that carry raw HPACK-encoded header fragments (RFC 9113 §4.3).
type FrameWithHeaders interface {
	Headers() []byte
}

// ConnOpts defines connection configuration options.
type ConnOpts struct {
	PingInterval        time.Duration
	DisablePingChecking bool
	OnDisconnect        func(ctx context.Context, c *Conn)
	OnRTT               func(time.Duration)
	OnPushPromise       func(pushReq *h1.Request, pushResp *h1.Response)
	Settings            *coreh2.Settings
}

const (
	streamTableSize = 2048
	streamTableMask = streamTableSize - 1
	streamMaxProbes = 8
	streamNumShards = 16
	streamShardMask = streamNumShards - 1
)

type streamShard struct {
	mu       sync.RWMutex
	overflow map[uint32]*Context
}

// Conn manages a multiplexed HTTP/2 connection over a net.Conn socket (RFC 9113 §3, §4, §5 & §6).
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

// NewConn instantiates a new Conn wrapping socket c.
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

	nc.enc.DisableDynamicTable = false

	return nc
}

// SetOrderedHeaders configures custom HPACK header sequence.
func (c *Conn) SetOrderedHeaders(keys []string) {
	c.orderedKeys = keys
}

func (c *Conn) getStream(streamID uint32) *Context {
	baseIdx := int((streamID / 2) & streamTableMask)
	for i := range streamMaxProbes {
		idx := (baseIdx + i) & streamTableMask
		if ctx := c.reqStreams[idx].Load(); ctx != nil && ctx.StreamID == streamID {
			return ctx
		}
	}

	shardIdx := int((streamID / 2) & streamShardMask)
	shard := &c.reqShards[shardIdx]
	shard.mu.RLock()
	ctx := shard.overflow[streamID]
	shard.mu.RUnlock()

	return ctx
}

func (c *Conn) storeStream(ctx *Context) {
	baseIdx := int((ctx.StreamID / 2) & streamTableMask)
	for i := range streamMaxProbes {
		idx := (baseIdx + i) & streamTableMask
		if c.reqStreams[idx].CompareAndSwap(nil, ctx) {
			return
		}
	}

	shardIdx := int((ctx.StreamID / 2) & streamShardMask)
	shard := &c.reqShards[shardIdx]
	shard.mu.Lock()
	if shard.overflow == nil {
		shard.overflow = make(map[uint32]*Context, 8)
	}

	shard.overflow[ctx.StreamID] = ctx
	shard.mu.Unlock()
}

func (c *Conn) deleteStream(streamID uint32) {
	baseIdx := int((streamID / 2) & streamTableMask)
	for i := range streamMaxProbes {
		idx := (baseIdx + i) & streamTableMask
		if cur := c.reqStreams[idx].Load(); cur != nil && cur.StreamID == streamID {
			c.reqStreams[idx].Store(nil)
			return
		}
	}

	shardIdx := int((streamID / 2) & streamShardMask)
	shard := &c.reqShards[shardIdx]
	shard.mu.Lock()
	if shard.overflow != nil {
		delete(shard.overflow, streamID)
	}

	shard.mu.Unlock()
}

func (c *Conn) broadcastErrorToAllStreams(err error) {
	for i := range streamTableSize {
		if ctx := c.reqStreams[i].Load(); ctx != nil {
			select {
			case ctx.Err <- err:
			default:
			}
		}
	}

	for i := range streamNumShards {
		shard := &c.reqShards[i]
		shard.mu.RLock()

		for _, ctx := range shard.overflow {
			if ctx != nil {
				select {
				case ctx.Err <- err:
				default:
				}
			}
		}

		shard.mu.RUnlock()
	}
}

func (c *Conn) purgeStreamsAfterID(lastStreamID uint32, err error) {
	for i := range streamTableSize {
		if ctx := c.reqStreams[i].Load(); ctx != nil && ctx.StreamID > lastStreamID {
			c.reqStreams[i].Store(nil)

			select {
			case ctx.Err <- err:
			default:
			}
		}
	}

	for i := range streamNumShards {
		shard := &c.reqShards[i]
		shard.mu.Lock()

		for streamID, ctx := range shard.overflow {
			if ctx != nil && streamID > lastStreamID {
				delete(shard.overflow, streamID)

				select {
				case ctx.Err <- err:
				default:
				}
			}
		}

		shard.mu.Unlock()
	}
}

// CancelStream terminates an active HTTP/2 stream by transmitting an RST_STREAM frame.
func (c *Conn) CancelStream(ctx *Context) {
	if ctx == nil || ctx.StreamID == 0 {
		return
	}

	if ctx.State() == streamClosed {
		return
	}

	ctx.SetState(streamClosed)
	c.deleteStream(ctx.StreamID)
	c.openStreams.Add(-1)

	fr := coreh2.AcquireFrameHeader()
	fr.SetStream(ctx.StreamID)

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

// Close gracefully terminates the HTTP/2 connection.
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

// Handshake performs HTTP/2 connection initialization.
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

		if st.HeaderTableSize() <= coreh2.DefaultHeaderTableSize {
			c.enc.SetMaxTableSize(st.HeaderTableSize())
		}

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

// CanOpenStream reports whether the client can open new concurrent streams.
func (c *Conn) CanOpenStream() bool {
	if c.nextID.Load() >= (1<<31 - 1) {
		return false
	}

	return c.openStreams.Load() < int32(c.serverS.MaxStreams) //nolint:gosec
}

// Closed reports whether the connection has been closed.
func (c *Conn) Closed() bool {
	return c.closed.Load() == 1
}

// Write enqueues a request context for execution.
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

func (c *Conn) writeLoop() {
	var lastErr error

	defer func() { _ = c.Close() }()
	defer c.recoverWriteLoop(&lastErr)

	if c.pingInterval <= 0 {
		c.pingInterval = DefaultPingInterval
	}

	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()

	for {
		if stop, err := c.selectWriteEvent(ticker.C); stop {
			lastErr = err
			break
		}

		if !c.disableAcks && c.pingUnacks >= 5 {
			lastErr = coreh2.ErrTimeout
			break
		}
	}
}

func (c *Conn) selectWriteEvent(pingChan <-chan time.Time) (bool, error) {
	if fr := c.outRing.Pop(); fr != nil {
		c.writeMu.Lock()

		var batch [16]*coreh2.FrameHeader

		batch[0] = fr
		n := 1

		for n < 16 {
			next := c.outRing.Pop()
			if next == nil {
				break
			}

			batch[n] = next
			n++
		}

		var wErr error

		if n == 1 {
			_, wErr = fr.WriteTo(c.bw)
			if wErr == nil {
				wErr = c.bw.Flush()
			}

			coreh2.ReleaseFrameHeader(fr)
		} else {
			var bufs [][]byte

			for i := 0; i < n; i++ {
				_, _ = batch[i].WriteTo(c.bw)
				coreh2.ReleaseFrameHeader(batch[i])
			}

			if c.bw.Buffered() > 0 {
				writtenBuf := c.bw.AvailableBuffer()
				if len(writtenBuf) > 0 {
					bufs = [][]byte{writtenBuf}
					_, wErr = sysnet.WriteVectorBuffers(c.c, bufs)
				}

				if wErr == nil {
					wErr = c.bw.Flush()
				}
			}
		}

		c.writeMu.Unlock()

		if wErr != nil {
			return true, wErr
		}

		return false, nil
	}

	select {
	case ctx, ok := <-c.in:
		if !ok {
			return true, nil
		}

		if err := c.writeRequest(ctx); err != nil {
			ctx.Err <- err

			if errors.Is(err, coreh2.ErrNoAvailableStreams) {
				return false, nil
			}

			return true, err
		}

	case fr, ok := <-c.out:
		if !ok {
			return true, nil
		}

		defer coreh2.ReleaseFrameHeader(fr)

		c.writeMu.Lock()

		_, wErr := fr.WriteTo(c.bw)
		if wErr == nil {
			wErr = c.bw.Flush()
		}

		c.writeMu.Unlock()

		if wErr != nil {
			return true, wErr
		}

	case <-pingChan:
		if err := c.writePing(); err != nil {
			return true, err
		}
	}

	return false, nil
}

func (c *Conn) recoverWriteLoop(lastErr *error) {
	if r := recover(); r != nil && *lastErr == nil {
		if err, ok := r.(error); ok {
			*lastErr = err
		} else {
			*lastErr = fmt.Errorf("h2engine panic: %v", r)
		}
	}

	if *lastErr == nil {
		*lastErr = io.ErrUnexpectedEOF
	}

	c.broadcastErrorToAllStreams(*lastErr)
}

func (c *Conn) finish(r *Context, stream uint32, err error) {
	c.openStreams.Add(-1)

	select {
	case r.Err <- err:
	default:
	}

	c.deleteStream(stream)
	close(r.Err)
}

func (c *Conn) readLoop() {
	defer func() { _ = c.Close() }()

	for {
		fr, err := c.readNext()
		if err != nil {
			c.lastErr = err
			break
		}

		r := c.getStream(fr.Stream())

		// HPACK decoder state MUST be processed even if stream is dead or missing
		// per RFC 9113 §4.3 & §8.1 to preserve connection table synchronization.
		if r == nil || r.State() == streamClosed {
			if fr.Type() == coreh2.FrameHeaders || fr.Type() == coreh2.FrameContinuation {
				if h, ok := fr.Body().(FrameWithHeaders); ok {
					resp := h1.AcquireResponse()
					if _, decErr := c.readHeader(h.Headers(), resp); decErr != nil {
						c.lastErr = decErr

						h1.ReleaseResponse(resp)
						coreh2.ReleaseFrameHeader(fr)

						break
					}

					h1.ReleaseResponse(resp)
				}
			}

			coreh2.ReleaseFrameHeader(fr)

			continue
		}

		err = c.readStream(fr, r)
		if err == nil {
			if fr.Flags().Has(coreh2.FlagEndStream) {
				r.SetState(streamClosed)
				c.finish(r, fr.Stream(), nil)
			}
		} else {
			r.SetState(streamClosed)
			c.finish(r, fr.Stream(), err)

			if errors.Is(err, coreh2.FlowControlError) {
				coreh2.ReleaseFrameHeader(fr)
				break
			}
		}

		coreh2.ReleaseFrameHeader(fr)
	}
}

func isExpectContinue(req *h1.Request) bool {
	expect := req.Header.Peek("Expect")
	return bytes.EqualFold(expect, []byte("100-continue"))
}

func (c *Conn) waitExpectContinue(ctx *Context) {
	t := time.NewTimer(1 * time.Second)
	defer t.Stop()

	select {
	case <-t.C:
		// ExpectContinueTimeout elapsed; proceed to transmit body payload
	case <-ctx.Err:
		// Early response arrived (100 Continue or 4xx/5xx error rejection)
	}
}

func (c *Conn) writeRequest(ctx *Context) error {
	if !c.CanOpenStream() {
		return coreh2.ErrNoAvailableStreams
	}

	// RFC 9113 §5.1.1: Streams initiated by a client MUST use odd-numbered stream identifiers (1, 3, 5, ...).
	id := c.nextID.Add(2) - 2
	if id >= (1<<31 - 1) {
		// RFC 9113 §5.1.1: Stream identifiers must be 31-bit unsigned integers.
		// When reaching 2^31-1, stream identifiers cannot be reused and connection must close.
		_ = c.Close()
		return coreh2.ErrStreamClosed
	}

	req := ctx.Request
	hasBody := len(req.Body()) != 0

	ctx.StreamID = id
	ctx.SetState(streamOpen)

	maxWin := c.serverS.MaxWindowSize()
	initWin := int32(65535)

	if maxWin > 0 && maxWin <= 0x7fffffff {
		initWin = int32(maxWin)
	}

	ctx.streamWindow.Store(initWin)
	ctx.streamRxWindow.Store(6291456)

	fr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(fr)

	fr.SetStream(id)

	h := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	fr.SetBody(h)

	c.encodeRequestHeaders(h, req)

	h.SetPadding(false)
	h.SetEndStream(!hasBody)
	h.SetEndHeaders(true)

	if !hasBody {
		ctx.SetState(streamHalfClosed)
	}

	c.storeStream(ctx)

	c.writeMu.Lock()

	_, err := fr.WriteTo(c.bw)
	if err == nil && !hasBody {
		err = c.bw.Flush()
	}

	c.writeMu.Unlock()

	if err != nil {
		c.lastErr = err
		c.deleteStream(id)
		ctx.SetState(streamClosed)

		return err
	}

	if hasBody {
		if isExpectContinue(req) {
			c.waitExpectContinue(ctx)
		}

		if ctx.State() != streamClosed {
			err = c.writeData(fr, ctx, req.Body())
		}
	}

	if err == nil {
		c.openStreams.Add(1)
	} else {
		c.lastErr = err
		c.deleteStream(id)
		ctx.SetState(streamClosed)
	}

	return err
}

func (c *Conn) writeData(fh *coreh2.FrameHeader, ctx *Context, body []byte) error {
	data := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
	fh.SetBody(data)

	offset := 0
	bodyLen := len(body)

	for offset < bodyLen {
		if c.Closed() || ctx.State() == streamClosed {
			return coreh2.ErrStreamClosed
		}

		remaining := bodyLen - offset
		chunkSize := c.calculateChunkSize(ctx, remaining)

		if chunkSize <= 0 {
			c.waitForWindowUpdate(ctx, remaining)
			continue
		}

		end := offset + chunkSize
		data.SetEndStream(end == bodyLen)
		data.SetPadding(false)
		data.SetData(body[offset:end])

		c.writeMu.Lock()

		_, wErr := fh.WriteTo(c.bw)
		if wErr == nil {
			wErr = c.bw.Flush()
		}

		c.writeMu.Unlock()

		if wErr != nil {
			return wErr
		}

		c.serverWindow.Add(-int32(chunkSize))   //nolint:gosec // chunkSize bounded by H2 frame size <= 16MB
		ctx.streamWindow.Add(-int32(chunkSize)) //nolint:gosec // chunkSize bounded by H2 frame size <= 16MB

		offset = end
	}

	if bodyLen > 0 {
		ctx.State() // half-closed
	}

	return nil
}

// waitForWindowUpdate blocks the writing goroutine until flow control window capacity expands
// or until the stream/connection terminates.
//
// Preconditions:
//   - Evaluates calculateChunkSize inside c.windowMu lock to prevent lost wake-up signal races.
func (c *Conn) waitForWindowUpdate(ctx *Context, remaining int) {
	c.windowMu.Lock()
	defer c.windowMu.Unlock()

	if c.Closed() || ctx.State() == streamClosed {
		return
	}

	if c.calculateChunkSize(ctx, remaining) > 0 {
		return
	}

	c.windowCond.Wait()
}

func (c *Conn) broadcastWindowUpdate() {
	c.windowMu.Lock()
	c.windowCond.Broadcast()
	c.windowMu.Unlock()
}

func (c *Conn) calculateChunkSize(ctx *Context, remaining int) int {
	maxFrame := int(c.serverS.MaxFrameSize())
	if maxFrame <= 0 {
		maxFrame = 16384
	}

	serverWin := c.serverWindow.Load()
	streamWin := ctx.streamWindow.Load()

	win := min(int(streamWin), int(serverWin))

	if win <= 0 {
		return 0
	}

	chunk := min(remaining, win)

	return min(chunk, maxFrame)
}

func isForbiddenH2Header(key, value []byte) bool {
	if len(key) == 0 || key[0] == ':' {
		return true
	}

	keyStr := bytesconv.B2S(key)

	if bytesconv.EqualFoldASCII(keyStr, "te") {
		return !bytesconv.EqualFoldASCII(bytesconv.B2S(value), "trailers")
	}

	return bytesconv.EqualFoldASCII(keyStr, "connection") ||
		bytesconv.EqualFoldASCII(keyStr, "keep-alive") ||
		bytesconv.EqualFoldASCII(keyStr, "proxy-connection") ||
		bytesconv.EqualFoldASCII(keyStr, "transfer-encoding") ||
		bytesconv.EqualFoldASCII(keyStr, "upgrade") ||
		bytesconv.EqualFoldASCII(keyStr, "host")
}

func isForbiddenH2HeaderStr(key string) bool {
	if key == "" || key[0] == ':' {
		return true
	}

	return bytesconv.EqualFoldASCII(key, "connection") ||
		bytesconv.EqualFoldASCII(key, "keep-alive") ||
		bytesconv.EqualFoldASCII(key, "proxy-connection") ||
		bytesconv.EqualFoldASCII(key, "transfer-encoding") ||
		bytesconv.EqualFoldASCII(key, "upgrade") ||
		bytesconv.EqualFoldASCII(key, "host")
}

var defaultPseudoOrder = [4]string{":method", ":authority", ":scheme", ":path"}

func (c *Conn) encodeRequestHeaders(h *coreh2.Headers, req *h1.Request) {
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	enc := c.enc

	method := req.Header.Method()
	if len(method) == 0 {
		method = []byte("GET")
	}

	host := req.URI().Host()
	if idx := bytes.IndexByte(host, ':'); idx != -1 {
		if bytes.Equal(host[idx:], []byte(":443")) || bytes.Equal(host[idx:], []byte(":80")) {
			host = host[:idx]
		}
	}

	if len(host) == 0 {
		host = req.Header.Peek("Host")
	}

	scheme := req.URI().Scheme()
	if len(scheme) == 0 {
		scheme = []byte("https")
	}

	path := req.URI().RequestURI()
	if len(path) == 0 {
		path = []byte("/")
	}

	pseudoOrder := defaultPseudoOrder[:]
	if len(c.orderedKeys) > 0 {
		customPseudo := generic.Filter(c.orderedKeys, func(k string) bool {
			return len(k) > 0 && k[0] == ':'
		})

		if len(customPseudo) == 4 {
			pseudoOrder = customPseudo
		}
	}

	for _, pk := range pseudoOrder {
		switch pk {
		case ":method":
			hf.SetBytes(coreh2.StringMethod, method)
		case ":authority":
			hf.SetBytes(coreh2.StringAuthority, host)
		case ":scheme":
			hf.SetBytes(coreh2.StringScheme, scheme)
		case ":path":
			hf.SetBytes(coreh2.StringPath, path)
		}

		h.AppendHeaderField(enc, hf, true)
	}

	if len(c.orderedKeys) > 0 {
		c.appendOrderedHeaders(h, req, hf)
	} else {
		ua := req.Header.UserAgent()
		if len(ua) > 0 {
			hf.SetBytes(coreh2.StringUserAgent, ua)
			h.AppendHeaderField(enc, hf, true)
		}

		for k, v := range req.Header.All() {
			if isForbiddenH2Header(k, v) {
				continue
			}

			hf.SetBytes(coreh2.ToLowerCopy(k), v)
			h.AppendHeaderField(enc, hf, false)
		}
	}
}

func getFastHTTPCookieHeader(req *h1.Request) []byte {
	var sb strings.Builder

	for key, value := range req.Header.Cookies() {
		if sb.Len() > 0 {
			sb.WriteString("; ")
		}

		sb.Write(key)
		sb.WriteByte('=')
		sb.Write(value)
	}

	if sb.Len() > 0 {
		return bytesconv.S2B(sb.String())
	}

	if raw := req.Header.Peek("Cookie"); len(raw) > 0 {
		return raw
	}

	if raw := req.Header.Peek("cookie"); len(raw) > 0 {
		return raw
	}

	return nil
}

func peekHeaderCaseInsensitive(req *h1.Request, key string) []byte {
	for k, v := range req.Header.All() {
		if bytesconv.EqualFoldASCII(bytesconv.B2S(k), key) {
			return v
		}
	}

	return nil
}

func (c *Conn) appendOrderedHeaders(h *coreh2.Headers, req *h1.Request, hf *hpack.HeaderField) {
	var visitedBits uint64

	numOrdered := min(len(c.orderedKeys), 64)

	for i := range numOrdered {
		key := c.orderedKeys[i]
		if isForbiddenH2HeaderStr(key) {
			continue
		}

		var val []byte
		if bytesconv.EqualFoldASCII(key, "cookie") {
			val = getFastHTTPCookieHeader(req)
		} else {
			val = req.Header.Peek(key)
			if len(val) == 0 && len(key) > 0 {
				val = peekHeaderCaseInsensitive(req, key)
			}
		}

		if len(val) > 0 {
			hf.SetKey(key)
			hf.SetValueBytes(val)
			h.AppendHeaderField(c.enc, hf, false)

			visitedBits |= (1 << i)
		}
	}

	for k, v := range req.Header.All() {
		if isForbiddenH2Header(k, v) {
			continue
		}

		kStr := bytesconv.B2S(k)
		skip := false

		for i := range numOrdered {
			if (visitedBits&(1<<i)) != 0 && bytesconv.EqualFoldASCII(kStr, c.orderedKeys[i]) {
				skip = true
				break
			}
		}

		if skip {
			continue
		}

		hf.SetKeyBytes(bytesconv.AppendToLower(nil, k))
		hf.SetValueBytes(v)
		h.AppendHeaderField(c.enc, hf, false)
	}
}

func (c *Conn) readNext() (*coreh2.FrameHeader, error) {
	for {
		fr, err := coreh2.ReadFrameFromWithSize(c.br, coreh2.DefaultMaxLen)
		if err != nil {
			return nil, err
		}

		if fr.Type() == coreh2.FrameData || fr.Type() == coreh2.FrameHeaders {
			c.consecutiveControlFrames = 0
		} else {
			c.consecutiveControlFrames++
			if c.consecutiveControlFrames > maxConsecutiveControlFrames {
				coreh2.ReleaseFrameHeader(fr)
				return nil, coreh2.ErrControlFrameFlood
			}
		}

		if fr.Stream() != 0 {
			return fr, nil
		}

		if err := c.handleConnectionFrame(fr); err != nil {
			coreh2.ReleaseFrameHeader(fr)
			return nil, err
		}

		coreh2.ReleaseFrameHeader(fr)
	}
}

func (c *Conn) handleConnectionFrame(fr *coreh2.FrameHeader) error {
	switch fr.Type() {
	case coreh2.FrameSettings:
		if fr.Body() != nil {
			st := fr.Body().(*coreh2.Settings)
			if !st.IsAck() {
				c.handleSettings(st)
			}
		}

	case coreh2.FrameWindowUpdate:
		return c.handleWindowUpdate(fr)

	case coreh2.FramePing:
		if fr.Body() != nil {
			ping := fr.Body().(*coreh2.Ping)
			if !ping.IsAck() {
				c.handlePing(ping)
			} else {
				c.handlePingAck(ping)
			}
		}

	case coreh2.FrameGoAway:
		if fr.Body() != nil {
			ga := fr.Body().(*coreh2.GoAway)
			c.handleGoAway(ga)
		}
	}

	return nil
}

func (c *Conn) handlePingAck(ping *coreh2.Ping) {
	if c.pingUnacks > 0 {
		c.pingUnacks--
	}

	pingTime := time.Unix(0, int64(binary.BigEndian.Uint64(ping.Data()))) //nolint:gosec // timestamp int64 conversion
	if c.onDisconnect != nil && pingTime.IsZero() {
		return
	}

	rtt := time.Since(pingTime)
	if rtt > 0 && rtt < 10*time.Second && c.onDisconnect != nil {
		if c.windowCond != nil {
			c.recordRTT(rtt)
		}
	}
}

func (c *Conn) recordRTT(rtt time.Duration) {
	c.windowMu.Lock()
	defer c.windowMu.Unlock()

	if c.c != nil && c.onDisconnect != nil && c.onRTT != nil {
		c.onRTT(rtt)
	}
}

func (c *Conn) handleWindowUpdate(fr *coreh2.FrameHeader) error {
	wu := fr.Body().(*coreh2.WindowUpdate)

	inc := int32(wu.Increment()) //nolint:gosec
	if inc <= 0 {
		return coreh2.ErrInvalidWindowIncrement
	}

	streamID := fr.Stream()

	var err error
	if streamID == 0 {
		err = c.updateServerWindow(inc)
	} else {
		err = c.updateStreamWindow(streamID, inc)
	}

	if err == nil {
		c.broadcastWindowUpdate()
	}

	return err
}

func (c *Conn) updateServerWindow(inc int32) error {
	for {
		old := c.serverWindow.Load()
		if int64(old)+int64(inc) > int64(1<<31-1) {
			return coreh2.ErrWindowAboveLimits
		}

		if c.serverWindow.CompareAndSwap(old, old+inc) {
			return nil
		}
	}
}

func (c *Conn) updateStreamWindow(streamID uint32, inc int32) error {
	reqCtx := c.getStream(streamID)

	if reqCtx == nil {
		return nil
	}

	for {
		old := reqCtx.streamWindow.Load()
		if int64(old)+int64(inc) > int64(1<<31-1) {
			return coreh2.ErrWindowAboveLimits
		}

		if reqCtx.streamWindow.CompareAndSwap(old, old+inc) {
			return nil
		}
	}
}

// handleGoAway processes a received GOAWAY frame (RFC 9113 §6.8 & §8.7).
func (c *Conn) handleGoAway(ga *coreh2.GoAway) {
	lastStreamID := ga.Stream()
	// RFC 9113 §6.8 & §8.7: Streams with ID > lastStreamID were never processed and are safe for auto-retry.
	c.purgeStreamsAfterID(lastStreamID, coreh2.ErrGoAwayRetryable)

	_ = c.Close()
}

func (c *Conn) writePing() error {
	fr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(fr)

	ping := coreh2.AcquireFrame(coreh2.FramePing).(*coreh2.Ping)
	binary.BigEndian.PutUint64(ping.Data(), uint64(clock.CoarseNowNano())) //nolint:gosec // timestamp uint64 conversion
	fr.SetBody(ping)

	c.writeMu.Lock()

	_, err := fr.WriteTo(c.bw)
	if err == nil {
		err = c.bw.Flush()
		if err == nil {
			c.pingUnacks++
		}
	}

	c.writeMu.Unlock()

	return err
}

// handleSettings applies incoming peer parameters and immediately emits an acknowledgment (RFC 9113 §6.5.3).
func (c *Conn) handleSettings(st *coreh2.Settings) {
	st.CopyTo(&c.serverS)
	c.serverStreamWindow += c.serverS.MaxWindowSize()
	c.enc.SetMaxTableSize(st.HeaderTableSize())

	fr := coreh2.AcquireFrameHeader()
	stRes := coreh2.AcquireFrame(coreh2.FrameSettings).(*coreh2.Settings)
	stRes.SetAck(true)
	fr.SetBody(stRes)

	c.out <- fr
}

// handlePing replies to received PING frames with an identical payload and ACK bit set (RFC 9113 §6.7).
func (c *Conn) handlePing(ping *coreh2.Ping) {
	fr := coreh2.AcquireFrameHeader()

	ping.SetAck(true)
	fr.SetBody(ping)

	c.out <- fr
}

func (c *Conn) readStream(fr *coreh2.FrameHeader, reqCtx *Context) error {
	switch fr.Type() {
	case coreh2.FrameHeaders, coreh2.FrameContinuation:
		h := fr.Body().(FrameWithHeaders)
		if reqCtx.headersParsed {
			return c.readTrailers(h.Headers(), reqCtx)
		}

		statusCode, err := c.readHeader(h.Headers(), reqCtx.Response)
		if err == nil {
			if (statusCode < 100 || statusCode >= 200 || statusCode == 101) && fr.Flags().Has(coreh2.FlagEndHeaders) {
				reqCtx.headersParsed = true
			}
		}

		return err

	case coreh2.FramePushPromise:
		pp, ok := fr.Body().(*coreh2.PushPromise)
		if !ok {
			return coreh2.ErrUnknownFrameType
		}

		return c.handlePushPromise(pp)

	case coreh2.FrameData:
		data := fr.Body().(*coreh2.Data)
		dataLen := int32(fr.Len()) //nolint:gosec // H2 frame length is 24-bit max (<= 16MB)

		if data.Len() != 0 {
			reqCtx.Response.AppendBody(data.Data())

			reqCtx.streamRxWindow.Add(-dataLen)

			if reqCtx.streamRxWindow.Load() < 3145728 {
				inc := 6291456 - reqCtx.streamRxWindow.Load()
				reqCtx.streamRxWindow.Store(6291456)
				c.updateWindow(fr.Stream(), int(inc))
			}
		}

		c.currentWindow -= dataLen
		if c.currentWindow < c.maxWindow/2 {
			inc := c.maxWindow - c.currentWindow
			c.currentWindow = c.maxWindow
			c.updateWindow(0, int(inc))
		}

	case coreh2.FrameResetStream:
		if rst, ok := fr.Body().(*coreh2.RstStream); ok {
			return rst.Error()
		}

		return coreh2.ErrStreamClosed

	case coreh2.FrameGoAway:
		return coreh2.ErrGoAwayRetryable

	case coreh2.FrameWindowUpdate:
		return c.handleWindowUpdate(fr)
	}

	return nil
}

// handlePushPromise processes incoming server push promises (RFC 9113 §6.6 & §8.4).
func (c *Conn) handlePushPromise(pp *coreh2.PushPromise) error {
	if !c.current.Push() {
		return nil
	}

	promisedID := pp.PromisedStream()
	// RFC 9113 §5.1.1: Server-initiated streams MUST use even-numbered stream identifiers.
	if promisedID == 0 || (promisedID%2 != 0) {
		return coreh2.NewGoAwayError(coreh2.ProtocolError, "invalid promised stream id (RFC 9113 §5.1.1)")
	}

	pushReq := h1.AcquireRequest()
	if err := c.decodePushHeaders(pp.Headers(), pushReq); err != nil {
		h1.ReleaseRequest(pushReq)
		return err
	}

	method := string(pushReq.Header.Method())
	if method != "GET" && method != "HEAD" {
		h1.ReleaseRequest(pushReq)
		c.resetStream(promisedID, coreh2.StreamCanceled)
		return nil
	}

	pushResp := h1.AcquireResponse()
	errCh := make(chan error, 1)

	ctx := &Context{
		Request:  pushReq,
		Response: pushResp,
		Err:      errCh,
		StreamID: promisedID,
	}

	ctx.SetState(streamOpen)
	c.storeStream(ctx)

	go c.awaitPushedResponse(ctx, pushReq, pushResp)

	return nil
}

func (c *Conn) decodePushHeaders(headerBlock []byte, pushReq *h1.Request) error {
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	b := headerBlock
	for len(b) > 0 {
		var err error

		b, err = c.dec.Next(hf, b)
		if err != nil {
			return err
		}

		key := hf.Key()
		val := hf.Value()

		switch key {
		case ":method":
			pushReq.Header.SetMethod(val)
		case ":authority":
			pushReq.Header.SetHost(val)
		case ":scheme":
			pushReq.URI().SetScheme(val)
		case ":path":
			pushReq.SetRequestURI(val)
		default:
			if !hf.IsPseudo() {
				pushReq.Header.Add(key, val)
			}
		}
	}

	return nil
}

func (c *Conn) awaitPushedResponse(ctx *Context, pushReq *h1.Request, pushResp *h1.Response) {
	err := <-ctx.Err
	if err == nil && c.onPushPromise != nil {
		c.onPushPromise(pushReq, pushResp)
	}

	h1.ReleaseRequest(pushReq)
	h1.ReleaseResponse(pushResp)
}

func (c *Conn) resetStream(streamID uint32, code coreh2.ErrorCode) {
	fr := coreh2.AcquireFrameHeader()
	fr.SetStream(streamID)

	rst := coreh2.AcquireFrame(coreh2.FrameResetStream).(*coreh2.RstStream)
	rst.SetCode(code)
	fr.SetBody(rst)

	select {
	case c.out <- fr:
	default:
		coreh2.ReleaseFrameHeader(fr)
	}
}

func (c *Conn) readTrailers(b []byte, reqCtx *Context) error {
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	var (
		totalSize uint32
		maxList   = c.current.MaxHeaderListSize()
	)

	if maxList == 0 {
		maxList = defaultMaxHeaderListSize
	}

	if reqCtx.Trailers == nil {
		reqCtx.Trailers = make(map[string][]string)
	}

	for len(b) > 0 {
		var err error

		b, err = c.dec.Next(hf, b)
		if err != nil {
			return err
		}

		totalSize += hf.Size()
		if totalSize > maxList {
			return coreh2.ErrPayloadExceeds
		}

		if !hf.IsPseudo() && len(hf.KeyBytes()) > 0 {
			key := hf.Key()
			reqCtx.Trailers[key] = append(reqCtx.Trailers[key], hf.Value())
		}
	}

	return nil
}

func (c *Conn) updateWindow(streamID uint32, size int) {
	if size <= 0 || c.Closed() {
		return
	}

	fr := coreh2.AcquireFrameHeader()
	fr.SetStream(streamID)

	wu := coreh2.AcquireFrame(coreh2.FrameWindowUpdate).(*coreh2.WindowUpdate)
	wu.SetIncrement(size)
	fr.SetBody(wu)

	select {
	case c.out <- fr:
	case <-time.After(1 * time.Second):
		coreh2.ReleaseFrameHeader(fr)
	}
}

const defaultMaxHeaderListSize = 10 * 1024 * 1024

func (c *Conn) readHeader(b []byte, res *h1.Response) (int, error) {
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	var (
		err        error
		statusCode int
		totalSize  uint32
		maxList    = c.current.MaxHeaderListSize()
	)

	if maxList == 0 {
		maxList = defaultMaxHeaderListSize
	}

	informationalHeader := make(http.Header)

	for len(b) > 0 {
		b, err = c.dec.Next(hf, b)
		if err != nil {
			return 0, err
		}

		totalSize += hf.Size()
		if totalSize > maxList {
			return 0, coreh2.ErrPayloadExceeds
		}

		if hf.IsPseudo() && len(hf.KeyBytes()) > 1 && hf.KeyBytes()[1] == 's' {
			n, pErr := strconv.ParseInt(hf.Value(), 10, 64)
			if pErr != nil {
				return 0, pErr
			}

			statusCode = int(n)
			if statusCode < 100 || statusCode >= 200 || statusCode == 101 {
				res.SetStatusCode(statusCode)
			}

			continue
		}

		if statusCode >= 100 && statusCode < 200 && statusCode != 101 {
			informationalHeader.Add(hf.Key(), hf.Value())
			continue
		}

		if bytes.Equal(hf.KeyBytes(), coreh2.StringContentLength) {
			n, _ := strconv.Atoi(hf.Value())
			res.Header.SetContentLength(n)
		} else {
			res.Header.AddBytesKV(hf.KeyBytes(), hf.ValueBytes())
		}
	}

	if statusCode == 103 {
		trace := httptrace.ContextClientTrace(context.Background())
		if trace != nil && trace.Got1xxResponse != nil {
			_ = trace.Got1xxResponse(statusCode, textproto.MIMEHeader(informationalHeader))
		}
	}

	return statusCode, nil
}

// Dialer establishes HTTP/2 TLS connections using custom network dialers.
type Dialer struct {
	Addr           string
	TLSConfig      *tls.Config
	PingInterval   time.Duration
	NetDial        func(addr string) (net.Conn, error)
	RawDial        func(addr string) (net.Conn, error)
	RawDialContext func(ctx context.Context, addr string) (net.Conn, error)
}

// Dial creates and performs an HTTP/2 handshake on a new connection.
func (d *Dialer) Dial(opts ConnOpts) (*Conn, error) {
	return d.DialContext(context.Background(), opts)
}

// DialContext creates and performs an HTTP/2 handshake using the provided context.
func (d *Dialer) DialContext(ctx context.Context, opts ConnOpts) (*Conn, error) {
	c, err := d.tryDial(ctx)
	if err != nil {
		return nil, err
	}

	nc := NewConn(c, opts)
	err = nc.Handshake()

	return nc, err
}

func (d *Dialer) tryDial(ctx context.Context) (net.Conn, error) {
	if d.RawDialContext != nil {
		return d.RawDialContext(ctx, d.Addr)
	}

	if d.RawDial != nil {
		return d.RawDial(d.Addr)
	}

	if d.TLSConfig == nil {
		d.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS13,
		}
	}

	if d.TLSConfig.ServerName == "" {
		host, _, err := net.SplitHostPort(d.Addr)
		if err != nil {
			host = d.Addr
		}

		d.TLSConfig.ServerName = host
	}

	d.TLSConfig.NextProtos = append(d.TLSConfig.NextProtos, "h2")

	var (
		c   net.Conn
		err error
	)

	if d.NetDial != nil {
		c, err = d.NetDial(d.Addr)
	} else {
		var dialer net.Dialer

		c, err = dialer.DialContext(ctx, "tcp", d.Addr)
	}

	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(c, d.TLSConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}

	if tlsConn.ConnectionState().NegotiatedProtocol != "h2" {
		_ = c.Close()
		return nil, coreh2.ErrServerSupport
	}

	return tlsConn, nil
}

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
