// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/codec/qpack"
)

func ExampleEncoder() {
	enc := qpack.NewEncoderWithDefaults(nil)
	headers := []qpack.HeaderField{
		{Name: ":method", Value: "GET"},
	}
	var sent uint64
	out := enc.EncodeHeaderList(1, headers, &sent)
	fmt.Printf("Encoded %d headers\n", len(headers))
	_ = out

	// Output:
	// Encoded 1 headers
}
