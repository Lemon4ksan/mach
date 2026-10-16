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

	"github.com/lemon4ksan/mach/proto/codec/qpack"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

// TestChromium_QPACK_DynamicTableCapacityLimitExceeded verifies that attempting to
// set the dynamic table capacity above the maximum advertised limit results in a
// QPACK_ENCODER_STREAM_ERROR (0x0201) connection error (RFC 9204 §4.3.1, ported from
// Chromium QpackDecoderTest.TableCapacityMustNotExceedMaximum).
func TestChromium_QPACK_DynamicTableCapacityLimitExceeded(t *testing.T) {
	var reportedErr error
	codec := coreh3.NewQPACKCodecWithOptions(1024, 100, func(err error) {
		reportedErr = err
	})

	// Encoder instruction 0x3f 0xe1 0x0f sets dynamic table capacity to 2048, which exceeds 1024
	exceedsData := []byte{0x3f, 0xe1, 0x0f}
	_, err := codec.ReadEncoderStream(bytes.NewReader(exceedsData))
	require.Error(t, err)

	assert.NotNil(t, reportedErr)
	var qErr *coreh3.QPACKStreamError
	require.True(t, errors.As(reportedErr, &qErr))
	assert.Equal(t, coreh3.ErrCodeQpackEncoderStreamError, qErr.Code)
	assert.True(t, qErr.IsEncoder)
}

// TestChromium_QPACK_ZeroDynamicTableCapacityRejectsAnyCapacity verifies that if
// SETTINGS_QPACK_MAX_TABLE_CAPACITY is zero (or default), any attempt to increase
// table capacity triggers QPACK_ENCODER_STREAM_ERROR (0x0201) (RFC 9204 §5, ported
// from Chromium QpackDecoderTest.ZeroCapacityRejectsEntries).
func TestChromium_QPACK_ZeroDynamicTableCapacityRejectsAnyCapacity(t *testing.T) {
	var reportedErr error
	codec := coreh3.NewQPACKCodecWithOptions(0, 0, func(err error) {
		reportedErr = err
	})

	// Encoder instruction 0x3f 0x01 attempts to set dynamic table capacity to 32 (> 0)
	capData := []byte{0x3f, 0x01}
	_, err := codec.ReadEncoderStream(bytes.NewReader(capData))
	require.Error(t, err)

	assert.NotNil(t, reportedErr)
	var qErr *coreh3.QPACKStreamError
	require.True(t, errors.As(reportedErr, &qErr))
	assert.Equal(t, coreh3.ErrCodeQpackEncoderStreamError, qErr.Code)
}

// TestChromium_QPACK_SettingsMaxTableCapacityPropagation verifies that SETTINGS frame
// parameters SETTINGS_QPACK_MAX_TABLE_CAPACITY and SETTINGS_QPACK_BLOCKED_STREAMS
// properly configure the local encoder's dynamic table limits (RFC 9204 §5).
func TestChromium_QPACK_SettingsMaxTableCapacityPropagation(t *testing.T) {
	codec := coreh3.NewQPACKCodec()

	// Initial SETTINGS advertisement with 8192 capacity and 50 blocked streams
	codec.ApplySettings(8192, 50)
	assert.Equal(t, uint64(8192), codec.Encoder().MaximumDynamicTableCapacity())
	assert.Equal(t, uint64(50), codec.Encoder().MaximumBlockedStreams())

	// Separate connection where peer advertises 0 capacity
	zeroCodec := coreh3.NewQPACKCodec()
	zeroCodec.ApplySettings(0, 0)
	assert.Equal(t, uint64(0), zeroCodec.Encoder().MaximumDynamicTableCapacity())
	assert.Equal(t, uint64(0), zeroCodec.Encoder().MaximumBlockedStreams())
}

// TestChromium_QPACK_StreamCancellationOnReset verifies that when a stream is cancelled
// or reset, a Stream Cancellation instruction is sent on the decoder stream and correctly
// processed by the peer encoder (RFC 9204 §2.2.2.2, ported from Chromium QpackStreamSenderTest).
func TestChromium_QPACK_StreamCancellationOnReset(t *testing.T) {
	var decoderStreamBuf bytes.Buffer

	// Sender delegate capturing instructions emitted by Decoder
	codec := coreh3.NewQPACKCodec()
	codec.Decoder().SetStreamSenderDelegate(qpack.StreamWriter(&decoderStreamBuf))

	// Stream 42 cancelled
	codec.Decoder().SendStreamCancellation(42)
	codec.Decoder().FlushDecoderStream()

	// Verify instruction was emitted to buffer
	require.Greater(t, decoderStreamBuf.Len(), 0)

	// Peer encoder processes decoder stream instruction
	peerCodec := coreh3.NewQPACKCodec()
	_, err := peerCodec.ReadDecoderStream(&decoderStreamBuf)
	require.NoError(t, err)
	assert.Nil(t, peerCodec.Err())
}
