// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/lemon4ksan/mach/proto/semantic/auth"
)

// ForEachChallengeParam forwarded from proto/semantic/auth.
func ForEachChallengeParam[S ~string | ~[]byte](header S, scheme string, yield func(key, val string) bool) bool {
	return auth.ForEachChallengeParam(header, scheme, yield)
}

// ExtractChallengeParams forwarded from proto/semantic/auth.
func ExtractChallengeParams(header, scheme string) (map[string]string, bool) {
	return auth.ExtractChallengeParams(header, scheme)
}
