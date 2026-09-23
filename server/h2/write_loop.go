// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

func (sc *ServerConn) writeResponse(streamID uint32, res *ServerResponse) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	// 1. Serialize HEADERS Frame (RFC 9113 §6.2)
	hdrFr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(hdrFr)

	sc.encMu.Lock()
	hFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	coreh2.SerializeResponseHeaders(hFrame, sc.hpackEnc, res.StatusCode, res.Headers, len(res.Body))
	sc.encMu.Unlock()

	hdrFr.SetStream(streamID)
	hdrFr.SetFlags(coreh2.FlagEndHeaders)

	if len(res.Body) == 0 {
		hdrFr.SetFlags(coreh2.FlagEndHeaders | coreh2.FlagEndStream)
	}

	hdrFr.SetBody(hFrame)

	if _, err := hdrFr.WriteTo(sc.bw); err != nil {
		return err
	}

	// 2. Serialize DATA Frames (RFC 9113 §6.1)
	body := res.Body

	maxChunk := int(sc.peerMaxFrameSize)
	if maxChunk <= 0 {
		maxChunk = coreh2.DefaultMaxLen
	}

	for len(body) > 0 {
		chunkSize := min(len(body), maxChunk)
		chunk := body[:chunkSize]
		body = body[chunkSize:]

		dataFr := coreh2.AcquireFrameHeader()
		dFrame := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
		dFrame.SetData(chunk)

		dataFr.SetStream(streamID)

		if len(body) == 0 {
			dataFr.SetFlags(coreh2.FlagEndStream)
		}

		dataFr.SetBody(dFrame)

		_, err := dataFr.WriteTo(sc.bw)
		coreh2.ReleaseFrameHeader(dataFr)

		if err != nil {
			return err
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
