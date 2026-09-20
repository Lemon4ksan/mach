// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"bufio"

	"github.com/lemon4ksan/foundation/net/hpack"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

type HPACK = hpack.HPACK

func AcquireHPACK() *HPACK   { return hpack.AcquireHPACK() }
func ReleaseHPACK(hp *HPACK) { hpack.ReleaseHPACK(hp) }

type (
	FrameType = coreh2.FrameType
	Frame     = coreh2.Frame
)

func AcquireFrame(t FrameType) Frame { return coreh2.AcquireFrame(t) }
func ReleaseFrame(f Frame)           { coreh2.ReleaseFrame(f) }

type HeaderField = hpack.HeaderField

func AcquireHeaderField() *HeaderField   { return hpack.AcquireHeaderField() }
func ReleaseHeaderField(hf *HeaderField) { hpack.ReleaseHeaderField(hf) }

const FrameHeaders = coreh2.FrameHeaders

type (
	Headers     = coreh2.Headers
	FrameHeader = coreh2.FrameHeader
)

func AcquireFrameHeader() *FrameHeader   { return coreh2.AcquireFrameHeader() }
func ReleaseFrameHeader(fh *FrameHeader) { coreh2.ReleaseFrameHeader(fh) }

const FrameSettings = coreh2.FrameSettings

type Settings = coreh2.Settings

func ReadFrameFrom(br *bufio.Reader) (*FrameHeader, error) { return coreh2.ReadFrameFrom(br) }

const FrameData = coreh2.FrameData

type Data = coreh2.Data

const FrameWindowUpdate = coreh2.FrameWindowUpdate

type WindowUpdate = coreh2.WindowUpdate
