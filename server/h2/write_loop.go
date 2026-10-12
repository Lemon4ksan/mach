// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"net/http"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

func (sc *ServerConn) writeHeaders(streamID uint32, statusCode int, headers http.Header, endStream bool) error {
	hdrFr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(hdrFr)

	sc.encMu.Lock()
	hFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	coreh2.SerializeResponseHeaders(hFrame, sc.hpackEnc, statusCode, headers, -1)
	sc.encMu.Unlock()

	hdrFr.SetStream(streamID)

	flags := coreh2.FlagEndHeaders
	if endStream {
		flags |= coreh2.FlagEndStream
	}

	hdrFr.SetFlags(flags)
	hdrFr.SetBody(hFrame)

	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	if _, err := hdrFr.WriteTo(sc.bw); err != nil {
		return err
	}

	return sc.bw.Flush()
}

func (sc *ServerConn) writeStreamData(streamID uint32, body []byte, endStream bool) (int, error) {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	maxChunk := int(sc.peerMaxFrameSize)
	if maxChunk <= 0 {
		maxChunk = coreh2.DefaultMaxLen
	}

	written := len(body)
	if len(body) == 0 {
		if endStream {
			dataFr := coreh2.AcquireFrameHeader()
			defer coreh2.ReleaseFrameHeader(dataFr)

			dFrame := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
			dFrame.SetData(nil)
			dataFr.SetStream(streamID)
			dataFr.SetFlags(coreh2.FlagEndStream)
			dataFr.SetBody(dFrame)

			if _, err := dataFr.WriteTo(sc.bw); err != nil {
				return 0, err
			}

			return 0, sc.bw.Flush()
		}

		return 0, nil
	}

	dataFr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(dataFr)

	dFrame := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)

	for len(body) > 0 {
		chunkSize := min(len(body), maxChunk)
		chunk := body[:chunkSize]
		body = body[chunkSize:]

		dFrame.SetData(chunk)
		dataFr.SetStream(streamID)

		if endStream && len(body) == 0 {
			dataFr.SetFlags(coreh2.FlagEndStream)
		} else {
			dataFr.SetFlags(0)
		}

		dataFr.SetBody(dFrame)

		if _, err := dataFr.WriteTo(sc.bw); err != nil {
			return 0, err
		}
	}

	return written, sc.bw.Flush()
}

func (sc *ServerConn) writeResponse(streamID uint32, res *ServerResponse) error {
	// 1. Serialize HEADERS Frame (RFC 9113 §6.2) outside writeMu to minimize contention
	hdrFr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(hdrFr)

	sc.encMu.Lock()
	hFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	coreh2.SerializeResponseHeaders(hFrame, sc.hpackEnc, res.StatusCode, res.Headers, len(res.Body))
	sc.encMu.Unlock()

	hdrFr.SetStream(streamID)

	flags := coreh2.FlagEndHeaders
	if len(res.Body) == 0 {
		flags |= coreh2.FlagEndStream
	}

	hdrFr.SetFlags(flags)
	hdrFr.SetBody(hFrame)

	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	if _, err := hdrFr.WriteTo(sc.bw); err != nil {
		return err
	}

	// 2. Serialize DATA Frames (RFC 9113 §6.1)
	body := res.Body
	if len(body) > 0 {
		maxChunk := int(sc.peerMaxFrameSize)
		if maxChunk <= 0 {
			maxChunk = coreh2.DefaultMaxLen
		}

		dataFr := coreh2.AcquireFrameHeader()
		defer coreh2.ReleaseFrameHeader(dataFr)

		dFrame := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)

		for len(body) > 0 {
			chunkSize := min(len(body), maxChunk)
			chunk := body[:chunkSize]
			body = body[chunkSize:]

			dFrame.SetData(chunk)
			dataFr.SetStream(streamID)

			if len(body) == 0 {
				dataFr.SetFlags(coreh2.FlagEndStream)
			} else {
				dataFr.SetFlags(0)
			}

			dataFr.SetBody(dFrame)

			if _, err := dataFr.WriteTo(sc.bw); err != nil {
				return err
			}
		}
	}

	return sc.bw.Flush()
}

func (sc *ServerConn) writePingAck(data []byte) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	ackFr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(ackFr)

	ackPing := coreh2.AcquireFrame(coreh2.FramePing).(*coreh2.Ping)
	ackPing.SetData(data)

	ackFr.SetFlags(coreh2.FlagAck)
	ackFr.SetBody(ackPing)

	if _, err := ackFr.WriteTo(sc.bw); err != nil {
		return err
	}

	return sc.bw.Flush()
}

func (sc *ServerConn) sendWindowUpdateFrame(streamID, inc uint32) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	wuFr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(wuFr)

	wuFr.SetStream(streamID)

	wu := coreh2.AcquireFrame(coreh2.FrameWindowUpdate).(*coreh2.WindowUpdate)
	wu.SetIncrement(int(inc))
	wuFr.SetBody(wu)

	if _, err := wuFr.WriteTo(sc.bw); err != nil {
		return err
	}

	return sc.bw.Flush()
}

func (sc *ServerConn) sendBatchedWindowUpdates(connInc uint32, st *serverStream, stInc uint32) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	if sc.Closed() {
		return nil
	}

	if connInc > 0 {
		wuFr := coreh2.AcquireFrameHeader()
		wu := coreh2.AcquireFrame(coreh2.FrameWindowUpdate).(*coreh2.WindowUpdate)
		wu.SetIncrement(int(connInc))
		wuFr.SetStream(0)
		wuFr.SetBody(wu)
		_, err := wuFr.WriteTo(sc.bw)
		coreh2.ReleaseFrameHeader(wuFr)

		if err != nil {
			return err
		}
	}

	if stInc > 0 && st != nil {
		wuFr := coreh2.AcquireFrameHeader()
		wu := coreh2.AcquireFrame(coreh2.FrameWindowUpdate).(*coreh2.WindowUpdate)
		wu.SetIncrement(int(stInc))
		wuFr.SetStream(st.id)
		wuFr.SetBody(wu)
		_, err := wuFr.WriteTo(sc.bw)
		coreh2.ReleaseFrameHeader(wuFr)

		if err != nil {
			return err
		}
	}

	return sc.bw.Flush()
}

// writeRstStream emits an RST_STREAM frame to terminate an individual stream with an error code (RFC 9113 §6.4 & §5.4.2).
func (sc *ServerConn) writeRstStream(streamID uint32, code coreh2.ErrorCode) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	rstFr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(rstFr)

	rstFr.SetStream(streamID)

	rst := coreh2.AcquireFrame(coreh2.FrameResetStream).(*coreh2.RstStream)
	rst.SetCode(code)
	rstFr.SetBody(rst)

	if _, err := rstFr.WriteTo(sc.bw); err != nil {
		return err
	}

	return sc.bw.Flush()
}

// writeGoAway emits a GOAWAY frame to signal connection-level shutdown (RFC 9113 §6.8).
func (sc *ServerConn) writeGoAway(lastStreamID uint32, code coreh2.ErrorCode) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	gaFr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(gaFr)

	ga := coreh2.AcquireFrame(coreh2.FrameGoAway).(*coreh2.GoAway)
	ga.SetStream(lastStreamID)
	ga.SetCode(code)
	gaFr.SetBody(ga)

	if _, err := gaFr.WriteTo(sc.bw); err != nil {
		return err
	}

	return sc.bw.Flush()
}

func (sc *ServerConn) closeWithGoAway(lastStreamID uint32, code coreh2.ErrorCode) {
	_ = sc.writeGoAway(lastStreamID, code)
	sc.closeErr = code
	sc.isClosed.Store(true)
}
