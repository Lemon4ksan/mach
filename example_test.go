// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mach_test

import (
	"fmt"

	"github.com/lemon4ksan/mach"
)

// Example provides a quick overview of mach's universal protocol-agnostic handler contract.
func Example() {
	// A universal HandlerFunc processes streams transparently across HTTP/1.1, HTTP/2, and HTTP/3.
	handler := mach.HandlerFunc(func(stream mach.Stream) error {
		// Read wire frames without heap allocations
		frame, err := stream.ReadFrame()
		if err != nil {
			return err
		}

		// Inspect protocol-agnostic frame types and zero-copy payloads
		switch frame.Type() {
		case mach.FrameHeaders:
			_ = frame.Payload()
		case mach.FrameData:
			_ = frame.Payload()
		}

		return nil
	})

	_ = handler
	fmt.Println("mach protocol engine ready")

	// Output:
	// mach protocol engine ready
}

// ExampleConn demonstrates the abstract protocol connection contract.
func ExampleConn() {
	var _ mach.Conn // Interface compliance check
	fmt.Println("mach protocol engine contract")

	// Output:
	// mach protocol engine contract
}
