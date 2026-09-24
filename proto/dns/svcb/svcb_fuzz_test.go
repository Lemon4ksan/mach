// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package svcb_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/dns/svcb"
)

func FuzzSVCBWireParse(f *testing.F) {
	// Priority 1, TargetName "." (0x00), Param alpn="h2,h3"
	f.Add([]byte("\x00\x01\x00\x00\x01\x00\x06\x02h2\x02h3"))
	f.Add([]byte("\x00\x00\x07example\x03com\x00"))
	f.Add([]byte(""))
	f.Add([]byte("\x00\x01\x00\x00\x03\x00\x02\x01\xbb"))

	f.Fuzz(func(t *testing.T, data []byte) {
		rec, err := svcb.ParseRDATA(data)
		if err == nil {
			_ = rec.ALPN()
			_ = rec.IPv4Hints()
			_ = rec.IPv6Hints()
			_ = rec.ECHConfig()
			_ = rec.MandatoryKeys()
			encoded, err2 := rec.MarshalRDATA()
			if err2 == nil && len(encoded) > 0 {
				_, _ = svcb.ParseRDATA(encoded)
			}
		}
	})
}
