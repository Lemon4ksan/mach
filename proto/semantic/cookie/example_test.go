// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cookie_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/semantic/cookie"
)

func ExampleCookie() {
	c := cookie.AcquireCookie()
	defer cookie.ReleaseCookie(c)

	c.SetKey("token")
	c.SetValue("jwt-abc-123")
	c.SetPath("/")
	c.SetSecure(true)
	c.SetHTTPOnly(true)

	fmt.Printf("Key: %s\n", c.Key())
	fmt.Printf("Value: %s\n", c.Value())
	fmt.Printf("HTTPOnly: %t\n", c.HTTPOnly())

	// Output:
	// Key: token
	// Value: jwt-abc-123
	// HTTPOnly: true
}
