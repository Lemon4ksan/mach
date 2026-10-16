// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import "github.com/lemon4ksan/foundation/silicon/hexkit"

// FormatHexUint writes the hexadecimal representation of val into buf and returns the number of bytes written.
// FormatHexUint panics if val is negative.
// Accelerated via foundation/silicon/hexkit vector and LUT kernels.
//
//go:inline
func FormatHexUint(buf *[16]byte, val int) int {
	return hexkit.FormatHexUint(buf, val)
}
