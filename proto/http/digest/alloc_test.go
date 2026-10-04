// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package digest_test

import (
	"net/http"
	"testing"

	"github.com/lemon4ksan/mach/proto/http/digest"
)

func TestAlloc(t *testing.T) {
	tr := &digest.Transport{
		Username:  "Mufasa",
		Password:  "CircleOfLife",
		Transport: http.DefaultTransport,
	}

	allocs := testing.AllocsPerRun(1000, func() {
		_ = tr.Unwrap()
	})
	if allocs != 0 {
		t.Errorf("expected 0 allocs, got %v", allocs)
	}
}
