// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package digest_test

import (
	"fmt"
	"net/http"

	"github.com/lemon4ksan/mach/proto/http/digest"
)

func ExampleTransport() {
	tr := &digest.Transport{
		Username:  "user",
		Password:  "pass",
		Transport: http.DefaultTransport,
	}

	fmt.Printf("Digest user: %s, has transport: %v\n", tr.Username, tr.Unwrap() != nil)

	// Output:
	// Digest user: user, has transport: true
}
