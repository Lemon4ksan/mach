// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	machcompress "github.com/lemon4ksan/mach/proto/codec/compress"
)

func TestBodyCompress_PanicRecovery(t *testing.T) {
	t.Parallel()

	panickingCompress := func(sw *bufio.Writer, bodyStream io.Reader, level int) error {
		panic("simulated compressor panic")
	}

	stream := strings.NewReader("sample payload")
	cs := newCompressedBodyStream(stream, 6, panickingCompress)

	buf := make([]byte, 128)
	_, _ = cs.Read(buf)

	err := cs.Close()
	require.Error(t, err)

	var pErr *ErrBodyStreamWritePanic
	assert.True(t, errors.As(err, &pErr))
	assert.Contains(t, err.Error(), "simulated compressor panic")
}

func TestBodyCompress_BrotliAndZstdStreaming(t *testing.T) {
	t.Parallel()

	raw := []byte("Streaming compression test payload for brotli and zstandard engines 1234567890!")

	// 1. Brotli stream
	var brBuf bytes.Buffer
	bw := bufio.NewWriter(&brBuf)
	err := compressBrotliBodyStream(bw, bytes.NewReader(raw), machcompress.CompressBrotliDefaultCompression)
	require.NoError(t, err)
	_ = bw.Flush()

	uncompressedBr, err := machcompress.AppendUnbrotliBytes(nil, brBuf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, raw, uncompressedBr)

	// 2. Zstd stream
	var zstdBuf bytes.Buffer
	zw := bufio.NewWriter(&zstdBuf)
	err = compressZstdBodyStream(zw, bytes.NewReader(raw), machcompress.CompressZstdDefault)
	require.NoError(t, err)
	_ = zw.Flush()

	uncompressedZstd, err := machcompress.AppendUnzstdBytes(nil, zstdBuf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, raw, uncompressedZstd)
}

func TestRequestBody_ContentEncodingNormalization(t *testing.T) {
	t.Parallel()

	raw := []byte("Request body content encoding normalization test")
	gzBytes := machcompress.AppendGzipBytes(nil, raw)

	// Test uppercase GZIP
	var req Request
	req.Header.Set("Content-Encoding", "GZIP")
	req.SetBody(gzBytes)

	res, err := req.BodyUncompressedWithLimit(1024)
	require.NoError(t, err)
	assert.Equal(t, raw, res)

	// Test x-gzip alias
	req.Header.Set("Content-Encoding", "x-gzip")
	res, err = req.BodyUncompressedWithLimit(1024)
	require.NoError(t, err)
	assert.Equal(t, raw, res)

	// Test whitespace padding
	req.Header.Set("Content-Encoding", "  gzip  ")
	res, err = req.BodyUncompressedWithLimit(1024)
	require.NoError(t, err)
	assert.Equal(t, raw, res)

	// Test identity
	req.Header.Set("Content-Encoding", "identity")
	req.SetBody(raw)
	res, err = req.BodyUncompressedWithLimit(1024)
	require.NoError(t, err)
	assert.Equal(t, raw, res)
}

func TestResponseBody_ContentEncodingNormalization(t *testing.T) {
	t.Parallel()

	raw := []byte("Response body content encoding normalization test")
	brBytes := machcompress.AppendBrotliBytes(nil, raw)

	var resp Response
	resp.Header.Set("Content-Encoding", "BR")
	resp.SetBody(brBytes)

	res, err := resp.BodyUncompressedWithLimit(1024)
	require.NoError(t, err)
	assert.Equal(t, raw, res)

	// Test zstd
	zstdBytes := machcompress.AppendZstdBytes(nil, raw)
	resp.Header.Set("Content-Encoding", "ZSTD")
	resp.SetBody(zstdBytes)

	res, err = resp.BodyUncompressedWithLimit(1024)
	require.NoError(t, err)
	assert.Equal(t, raw, res)
}

func TestBodyUncompressed_DecompressionBombLimit(t *testing.T) {
	t.Parallel()

	zeros := make([]byte, 50*1024)
	gz := machcompress.AppendGzipBytes(nil, zeros)

	var req Request
	req.Header.Set("Content-Encoding", "gzip")
	req.SetBody(gz)

	// Limit to 512 bytes
	_, err := req.BodyUncompressedWithLimit(512)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBodyTooLarge)

	var resp Response
	resp.Header.Set("Content-Encoding", "gzip")
	resp.SetBody(gz)

	_, err = resp.BodyUncompressedWithLimit(512)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBodyTooLarge)
}
