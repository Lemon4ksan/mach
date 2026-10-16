// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hpack_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/codec/hpack"
)

func ExampleHPACK() {
	hp := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(hp)

	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	hf.Set(":method", "GET")
	fmt.Println(hf.Key())

	// Output:
	// :method
}
