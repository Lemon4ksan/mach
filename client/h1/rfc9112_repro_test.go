// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/client/h1"
	"github.com/lemon4ksan/mach/proto/http"
)

// TestRepro_ClientConnReusedAfterConnectionClose demonstrates that ClientConn
// does not prevent subsequent Do calls after receiving Connection: close (RFC 9112 §9.3).
func TestRepro_ClientConnReusedAfterConnectionClose(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		br := bufio.NewReader(c2)
		bw := bufio.NewWriter(c2)

		req := http.AcquireRequest()
		defer http.ReleaseRequest(req)
		_ = req.Read(br)

		resp := http.AcquireResponse()
		defer http.ReleaseResponse(resp)
		resp.SetStatusCode(200)
		resp.Header.Set("Connection", "close")
		resp.SetBodyString("closing")
		_ = resp.Write(bw)
		_ = bw.Flush()
	}()

	cc := h1.NewClientConn(c1)
	defer cc.Close()

	req1 := http.AcquireRequest()
	resp1 := http.AcquireResponse()
	defer http.ReleaseRequest(req1)
	defer http.ReleaseResponse(resp1)

	req1.Header.SetMethod("GET")
	req1.SetRequestURI("/close-me")
	req1.Header.SetHost("localhost")

	err := cc.Do(context.Background(), req1, resp1)
	require.NoError(t, err)

	// RFC 9112 §9.3: Connection received Connection: close; subsequent request on same conn must fail immediately
	req2 := http.AcquireRequest()
	resp2 := http.AcquireResponse()
	defer http.ReleaseRequest(req2)
	defer http.ReleaseResponse(resp2)

	req2.Header.SetMethod("GET")
	req2.SetRequestURI("/should-fail")
	req2.Header.SetHost("localhost")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = cc.Do(ctx, req2, resp2)
	if err == nil {
		t.Fatal("RFC 9112 §9.3 violation: ClientConn allowed Do() after Connection: close")
	}
}

// TestRepro_ClientConnReusedAfter101Upgrade demonstrates that ClientConn
// does not prevent subsequent Do calls after receiving 101 Switching Protocols (RFC 9112 §9.3).
func TestRepro_ClientConnReusedAfter101Upgrade(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		br := bufio.NewReader(c2)
		bw := bufio.NewWriter(c2)

		req := http.AcquireRequest()
		defer http.ReleaseRequest(req)
		_ = req.Read(br)

		resp := http.AcquireResponse()
		defer http.ReleaseResponse(resp)
		resp.SetStatusCode(101)
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "Upgrade")
		_ = resp.Write(bw)
		_ = bw.Flush()
	}()

	cc := h1.NewClientConn(c1)
	defer cc.Close()

	req1 := http.AcquireRequest()
	resp1 := http.AcquireResponse()
	defer http.ReleaseRequest(req1)
	defer http.ReleaseResponse(resp1)

	req1.Header.SetMethod("GET")
	req1.SetRequestURI("/ws")
	req1.Header.SetHost("localhost")
	req1.Header.Set("Upgrade", "websocket")
	req1.Header.Set("Connection", "Upgrade")

	err := cc.Do(context.Background(), req1, resp1)
	require.NoError(t, err)

	// RFC 9112 §9.3: Once upgraded, connection cannot be used for further HTTP requests
	req2 := http.AcquireRequest()
	resp2 := http.AcquireResponse()
	defer http.ReleaseRequest(req2)
	defer http.ReleaseResponse(resp2)

	req2.Header.SetMethod("GET")
	req2.SetRequestURI("/after-upgrade")
	req2.Header.SetHost("localhost")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = cc.Do(ctx, req2, resp2)
	if err == nil {
		t.Fatal("RFC 9112 §9.3 violation: ClientConn allowed HTTP Do() after 101 Switching Protocols")
	}
}
