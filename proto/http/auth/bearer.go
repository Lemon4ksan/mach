// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/lemon4ksan/mach/proto/semantic/auth"
)

// Standard OAuth 2.0 Bearer error codes forwarded from proto/semantic/auth.
const (
	ErrBearerInvalidRequest   = auth.ErrBearerInvalidRequest
	ErrBearerInvalidToken     = auth.ErrBearerInvalidToken
	ErrBearerInsufficientScope = auth.ErrBearerInsufficientScope
)

// BearerChallenge forwarded from proto/semantic/auth.
type BearerChallenge = auth.BearerChallenge

// AppendBearer forwarded from proto/semantic/auth.
func AppendBearer[S ~string | ~[]byte](dst []byte, token S) []byte {
	return auth.AppendBearer(dst, token)
}

// FormatBearer forwarded from proto/semantic/auth.
func FormatBearer(token string) string {
	return auth.FormatBearer(token)
}

// IsValidBearerToken forwarded from proto/semantic/auth.
func IsValidBearerToken[S ~string | ~[]byte](token S) bool {
	return auth.IsValidBearerToken(token)
}

// ParseBearer forwarded from proto/semantic/auth.
func ParseBearer(authHeader string) (token string, ok bool) {
	return auth.ParseBearer(authHeader)
}

// ParseBearerBytes forwarded from proto/semantic/auth.
func ParseBearerBytes(authHeader []byte) (token []byte, ok bool) {
	return auth.ParseBearerBytes(authHeader)
}

// ParseBearerChallenge forwarded from proto/semantic/auth.
func ParseBearerChallenge(challengeHeader string) (BearerChallenge, bool) {
	return auth.ParseBearerChallenge(challengeHeader)
}
