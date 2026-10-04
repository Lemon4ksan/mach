// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package digest implements Digest Access Authentication conforming to RFC 7616 and RFC 2617.
//
// It provides high-performance challenge generation, credential verification, and cryptographic response
// hashing with pooled buffers to eliminate heap allocation overhead in secure L7 workflows.
package digest
