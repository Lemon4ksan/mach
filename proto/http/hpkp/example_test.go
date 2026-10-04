// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hpkp_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/http/hpkp"
)

func ExampleParseHeader() {
	raw := `pin-sha256="cUPcTAZWKaASuYWhhneDttWpY3oBAkE3h2+soZS7sWs="; max-age=5184000; includeSubDomains`

	policy, err := hpkp.ParseHeader(raw)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("MaxAge: %v, Pins: %d, SubDomains: %v\n", policy.MaxAge, len(policy.Pins), policy.IncludeSubDomains)

	// Output:
	// MaxAge: 1440h0m0s, Pins: 1, SubDomains: true
}
