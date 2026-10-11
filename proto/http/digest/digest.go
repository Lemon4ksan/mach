// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package digest

import (
	"github.com/lemon4ksan/mach/proto/semantic/digest"
)

var (
	// ErrDigestBadChallenge forwarded from proto/semantic/digest.
	ErrDigestBadChallenge = digest.ErrDigestBadChallenge

	// ErrDigestInvalidCharset forwarded from proto/semantic/digest.
	ErrDigestInvalidCharset = digest.ErrDigestInvalidCharset

	// ErrDigestAlgNotSupported forwarded from proto/semantic/digest.
	ErrDigestAlgNotSupported = digest.ErrDigestAlgNotSupported

	// ErrDigestQopNotSupported forwarded from proto/semantic/digest.
	ErrDigestQopNotSupported = digest.ErrDigestQopNotSupported
)

// Transport forwarded from proto/semantic/digest.
type Transport = digest.Transport

// Challenge forwarded from proto/semantic/digest.
type Challenge = digest.Challenge

// ParseChallenge forwarded from proto/semantic/digest.
func ParseChallenge(input string) (*Challenge, error) {
	return digest.ParseChallenge(input)
}
