// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etag_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/semantic/etag"
)

func ExampleWeakMatch() {
	tag1 := `"xyzzy"`
	tag2 := `W/"xyzzy"`

	fmt.Printf("Weak match: %v\n", etag.WeakMatch(tag1, tag2))
	fmt.Printf("Strong match: %v\n", etag.StrongMatch(tag1, tag2))

	// Output:
	// Weak match: true
	// Strong match: false
}
