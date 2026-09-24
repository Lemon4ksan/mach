// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"bufio"

	"github.com/lemon4ksan/mach/hpack"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

// HPACK provides stateful HTTP/2 header compression and decompression per RFC 7541.
type HPACK = hpack.HPACK

// AcquireHPACK retrieves an HPACK compressor/decompressor instance from the shared object pool.
func AcquireHPACK() *HPACK { return hpack.AcquireHPACK() }

// ReleaseHPACK returns an HPACK instance to the shared pool after resetting its internal tables.
func ReleaseHPACK(hp *HPACK) { hpack.ReleaseHPACK(hp) }

type (
	// FrameType denotes an 8-bit HTTP/2 frame type identifier (RFC 9113 §6).
	FrameType = coreh2.FrameType

	// Frame defines the interface implemented by all concrete HTTP/2 frame payloads (RFC 9113 §6).
	Frame = coreh2.Frame
)

// AcquireFrame allocates or reuses a Frame instance corresponding to the given FrameType.
func AcquireFrame(t FrameType) Frame { return coreh2.AcquireFrame(t) }

// ReleaseFrame returns a Frame instance to the appropriate frame slab pool.
func ReleaseFrame(f Frame) { coreh2.ReleaseFrame(f) }

// HeaderField represents an individual HTTP header key-value tuple (RFC 7541 §1.3).
type HeaderField = hpack.HeaderField

// AcquireHeaderField retrieves a pooled HeaderField instance.
func AcquireHeaderField() *HeaderField { return hpack.AcquireHeaderField() }

// ReleaseHeaderField returns a HeaderField to the shared pool.
func ReleaseHeaderField(hf *HeaderField) { hpack.ReleaseHeaderField(hf) }

// FrameHeaders identifies the HEADERS frame type (0x01) carrying HPACK header blocks (RFC 9113 §6.2).
const FrameHeaders = coreh2.FrameHeaders

type (
	// Headers represents an HTTP/2 HEADERS frame payload (RFC 9113 §6.2).
	Headers = coreh2.Headers

	// FrameHeader represents the fixed 9-octet HTTP/2 frame header (RFC 9113 §4.1).
	FrameHeader = coreh2.FrameHeader
)

// AcquireFrameHeader retrieves a pooled 9-byte FrameHeader instance.
func AcquireFrameHeader() *FrameHeader { return coreh2.AcquireFrameHeader() }

// ReleaseFrameHeader returns a FrameHeader instance to the pool.
func ReleaseFrameHeader(fh *FrameHeader) { coreh2.ReleaseFrameHeader(fh) }

// FrameSettings identifies the SETTINGS frame type (0x04) (RFC 9113 §6.5).
const FrameSettings = coreh2.FrameSettings

// Settings encapsulates HTTP/2 configuration parameters exchanged by peers (RFC 9113 §6.5).
type Settings = coreh2.Settings

// ReadFrameFrom reads and deserializes the next HTTP/2 frame header and payload from br (RFC 9113 §4.1).
func ReadFrameFrom(br *bufio.Reader) (*FrameHeader, error) { return coreh2.ReadFrameFrom(br) }

// FrameData identifies the DATA frame type (0x00) carrying stream payload octets (RFC 9113 §6.1).
const FrameData = coreh2.FrameData

// Data represents an HTTP/2 DATA frame payload (RFC 9113 §6.1).
type Data = coreh2.Data

// FrameWindowUpdate identifies the WINDOW_UPDATE frame type (0x08) (RFC 9113 §6.9).
const FrameWindowUpdate = coreh2.FrameWindowUpdate

// WindowUpdate represents an HTTP/2 WINDOW_UPDATE frame (RFC 9113 §6.9).
type WindowUpdate = coreh2.WindowUpdate
