// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"github.com/lemon4ksan/foundation/net/hpack"

	"bufio"
	"bytes"
	"net"
	"testing"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

func runMockH2Server(
	t *testing.T,
	serverConn net.Conn,
	handler func(req *h1.Request, resp *h1.Response, rawHeaders []string),
) {
	br := bufio.NewReader(serverConn)
	bw := bufio.NewWriter(serverConn)

	if !coreh2.ReadPreface(br) {
		t.Errorf("server: invalid client preface")
		return
	}

	serverSettings := &coreh2.Settings{}
	serverSettings.SetMaxWindowSize(1 << 20)

	if err := coreh2.PerformHandshake(false, bw, serverSettings, 1<<20); err != nil {
		t.Errorf("server: handshake failed: %v", err)
		return
	}

	frClientSettings, err := coreh2.ReadFrameFrom(br)
	if err != nil {
		t.Errorf("server: read client settings failed: %v", err)
		return
	}

	coreh2.ReleaseFrameHeader(frClientSettings)

	ackFrame := coreh2.AcquireFrameHeader()

	stRes := coreh2.AcquireFrame(coreh2.FrameSettings).(*coreh2.Settings)
	stRes.SetAck(true)
	ackFrame.SetBody(stRes)

	if _, err := ackFrame.WriteTo(bw); err != nil {
		t.Errorf("server: write settings ack failed: %v", err)
		return
	}

	_ = bw.Flush()

	coreh2.ReleaseFrameHeader(ackFrame)

	dec := hpack.AcquireHPACK()
	enc := hpack.AcquireHPACK()

	defer hpack.ReleaseHPACK(dec)
	defer hpack.ReleaseHPACK(enc)

	for {
		fr, err := coreh2.ReadFrameFrom(br)
		if err != nil {
			return
		}

		if fr.Type() == coreh2.FrameHeaders {
			// Save the request stream ID before releasing the frame header object back to pool
			streamID := fr.Stream()

			hFrame := fr.Body().(FrameWithHeaders)
			req := &h1.Request{}
			resp := &h1.Response{}

			hf := hpack.AcquireHeaderField()
			b := hFrame.Headers()

			var rawHeaders []string

			for len(b) > 0 {
				var nErr error

				b, nErr = dec.Next(hf, b)
				if nErr != nil {
					t.Logf("runMockH2Server: dec.Next error: %v, remaining: %x", nErr, b)
					break
				}

				if !hf.IsPseudo() {
					rawHeaders = append(rawHeaders, hf.Key())
				}

				switch {
				case !hf.IsPseudo():
					req.Header.AddBytesKV(hf.KeyBytes(), hf.ValueBytes())
				case bytes.Equal(hf.KeyBytes(), coreh2.StringMethod):
					req.Header.SetMethodBytes(hf.ValueBytes())
				case bytes.Equal(hf.KeyBytes(), coreh2.StringPath):
					req.Header.SetRequestURIBytes(hf.ValueBytes())
				}

				hf.Reset()
			}

			hpack.ReleaseHeaderField(hf)
			coreh2.ReleaseFrameHeader(fr)

			handler(req, resp, rawHeaders)

			respFH := coreh2.AcquireFrameHeader()
			respFH.SetStream(streamID)

			respH := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
			respH.SetEndHeaders(true)
			respH.SetEndStream(len(resp.Body()) == 0)

			respFH.SetBody(respH)

			coreh2.FasthttpResponseHeaders(respH, enc, resp)

			if _, err := respFH.WriteTo(bw); err != nil {
				return
			}

			_ = bw.Flush()

			coreh2.ReleaseFrameHeader(respFH)

			if len(resp.Body()) > 0 {
				dataFH := coreh2.AcquireFrameHeader()

				dataFH.SetStream(streamID)

				dataF := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
				dataF.SetEndStream(true)
				dataF.SetData(resp.Body())

				dataFH.SetBody(dataF)

				if _, err := dataFH.WriteTo(bw); err != nil {
					return
				}

				_ = bw.Flush()

				coreh2.ReleaseFrameHeader(dataFH)
			}

			continue
		}

		coreh2.ReleaseFrameHeader(fr)
	}
}
