// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import "errors"

// ErrTimeout is returned from timed out calls.
var ErrTimeout = errors.New("timeout")

// ErrConnectionClosed may be returned from client methods if the server
// closes connection before returning the first response byte.
var ErrConnectionClosed = errors.New(
	"the server closed connection before returning the first response byte. Make sure the server returns 'Connection: close' response header before closing the connection",
)

// maxSmallFileSize is used in some parsing logic
const maxSmallFileSize = 2 * 1024 * 1024
