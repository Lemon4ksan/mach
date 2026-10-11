//go:build !linux

// Copyright (c) 2016 the quic-go authors. All rights reserved.

package quic

import "errors"

var (
	errGSO          = errors.New("fake GSO error")
	errNotPermitted = errors.New("fake not permitted error")
)
