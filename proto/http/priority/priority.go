// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package priority

import (
	"github.com/lemon4ksan/mach/proto/semantic/priority"
)

// HeaderPriority forwarded from proto/semantic/priority.
const HeaderPriority = priority.HeaderPriority

// Standard RFC 9218 urgency levels forwarded from proto/semantic/priority.
const (
	UrgencyUserBlocking = priority.UrgencyUserBlocking
	UrgencyHigh         = priority.UrgencyHigh
	UrgencyMedium       = priority.UrgencyMedium
	UrgencyLow          = priority.UrgencyLow
	UrgencyVeryLow      = priority.UrgencyVeryLow
	UrgencyBackground   = priority.UrgencyBackground
	UrgencyIdle         = priority.UrgencyIdle
	UrgencyMinimum      = priority.UrgencyMinimum
)

// Standard predefined configurations forwarded from proto/semantic/priority.
var (
	UserBlocking = priority.UserBlocking
	Interactive  = priority.Interactive
	Low          = priority.Low
	Background   = priority.Background
)

// Priority forwarded from proto/semantic/priority.
type Priority = priority.Priority

// New forwarded from proto/semantic/priority.
func New(urgency int, incremental bool) Priority {
	return priority.New(urgency, incremental)
}

// ParsePriority forwarded from proto/semantic/priority.
func ParsePriority[S ~string | ~[]byte](s S) (Priority, error) {
	return priority.ParsePriority(s)
}

// Parse forwarded from proto/semantic/priority.
func Parse(s string) (Priority, error) {
	return priority.Parse(s)
}

// ParseBytes forwarded from proto/semantic/priority.
func ParseBytes(b []byte) (Priority, error) {
	return priority.ParseBytes(b)
}
