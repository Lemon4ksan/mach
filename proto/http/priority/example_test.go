// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package priority_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/http/priority"
)

func ExampleParse() {
	p, err := priority.Parse("u=1, i")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Urgency: %d, Incremental: %v\n", p.Urgency, p.Incremental)

	// Output:
	// Urgency: 1, Incremental: true
}
