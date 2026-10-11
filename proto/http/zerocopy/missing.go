// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import "github.com/lemon4ksan/foundation/silicon/bytesconv"

// B2S converts byte slice b to a string without memory allocation.
func B2S(b []byte) string { return bytesconv.B2S(b) }

// S2B converts string s to a byte slice without memory allocation.
func S2B(s string) []byte { return bytesconv.S2B(s) }
