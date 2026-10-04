// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/http/zerocopy"
)

func ExampleArgs() {
	args := zerocopy.AcquireArgs()
	defer zerocopy.ReleaseArgs(args)

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
	uri := zerocopy.AcquireURI()
	defer zerocopy.ReleaseURI(uri)

	uri.Parse(nil, []byte("https://example.com/api/v1/metrics?format=json"))

	fmt.Printf("Host: %s\n", uri.Host())
	fmt.Printf("Path: %s\n", uri.Path())

	// Output:
	// Host: example.com
	// Path: /api/v1/metrics
}
