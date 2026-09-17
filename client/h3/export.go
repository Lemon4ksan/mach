// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import coreh3 "github.com/lemon4ksan/mach/proto/h3"

type QPACKCodec = coreh3.QPACKCodec

func NewQPACKCodec() *QPACKCodec { return coreh3.NewQPACKCodec() }

const FrameTypeHeaders = coreh3.FrameTypeHeaders

var ReadFrameHeader = coreh3.ReadFrameHeader
