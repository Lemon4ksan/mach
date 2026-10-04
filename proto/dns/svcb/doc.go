// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package svcb implements Service Binding (SVCB) and HTTPS Resource Records conforming to RFC 9460.
//
// It provides zero-allocation wire-format parsing and serialization for SVCB (RR 64) and HTTPS (RR 65)
// records, facilitating high-speed ALPN resolution, port redirection, and Encrypted Client Hello (ECH) bootstrapping.
package svcb
