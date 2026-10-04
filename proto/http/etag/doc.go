// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package etag implements HTTP Entity Tag evaluation and conditional request validation (RFC 7232, RFC 9110).
//
// It provides zero-allocation strong and weak validator comparisons (If-Match, If-None-Match)
// and cache revalidation logic for fast conditional HTTP request handling.
package etag
