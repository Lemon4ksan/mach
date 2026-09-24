// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpcweb_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/grpcweb"
)

func TestGRPCWeb_Framing_Roundtrip_And_Errors(t *testing.T) {
	t.Parallel()

	framer := grpcweb.NewFramer(1024)

	// 1. Valid write and read
	var buf bytes.Buffer
	payload := []byte("hello grpc-web payload")
	n, err := framer.WriteFrame(&buf, 0x00, payload)
	require.NoError(t, err)
	assert.Equal(t, 5+len(payload), n)

	flags, readPayload, err := framer.ReadFrame(&buf)
	require.NoError(t, err)
	assert.Equal(t, byte(0x00), flags)
	assert.Equal(t, payload, readPayload)

	// 2. Empty payload (length = 0)
	buf.Reset()
	n, err = framer.WriteFrame(&buf, grpcweb.FlagCompressed, nil)
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	flags, readEmpty, err := framer.ReadFrame(&buf)
	require.NoError(t, err)
	assert.Equal(t, grpcweb.FlagCompressed, flags)
	assert.Nil(t, readEmpty)

	// 3. Read on EOF
	_, _, err = framer.ReadFrame(&buf)
	assert.ErrorIs(t, err, io.EOF)

	// 4. Truncated 5-byte header
	shortHeaderBuf := bytes.NewBuffer([]byte{0x00, 0x00})
	_, _, err = framer.ReadFrame(shortHeaderBuf)
	assert.ErrorIs(t, err, grpcweb.ErrTruncatedHeader)

	// 5. Truncated payload
	var truncBuf bytes.Buffer
	_, _ = framer.WriteFrame(&truncBuf, 0x00, []byte("long message"))
	truncData := truncBuf.Bytes()[:8] // truncates payload
	_, _, err = framer.ReadFrame(bytes.NewReader(truncData))
	assert.ErrorIs(t, err, grpcweb.ErrTruncatedPayload)

	// 6. Payload too large
	bigPayload := make([]byte, 2048)
	_, err = framer.WriteFrame(&buf, 0x00, bigPayload)
	assert.ErrorIs(t, err, grpcweb.ErrPayloadTooLarge)
}

func TestGRPCWeb_Trailer_Validation_And_Details(t *testing.T) {
	t.Parallel()

	// 1. OK status (0)
	okTrailer := []byte("grpc-status: 0\r\ngrpc-message: OK\r\n")
	require.NoError(t, grpcweb.VerifyTrailer(okTrailer))

	// 2. Error status (7 = PERMISSION_DENIED) with details bin
	rawDetails := []byte("error detail binary")
	b64Details := base64.StdEncoding.EncodeToString(rawDetails)
	errTrailer := []byte(
		"line without colon\ngrpc-status: 7\ngrpc-message: Permission denied\ngrpc-status-details-bin: " +
			b64Details + "\n",
	)

	err := grpcweb.VerifyTrailer(errTrailer)
	require.Error(t, err)

	var grpcErr *grpcweb.GRPCWebError
	require.ErrorAs(t, err, &grpcErr)
	assert.Equal(t, "7", grpcErr.StatusCode)
	assert.Equal(t, "Permission denied", grpcErr.StatusMsg)
	assert.Equal(t, rawDetails, grpcErr.StatusDetails)
	assert.Equal(t, "grpc-web status 7: Permission denied", grpcErr.Error())
	assert.ErrorIs(t, grpcErr.Unwrap(), grpcweb.ErrStatusError)

	// 3. Error formatting variants
	errOnlyCode := &grpcweb.GRPCWebError{StatusCode: "14"}
	assert.Equal(t, "grpc-web status 14", errOnlyCode.Error())

	errWithOp := &grpcweb.GRPCWebError{Op: "dial", Err: errors.New("conn refused")}
	assert.Equal(t, "grpc-web dial: conn refused", errWithOp.Error())

	errBare := &grpcweb.GRPCWebError{Err: errors.New("bare err")}
	assert.Equal(t, "bare err", errBare.Error())
}

func TestIsBase64Header(t *testing.T) {
	t.Parallel()

	assert.False(t, grpcweb.IsBase64Header(nil))
	assert.False(t, grpcweb.IsBase64Header([]byte("123")))
	assert.True(t, grpcweb.IsBase64Header([]byte("AAAAA")))
	assert.True(t, grpcweb.IsBase64Header([]byte("aaaaa")))
	assert.True(t, grpcweb.IsBase64Header([]byte("00000")))
	assert.False(t, grpcweb.IsBase64Header([]byte{0x00, 0x00, 0x00, 0x00, 0x01}))
}
