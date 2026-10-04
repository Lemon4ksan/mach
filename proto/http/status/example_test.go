// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/http/status"
)

func ExampleMessage() {
	msg := status.Message(status.OK)
	fmt.Printf("%d %s\n", status.OK, msg)

	// Output:
	// 200 OK
}
