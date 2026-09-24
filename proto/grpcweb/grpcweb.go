// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package grpcweb implements gRPC-Web wire framing and trailer validation strictly conforming to the gRPC-Web protocol specification.
package grpcweb

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Standard gRPC-Web frame flag masks.
const (
	// FlagCompressed indicates that the gRPC frame payload is compressed.
	FlagCompressed byte = 0x01

	// FlagTrailer indicates that the frame contains trailing metadata / status trailers (0x80).
	FlagTrailer byte = 0x80
)

var (
	// ErrPayloadTooLarge is returned when a binary frame length exceeds MaxPayloadSize.
	ErrPayloadTooLarge = errors.New("grpcweb: frame payload exceeds maximum allowed size")

	// ErrTruncatedHeader is returned when the 5-byte frame header is truncated.
	ErrTruncatedHeader = errors.New("grpcweb: truncated 5-byte frame header")

	// ErrTruncatedPayload is returned when the frame payload stream is truncated.
	ErrTruncatedPayload = errors.New("grpcweb: truncated frame payload")

	// ErrInvalidFrame is returned when the gRPC-Web frame is malformed.
	ErrInvalidFrame = errors.New("grpcweb: invalid frame format")

	// ErrStatusError is returned when grpc-status trailer indicates a non-zero error code.
	ErrStatusError = errors.New("grpcweb: non-zero grpc-status received")
)

// GRPCWebError encapsulates detailed error metadata returned in gRPC-Web trailers.
type GRPCWebError struct {
	Op            string
	StatusCode    string
	StatusMsg     string
	StatusDetails []byte
	Err           error
}

func (e *GRPCWebError) Error() string {
	if e.StatusCode != "" {
		if e.StatusMsg != "" {
			return fmt.Sprintf("grpc-web status %s: %s", e.StatusCode, e.StatusMsg)
		}

		return "grpc-web status " + e.StatusCode
	}

	if e.Op != "" {
		return fmt.Sprintf("grpc-web %s: %v", e.Op, e.Err)
	}

	return e.Err.Error()
}

func (e *GRPCWebError) Unwrap() error {
	return e.Err
}

// Framer decodes and encodes 5-byte length-prefixed gRPC-Web frames:
// [1 byte flags][4 bytes length (BigEndian)][payload bytes].
type Framer struct {
	MaxPayloadSize uint32
}

// NewFramer initializes a [Framer]. If maxPayloadSize is 0, default 16MB cap is applied.
func NewFramer(maxPayloadSize uint32) *Framer {
	if maxPayloadSize == 0 {
		maxPayloadSize = 16 * 1024 * 1024
	}

	return &Framer{MaxPayloadSize: maxPayloadSize}
}

// ReadFrame reads a 5-byte header [flags + uint32 length] and payload from r.
func (f *Framer) ReadFrame(r io.Reader) (flags byte, payload []byte, err error) {
	var header [5]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return 0, nil, err
		}

		return 0, nil, ErrTruncatedHeader
	}

	flags = header[0]
	length := binary.BigEndian.Uint32(header[1:5])

	if f.MaxPayloadSize > 0 && length > f.MaxPayloadSize {
		return 0, nil, ErrPayloadTooLarge
	}

	if length == 0 {
		return flags, nil, nil
	}

	payload = make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, ErrTruncatedPayload
	}

	return flags, payload, nil
}

// WriteFrame encodes and writes a 5-byte header [flags + uint32 length] and payload to w.
func (f *Framer) WriteFrame(w io.Writer, flags byte, payload []byte) (int, error) {
	length := uint32(len(payload))
	if f.MaxPayloadSize > 0 && length > f.MaxPayloadSize {
		return 0, ErrPayloadTooLarge
	}

	var header [5]byte
	header[0] = flags
	binary.BigEndian.PutUint32(header[1:5], length)

	n1, err := w.Write(header[:])
	if err != nil {
		return n1, err
	}

	if length == 0 {
		return n1, nil
	}

	n2, err := w.Write(payload)

	return n1 + n2, err
}

// VerifyTrailer inspects raw gRPC-Web trailer lines and returns a [GRPCWebError] if grpc-status != 0.
func VerifyTrailer(trailerPayload []byte) error {
	var (
		statusCode, statusMsg string
		statusDetails         []byte
	)

	for len(trailerPayload) > 0 {
		var line []byte

		if idx := bytes.IndexByte(trailerPayload, '\n'); idx >= 0 {
			line = trailerPayload[:idx]
			trailerPayload = trailerPayload[idx+1:]
		} else {
			line = trailerPayload
			trailerPayload = nil
		}

		keyBytes, valBytes, ok := parseTrailerKeyValue(line)
		if !ok {
			continue
		}

		keyStr := bytesconv.B2S(keyBytes)

		switch {
		case strings.EqualFold(keyStr, "grpc-status"):
			statusCode = bytesconv.B2S(valBytes)
		case strings.EqualFold(keyStr, "grpc-message"):
			statusMsg = bytesconv.B2S(valBytes)
		case strings.EqualFold(keyStr, "grpc-status-details-bin"):
			valStr := bytesconv.B2S(valBytes)
			if decoded, err := base64.RawStdEncoding.DecodeString(valStr); err == nil {
				statusDetails = decoded
			} else if decoded, err := base64.StdEncoding.DecodeString(valStr); err == nil {
				statusDetails = decoded
			}
		}
	}

	if statusCode != "" && statusCode != "0" {
		return &GRPCWebError{
			StatusCode:    statusCode,
			StatusMsg:     statusMsg,
			StatusDetails: statusDetails,
			Err:           ErrStatusError,
		}
	}

	return nil
}

func parseTrailerKeyValue(line []byte) (keyBytes, valBytes []byte, ok bool) {
	before, after, ok := bytes.Cut(line, []byte{':'})
	if !ok {
		return nil, nil, false
	}

	return bytes.TrimSpace(before), bytes.TrimSpace(after), true
}

// IsBase64Header checks whether a 5-byte header prefix matches Base64 text encoding.
func IsBase64Header(header []byte) bool {
	if len(header) < 5 {
		return false
	}

	first := header[0]

	return (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z') || (first >= '0' && first <= '9')
}
