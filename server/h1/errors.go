// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"errors"
)

// ErrServerClosed is returned by the server's Serve functions after a call to Close or Shutdown (RFC 9112).
var ErrServerClosed = errors.New("h1: server is closed")
