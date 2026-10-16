// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/semantic/auth"
)

func ExampleParseBasic() {
	header := "Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ=="

	username, password, err := auth.ParseBasic(header)
	if err == nil {
		fmt.Printf("User: %s, Pass: %s\n", username, password)
	}

	// Output:
	// User: Aladdin, Pass: open sesame
}
