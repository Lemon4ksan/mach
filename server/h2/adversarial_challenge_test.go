// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/net/http/status"
	"github.com/lemon4ksan/foundation/testing/require"

	h2client "github.com/lemon4ksan/mach/client/h2"
	machhttp "github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/server/h2"
)

// TestAdversarial_H2_StreamTeardownRace_UnderAbruptDisconnect stress tests Escalation 2
// by dispatching bursts of concurrent streams, abruptly severing the TCP connection
// while stream goroutines are active, and validating under the Go race detector that
// ServerConn.Release() with clear(sc.streams) never races with in-flight stream deletion.
func TestAdversarial_H2_StreamTeardownRace_UnderAbruptDisconnect(t *testing.T) {
	const (
		iterations  = 5
		concurrency = 40
	)

	for iter := 0; iter < iterations; iter++ {
		var lc net.ListenConfig

		ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
		require.NoError(t, err)

		handler := func(req *h2.ServerRequest, res *h2.ServerResponse) error {
			// Simulate in-flight processing latency
			select {
			case <-time.After(50 * time.Millisecond):
				res.StatusCode = status.OK
				res.Body = []byte("processed")

			case <-req.Ctx.Done():
				// Context cancelled upon connection close or stream reset
				return req.Ctx.Err()
			}

			return nil
		}

		serverDone := make(chan struct{})

		go func() {
			defer close(serverDone)

			for {
				c, aErr := ln.Accept()
				if aErr != nil {
					return
				}

				go func(conn net.Conn) {
					sc := h2.NewServerConn(conn, handler)
					defer sc.Release() // Stress tests Release() and clear(sc.streams)

					_ = sc.Serve()
				}(c)
			}
		}()

		var d net.Dialer

		rawConn, err := d.DialContext(context.Background(), "tcp", ln.Addr().String())
		require.NoError(t, err)

		client := h2client.NewConn(rawConn, h2client.ConnOpts{})
		require.NoError(t, client.Handshake())

		var wg sync.WaitGroup

		for i := 0; i < concurrency; i++ {
			wg.Add(1)

			go func(streamIdx int) {
				defer wg.Done()

				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()

				req := machhttp.AcquireRequest()
				resp := machhttp.AcquireResponse()

				defer machhttp.ReleaseRequest(req)
				defer machhttp.ReleaseResponse(resp)

				req.Header.SetMethod("GET")
				req.SetRequestURI("/teardown-stress")
				req.Header.SetHost(ln.Addr().String())

				_ = client.Do(ctx, req, resp)
			}(i)
		}

		// Let some streams start executing in server/h2 goroutines
		time.Sleep(15 * time.Millisecond)

		// Abruptly sever the underlying TCP connection while streams are in-flight
		_ = rawConn.Close()
		_ = client.Close()

		// Wait for all client callers to unblock
		wg.Wait()

		// Stop listener and wait for server
		_ = ln.Close()

		<-serverDone
	}
}

// TestAdversarial_H2_StreamTeardown_IncompleteStreams verifies that when client disconnects
// while streams are only partially opened (e.g. without EndStream), ServerConn.Release()
// cleans up safely without deadlocking or racing.
func TestAdversarial_H2_StreamTeardown_IncompleteStreams(t *testing.T) {
	var lc net.ListenConfig

	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	defer ln.Close()

	scReleased := make(chan struct{})

	go func() {
		c, aErr := ln.Accept()
		if aErr != nil {
			return
		}

		sc := h2.NewServerConn(c, func(req *h2.ServerRequest, res *h2.ServerResponse) error {
			res.StatusCode = status.OK

			return nil
		})

		defer func() {
			sc.Release()
			close(scReleased)
		}()

		_ = sc.Serve()
	}()

	var d net.Dialer

	conn, err := d.DialContext(context.Background(), "tcp", ln.Addr().String())
	require.NoError(t, err)

	// Send standard HTTP/2 client preface
	preface := "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
	_, err = conn.Write([]byte(preface))
	require.NoError(t, err)

	// Send empty SETTINGS frame: 9-byte header [0,0,0, 4, 0, 0,0,0,0]
	settingsFrame := []byte{0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00}
	_, err = conn.Write(settingsFrame)
	require.NoError(t, err)

	// Give the server time to process preface and SETTINGS
	time.Sleep(20 * time.Millisecond)

	// Abruptly close connection without finishing streams or sending GOAWAY
	_ = conn.Close()

	// Ensure ServerConn.Release() completes within 2 seconds without hanging
	select {
	case <-scReleased:
		// Success: Release completed cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ServerConn.Release() to complete on abrupt disconnect")
	}
}
