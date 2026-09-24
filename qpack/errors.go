// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"errors"
	"fmt"
)

// ErrorCode identifies RFC 9204 and HTTP/3 QPACK error conditions.
type ErrorCode uint64

const (
	// ErrCodeNoError indicates no error occurred.
	ErrCodeNoError ErrorCode = 0
	// ErrCodeInternalError indicates an internal implementation error.
	ErrCodeInternalError ErrorCode = 1
	// ErrCodeDecompressionFailed indicates a fatal decompression failure (RFC 9204 §2.2.3.1).
	ErrCodeDecompressionFailed ErrorCode = 0x0200
	// ErrCodeEncoderStreamError indicates an error on the encoder stream (RFC 9204 §2.2.3.2).
	ErrCodeEncoderStreamError ErrorCode = 0x0201
	// ErrCodeDecoderStreamError indicates an error on the decoder stream (RFC 9204 §2.2.3.3).
	ErrCodeDecoderStreamError ErrorCode = 0x0202

	// Detailed RFC 9204 / QUIC encoder stream error subcodes.
	ErrCodeEncoderIntegerTooLarge               ErrorCode = 174
	ErrCodeEncoderStringLiteralTooLong          ErrorCode = 175
	ErrCodeEncoderHuffmanEncodingError          ErrorCode = 176
	ErrCodeEncoderInvalidStaticEntry            ErrorCode = 177
	ErrCodeEncoderErrorInsertingStatic          ErrorCode = 178
	ErrCodeEncoderInsertionInvalidRelativeIndex ErrorCode = 179
	ErrCodeEncoderInsertionDynamicEntryNotFound ErrorCode = 180
	ErrCodeEncoderErrorInsertingDynamic         ErrorCode = 181
	ErrCodeEncoderErrorInsertingLiteral         ErrorCode = 182
	ErrCodeEncoderDuplicateInvalidRelativeIndex ErrorCode = 183
	ErrCodeEncoderDuplicateDynamicEntryNotFound ErrorCode = 184
	ErrCodeEncoderSetDynamicTableCapacity       ErrorCode = 185

	// Detailed RFC 9204 / QUIC decoder stream error subcodes.
	ErrCodeDecoderIntegerTooLarge          ErrorCode = 186
	ErrCodeDecoderInvalidZeroIncrement     ErrorCode = 187
	ErrCodeDecoderIncrementOverflow        ErrorCode = 188
	ErrCodeDecoderImpossibleInsertCount    ErrorCode = 189
	ErrCodeDecoderIncorrectAcknowledgement ErrorCode = 190
)

// Standard Go sentinel errors for QPACK.
var (
	// ErrDecompressionFailed is returned when header block decompression fails.
	ErrDecompressionFailed = errors.New("qpack: decompression failed")
	// ErrEncoderStream is returned when an error occurs on the encoder control stream.
	ErrEncoderStream = errors.New("qpack: encoder stream error")
	// ErrDecoderStream is returned when an error occurs on the decoder control stream.
	ErrDecoderStream = errors.New("qpack: decoder stream error")
	// ErrIntegerOverflow is returned when an integer exceeds the maximum representable range.
	ErrIntegerOverflow = errors.New("qpack: integer overflow")
	// ErrInvalidInteger is returned when prefix integer parameters or bytes are invalid.
	ErrInvalidInteger = errors.New("qpack: invalid prefix integer")
	// ErrStringLiteralTooLong is returned when a string literal exceeds maximum permitted length.
	ErrStringLiteralTooLong = errors.New("qpack: string literal too long")
	// ErrHuffman is returned when Huffman-encoded data is malformed.
	ErrHuffman = errors.New("qpack: huffman decoding error")
	// ErrCapacityExceeded is returned when dynamic table capacity limit is exceeded.
	ErrCapacityExceeded = errors.New("qpack: dynamic table capacity exceeded")
	// ErrStreamBlocked is returned when the blocked stream count exceeds the allowed limit.
	ErrStreamBlocked = errors.New("qpack: blocked stream limit exceeded")
	// ErrEntryNotFound is returned when a dynamic table index does not resolve to an entry.
	ErrEntryNotFound = errors.New("qpack: dynamic table entry not found")
)

// Error represents a structured QPACK protocol error.
type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

// Error formats the QPACK error as a string.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err != nil {
		return fmt.Sprintf("qpack: error %d: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("qpack: error %d: %s", e.Code, e.Message)
}

// Unwrap returns the underlying wrapped error, if any.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Is reports whether this error matches target.
func (e *Error) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	if qerr, ok := target.(*Error); ok {
		return e.Code == qerr.Code
	}
	if e.Err != nil && errors.Is(e.Err, target) {
		return true
	}
	switch target {
	case ErrDecompressionFailed:
		return e.Code == ErrCodeDecompressionFailed
	case ErrEncoderStream:
		switch e.Code {
		case ErrCodeEncoderStreamError,
			ErrCodeEncoderIntegerTooLarge,
			ErrCodeEncoderStringLiteralTooLong,
			ErrCodeEncoderHuffmanEncodingError,
			ErrCodeEncoderInvalidStaticEntry,
			ErrCodeEncoderErrorInsertingStatic,
			ErrCodeEncoderInsertionInvalidRelativeIndex,
			ErrCodeEncoderInsertionDynamicEntryNotFound,
			ErrCodeEncoderErrorInsertingDynamic,
			ErrCodeEncoderErrorInsertingLiteral,
			ErrCodeEncoderDuplicateInvalidRelativeIndex,
			ErrCodeEncoderDuplicateDynamicEntryNotFound,
			ErrCodeEncoderSetDynamicTableCapacity:
			return true
		}
	case ErrDecoderStream:
		switch e.Code {
		case ErrCodeDecoderStreamError,
			ErrCodeDecoderIntegerTooLarge,
			ErrCodeDecoderInvalidZeroIncrement,
			ErrCodeDecoderIncrementOverflow,
			ErrCodeDecoderImpossibleInsertCount,
			ErrCodeDecoderIncorrectAcknowledgement:
			return true
		}
	case ErrIntegerOverflow:
		return e.Code == ErrCodeEncoderIntegerTooLarge || e.Code == ErrCodeDecoderIntegerTooLarge
	case ErrStringLiteralTooLong:
		return e.Code == ErrCodeEncoderStringLiteralTooLong
	case ErrHuffman:
		return e.Code == ErrCodeEncoderHuffmanEncodingError
	case ErrCapacityExceeded:
		return e.Code == ErrCodeEncoderSetDynamicTableCapacity
	case ErrEntryNotFound:
		return e.Code == ErrCodeEncoderInsertionDynamicEntryNotFound ||
			e.Code == ErrCodeEncoderDuplicateDynamicEntryNotFound
	}
	return false
}

// NewError constructs a new *Error with code and message.
func NewError(code ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WrapError constructs a new *Error wrapping an underlying error.
func WrapError(code ErrorCode, message string, err error) *Error {
	return &Error{Code: code, Message: message, Err: err}
}
