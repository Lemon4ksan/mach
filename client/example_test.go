// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client_test

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/lemon4ksan/mach"
	"github.com/lemon4ksan/mach/client"
	machhttp "github.com/lemon4ksan/mach/proto/http"
)

func ExampleEngine() {
	engine := client.NewEngine(
		client.WithIdleTimeout(60*time.Second),
		client.WithMaxConnsPerHost(20),
	)
	defer func() { _ = engine.Close() }()

	var _ mach.Engine = engine
	fmt.Println("mach intelligent client engine")

	// Output:
	// mach intelligent client engine
}

// ExampleEngine_Do demonstrates executing a client request with automatic
// Alt-Svc discovery, transparent HTTP/3 upgrade tracking, and connection pooling.
func ExampleEngine_Do() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Printf("listen error: %v\n", err)
		return
	}
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
				resp := "HTTP/1.1 200 OK\r\n" +
					"Alt-Svc: h3=\":8443\"; ma=3600\r\n" +
					"Content-Length: 17\r\n\r\n" +
					"hello from origin"
				_, _ = c.Write([]byte(resp))
			}(conn)
		}
	}()

	engine := client.NewEngine(
		client.WithIdleTimeout(30 * time.Second),
		client.WithMaxConnsPerHost(10),
	)
	defer func() { _ = engine.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Initial request: client contacts origin and discovers Alt-Svc advertisement
	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("http://" + ln.Addr().String() + "/api/v1")

	if err := engine.Do(ctx, req, resp); err != nil {
		fmt.Printf("do error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Printf("Body: %s\n", string(resp.Body()))

	// 2. Verify that Alt-Svc header was automatically parsed and cached for H3 upgrade
	h3Target, ok := engine.AltSvcCache.GetH3(ln.Addr().String())
	fmt.Printf("Alt-Svc H3 Discovered: %t\n", ok)
	fmt.Printf("Alt-Svc Target: %s\n", h3Target)

	// Output:
	// Status: 200
	// Body: hello from origin
	// Alt-Svc H3 Discovered: true
	// Alt-Svc Target: 127.0.0.1:8443
}
