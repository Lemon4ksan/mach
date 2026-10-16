// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package overlay_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/mach/proto/h2/overlay"
)

// TestOverlay_HeadersFrame_PriorityAndPadding verifies that HeadersFrame.HeaderBlockFragment
// properly strips RFC 9113 §6.2 PRIORITY fields (5 bytes: 4 bytes dependency + 1 byte weight)
// and PADDED fields (1 byte pad length + trailing pad bytes).
func TestOverlay_HeadersFrame_PriorityAndPadding(t *testing.T) {
	t.Run("PriorityFlagSet", func(t *testing.T) {
		// HEADERS frame with PRIORITY flag (0x20) and END_HEADERS (0x04) = 0x24
		// Payload:
		//   Dependency: 0x00000000 (4 bytes)
		//   Weight: 0x0F (1 byte)
		//   Header block fragment: [0x82, 0x86] (2 bytes)
		raw := []byte{
			0x00, 0x00, 0x07, // Length = 7
			0x01,                   // Type = HEADERS
			0x24,                   // Flags = PRIORITY | END_HEADERS
			0x00, 0x00, 0x00, 0x01, // StreamID = 1
			0x00, 0x00, 0x00, 0x00, // Dependency = 0
			0x0F,       // Weight = 15
			0x82, 0x86, // Header block fragment
		}

		hf := overlay.HeadersFrame(overlay.Frame(raw))
		if !overlay.Frame(hf).IsValid() {
			t.Fatalf("frame must be valid")
		}

		got := hf.HeaderBlockFragment()
		want := []byte{0x82, 0x86}
		if !bytes.Equal(got, want) {
			t.Fatalf("HeaderBlockFragment() with PRIORITY = %x, want %x", got, want)
		}
	})

	t.Run("PaddedFlagSet", func(t *testing.T) {
		// HEADERS frame with PADDED flag (0x08) and END_HEADERS (0x04) = 0x0C
		// Payload:
		//   PadLength: 2 (1 byte)
		//   Header block fragment: [0x82, 0x86] (2 bytes)
		//   Padding: [0x00, 0x00] (2 bytes)
		raw := []byte{
			0x00, 0x00, 0x05, // Length = 5
			0x01,                   // Type = HEADERS
			0x0C,                   // Flags = PADDED | END_HEADERS
			0x00, 0x00, 0x00, 0x01, // StreamID = 1
			0x02,       // PadLength = 2
			0x82, 0x86, // Header block fragment
			0x00, 0x00, // Padding
		}

		hf := overlay.HeadersFrame(overlay.Frame(raw))
		if !overlay.Frame(hf).IsValid() {
			t.Fatalf("frame must be valid")
		}

		got := hf.HeaderBlockFragment()
		want := []byte{0x82, 0x86}
		if !bytes.Equal(got, want) {
			t.Fatalf("HeaderBlockFragment() with PADDED = %x, want %x", got, want)
		}
	})

	t.Run("PriorityAndPaddedFlagsSet", func(t *testing.T) {
		// HEADERS frame with PRIORITY (0x20), PADDED (0x08), END_HEADERS (0x04) = 0x2C
		// Payload:
		//   PadLength: 1 (1 byte)
		//   Dependency: 0x00000000 (4 bytes)
		//   Weight: 0x0A (1 byte)
		//   Header block fragment: [0x82, 0x86] (2 bytes)
		//   Padding: [0x00] (1 byte)
		raw := []byte{
			0x00, 0x00, 0x09, // Length = 9
			0x01,                   // Type = HEADERS
			0x2C,                   // Flags = PRIORITY | PADDED | END_HEADERS
			0x00, 0x00, 0x00, 0x01, // StreamID = 1
			0x01,                   // PadLength = 1
			0x00, 0x00, 0x00, 0x00, // Dependency = 0
			0x0A,       // Weight = 10
			0x82, 0x86, // Header block fragment
			0x00, // Padding
		}

		hf := overlay.HeadersFrame(overlay.Frame(raw))
		if !overlay.Frame(hf).IsValid() {
			t.Fatalf("frame must be valid")
		}

		got := hf.HeaderBlockFragment()
		want := []byte{0x82, 0x86}
		if !bytes.Equal(got, want) {
			t.Fatalf("HeaderBlockFragment() with PRIORITY|PADDED = %x, want %x", got, want)
		}
	})
}
