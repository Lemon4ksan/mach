// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package overlay provides in-situ, zero-copy HTTP/2 frame inspection over raw byte buffers (RFC 9113).
//
// By leveraging Bounds Check Elimination (BCE) and type aliasing directly over incoming wire slices,
// it enables zero-allocation frame header parsing and payload extraction on hot network paths.
package overlay
