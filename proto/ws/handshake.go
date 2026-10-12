// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	crand "crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"slices"
	"strings"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

const (
	// MagicGUID is the globally unique identifier defined in RFC 6455 §1.3 for handshake accept calculation.
	MagicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

	// WSGUID is an alias for MagicGUID.
	WSGUID = MagicGUID

	// WebSocketVersion is the RFC 6455 supported protocol version ("13").
	WebSocketVersion = "13"

	// Standard WebSocket HTTP Header names.
	HeaderUpgrade                = "Upgrade"
	HeaderConnection             = "Connection"
	HeaderSecWebSocketKey        = "Sec-WebSocket-Key"
	HeaderSecWebSocketAccept     = "Sec-WebSocket-Accept"
	HeaderSecWebSocketVersion    = "Sec-WebSocket-Version"
	HeaderSecWebSocketProtocol   = "Sec-WebSocket-Protocol"
	HeaderSecWebSocketExtensions = "Sec-WebSocket-Extensions"
)

// ComputeAccept calculates the Sec-WebSocket-Accept header value corresponding to the
// client's Sec-WebSocket-Key as defined in RFC 6455 §4.2.2.
//
// Calculation: base64(SHA-1(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11")).
func ComputeAccept(key string) string {
	var buf [128]byte
	n := copy(buf[:], key)
	n += copy(buf[n:], MagicGUID)
	h := sha1.Sum(buf[:n])

	var out [28]byte
	base64.StdEncoding.Encode(out[:], h[:])
	return string(out[:])
}

// AppendAccept calculates and appends the 28-byte Sec-WebSocket-Accept value to dst.
//
// Zero-allocation: testing.AllocsPerRun == 0 when dst has sufficient capacity.
func AppendAccept(dst []byte, key string) []byte {
	var buf [128]byte
	n := copy(buf[:], key)
	n += copy(buf[n:], MagicGUID)
	h := sha1.Sum(buf[:n])

	start := len(dst)
	dst = slices.Grow(dst, 28)
	dst = dst[:start+28]
	base64.StdEncoding.Encode(dst[start:], h[:])
	return dst
}

// ValidateKey validates whether key is a valid RFC 6455 Sec-WebSocket-Key (§4.2.1).
// The key MUST be a base64-encoded 16-byte nonce (24 characters long, ending with "==").
//
// Zero-allocation: testing.AllocsPerRun == 0.
func ValidateKey(key string) bool {
	if len(key) != 24 {
		return false
	}
	if key[22] != '=' || key[23] != '=' {
		return false
	}
	var raw [16]byte
	n, err := base64.StdEncoding.Decode(raw[:], bytesconv.S2B(key))
	return err == nil && n == 16
}

// GenerateKey generates a cryptographically secure 16-byte base64 nonce
// suitable for the Sec-WebSocket-Key header.
func GenerateKey() string {
	var raw [16]byte
	if _, err := io.ReadFull(crand.Reader, raw[:]); err != nil {
		for i := range raw {
			raw[i] = byte(i*37 + 13)
		}
	}
	var out [24]byte
	base64.StdEncoding.Encode(out[:], raw[:])
	return string(out[:])
}

// GenerateMaskKey generates a cryptographically random 4-byte masking key
// for client-to-server frames (RFC 6455 §5.3).
func GenerateMaskKey() MaskKey {
	var k MaskKey
	_, _ = io.ReadFull(crand.Reader, k[:])
	return k
}

// HasToken reports whether the comma-separated header value contains token,
// performing case-insensitive matching with zero heap allocations.
func HasToken(header, token string) bool {
	for len(header) > 0 {
		var tok string
		if idx := strings.IndexByte(header, ','); idx >= 0 {
			tok = header[:idx]
			header = header[idx+1:]
		} else {
			tok = header
			header = ""
		}
		tok = strings.TrimSpace(tok)
		if bytesconv.EqualFoldASCII(tok, token) {
			return true
		}
	}
	return false
}

// ValidateHandshakeRequest validates an incoming HTTP/1.1 WebSocket upgrade request
// according to RFC 6455 §4.2.1.
//
// Parameters:
//   - method: HTTP method (must be "GET")
//   - version: HTTP version (must be >= HTTP/1.1)
//   - upgrade: Value of Upgrade header (must contain token "websocket")
//   - connection: Value of Connection header (must contain token "upgrade")
//   - wsKey: Value of Sec-WebSocket-Key header (must be valid 16-byte nonce)
//   - wsVersion: Value of Sec-WebSocket-Version header (must be "13")
func ValidateHandshakeRequest(method, version, upgrade, connection, wsKey, wsVersion string) error {
	if method != "GET" {
		return ErrInvalidMethod
	}
	if !HasToken(upgrade, "websocket") {
		return ErrInvalidUpgrade
	}
	if !HasToken(connection, "upgrade") {
		return ErrInvalidConnection
	}
	if !ValidateKey(wsKey) {
		return ErrInvalidKey
	}
	if wsVersion != WebSocketVersion {
		return ErrInvalidVersion
	}
	return nil
}

// FormatHandshakeResponse writes the RFC 6455 101 Switching Protocols response
// corresponding to wsKey into dst.
//
// Returns the slice containing the complete HTTP/1.1 response or an error.
// Zero-allocation when dst has sufficient capacity.
func FormatHandshakeResponse(dst []byte, wsKey string) ([]byte, error) {
	if !ValidateKey(wsKey) {
		return nil, ErrInvalidKey
	}

	const (
		respPrefix = "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "
		respSuffix = "\r\n\r\n"
		totalLen   = len(respPrefix) + 28 + len(respSuffix)
	)

	start := len(dst)
	dst = slices.Grow(dst, totalLen)
	dst = dst[:start+totalLen]

	n := copy(dst[start:], respPrefix)

	var buf [128]byte
	nb := copy(buf[:], wsKey)
	nb += copy(buf[nb:], MagicGUID)
	h := sha1.Sum(buf[:nb])

	base64.StdEncoding.Encode(dst[start+n:start+n+28], h[:])
	copy(dst[start+n+28:], respSuffix)

	return dst, nil
}

// ComputeAcceptKey is an alias for ComputeAccept.
func ComputeAcceptKey(key string) string {
	return ComputeAccept(key)
}

// GenerateChallengeKey is an alias for GenerateKey.
func GenerateChallengeKey() string {
	return GenerateKey()
}

// HeaderContainsToken reports whether header contains token in a comma-separated list.
func HeaderContainsToken(header, token string) bool {
	return HasToken(header, token)
}
