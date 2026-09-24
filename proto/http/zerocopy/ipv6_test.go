// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"errors"
	"testing"
)

func TestValidateIPv6Literal(t *testing.T) {
	// Non-IPv6
	if err := validateIPv6Literal([]byte("example.com")); err != nil {
		t.Fatalf("expected nil for regular hostname, got %v", err)
	}
	if err := validateIPv6Literal(nil); err != nil {
		t.Fatalf("expected nil for empty host, got %v", err)
	}

	// Missing bracket or empty brackets
	if err := validateIPv6Literal([]byte("[fe80::1")); !errors.Is(err, errInvalidIPv6Host) {
		t.Fatalf("expected errInvalidIPv6Host for missing bracket, got %v", err)
	}
	if err := validateIPv6Literal([]byte("[]")); !errors.Is(err, errInvalidIPv6Host) {
		t.Fatalf("expected errInvalidIPv6Host for [], got %v", err)
	}

	// Zone identifier
	if err := validateIPv6Literal([]byte("[fe80::1%]")); !errors.Is(err, errInvalidIPv6Zone) {
		t.Fatalf("expected errInvalidIPv6Zone for empty zone, got %v", err)
	}
	if err := validateIPv6Literal([]byte("[fe80::1%eth0]")); err != nil {
		t.Fatalf("expected valid for [fe80::1%%eth0], got %v", err)
	}

	// Address without colon
	if err := validateIPv6Literal([]byte("[12345]")); !errors.Is(err, errInvalidIPv6Address) {
		t.Fatalf("expected errInvalidIPv6Address for [12345], got %v", err)
	}

	// Valid pure IPv6 literals
	validLiterals := []string{
		"[::]",
		"[::1]",
		"[2001:db8::1]",
		"[2001:db8:85a3:8d3:1319:8a2e:370:7348]",
		"[fe80::1]",
		"[1:2:3:4:5:6:7:8]",
		"[::ffff:0:0]",
	}
	for _, lit := range validLiterals {
		if err := validateIPv6Literal([]byte(lit)); err != nil {
			t.Fatalf("expected valid for %s, got %v", lit, err)
		}
	}

	// Invalid pure IPv6 literals
	invalidLiterals := []string{
		"[1:2:3:4:5:6:7:8:9]", // 9 groups without compression
		"[1:2:3:4:5:6:7]",     // 7 groups without compression
		"[2001::db8::1]",      // multiple ::
		"[1:2:3:4:5:6:7:8::]", // 8 groups with compression
		"[12345::1]",          // hextet too long
		"[:1:2:3:4:5:6:7]",    // single colon at start
		"[1:2:3:4:5:6:7:]",    // trailing colon without allowTrailingColon
		"[2001:xyz::1]",       // non-hex characters
	}
	for _, lit := range invalidLiterals {
		if err := validateIPv6Literal([]byte(lit)); !errors.Is(err, errInvalidIPv6Address) {
			t.Fatalf("expected errInvalidIPv6Address for %s, got %v", lit, err)
		}
	}

	// Valid IPv4-embedded IPv6 literals
	validIPv4Embedded := []string{
		"[::ffff:192.0.2.1]",
		"[::192.0.2.1]",
		"[2001:db8:1:2:3:4:192.0.2.1]",
		"[64:ff9b::192.0.2.33]",
	}
	for _, lit := range validIPv4Embedded {
		if err := validateIPv6Literal([]byte(lit)); err != nil {
			t.Fatalf("expected valid for IPv4-embedded %s, got %v", lit, err)
		}
	}

	// Invalid IPv4-embedded IPv6 literals
	invalidIPv4Embedded := []string{
		"[192.168.1.1]",             // no colon
		"[:::192.168.1.1]",          // triple colon
		"[::ffff:999.0.2.1]",        // invalid IPv4 tail
		"[2001::db8::192.0.2.1]",    // double ::
		"[1:2:3:4:5:6:7:192.0.2.1]", // 7 hextets + 2 = 9 groups
	}
	for _, lit := range invalidIPv4Embedded {
		if err := validateIPv6Literal([]byte(lit)); !errors.Is(err, errInvalidIPv6Address) {
			t.Fatalf("expected errInvalidIPv6Address for %s, got %v", lit, err)
		}
	}
}

func TestParseIPv6Hextets(t *testing.T) {
	// Empty
	g, double, ok := parseIPv6Hextets(nil, false)
	if g != 0 || double || !ok {
		t.Fatalf("expected 0, false, true for empty, got %d, %v, %v", g, double, ok)
	}

	// Trailing colon allowed vs disallowed
	_, _, okTrailingDisallowed := parseIPv6Hextets([]byte("2001:db8:"), false)
	if okTrailingDisallowed {
		t.Fatal("expected false when allowTrailingColon=false")
	}

	gTrailing, _, okTrailingAllowed := parseIPv6Hextets([]byte("2001:db8:"), true)
	if !okTrailingAllowed || gTrailing != 2 {
		t.Fatalf("expected ok=true, groups=2 when allowTrailingColon=true, got %v, %d", okTrailingAllowed, gTrailing)
	}

	// Single colon at start
	_, _, okStartColon := parseIPv6Hextets([]byte(":2001"), false)
	if okStartColon {
		t.Fatal("expected false for single colon at start")
	}

	// Double colon followed immediately by invalid
	_, _, okMultipleDouble := parseIPv6Hextets([]byte("::1::2"), false)
	if okMultipleDouble {
		t.Fatal("expected false for multiple ::")
	}
}

func TestValidIPv4(t *testing.T) {
	validIPs := []string{
		"0.0.0.0",
		"127.0.0.1",
		"192.168.1.1",
		"255.255.255.255",
		"10.20.30.40",
	}
	for _, ip := range validIPs {
		if !validIPv4([]byte(ip)) {
			t.Fatalf("expected validIPv4=true for %q", ip)
		}
	}

	invalidIPs := []string{
		"",
		"192.168.1",
		"192.168.1.1.1",
		"192.168.1.256",
		"192.168.1.01", // leading zero
		"192.168.00.1", // double zero
		"192.168.1.1extra",
		"192.168..1",
		"192.168.1.a",
		"192.168.1.1234",
	}
	for _, ip := range invalidIPs {
		if validIPv4([]byte(ip)) {
			t.Fatalf("expected validIPv4=false for %q", ip)
		}
	}
}
