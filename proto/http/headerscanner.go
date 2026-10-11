// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/lemon4ksan/foundation/silicon/simd"
)

var (
	// errInvalidHeaderLeadingWhitespace indicates that headers cannot start with space or tab.
	errInvalidHeaderLeadingWhitespace = errors.New("invalid headers, headers cannot start with space or tab")
)

type headerScanner struct {
	b        []byte
	r        int
	blockEnd int

	key   []byte
	value []byte

	err         error
	initialized bool
}

func (s *headerScanner) next() bool {
	if !s.initialized {
		if len(s.b) >= 2 && s.b[0] == '\r' && s.b[1] == '\n' {
			s.r = 2
			return false
		}

		if s.blockEnd >= 4 && s.blockEnd <= len(s.b) &&
			s.b[s.blockEnd-4] == '\r' && s.b[s.blockEnd-3] == '\n' &&
			s.b[s.blockEnd-2] == '\r' && s.b[s.blockEnd-1] == '\n' {
			s.b = s.b[:s.blockEnd]
		} else {
			i := simd.IndexCRLFCRLF(s.b)
			if i < 0 {
				s.err = ErrNeedMore
				return false
			}

			s.b = s.b[:i]
		}

		// RFC 9112 §5.2: Obsolete line folding: message cannot begin with leading whitespace.
		if len(s.b) > 0 && (s.b[0] == ' ' || s.b[0] == '\t') {
			s.err = errInvalidHeaderLeadingWhitespace
			return false
		}

		s.initialized = true
	}

	kv, colon, err := s.readContinuedLineSlice()
	if len(kv) == 0 {
		s.err = err
		return false
	}

	k, v := kv[:colon], kv[colon+1:]

	// RFC 9112 §5.1: Field name validation (no whitespace before colon, valid token characters).
	if !isValidHeaderKey(k) {
		s.err = fmt.Errorf("malformed mime header line: %q", kv)
		return false
	}

	s.key = k
	s.value = simd.TrimLeadingSpaceSWAR(v)

	if err != nil {
		s.err = err
		return false
	}

	return true
}

func (s *headerScanner) readLine() []byte {
	i := bytes.IndexByte(s.b[s.r:], '\n')
	if i < 0 {
		return nil
	}

	line := s.b[s.r : s.r+i]

	s.r += i + 1
	if i > 0 && line[i-1] == '\r' {
		line = line[:i-1]
	}

	return line
}

func (s *headerScanner) readContinuedLineSlice() ([]byte, int, error) {
	line := s.readLine()
	if len(line) == 0 {
		return line, -1, nil
	}

	colon := simd.IndexColonSWAR(line)
	if colon < 0 {
		return nil, -1, fmt.Errorf("malformed mime header: missing colon: %q", line)
	}

	if len(s.b)-s.r > 1 {
		peek := s.b[s.r : s.r+2]
		if len(peek) > 0 && (isASCIILetter(peek[0]) || peek[0] == '\n') ||
			len(peek) == 2 && peek[0] == '\r' && peek[1] == '\n' {
			return simd.TrimSpaceSWAR(line), colon, nil
		}
	}

	mline := simd.TrimSpaceSWAR(line)

	for s.skipSpace() {
		mline = append(mline, ' ')
		nxt := s.readLine()
		mline = append(mline, simd.TrimSpaceSWAR(nxt)...)
	}

	return mline, colon, nil
}

func (s *headerScanner) skipSpace() bool {
	if s.r >= len(s.b) {
		return false
	}

	trimmed := simd.TrimLeadingSpaceSWAR(s.b[s.r:])

	skipped := len(s.b[s.r:]) - len(trimmed)
	if skipped > 0 {
		s.r += skipped
		return true
	}

	return false
}

func isASCIILetter(b byte) bool {
	b |= 0x20
	return 'a' <= b && b <= 'z'
}


// trim returns s with leading and trailing spaces and tabs removed using SWAR vectorization.
//
//go:inline
func trim(s []byte) []byte {
	return simd.TrimSpaceSWAR(s)
}

// trimTrailingSpace returns s with trailing spaces and tabs removed using SWAR vectorization.
//
//go:inline
func trimTrailingSpace(s []byte) []byte {
	return simd.TrimTrailingSpaceSWAR(s)
}
