// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package overlay_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/h2/overlay"
)

func ExampleFrame() {
	// Raw 9-byte header + 5 bytes payload
	raw := []byte{0x00, 0x00, 0x05, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 'h', 'e', 'l', 'l', 'o'}

	frame := overlay.Frame(raw)
	fmt.Printf("StreamID: %d, Type: %d, Length: %d, Payload: %s\n",
		frame.StreamID(), frame.Type(), frame.Length(), frame.Payload())

	// Output:
	// StreamID: 1, Type: 0, Length: 5, Payload: hello
}
