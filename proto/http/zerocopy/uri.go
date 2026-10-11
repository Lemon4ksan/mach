// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"github.com/lemon4ksan/mach/proto/semantic/uri"
)

// URI type alias forwarded from proto/semantic/uri.
type URI = uri.URI

// EscapeError forwarded from proto/semantic/uri.
type EscapeError = uri.EscapeError

// InvalidHostError forwarded from proto/semantic/uri.
type InvalidHostError = uri.InvalidHostError

var (
	// AcquireURI returns an empty [URI] instance from the pool.
	AcquireURI = uri.AcquireURI

	// ReleaseURI resets u and returns it to the pool.
	ReleaseURI = uri.ReleaseURI

	// ErrorInvalidURI is returned when uri cannot be parsed.
	ErrorInvalidURI = uri.ErrorInvalidURI

	// IsValidScheme reports whether the scheme contains only valid characters.
	IsValidScheme = uri.IsValidScheme

	// NormalizePath normalizes path by removing extra slashes and dot segments.
	NormalizePath = uri.NormalizePath

	// SplitHostURI splits host and URI path.
	SplitHostURI = uri.SplitHostURI

	// StringContainsCTLByte reports whether s contains any ASCII control character.
	StringContainsCTLByte = uri.StringContainsCTLByte
)
