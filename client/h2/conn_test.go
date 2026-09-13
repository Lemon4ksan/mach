// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	coreh2 "github.com/lemon4ksan/mach/core/h2"

	"bufio"
	"bytes"
	"context"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/client/h1"
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

	dec := coreh2.AcquireHPACK()
	enc := coreh2.AcquireHPACK()

	defer coreh2.ReleaseHPACK(dec)
	defer coreh2.ReleaseHPACK(enc)

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

			hf := coreh2.AcquireHeaderField()
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

			coreh2.ReleaseHeaderField(hf)
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

func TestClientServerEndToEnd(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		serverConn, err := ln.Accept()
		if err != nil {
			return
		}
		defer serverConn.Close()

		runMockH2Server(t, serverConn, func(req *h1.Request, resp *h1.Response, _ []string) {
			if string(req.Header.Method()) != "GET" {
				t.Errorf("server: method mismatch: got %s, want GET", req.Header.Method())
			}

			resp.SetStatusCode(200)
			resp.SetBodyString("h2engine success")
		})
	}()

	dialer := &Dialer{
		RawDialContext: func(ctx context.Context, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", ln.Addr().String())
		},
	}

	client := NewClient(dialer, ClientOpts{PingInterval: 5 * time.Second})

	req := h1.AcquireRequest()
	resp := h1.AcquireResponse()

	defer h1.ReleaseRequest(req)
	defer h1.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://example.com/test")

	if err := client.Do(context.Background(), req, resp); err != nil {
		t.Fatalf("client.Do failed: %v", err)
	}

	if resp.StatusCode() != 200 {
		t.Fatalf("expected status code 200, got %d", resp.StatusCode())
	}

	if string(resp.Body()) != "h2engine success" {
		t.Fatalf("body mismatch: got %q, want %q", resp.Body(), "h2engine success")
	}
}

func TestOrderedHeadersSequenceOnWire(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	orderedKeys := []string{"accept-language", "user-agent", "x-custom-a"}

	var (
		capturedHeaders []string
		mu              sync.Mutex
	)

	go func() {
		serverConn, err := ln.Accept()
		if err != nil {
			return
		}
		defer serverConn.Close()

		runMockH2Server(t, serverConn, func(_ *h1.Request, resp *h1.Response, rawHeaders []string) {
			mu.Lock()
			capturedHeaders = slices.Clone(rawHeaders)
			mu.Unlock()

			resp.SetStatusCode(200)
		})
	}()

	dialer := &Dialer{
		RawDialContext: func(ctx context.Context, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", ln.Addr().String())
		},
	}

	client := NewClient(dialer, ClientOpts{PingInterval: 5 * time.Second})
	client.SetOrderedHeaders(orderedKeys)

	req := h1.AcquireRequest()
	resp := h1.AcquireResponse()

	defer h1.ReleaseRequest(req)
	defer h1.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://example.com/test")

	req.Header.Set("x-custom-a", "val-a")
	req.Header.Set("user-agent", "aoni-agent")
	req.Header.Set("accept-language", "en-US")

	if err := client.Do(context.Background(), req, resp); err != nil {
		t.Fatalf("client.Do failed: %v", err)
	}

	mu.Lock()
	headers := slices.Clone(capturedHeaders)
	mu.Unlock()

	if len(headers) < 3 {
		t.Fatalf("expected at least 3 headers, got %d: %v", len(headers), headers)
	}

	if headers[0] != "accept-language" || headers[1] != "user-agent" ||
		headers[2] != "x-custom-a" {
		t.Fatalf("headers order sequence violated: got %v, want %v", headers, orderedKeys)
	}
}

