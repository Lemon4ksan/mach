// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package uri

import (
	"errors"
	"math"
	"strconv"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

var (
	errEmptyInt               = errors.New("empty integer")
	errUnexpectedFirstChar    = errors.New("unexpected first char found: expecting 0-9")
	errUnexpectedTrailingChar = errors.New("unexpected trailing char found: expecting 0-9")
	errTooLongInt             = errors.New("too long int")
	errNegativeFloat          = errors.New("negative input is invalid")
)

const (
	maxIntDiv10      = math.MaxInt / 10
	maxSafeIntDigits = 9 * (strconv.IntSize / 32)
)

func parseUint(buf []byte) (int, error) {
	v, n, err := parseUintBuf(buf)
	if n != len(buf) {
		return -1, errUnexpectedTrailingChar
	}

	return v, err
}

func parseUintBuf(b []byte) (int, int, error) {
	if len(b) == 0 {
		return -1, 0, errEmptyInt
	}

	v := 0
	for i, c := range b {
		k := c - '0'
		if k > 9 {
			if i == 0 {
				return -1, i, errUnexpectedFirstChar
			}

			return v, i, nil
		}

		vNew := 10*v + int(k)
		if i >= maxSafeIntDigits && (v > maxIntDiv10 || vNew < 0) {
			return -1, i, errTooLongInt
		}

		v = vNew
	}

	return v, len(b), nil
}

func parseUfloat(buf []byte) (float64, error) {
	res, err := strconv.ParseFloat(bytesconv.B2S(buf), 64)
	if res < 0 {
		return -1, errNegativeFloat
	}

	if err != nil {
		return -1, err
	}

	return res, err
}

func appendUint(dst []byte, n int) []byte {
	if n < 0 {
		panic("negative integer passed to appendUint")
	}

	return strconv.AppendUint(dst, uint64(n), 10)
}
