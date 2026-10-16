// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package weblink_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/weblink"
)

func ExampleAll() {
	header := `</items?page=2>; rel="next", </items?page=10>; rel="last"`

	for link, err := range weblink.All(header) {
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		fmt.Printf("Target: %s, Rel: %s\n", link.Target, link.Rel)
	}

	// Output:
	// Target: /items?page=2, Rel: next
	// Target: /items?page=10, Rel: last
}
