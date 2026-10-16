// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2_test

import (
	"fmt"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

func TestAllocations(t *testing.T) {
	var rawHeader [9]byte

	t.Run("PackUnpackFrameHeader", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			coreh2.PackFrameHeader(
				rawHeader[:],
				128,
				coreh2.FrameHeaders,
				coreh2.FlagEndHeaders|coreh2.FlagEndStream,
				42,
			)
			l, kind, flags, stream := coreh2.UnpackFrameHeader(rawHeader[:])
			if l != 128 || kind != coreh2.FrameHeaders || stream != 42 || !flags.Has(coreh2.FlagEndHeaders) {
				t.Fatal("mismatch")
			}
		})
		assert.Equal(t, 0.0, allocs, "PackFrameHeader/UnpackFrameHeader must be 0 allocs")
	})

	t.Run("FrameHeaderAcquireRelease", func(t *testing.T) {
		fr := coreh2.AcquireFrameHeader()
		coreh2.ReleaseFrameHeader(fr)

		allocs := testing.AllocsPerRun(1000, func() {
			hdr := coreh2.AcquireFrameHeader()
			hdr.SetStream(1)
			hdr.SetFlags(coreh2.FlagEndStream)
			coreh2.ReleaseFrameHeader(hdr)
		})
		assert.Equal(t, 0.0, allocs, "FrameHeader pool acquire/release must be 0 allocs")
	})

	t.Run("ControlFramePoolAcquireRelease", func(t *testing.T) {
		types := []coreh2.FrameType{
			coreh2.FramePing,
			coreh2.FrameWindowUpdate,
			coreh2.FrameResetStream,
			coreh2.FramePriority,
			coreh2.FrameSettings,
		}

		for _, ft := range types {
			f := coreh2.AcquireFrame(ft)
			coreh2.ReleaseFrame(f)

			allocs := testing.AllocsPerRun(1000, func() {
				frame := coreh2.AcquireFrame(ft)
				coreh2.ReleaseFrame(frame)
			})
			assert.Equal(t, 0.0, allocs, fmt.Sprintf("AcquireFrame/ReleaseFrame for %s must be 0 allocs", ft))
		}
	})

	t.Run("PingSerializeDeserialize", func(t *testing.T) {
		hdr := coreh2.AcquireFrameHeader()
		defer coreh2.ReleaseFrameHeader(hdr)

		ping := coreh2.AcquireFrame(coreh2.FramePing).(*coreh2.Ping)
		defer coreh2.ReleaseFrame(ping)

		var payload [8]byte
		copy(payload[:], "12345678")
		ping.SetData(payload[:])

		allocs := testing.AllocsPerRun(1000, func() {
			ping.Serialize(hdr)
			_ = ping.Deserialize(hdr)
		})
		assert.Equal(t, 0.0, allocs, "Ping Serialize/Deserialize must be 0 allocs")
	})

	t.Run("WindowUpdateSerializeDeserialize", func(t *testing.T) {
		hdr := coreh2.AcquireFrameHeader()
		defer coreh2.ReleaseFrameHeader(hdr)
		hdr.SetStream(1)

		wu := coreh2.AcquireFrame(coreh2.FrameWindowUpdate).(*coreh2.WindowUpdate)
		defer coreh2.ReleaseFrame(wu)
		wu.SetIncrement(65535)

		allocs := testing.AllocsPerRun(1000, func() {
			wu.Serialize(hdr)
			_ = wu.Deserialize(hdr)
		})
		assert.Equal(t, 0.0, allocs, "WindowUpdate Serialize/Deserialize must be 0 allocs")
	})
}
