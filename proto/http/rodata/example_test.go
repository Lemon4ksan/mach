// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rodata_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/http/rodata"
)

func ExampleInternKey() {
	staticKey := rodata.InternKey("content-type")
	fmt.Printf("%s\n", staticKey)

	// Output:
	// content-type
}
