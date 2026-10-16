// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
)

// Parse parses a raw proxy string into a structured [*url.URL] (RFC 3986 §3).
// If no scheme delimiter ("://") is present, it handles standard port conventions (1080/9050 -> socks5h)
// or defaults to "http". It performs zero blocking network I/O.
func Parse(proxyStr string) (*url.URL, error) {
	if proxyStr == "" {
		return nil, errors.New("proxy: empty proxy string")
	}

	if strings.Contains(proxyStr, "://") {
		return url.Parse(proxyStr)
	}

	u, err := url.Parse("http://" + proxyStr)
	if err != nil {
		return nil, err
	}

	_, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		portStr = ""
	}

	scheme := "http"
	switch portStr {
	case "1080", "1081", "9050", "9051", "10808":
		scheme = "socks5h"
	}

	u.Scheme = scheme

	return u, nil
}

// ProbeProxy actively tests an endpoint to determine if it speaks SOCKS5, HTTP CONNECT, or other protocols.
func ProbeProxy(ctx context.Context, addr string) (string, error) {
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))

	_, err = conn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		return "http", nil
	}

	resp := make([]byte, 2)

	n, err := conn.Read(resp)
	if err == nil && n == 2 && resp[0] == 0x05 {
		return "socks5h", nil
	}

	return "http", nil
}
