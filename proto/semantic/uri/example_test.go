// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package uri_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/semantic/uri"
)

func ExampleArgs() {
	args := uri.AcquireArgs()
	defer uri.ReleaseArgs(args)

	args.Set("name", "mach")
	args.Add("protocol", "http/2")
	args.Add("protocol", "http/3")

	fmt.Printf("name: %s\n", args.Peek("name"))
	fmt.Printf("protocols count: %d\n", len(args.PeekMulti("protocol")))

	// Output:
	// name: mach
	// protocols count: 2
}

func ExampleURI() {
	u := uri.AcquireURI()
	defer uri.ReleaseURI(u)

	u.Parse(nil, []byte("https://example.com/api/v1/metrics?format=json"))

	fmt.Printf("Host: %s\n", u.Host())
	fmt.Printf("Path: %s\n", u.Path())

	// Output:
	// Host: example.com
	// Path: /api/v1/metrics
}
