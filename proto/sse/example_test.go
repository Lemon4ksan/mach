// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sse_test

import (
	"fmt"
	"strings"

	"github.com/lemon4ksan/mach/proto/sse"
)

func ExampleReader_All() {
	raw := "event: notification\ndata: Server online\n\nevent: ping\ndata: keepalive\n\n"
	reader := sse.NewReader[sse.Event](strings.NewReader(raw))

	for ev, err := range reader.All() {
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		fmt.Printf("Event: %s, Data: %s\n", ev.Event, ev.Data)
	}

	// Output:
	// Event: notification, Data: Server online
	// Event: ping, Data: keepalive
}
