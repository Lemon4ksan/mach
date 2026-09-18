// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import coreheaders "github.com/lemon4ksan/foundation/net/headkit"

type (
	Headers     = coreheaders.Headers
	HeaderEntry = coreheaders.HeaderEntry
)

var NewHeadersWithCapacity = coreheaders.NewWithCapacity
