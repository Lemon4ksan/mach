// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package altsvc_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/http/altsvc"
)

func ExampleAll() {
	header := `h3=":443"; ma=2592000, h2="alt.example.com:8443"`

	for svc := range altsvc.All(header) {
		fmt.Printf("Proto: %s, Host: %s, Port: %d\n", svc.ProtocolID, svc.Host, svc.Port)
	}

	// Output:
	// Proto: h3, Host: , Port: 443
	// Proto: h2, Host: alt.example.com, Port: 8443
}
