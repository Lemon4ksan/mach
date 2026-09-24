// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import "math"

// EncodeRequiredInsertCount encodes Required Insert Count according to RFC 9204 §4.5.1.1.
func EncodeRequiredInsertCount(requiredInsertCount, maxEntries uint64) uint64 {
	if requiredInsertCount == 0 {
		return 0
	}
	return (requiredInsertCount % (2 * maxEntries)) + 1
}

// DecodeRequiredInsertCount decodes Required Insert Count according to RFC 9204 §4.5.1.1.
// Returns (requiredInsertCount, true) on success, or (0, false) on invalid input, underflow, or overflow.
func DecodeRequiredInsertCount(encodedRequiredInsertCount, maxEntries, totalNumberOfInserts uint64) (uint64, bool) {
	if encodedRequiredInsertCount == 0 {
		return 0, true
	}

	if encodedRequiredInsertCount > 2*maxEntries {
		return 0, false
	}

	reqInsertCount := encodedRequiredInsertCount - 1
	currentWrapped := totalNumberOfInserts % (2 * maxEntries)

	if currentWrapped >= reqInsertCount+maxEntries {
		reqInsertCount += 2 * maxEntries
	} else if currentWrapped+maxEntries < reqInsertCount {
		currentWrapped += 2 * maxEntries
	}

	if reqInsertCount > math.MaxUint64-totalNumberOfInserts {
		return 0, false
	}
	reqInsertCount += totalNumberOfInserts

	if currentWrapped >= reqInsertCount {
		return 0, false
	}

	reqInsertCount -= currentWrapped
	return reqInsertCount, true
}
