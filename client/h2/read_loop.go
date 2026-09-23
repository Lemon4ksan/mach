// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"encoding/binary"
	"errors"
	"time"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

// maxConsecutiveControlFrames bounds consecutive control frames to prevent denial of service (RFC 9113 §10.5).
const maxConsecutiveControlFrames = 1000

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

// handleGoAway processes a received GOAWAY frame (RFC 9113 §6.8 & §8.7).
func (c *Conn) handleGoAway(ga *coreh2.GoAway) {
	lastStreamID := ga.Stream()
	// RFC 9113 §6.8 & §8.7: Streams with ID > lastStreamID were never processed and are safe for auto-retry.
	c.purgeStreamsAfterID(lastStreamID, coreh2.ErrGoAwayRetryable)

	_ = c.Close()
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
