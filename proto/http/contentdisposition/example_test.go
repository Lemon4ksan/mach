// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package contentdisposition_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/http/contentdisposition"
)

func ExampleParseContentDisposition() {
	header := `attachment; filename="data.csv"`

	cd := contentdisposition.ParseContentDisposition(header)
	fmt.Printf("Type: %s, Filename: %s\n", cd.Type, cd.Filename)

	// Output:
	// Type: attachment, Filename: data.csv
}
