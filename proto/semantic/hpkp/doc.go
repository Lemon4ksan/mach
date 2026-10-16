// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hpkp implements the Public Key Pinning Extension for HTTP strictly conforming to RFC 7469.
//
// It provides Subject Public Key Info (SPKI) SHA-256 fingerprint generation, Public-Key-Pins and
// Public-Key-Pins-Report-Only response header parsing, certificate chain validation, backup pin verification,
// and JSON failure report generation per RFC 7469 §3.
package hpkp
