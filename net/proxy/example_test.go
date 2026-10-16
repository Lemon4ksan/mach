// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy_test

import (
	"context"
	"fmt"
	"time"

	"github.com/lemon4ksan/mach/net/proxy"
)

func ExampleFromEnvironment() {
	// Creates a dialer based on HTTP_PROXY, HTTPS_PROXY, and NO_PROXY environment variables.
	dialer := proxy.FromEnvironment()
	if dialer != nil {
		fmt.Println("Dialer configured from environment")
	} else {
		fmt.Println("Dialer is nil")
	}

	// Output:
	// Dialer configured from environment
}

// ExampleSOCKS5 demonstrates configuring and dialing through a SOCKS5 proxy
// with username/password authentication (RFC 1929) and context deadline control.
func ExampleSOCKS5() {
	// 1. Configure authentication credentials for the SOCKS5 proxy gateway
	auth := &proxy.Auth{
		User:     "production_service",
		Password: "vault_secret_token",
	}

	// 2. Initialize the high-performance SOCKS5 proxy dialer
	dialer, err := proxy.SOCKS5("tcp", "10.0.0.1:1080", auth, proxy.Direct)
	if err != nil {
		fmt.Printf("dialer error: %v\n", err)
		return
	}

	// 3. Prepare context with timeout for connection and handshake
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// SOCKS5Dialer implements proxy.ContextDialer for cancellation and deadline propagation
	if cd, ok := dialer.(proxy.ContextDialer); ok {
		_ = cd
		_ = ctx
		// In production:
		// conn, err := cd.DialContext(ctx, "tcp", "internal-db.local:5432")
		// if err != nil { ... }
		// defer conn.Close()
		fmt.Println("SOCKS5 dialer configured with authentication and context support")
	}

	// Output:
	// SOCKS5 dialer configured with authentication and context support
}
