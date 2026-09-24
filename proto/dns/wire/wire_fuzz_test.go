// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/dns/wire"
)

func FuzzDNSWireMessage(f *testing.F) {
	// Sample DNS query for example.com IN A
	f.Add(
		[]byte("\x12\x34\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00\x07example\x03com\x00\x00\x01\x00\x01"),
		uint16(0x1234),
	)
	f.Add([]byte(""), uint16(0))
	f.Add(
		[]byte(
			"\x00\x00\x81\x80\x00\x01\x00\x01\x00\x00\x00\x00\x03www\x07example\x03com\x00\x00\x01\x00\x01\xc0\x0c\x00\x01\x00\x01\x00\x00\x00\x3c\x00\x04\x5d\xb8\xd8\x22",
		),
		uint16(0),
	)

	f.Fuzz(func(t *testing.T, data []byte, expectedID uint16) {
		_, _ = wire.ParseDNSResponse(data, expectedID)
		_, _ = wire.ParseDNSResponseRecords(data, expectedID)
		_, _ = wire.ExtractECHFromHTTPSResponse(data, expectedID)
	})
}

func FuzzPackDNSQuery(f *testing.F) {
	f.Add(uint16(1234), "example.com", wire.TypeA)
	f.Add(uint16(5678), "sub.domain.co.uk", wire.TypeHTTPS)
	f.Add(uint16(0), "", wire.TypeAAAA)

	f.Fuzz(func(t *testing.T, id uint16, domain string, qtype uint16) {
		packed, err := wire.PackDNSQuery(id, domain, qtype)
		if err == nil && len(packed) > 12 {
			_, _ = wire.ParseDNSResponse(packed, id)
		}
	})
}
