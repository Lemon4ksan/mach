// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package contentdisposition_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http/contentdisposition"
)

func FuzzContentDisposition(f *testing.F) {
	f.Add(`attachment; filename="report.pdf"`)
	f.Add(`attachment; filename*=UTF-8''%e2%82%ac%20rates.pdf`)
	f.Add(`form-data; name="field1"; filename="photo.jpg"`)
	f.Add(`inline`)
	f.Add(``)
	f.Add(`attachment; filename="../../etc/passwd"`)

	f.Fuzz(func(t *testing.T, s string) {
		cd := contentdisposition.ParseContentDisposition(s)
		_ = contentdisposition.FileName(s)
		_ = contentdisposition.FormatContentDisposition(cd.Type, cd.Filename)
	})
}
