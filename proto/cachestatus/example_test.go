// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cachestatus_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/cachestatus"
)

func ExampleAll() {
	header := `OriginCache; hit; ttl=120, EdgeCache; fwd=uri-miss`

	for entry, err := range cachestatus.All(header) {
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		fmt.Printf("Cache: %s, Hit: %v, Fwd: %q\n", entry.CacheName, entry.Hit, entry.Fwd)
	}

	// Output:
	// Cache: OriginCache, Hit: true, Fwd: ""
	// Cache: EdgeCache, Hit: false, Fwd: "uri-miss"
}
