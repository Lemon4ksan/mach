// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hpkp_test

import (
	"testing"
	"time"

	"github.com/lemon4ksan/mach/proto/semantic/hpkp"
)

func TestAlloc(t *testing.T) {
	header := `pin-sha256="cUPcTAZWKaASuYWhhneDttWpY3oBAkE3h2+soZS7sWs="; max-age=5184000; includeSubDomains`

	t.Run("ParseHeader", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			policy, err := hpkp.ParseHeader(header)
			if err != nil || len(policy.Pins) == 0 {
				t.Fatalf("unexpected parse result: policy=%v, err=%v", policy, err)
			}
		})
		if allocs > 10 {
			t.Errorf("expected minimal allocs, got %v", allocs)
		}
	})

	t.Run("PinMatches", func(t *testing.T) {
		pin, err := hpkp.NewPinSHA256("cUPcTAZWKaASuYWhhneDttWpY3oBAkE3h2+soZS7sWs=")
		if err != nil {
			t.Fatal(err)
		}
		var hash [32]byte
		copy(hash[:], pin.RawBytes)

		allocs := testing.AllocsPerRun(1000, func() {
			if !pin.Matches32(hash) {
				t.Fatal("expected match")
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for Pin.Matches32, got %v", allocs)
		}
	})

	t.Run("PinAppendFormat", func(t *testing.T) {
		pin, err := hpkp.NewPinSHA256("cUPcTAZWKaASuYWhhneDttWpY3oBAkE3h2+soZS7sWs=")
		if err != nil {
			t.Fatal(err)
		}
		var buf [128]byte

		allocs := testing.AllocsPerRun(1000, func() {
			_ = pin.AppendFormat(buf[:0])
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for Pin.AppendFormat, got %v", allocs)
		}
	})

	t.Run("PolicyAppendFormat", func(t *testing.T) {
		pin, _ := hpkp.NewPinSHA256("cUPcTAZWKaASuYWhhneDttWpY3oBAkE3h2+soZS7sWs=")
		pol := hpkp.Policy{
			Pins:              []hpkp.Pin{pin},
			MaxAge:            5184000 * time.Second,
			IncludeSubDomains: true,
		}
		var buf [256]byte

		allocs := testing.AllocsPerRun(1000, func() {
			_ = pol.AppendFormat(buf[:0])
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for Policy.AppendFormat, got %v", allocs)
		}
	})

	t.Run("AppendSPKIFingerprint", func(t *testing.T) {
		rawSPKI := []byte("example_subject_public_key_info")
		var buf [64]byte

		allocs := testing.AllocsPerRun(1000, func() {
			_ = hpkp.AppendSPKIFingerprint(buf[:0], rawSPKI)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for AppendSPKIFingerprint, got %v", allocs)
		}
	})

	t.Run("AcquireReleasePolicy", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			p := hpkp.AcquirePolicy()
			hpkp.ReleasePolicy(p)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for AcquirePolicy/ReleasePolicy, got %v", allocs)
		}
	})
}
