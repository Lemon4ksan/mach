package h3

import coreh3 "github.com/lemon4ksan/mach/core/h3"

type QPACKCodec = coreh3.QPACKCodec

func NewQPACKCodec() *QPACKCodec { return coreh3.NewQPACKCodec() }

const FrameTypeHeaders = coreh3.FrameTypeHeaders

var ReadFrameHeader = coreh3.ReadFrameHeader
