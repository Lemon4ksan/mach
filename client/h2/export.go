package h2

import (
	"bufio"
	coreh2 "github.com/lemon4ksan/mach/core/h2"
)

type HPACK = coreh2.HPACK

func AcquireHPACK() *HPACK   { return coreh2.AcquireHPACK() }
func ReleaseHPACK(hp *HPACK) { coreh2.ReleaseHPACK(hp) }

type FrameType = coreh2.FrameType
type Frame = coreh2.Frame

func AcquireFrame(t FrameType) Frame { return coreh2.AcquireFrame(t) }
func ReleaseFrame(f Frame)           { coreh2.ReleaseFrame(f) }

type HeaderField = coreh2.HeaderField

func AcquireHeaderField() *HeaderField   { return coreh2.AcquireHeaderField() }
func ReleaseHeaderField(hf *HeaderField) { coreh2.ReleaseHeaderField(hf) }

const FrameHeaders = coreh2.FrameHeaders

type Headers = coreh2.Headers
type FrameHeader = coreh2.FrameHeader

func AcquireFrameHeader() *FrameHeader   { return coreh2.AcquireFrameHeader() }
func ReleaseFrameHeader(fh *FrameHeader) { coreh2.ReleaseFrameHeader(fh) }

const FrameSettings = coreh2.FrameSettings

type Settings = coreh2.Settings

func ReadFrameFrom(br *bufio.Reader) (*FrameHeader, error) { return coreh2.ReadFrameFrom(br) }

const FrameData = coreh2.FrameData

type Data = coreh2.Data

const FrameWindowUpdate = coreh2.FrameWindowUpdate

type WindowUpdate = coreh2.WindowUpdate
