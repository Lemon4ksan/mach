// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client_test

import (
	"fmt"
	"time"

	"github.com/lemon4ksan/mach"
	"github.com/lemon4ksan/mach/client"
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
