// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/lemon4ksan/mach/server"
)

func ExampleServer() {
	srv := server.New(
		server.WithHandlerFunc(func(ctx *server.Context) error {
			ctx.SetStatusCode(200)
			ctx.SetHeader("X-Unified-Server", "mach")
			ctx.SetBodyString("Hello from Mach Unified L7 Server!")
			return nil
		}),
		server.WithAltSvcPort(8443),
		server.WithAltSvcMaxAge(86400*time.Second),
		server.WithReadTimeout(10*time.Second),
		server.WithWriteTimeout(10*time.Second),
	)

	_ = srv
	fmt.Println("Unified server configured")
	// Output: Unified server configured
}

// ExampleServer_unified demonstrates starting a multi-protocol L7 server
// with ALPN H1/H2 transparent negotiation and automated Alt-Svc H3 announcement.
func ExampleServer_unified() {
	// 1. Configure the unified server with a universal handler and Alt-Svc H3 advertisement
	srv := server.New(
		server.WithHandlerFunc(func(ctx *server.Context) error {
			ctx.SetStatusCode(200)
			ctx.SetHeader("X-Custom", "unified-mach")
			ctx.SetBodyString("Hello from Mach Unified Server!")
			return nil
		}),
		server.WithAltSvcPort(8443),
		server.WithAltSvcMaxAge(86400*time.Second),
	)

	// 2. Start a TCP listener for HTTP/1.1 and HTTP/2
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Printf("listen error: %v\n", err)
		return
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() { _ = srv.Close() }()

	// 3. Perform a request to verify transparent handling and Alt-Svc announcement
	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		fmt.Printf("request error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Body: %s\n", string(body))
	fmt.Printf("Alt-Svc: %s\n", resp.Header.Get("Alt-Svc"))

	// Output:
	// Status: 200
	// Body: Hello from Mach Unified Server!
	// Alt-Svc: h3=":8443"
}
