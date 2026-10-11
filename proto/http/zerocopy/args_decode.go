// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"github.com/lemon4ksan/foundation/silicon/hexkit"
	"github.com/lemon4ksan/foundation/silicon/simd"
)

// DecodeArgAppend decodes URL-encoded argument bytes from src into dst, replacing '+' with space (RFC 3986 §2.1).
// Vectorized using 64-bit SWAR/SIMD multi-byte scanning for '%' and '+' and foundation/silicon/hexkit nibble decoders.
func DecodeArgAppend(dst, src []byte) []byte {
	n := len(src)
	if n == 0 {
		return dst
	}

	idx := simd.IndexTwoBytesVector(src, '%', '+')
	if idx < 0 {
		return append(dst, src...)
	}

	dst = append(dst, src[:idx]...)
	i := idx

	for i < n {
		next := simd.IndexTwoBytesVector(src[i:], '%', '+')
		if next < 0 {
			return append(dst, src[i:]...)
		}

		if next > 0 {
			dst = append(dst, src[i:i+next]...)
			i += next
		}

		c := src[i]
		if c == '+' {
			dst = append(dst, ' ')
			i++

			continue
		}

		// Percent decoding '%XX'
		if i+2 >= n {
			return append(dst, src[i:]...)
		}

		h1, ok1 := hexkit.FromHexChar(src[i+1])
		if !ok1 {
			dst = append(dst, '%')
			i++

			continue
		}

		h2, ok2 := hexkit.FromHexChar(src[i+2])
		if !ok2 {
			dst = append(dst, '%')
			i++

			continue
		}

		dst = append(dst, (h1<<4)|h2)
		i += 3
	}

	return dst
}

// DecodeArgAppendNoPlus decodes percent-encoded bytes from src into dst without translating '+' into space (RFC 3986 §2.1).
// Vectorized using 64-bit SWAR/SIMD vector scanning for '%' and foundation/silicon/hexkit nibble decoders.
func DecodeArgAppendNoPlus(dst, src []byte) []byte {
	n := len(src)
	if n == 0 {
		return dst
	}

	idx := simd.IndexByteVector(src, '%')
	if idx < 0 {
		return append(dst, src...)
	}

	dst = append(dst, src[:idx]...)
	i := idx

	for i < n {
		next := simd.IndexByteVector(src[i:], '%')
		if next < 0 {
			return append(dst, src[i:]...)
		}

		if next > 0 {
			dst = append(dst, src[i:i+next]...)
			i += next
		}

		// Percent decoding '%XX'
		if i+2 >= n {
			return append(dst, src[i:]...)
		}

		h1, ok1 := hexkit.FromHexChar(src[i+1])
		if !ok1 {
			dst = append(dst, '%')
			i++

			continue
		}

		h2, ok2 := hexkit.FromHexChar(src[i+2])
		if !ok2 {
			dst = append(dst, '%')
			i++

			continue
		}

		dst = append(dst, (h1<<4)|h2)
		i += 3
	}

	return dst
}
