// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import "errors"

var (
	// ErrIncomplete indicates that the buffer contains an incomplete frame header.
	ErrIncomplete = errors.New("ws: incomplete frame header")

	// ErrInvalidLength indicates a protocol error where payload length was not encoded minimally
	// according to RFC 6455 §5.2.
	ErrInvalidLength = errors.New("ws: non-minimal extended payload length encoding")

	// ErrPayloadTooLarge indicates that the payload length exceeds the maximum allowed value
	// (most significant bit is set in 64-bit extended length or exceeds system limits).
	ErrPayloadTooLarge = errors.New("ws: payload length exceeds maximum allowed")

	// ErrControlFrameTooLarge indicates a control frame with payload exceeding 125 bytes (RFC 6455 §5.5).
	ErrControlFrameTooLarge = errors.New("ws: control frame payload exceeds 125 bytes")

	// ErrControlFrameFragmented indicates a fragmented control frame (FIN=0), which is forbidden by RFC 6455 §5.5.
	ErrControlFrameFragmented = errors.New("ws: control frame must not be fragmented")

	// ErrBufferTooSmall indicates that the destination buffer has insufficient capacity.
	ErrBufferTooSmall = errors.New("ws: destination buffer too small")

	// ErrInvalidClosePayload indicates a Close control frame payload of invalid length (1 byte).
	ErrInvalidClosePayload = errors.New("ws: invalid close frame payload length")

	// ErrInvalidCloseCode indicates a Close control frame status code that is reserved or forbidden on wire.
	ErrInvalidCloseCode = errors.New("ws: invalid close status code")

	// ErrInvalidUTF8 indicates invalid UTF-8 data in a Text frame or Close reason payload.
	ErrInvalidUTF8 = errors.New("ws: invalid utf-8 payload")

	// ErrInvalidMethod indicates a handshake HTTP request method other than GET.
	ErrInvalidMethod = errors.New("ws: handshake request method must be GET")

	// ErrInvalidUpgrade indicates a missing or invalid Upgrade header in the opening handshake.
	ErrInvalidUpgrade = errors.New("ws: invalid or missing Upgrade header")

	// ErrInvalidConnection indicates a missing or invalid Connection header in the opening handshake.
	ErrInvalidConnection = errors.New("ws: invalid or missing Connection header")

	// ErrInvalidKey indicates a missing or malformed Sec-WebSocket-Key header.
	ErrInvalidKey = errors.New("ws: invalid or missing Sec-WebSocket-Key")

	// ErrInvalidVersion indicates an unsupported Sec-WebSocket-Version (only 13 is supported).
	ErrInvalidVersion = errors.New("ws: unsupported Sec-WebSocket-Version (expected 13)")

	// ErrInvalidOpcode indicates an unknown or reserved opcode.
	ErrInvalidOpcode = errors.New("ws: unknown or reserved opcode")
)
