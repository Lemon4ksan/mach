// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	protoh1 "github.com/lemon4ksan/mach/proto/h1"
)

type headerScanner struct {
	inner protoh1.HeaderScanner

	b        []byte
	r        int
	blockEnd int

	key         []byte
	value       []byte
	keyHasSpace bool

	err error
}

func (s *headerScanner) next() bool {
	if !s.inner.Initialized {
		s.inner.B = s.b
		s.inner.BlockEnd = s.blockEnd
	}

	if !s.inner.Next() {
		s.r = s.inner.R
		s.err = s.inner.Err()
		return false
	}

	s.r = s.inner.R
	s.key = s.inner.Key()
	s.value = s.inner.Value()
	s.keyHasSpace = s.inner.KeyHasSpace()

	return true
}

// trim returns s with leading and trailing spaces and tabs removed using SWAR vectorization.
//
//go:inline
func trim(s []byte) []byte {
	return protoh1.TrimSpace(s)
}

// trimTrailingSpace returns s with trailing spaces and tabs removed using SWAR vectorization.
//
//go:inline
func trimTrailingSpace(s []byte) []byte {
	return protoh1.TrimTrailingSpace(s)
}
