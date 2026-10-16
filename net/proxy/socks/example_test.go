// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socks_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/net/proxy/socks"
)

func ExampleNewDialer() {
	dialer := socks.NewDialer("tcp", "127.0.0.1:1080")
	if dialer != nil {
		fmt.Println("SOCKS5 dialer created")
	}

	// Output:
	// SOCKS5 dialer created
}
