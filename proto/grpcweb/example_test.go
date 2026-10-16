// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpcweb_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/grpcweb"
)

func ExampleFrame() {
	payload := []byte("gRPC request message")
	raw := grpcweb.AppendFrame(nil, 0x00, payload)

	frame := grpcweb.Frame(raw)
	if frame.IsValid() {
		fmt.Printf("Length: %d, Payload: %s, Compressed: %v\n", frame.Length(), frame.Payload(), frame.IsCompressed())
	}

	// Output:
	// Length: 20, Payload: gRPC request message, Compressed: false
}
