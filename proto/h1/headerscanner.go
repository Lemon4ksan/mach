// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"errors"
	"fmt"
)

var (
	// ErrNeedMore indicates that more data is needed to parse headers.
	ErrNeedMore = errors.New("need more data")
	// ErrInvalidHeaderLeadingWhitespace indicates that headers cannot start with space or tab.
	ErrInvalidHeaderLeadingWhitespace = errors.New("invalid headers, headers cannot start with space or tab")
)

// HeaderScanner scans HTTP/1.1 headers using SWAR / SIMD vector acceleration.
type HeaderScanner struct {
	B           []byte
	R           int
	BlockEnd    int
	KeyVal      []byte
	ValVal      []byte
	ScanErr     error
	Initialized bool
}

// NewHeaderScanner creates a new HeaderScanner scanning b.
func NewHeaderScanner(b []byte) *HeaderScanner {
	return &HeaderScanner{B: b}
}

// Reset resets the scanner with a new buffer b.
func (s *HeaderScanner) Reset(b []byte) {
	s.B = b
	s.R = 0
	s.BlockEnd = 0
	s.KeyVal = nil
	s.ValVal = nil
	s.ScanErr = nil
	s.Initialized = false
}

// Key returns the last scanned header key.
func (s *HeaderScanner) Key() []byte {
	return s.KeyVal
}

// Value returns the last scanned header value.
func (s *HeaderScanner) Value() []byte {
	return s.ValVal
}

// Err returns the scanner error, if any.
func (s *HeaderScanner) Err() error {
	return s.ScanErr
}

// Next advances the scanner to the next header field.
// Returns true on success, false when done or on error.
func (s *HeaderScanner) Next() bool {
	if !s.Initialized {
		if len(s.B) >= 2 && s.B[0] == '\r' && s.B[1] == '\n' {
			s.R = 2
			return false
		}

		if s.BlockEnd >= 4 && s.BlockEnd <= len(s.B) &&
			s.B[s.BlockEnd-4] == '\r' && s.B[s.BlockEnd-3] == '\n' &&
			s.B[s.BlockEnd-2] == '\r' && s.B[s.BlockEnd-1] == '\n' {
			s.B = s.B[:s.BlockEnd]
		} else {
			i := IndexCRLFCRLF(s.B)
			if i < 0 {
				s.ScanErr = ErrNeedMore
				return false
			}

			s.B = s.B[:i]
		}

		// RFC 9112 §5.2: Obsolete line folding: message cannot begin with leading whitespace.
		if len(s.B) > 0 && (s.B[0] == ' ' || s.B[0] == '\t') {
			s.ScanErr = ErrInvalidHeaderLeadingWhitespace
			return false
		}

		s.Initialized = true
	}

	kv, colon, err := s.readContinuedLineSlice()
	if len(kv) == 0 {
		s.ScanErr = err
		return false
	}

	k, v := kv[:colon], kv[colon+1:]

	// RFC 9112 §5.1: Field name validation (no whitespace before colon, valid token characters).
	if !isValidHeaderKey(k) {
		s.ScanErr = fmt.Errorf("malformed mime header line: %q", kv)
		return false
	}

	s.KeyVal = k
	s.ValVal = TrimSpace(v)

	if err != nil {
		s.ScanErr = err
		return false
	}

	return true
}

func (s *HeaderScanner) readLine() []byte {
	i := IndexByte(s.B[s.R:], '\n')
	if i < 0 {
		return nil
	}

	line := s.B[s.R : s.R+i]

	s.R += i + 1
	if i > 0 && line[i-1] == '\r' {
		line = line[:i-1]
	}

	return line
}

func (s *HeaderScanner) readContinuedLineSlice() ([]byte, int, error) {
	line := s.readLine()
	if len(line) == 0 {
		return line, -1, nil
	}

	colon := IndexColon(line)
	if colon < 0 {
		return nil, -1, fmt.Errorf("malformed mime header: missing colon: %q", line)
	}

	// RFC 9112 §5.2: Check if subsequent line begins with obs-fold (SP or HTAB)
	if len(s.B)-s.R > 0 {
		peekChar := s.B[s.R]
		if peekChar != ' ' && peekChar != '\t' {
			return line, colon, nil
		}
	} else {
		return line, colon, nil
	}

	mline := line

	for s.skipSpace() {
		mline = append(mline, ' ')
		nxt := s.readLine()
		mline = append(mline, TrimSpace(nxt)...)
	}

	return mline, colon, nil
}

func (s *HeaderScanner) skipSpace() bool {
	if s.R >= len(s.B) {
		return false
	}

	trimmed := TrimLeadingSpace(s.B[s.R:])

	skipped := len(s.B[s.R:]) - len(trimmed)
	if skipped > 0 {
		s.R += skipped
		return true
	}

	return false
}

func isASCIILetter(b byte) bool {
	b |= 0x20
	return 'a' <= b && b <= 'z'
}

func isValidHeaderKey(a []byte) bool {
	if len(a) == 0 {
		return false
	}

	for _, c := range a {
		if c >= 128 || validTokenTable[c] == 0 {
			return false
		}
	}

	return true
}

var validTokenTable = [128]byte{
	'!': 1, '#': 1, '$': 1, '%': 1, '&': 1, '\'': 1, '*': 1, '+': 1,
	'-': 1, '.': 1, '^': 1, '_': 1, '`': 1, '|': 1, '~': 1,
	'0': 1, '1': 1, '2': 1, '3': 1, '4': 1, '5': 1, '6': 1, '7': 1, '8': 1, '9': 1,
	'a': 1, 'b': 1, 'c': 1, 'd': 1, 'e': 1, 'f': 1, 'g': 1, 'h': 1, 'i': 1, 'j': 1,
	'k': 1, 'l': 1, 'm': 1, 'n': 1, 'o': 1, 'p': 1, 'q': 1, 'r': 1, 's': 1, 't': 1,
	'u': 1, 'v': 1, 'w': 1, 'x': 1, 'y': 1, 'z': 1,
	'A': 1, 'B': 1, 'C': 1, 'D': 1, 'E': 1, 'F': 1, 'G': 1, 'H': 1, 'I': 1, 'J': 1,
	'K': 1, 'L': 1, 'M': 1, 'N': 1, 'O': 1, 'P': 1, 'Q': 1, 'R': 1, 'S': 1, 'T': 1,
	'U': 1, 'V': 1, 'W': 1, 'X': 1, 'Y': 1, 'Z': 1,
}
