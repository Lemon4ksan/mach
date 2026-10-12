// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client_test

import (
	"crypto/tls"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach"
	"github.com/lemon4ksan/mach/client"
	machhttp "github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/proto/semantic/altsvc"
)

func TestEngine_InterfaceCompliance(t *testing.T) {
	t.Parallel()

	engine := client.NewEngine()
	defer func() { _ = engine.Close() }()

	var _ mach.Engine = engine
}

func TestEngine_Options(t *testing.T) {
	t.Parallel()

	tlsConf := &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	dialer := net.Dialer{Timeout: 5 * time.Second}
	cache := client.NewAltSvcCache()

	engine := client.NewEngine(
		client.WithTLSConfig(tlsConf),
		client.WithDialer(dialer),
		client.WithIdleTimeout(45*time.Second),
		client.WithMaxConnsPerHost(50),
		client.WithDisableAltSvc(true),
		client.WithAltSvcCache(cache),
	)
	defer func() { _ = engine.Close() }()

	assert.Equal(t, tlsConf, engine.TLSConfig)
	assert.Equal(t, 45*time.Second, engine.IdleTimeout)
	assert.Equal(t, 50, engine.MaxConnsPerHost)
	assert.True(t, engine.DisableAltSvc)
	assert.Equal(t, cache, engine.AltSvcCache)
	assert.Equal(t, 45*time.Second, engine.PoolManager.IdleTimeout)
	assert.Equal(t, 50, engine.PoolManager.MaxConnsPerHost)
}

func TestAltSvcCache_BasicOperations(t *testing.T) {
	t.Parallel()

	cache := client.NewAltSvcCache()

	// 1. Initial lookup is empty
	addr, ok := cache.GetH3("api.example.com:443")
	assert.False(t, ok)
	assert.Equal(t, "", addr)

	// 2. Set H3 service
	cache.Set("api.example.com:443", altsvc.Service{
		ProtocolID: "h3",
		Host:       "",
		Port:       443,
		MaxAge:     1 * time.Hour,
	})

	addr, ok = cache.GetH3("api.example.com:443")
	assert.True(t, ok)
	assert.Equal(t, "api.example.com:443", addr)

	// Case insensitivity & port handling
	addr, ok = cache.GetH3("API.EXAMPLE.COM")
	assert.True(t, ok)
	assert.Equal(t, "api.example.com:443", addr)

	// 3. MarkFailed backoff
	cache.MarkFailed("api.example.com:443", 50*time.Millisecond)
	_, ok = cache.GetH3("api.example.com:443")
	assert.False(t, ok) // temporarily failed

	time.Sleep(70 * time.Millisecond)
	addr, ok = cache.GetH3("api.example.com:443")
	assert.True(t, ok)
	assert.Equal(t, "api.example.com:443", addr)

	// 4. UpdateFromHeader with clear
	cache.UpdateFromHeader("api.example.com:443", []byte("clear"))
	_, ok = cache.GetH3("api.example.com:443")
	assert.False(t, ok)
}

func TestAltSvcCache_UpdateFromHeader(t *testing.T) {
	t.Parallel()

	cache := client.NewAltSvcCache()

	// Header advertising H3 on port 8443
	hdr := []byte(`h3=":8443"; ma=3600, h2="alt.cdn:443"; ma=600`)
	cache.UpdateFromHeader("origin.test:443", hdr)

	addr, ok := cache.GetH3("origin.test:443")
	require.True(t, ok)
	assert.Equal(t, "origin.test:8443", addr)

	// Update with custom host
	hdr2 := []byte(`h3="h3.quic.cdn.net:443"; ma=1200`)
	cache.UpdateFromHeader("origin.test:443", hdr2)

	addr, ok = cache.GetH3("origin.test:443")
	require.True(t, ok)
	assert.Equal(t, "h3.quic.cdn.net:443", addr)

	// ClearAll
	cache.ClearAll()
	_, ok = cache.GetH3("origin.test:443")
	assert.False(t, ok)
}

func TestAltSvcCache_Expiration(t *testing.T) {
	t.Parallel()

	cache := client.NewAltSvcCache()

	cache.Set("expire.test:443", altsvc.Service{
		ProtocolID: "h3",
		Host:       "",
		Port:       443,
		MaxAge:     15 * time.Millisecond,
	})

	addr, ok := cache.GetH3("expire.test:443")
	assert.True(t, ok)
	assert.Equal(t, "expire.test:443", addr)

	time.Sleep(30 * time.Millisecond)

	_, ok = cache.GetH3("expire.test:443")
	assert.False(t, ok) // Expired
}

func TestEngine_CloseClosesResources(t *testing.T) {
	t.Parallel()

	engine := client.NewEngine()
	engine.AltSvcCache.Set("host:443", altsvc.Service{
		ProtocolID: "h3",
		Port:       443,
		MaxAge:     time.Hour,
	})

	err := engine.Close()
	require.NoError(t, err)

	// Alt-Svc cache should be cleared
	_, ok := engine.AltSvcCache.GetH3("host:443")
	assert.False(t, ok)

	// Subsequent Dial should fail with closed error
	_, err = engine.Dial(t.Context(), "host:443")
	assert.Error(t, err)
}

func TestEngine_EXP1206_ReentrantCloseNoDeadlock(t *testing.T) {
	t.Parallel()

	engine := client.NewEngine()

	done := make(chan struct{})
	go func() {
		_ = engine.Close()
		_ = engine.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Engine.Close deadlocked (EXP-1206)")
	}
}

func TestEngine_EXP1208_ResponseBodyDuplication(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				_, _ = c.Read(buf)
				respStr := "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nfresh"
				_, _ = c.Write([]byte(respStr))
			}(conn)
		}
	}()

	engine := client.NewEngine()
	defer func() { _ = engine.Close() }()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	// Simulate dirty response with pre-existing body
	resp.AppendBody([]byte("PREEXISTING-DATA-"))

	req.Header.SetMethod("GET")
	req.SetRequestURI("http://" + ln.Addr().String() + "/test")

	err = engine.Do(t.Context(), req, resp)
	require.NoError(t, err)
	assert.Equal(t, "fresh", string(resp.Body()))
}

func TestEngine_EXP1209_StaleSocketEOFRetry(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				count := attempts.Add(1)
				if count == 1 {
					// First connection dies with EOF immediately
					return
				}
				buf := make([]byte, 1024)
				_, _ = c.Read(buf)
				respStr := "HTTP/1.1 200 OK\r\nContent-Length: 7\r\n\r\nsuccess"
				_, _ = c.Write([]byte(respStr))
			}(conn)
		}
	}()

	engine := client.NewEngine()
	defer func() { _ = engine.Close() }()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("http://" + ln.Addr().String() + "/retry-test")

	err = engine.Do(t.Context(), req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "success", string(resp.Body()))
	assert.Equal(t, int32(2), attempts.Load())
}

func TestEngine_DoResult(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				_, _ = c.Read(buf)
				respStr := "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nmonad"
				_, _ = c.Write([]byte(respStr))
			}(conn)
		}
	}()

	engine := client.NewEngine()
	defer func() { _ = engine.Close() }()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("http://" + ln.Addr().String() + "/monad-test")

	res := engine.DoResult(t.Context(), req, resp)
	assert.True(t, res.IsSuccess())
	r, err := res.Unwrap()
	require.NoError(t, err)
	assert.Equal(t, 200, r.StatusCode())
	assert.Equal(t, "monad", string(r.Body()))
}
