// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import "errors"

// ErrTimeout is returned from timed out calls.
var ErrTimeout = errors.New("timeout")

// ErrConnectionClosed is returned when the remote server closes the connection
// before returning the first response byte (RFC 9112 Section 9.6).
var ErrConnectionClosed = errors.New(
	"the server closed connection before returning the first response byte. Make sure the server returns 'Connection: close' response header before closing the connection",
)

// ErrGetOnly is returned when a server configured with Server.GetOnly encounters
// a request method other than GET (RFC 9110 Section 9.3.1).
var ErrGetOnly = errors.New("mach: non-get request received")

// maxInterimResponses limits the number of consecutive informational responses
// accepted before ReadLimitBody returns errTooManyInterimResponses (RFC 9110 Section 15.2).
const maxInterimResponses = 100

var errTooManyInterimResponses = errors.New("mach: too many 1xx informational responses received")

var errRequestHostRequired = errors.New("missing required host header in request")

// maxSmallFileSize is used in some parsing logic
const maxSmallFileSize = 2 * 1024 * 1024
