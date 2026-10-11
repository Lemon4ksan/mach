// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/codec/compress"
)

func TestCompression_AllCodecs_Roundtrip(t *testing.T) {
	t.Parallel()

	payload := []byte(
		"Modern zero-alloc compression engine validation across all 4 codecs: Gzip, Deflate, Brotli, Zstandard! 1234567890",
	)

	// 1. Gzip
	gz := compress.AppendGzipBytes(nil, payload)
	require.NotEmpty(t, gz)
	unGz, err := compress.AppendGunzipBytes(nil, gz)
	require.NoError(t, err)
	assert.Equal(t, payload, unGz)

	var gzBuf bytes.Buffer
	n, err := compress.WriteGzip(&gzBuf, payload)
	require.NoError(t, err)
	assert.Equal(t, len(payload), n)
	var gzOut bytes.Buffer
	_, err = compress.WriteGunzip(&gzOut, gzBuf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, payload, gzOut.Bytes())

	// 2. Deflate
	flateBytes := compress.AppendDeflateBytes(nil, payload)
	require.NotEmpty(t, flateBytes)
	unFlate, err := compress.AppendInflateBytes(nil, flateBytes)
	require.NoError(t, err)
	assert.Equal(t, payload, unFlate)

	var flateBuf bytes.Buffer
	n, err = compress.WriteDeflate(&flateBuf, payload)
	require.NoError(t, err)
	assert.Equal(t, len(payload), n)
	var flateOut bytes.Buffer
	_, err = compress.WriteInflate(&flateOut, flateBuf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, payload, flateOut.Bytes())

	// 3. Brotli
	br := compress.AppendBrotliBytes(nil, payload)
	require.NotEmpty(t, br)
	unBr, err := compress.AppendUnbrotliBytes(nil, br)
	require.NoError(t, err)
	assert.Equal(t, payload, unBr)

	var brBuf bytes.Buffer
	n, err = compress.WriteBrotli(&brBuf, payload)
	require.NoError(t, err)
	assert.True(t, n > 0)
	var brOut bytes.Buffer
	_, err = compress.WriteUnbrotli(&brOut, brBuf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, payload, brOut.Bytes())

	// 4. Zstd
	zstdBytes := compress.AppendZstdBytes(nil, payload)
	require.NotEmpty(t, zstdBytes)
	unZstd, err := compress.AppendUnzstdBytes(nil, zstdBytes)
	require.NoError(t, err)
	assert.Equal(t, payload, unZstd)

	var zstdBuf bytes.Buffer
	n, err = compress.WriteZstd(&zstdBuf, payload)
	require.NoError(t, err)
	assert.True(t, n > 0)
	var zstdOut bytes.Buffer
	_, err = compress.WriteUnzstd(&zstdOut, zstdBuf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, payload, zstdOut.Bytes())
}

func TestCompression_DecompressionBombLimits(t *testing.T) {
	t.Parallel()

	zeros := make([]byte, 100*1024)
	gz := compress.AppendGzipBytes(nil, zeros)
	flateBytes := compress.AppendDeflateBytes(nil, zeros)
	br := compress.AppendBrotliBytes(nil, zeros)
	zstdBytes := compress.AppendZstdBytes(nil, zeros)

	var out bytes.Buffer
	limit := 1024

	_, err := compress.WriteGunzipLimit(&out, gz, limit)
	require.Error(t, err)

	out.Reset()
	_, err = compress.WriteInflateLimit(&out, flateBytes, limit)
	require.Error(t, err)

	out.Reset()
	_, err = compress.WriteUnbrotliLimit(&out, br, limit)
	require.Error(t, err)

	out.Reset()
	_, err = compress.WriteUnzstdLimit(&out, zstdBytes, limit)
	require.Error(t, err)
}

func TestCompression_CorruptInput(t *testing.T) {
	t.Parallel()

	corrupt := []byte("completely_garbage_data_xyz_!@#$%^")
	var out bytes.Buffer

	_, err := compress.WriteGunzip(&out, corrupt)
	require.Error(t, err)

	_, err = compress.WriteInflate(&out, corrupt)
	require.Error(t, err)

	_, err = compress.WriteUnbrotli(&out, corrupt)
	require.Error(t, err)

	_, err = compress.WriteUnzstd(&out, corrupt)
	require.Error(t, err)
}

func TestCompression_Append_PreservesDst(t *testing.T) {
	t.Parallel()

	prefix := []byte("PREFIX:")
	payload := []byte("Preserve dst test payload 12345")

	gz := compress.AppendGzipBytes(nil, payload)
	unGz, err := compress.AppendGunzipBytes(slices.Clone(prefix), gz)
	require.NoError(t, err)
	assert.Equal(t, append(slices.Clone(prefix), payload...), unGz)

	flateBytes := compress.AppendDeflateBytes(nil, payload)
	unFlate, err := compress.AppendInflateBytes(slices.Clone(prefix), flateBytes)
	require.NoError(t, err)
	assert.Equal(t, append(slices.Clone(prefix), payload...), unFlate)

	br := compress.AppendBrotliBytes(nil, payload)
	unBr, err := compress.AppendUnbrotliBytes(slices.Clone(prefix), br)
	require.NoError(t, err)
	assert.Equal(t, append(slices.Clone(prefix), payload...), unBr)

	zstdBytes := compress.AppendZstdBytes(nil, payload)
	unZstd, err := compress.AppendUnzstdBytes(slices.Clone(prefix), zstdBytes)
	require.NoError(t, err)
	assert.Equal(t, append(slices.Clone(prefix), payload...), unZstd)
}
