// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/headkit"
)

func ExampleDirectives() {
	header := "public, max-age=3600, must-revalidate"
	for k, v := range headkit.Directives(header) {
		if v != "" {
			fmt.Printf("%s: %s\n", k, v)
		} else {
			fmt.Printf("%s\n", k)
		}
	}

	// Output:
	// public
	// max-age: 3600
	// must-revalidate
}

func ExampleIsSensitive() {
	fmt.Println(headkit.IsSensitive("Authorization"))
	fmt.Println(headkit.IsSensitive("Content-Type"))

	// Output:
	// true
	// false
}
