// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"github.com/lemon4ksan/mach/proto/semantic/uri"
)

// DecodeArgAppend decodes URL-encoded argument bytes from src into dst, replacing '+' with space (RFC 3986 §2.1).
func DecodeArgAppend(dst, src []byte) []byte {
	return uri.DecodeArgAppend(dst, src)
}

// DecodeArgAppendNoPlus decodes percent-encoded bytes from src into dst without translating '+' into space (RFC 3986 §2.1).
func DecodeArgAppendNoPlus(dst, src []byte) []byte {
	return uri.DecodeArgAppendNoPlus(dst, src)
}
