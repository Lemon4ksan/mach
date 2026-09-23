// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"errors"
	"io"
	"net"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

func (sc *ServerConn) readLoop() error {
	for {
		if sc.isClosed.Load() {
			return sc.closeErr
		}

		fr, err := coreh2.ReadFrameFrom(sc.br)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil
			}

			return err
		}

		switch fr.Type() {
		case coreh2.FrameSettings:
			if err := sc.handleSettings(fr); err != nil {
				coreh2.ReleaseFrameHeader(fr)
				return err
			}

		case coreh2.FramePing:
			if err := sc.handlePing(fr); err != nil {
				coreh2.ReleaseFrameHeader(fr)
				return err
			}

		case coreh2.FrameHeaders:
			if err := sc.handleHeaders(fr); err != nil {
				coreh2.ReleaseFrameHeader(fr)
				return err
			}

		case coreh2.FrameContinuation:
			if err := sc.handleContinuation(fr); err != nil {
				coreh2.ReleaseFrameHeader(fr)
				return err
			}

		case coreh2.FrameData:
			if err := sc.handleData(fr); err != nil {
				coreh2.ReleaseFrameHeader(fr)
				return err
			}

		case coreh2.FrameWindowUpdate:
			if err := sc.handleWindowUpdate(fr); err != nil {
				coreh2.ReleaseFrameHeader(fr)
				return err
			}

		case coreh2.FrameResetStream:
			sc.handleResetStream(fr)

		case coreh2.FrameGoAway:
			coreh2.ReleaseFrameHeader(fr)
			return nil

		default:
			coreh2.ReleaseFrameHeader(fr)
		}
	}
}

func (sc *ServerConn) sendSettings(st *coreh2.Settings, ack bool) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	fr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(fr)

	stFrame := coreh2.AcquireFrame(coreh2.FrameSettings).(*coreh2.Settings)
	if ack {
		stFrame.SetAck(true)
		fr.SetFlags(coreh2.FlagAck)
	} else {
		st.CopyTo(stFrame)
	}

	fr.SetBody(stFrame)

	if _, err := fr.WriteTo(sc.bw); err != nil {
		return err
	}

	return sc.bw.Flush()
}

func (sc *ServerConn) handleSettings(fr *coreh2.FrameHeader) error {
	defer coreh2.ReleaseFrameHeader(fr)

	if fr.Flags().Has(coreh2.FlagAck) {
		// Client ACKed our settings (RFC 9113 §6.5.3)
		return nil
	}

	// Apply peer settings
	if body := fr.Body(); body != nil {
		if st, ok := body.(*coreh2.Settings); ok {
			if mfs := st.MaxFrameSize(); mfs >= 16384 && mfs <= 16777215 {
				sc.peerMaxFrameSize = mfs
			}

			if iws := st.MaxWindowSize(); iws > 0 && iws <= 0x7fffffff {
				sc.peerInitialWin = int32(iws) //nolint:gosec // bounds checked
			}
		}
	}

	// Send Settings ACK
	return sc.sendSettings(nil, true)
}

func (sc *ServerConn) handlePing(fr *coreh2.FrameHeader) error {
	defer coreh2.ReleaseFrameHeader(fr)

	if fr.Flags().Has(coreh2.FlagAck) {
		return nil
	}

	ping := fr.Body().(*coreh2.Ping)

	return sc.writePingAck(ping.Data())
}

func (sc *ServerConn) handleHeaders(fr *coreh2.FrameHeader) error {
	defer coreh2.ReleaseFrameHeader(fr)

	streamID := fr.Stream()
	// RFC 9113 §5.1.1: Client-initiated streams MUST use non-zero, odd-numbered stream identifiers
	if streamID == 0 || (streamID%2) == 0 {
		return coreh2.ProtocolError
	}

	hFrame := fr.Body().(*coreh2.Headers)
	endHeaders := fr.Flags().Has(coreh2.FlagEndHeaders)
	endStream := fr.Flags().Has(coreh2.FlagEndStream)

	st := sc.newServerStream(streamID, endHeaders, endStream)
	st.headerBlock.Write(hFrame.Headers())

	sc.streamsMu.Lock()
	sc.streams[streamID] = st
	sc.streamsMu.Unlock()

	if endHeaders {
		return sc.finishHeaderBlock(st)
	}

	return nil
}

func (sc *ServerConn) handleContinuation(fr *coreh2.FrameHeader) error {
	defer coreh2.ReleaseFrameHeader(fr)

	streamID := fr.Stream()

	sc.streamsMu.RLock()
	st, ok := sc.streams[streamID]
	sc.streamsMu.RUnlock()

	if !ok {
		return errors.New("h2: CONTINUATION on unknown stream (RFC 9113 §6.10)")
	}

	cFrame := fr.Body().(*coreh2.Continuation)
	st.headerBlock.Write(cFrame.Headers())

	if fr.Flags().Has(coreh2.FlagEndHeaders) {
		st.endHeaders = true
		return sc.finishHeaderBlock(st)
	}

	return nil
}

func (sc *ServerConn) handleData(fr *coreh2.FrameHeader) error {
	defer coreh2.ReleaseFrameHeader(fr)

	streamID := fr.Stream()

	sc.streamsMu.RLock()
	st, ok := sc.streams[streamID]
	sc.streamsMu.RUnlock()

	if !ok {
		return nil
	}

	dFrame := fr.Body().(*coreh2.Data)
	data := dFrame.Data()
	st.body.Write(data)

	// Flow control replenishment (RFC 9113 §6.9)
	if len(data) > 0 {
		sc.replenishReceiveWindow(streamID, len(data))
	}

	if fr.Flags().Has(coreh2.FlagEndStream) {
		st.endStream = true
		sc.startStream(st)
	}

	return nil
}

func (sc *ServerConn) handleResetStream(fr *coreh2.FrameHeader) {
	defer coreh2.ReleaseFrameHeader(fr)

	streamID := fr.Stream()

	sc.streamsMu.Lock()
	if st, ok := sc.streams[streamID]; ok {
		if st.cancel != nil {
			st.cancel()
		}

		delete(sc.streams, streamID)
	}

	sc.streamsMu.Unlock()
}
