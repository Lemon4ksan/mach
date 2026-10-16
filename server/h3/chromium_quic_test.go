// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"bytes"
	"errors"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

// TestChromium_Server_QPACK_DynamicTableCapacityExceeded verifies that if a client
// sends dynamic table capacity instructions on the QPACK encoder stream exceeding the
// server's advertised maximum capacity, the server records a QPACK_ENCODER_STREAM_ERROR (0x0201)
// (RFC 9204 §4.3.1, ported from Chromium QpackDecoderTest.TableCapacityMustNotExceedMaximum).
func TestChromium_Server_QPACK_DynamicTableCapacityExceeded(t *testing.T) {
	var reportedErr error
	codec := coreh3.NewQPACKCodecWithOptions(512, 50, func(err error) {
		reportedErr = err
	})

	// Encoder instruction setting capacity to 1024 (exceeding 512)
	exceedsData := []byte{0x3f, 0xe1, 0x07}
	_, err := codec.ReadEncoderStream(bytes.NewReader(exceedsData))
	require.Error(t, err)

	assert.NotNil(t, reportedErr)
	var qErr *coreh3.QPACKStreamError
	require.True(t, errors.As(reportedErr, &qErr))
	assert.Equal(t, coreh3.ErrCodeQpackEncoderStreamError, qErr.Code)
}

// TestChromium_Server_QPACK_ApplyClientSettings verifies that when client SETTINGS
// are decoded by the server, the server encoder's maximum dynamic table capacity
// and maximum blocked streams are strictly bounded (RFC 9204 §5).
func TestChromium_Server_QPACK_ApplyClientSettings(t *testing.T) {
	codec := coreh3.NewQPACKCodec()

	// Client advertises max table capacity 2048 and 10 blocked streams
	codec.ApplySettings(2048, 10)
	assert.Equal(t, uint64(2048), codec.Encoder().MaximumDynamicTableCapacity())
	assert.Equal(t, uint64(10), codec.Encoder().MaximumBlockedStreams())

	// A fresh codec where client advertises 0 table capacity (no dynamic table allowed)
	zeroCodec := coreh3.NewQPACKCodec()
	zeroCodec.ApplySettings(0, 0)
	assert.Equal(t, uint64(0), zeroCodec.Encoder().MaximumDynamicTableCapacity())
	assert.Equal(t, uint64(0), zeroCodec.Encoder().MaximumBlockedStreams())
}
