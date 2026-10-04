// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rodata provides read-only static byte slices for HTTP protocol processing.
//
// By anchoring canonical headers, methods, and protocol delimiters in the binary's .rodata segment,
// it enables zero-allocation framing, scanning, and interning on hot request and response paths.
package rodata
