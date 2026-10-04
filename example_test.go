// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mach_test

import (
	"fmt"

	"github.com/lemon4ksan/mach"
)

// ExampleConn demonstrates the abstract protocol connection contract.
func ExampleConn() {
	var _ mach.Conn // Interface compliance check
	fmt.Println("mach protocol engine contract")

	// Output:
	// mach protocol engine contract
}
