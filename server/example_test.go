// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server_test

import (
	"fmt"
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
