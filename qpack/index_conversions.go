// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import "math"

// AbsoluteIndexToEncoderStreamRelativeIndex converts an absolute index to an encoder stream relative index.
// Precondition: absoluteIndex < insertedEntryCount.
func AbsoluteIndexToEncoderStreamRelativeIndex(absoluteIndex, insertedEntryCount uint64) uint64 {
	if absoluteIndex >= insertedEntryCount {
		panic("qpack: absoluteIndex must be less than insertedEntryCount")
	}
	return insertedEntryCount - absoluteIndex - 1
}

// AbsoluteIndexToEncoderRelativeIndex is an alias for AbsoluteIndexToEncoderStreamRelativeIndex.
func AbsoluteIndexToEncoderRelativeIndex(absoluteIndex, insertedEntryCount uint64) uint64 {
	return AbsoluteIndexToEncoderStreamRelativeIndex(absoluteIndex, insertedEntryCount)
}

// EncoderStreamRelativeIndexToAbsoluteIndex converts an encoder stream relative index to an absolute index.
// Returns (absoluteIndex, true) on success, or (0, false) if relativeIndex >= insertedEntryCount.
func EncoderStreamRelativeIndexToAbsoluteIndex(relativeIndex, insertedEntryCount uint64) (uint64, bool) {
	if relativeIndex >= insertedEntryCount {
		return 0, false
	}
	return insertedEntryCount - relativeIndex - 1, true
}

// EncoderRelativeIndexToAbsoluteIndex is an alias for EncoderStreamRelativeIndexToAbsoluteIndex.
func EncoderRelativeIndexToAbsoluteIndex(relativeIndex, insertedEntryCount uint64) (uint64, bool) {
	return EncoderStreamRelativeIndexToAbsoluteIndex(relativeIndex, insertedEntryCount)
}

// AbsoluteIndexToRequestStreamRelativeIndex converts an absolute index to a request stream relative index.
// Precondition: absoluteIndex < base.
func AbsoluteIndexToRequestStreamRelativeIndex(absoluteIndex, base uint64) uint64 {
	if absoluteIndex >= base {
		panic("qpack: absoluteIndex must be less than base")
	}
	return base - absoluteIndex - 1
}

// AbsoluteIndexToRequestRelativeIndex is an alias for AbsoluteIndexToRequestStreamRelativeIndex.
func AbsoluteIndexToRequestRelativeIndex(absoluteIndex, base uint64) uint64 {
	return AbsoluteIndexToRequestStreamRelativeIndex(absoluteIndex, base)
}

// RequestStreamRelativeIndexToAbsoluteIndex converts a request stream relative index to an absolute index.
// Returns (absoluteIndex, true) on success, or (0, false) if relativeIndex >= base.
func RequestStreamRelativeIndexToAbsoluteIndex(relativeIndex, base uint64) (uint64, bool) {
	if relativeIndex >= base {
		return 0, false
	}
	return base - relativeIndex - 1, true
}

// RequestRelativeIndexToAbsoluteIndex is an alias for RequestStreamRelativeIndexToAbsoluteIndex.
func RequestRelativeIndexToAbsoluteIndex(relativeIndex, base uint64) (uint64, bool) {
	return RequestStreamRelativeIndexToAbsoluteIndex(relativeIndex, base)
}

// PostBaseIndexToAbsoluteIndex converts a post-base index to an absolute index.
// Returns (absoluteIndex, true) on success, or (0, false) on uint64 overflow.
func PostBaseIndexToAbsoluteIndex(postBaseIndex, base uint64) (uint64, bool) {
	if postBaseIndex >= math.MaxUint64-base {
		return 0, false
	}
	return base + postBaseIndex, true
}

// AbsoluteIndexToPostBaseIndex converts an absolute index to a post-base index.
// Precondition: absoluteIndex >= base.
func AbsoluteIndexToPostBaseIndex(absoluteIndex, base uint64) uint64 {
	if absoluteIndex < base {
		panic("qpack: absoluteIndex must be greater than or equal to base")
	}
	return absoluteIndex - base
}
