// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package contentdisposition implements Content-Disposition header parsing, RFC 8187 parameter
// encoding/decoding, and safe filename sanitization strictly conforming to RFC 6266 and RFC 8187.
//
// It provides zero-allocation serialization, fast-path parameter extraction, and cross-platform
// file name sanitization protecting against path traversal attacks.
package contentdisposition
