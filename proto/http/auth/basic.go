// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/lemon4ksan/mach/proto/semantic/auth"
)

// ErrInvalidBase64 forwarded from proto/semantic/auth.
var ErrInvalidBase64 = auth.ErrInvalidBase64

// BasicChallenge forwarded from proto/semantic/auth.
type BasicChallenge = auth.BasicChallenge

// AppendBasic forwarded from proto/semantic/auth.
func AppendBasic[S ~string | ~[]byte](dst []byte, username, password S) []byte {
	return auth.AppendBasic(dst, username, password)
}

// FormatBasic forwarded from proto/semantic/auth.
func FormatBasic(username, password string) string {
	return auth.FormatBasic(username, password)
}

// ParseBasic forwarded from proto/semantic/auth.
func ParseBasic(authHeader string) (username, password string, err error) {
	return auth.ParseBasic(authHeader)
}

// ParseBasicBytes forwarded from proto/semantic/auth.
func ParseBasicBytes(authHeader []byte) (username, password []byte, err error) {
	return auth.ParseBasicBytes(authHeader)
}

// ParseBasicChallenge forwarded from proto/semantic/auth.
func ParseBasicChallenge(challengeHeader string) (BasicChallenge, bool) {
	return auth.ParseBasicChallenge(challengeHeader)
}

// InScope forwarded from proto/semantic/auth.
func InScope(reqURL, scopeRootURL string) bool {
	return auth.InScope(reqURL, scopeRootURL)
}
