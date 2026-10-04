// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpcweb_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/mach/proto/grpcweb"
)

func TestAlloc(t *testing.T) {
	payload := []byte("hello grpc-web payload content")
	rawFrame := grpcweb.AppendFrame(nil, grpcweb.FlagCompressed, payload)

	t.Run("FrameOverlay", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			frame := grpcweb.Frame(rawFrame)
			if !frame.IsValid() {
				t.Fatal("invalid frame")
			}
			_ = frame.Flags()
			_ = frame.IsCompressed()
			_ = frame.IsTrailer()
			_ = frame.Length()
			_ = frame.Payload()
		})
		if allocs != 0 {
			t.Errorf("FrameOverlay expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("ReadFrameTo", func(t *testing.T) {
		framer := grpcweb.NewFramer(1024)
		reader := bytes.NewReader(rawFrame)
		buf := make([]byte, 128)

		allocs := testing.AllocsPerRun(1000, func() {
			reader.Reset(rawFrame)
			flags, n, err := framer.ReadFrameTo(reader, buf)
			if err != nil || n != len(payload) || flags != grpcweb.FlagCompressed {
				t.Fatalf("unexpected read result: flags=%d n=%d err=%v", flags, n, err)
			}
		})
		if allocs != 0 {
			t.Errorf("ReadFrameTo expected 0 allocs, got %v", allocs)
		}
	})
}
