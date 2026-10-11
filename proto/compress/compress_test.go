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

	"github.com/lemon4ksan/mach/proto/compress"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

func TestH1Engine_Compression_Gzip(t *testing.T) {
	t.Parallel()

	payload := []byte("Hello Gzip Compression via foundation compress engine! 1234567890")

	// 1. compress.AppendGzipBytes / compress.AppendGunzipBytes
	compressed := compress.AppendGzipBytes(nil, payload)
	require.NotEmpty(t, compressed)

	uncompressed, err := compress.AppendGunzipBytes(nil, compressed)
	require.NoError(t, err)
	assert.Equal(t, payload, uncompressed)

	// 2. Stream writers
	var buf bytes.Buffer

	n, err := compress.WriteGzip(&buf, payload)
	require.NoError(t, err)
	assert.Equal(t, len(payload), n)

	var out bytes.Buffer

	_, err = compress.WriteGunzip(&out, buf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, payload, out.Bytes())
}

func TestH1Engine_Compression_Brotli(t *testing.T) {
	t.Parallel()

	payload := []byte("Hello Brotli Compression via foundation compress engine! ABCDEFGHIJKLMNOPQRSTUVWXYZ")

	// 1. compress.AppendBrotliBytes / compress.AppendUnbrotliBytes
	compressed := compress.AppendBrotliBytes(nil, payload)
	require.NotEmpty(t, compressed)

	uncompressed, err := compress.AppendUnbrotliBytes(nil, compressed)
	require.NoError(t, err)
	assert.Equal(t, payload, uncompressed)

	// 2. Stream writers
	var buf bytes.Buffer

	n, err := compress.WriteBrotli(&buf, payload)
	require.NoError(t, err)
	assert.True(t, n > 0)

	var out bytes.Buffer

	_, err = compress.WriteUnbrotli(&out, buf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, payload, out.Bytes())
}

func TestH1Engine_Compression_Zstd(t *testing.T) {
	t.Parallel()

	payload := []byte("Hello Zstandard Compression via foundation compress engine! 9876543210")

	// 1. compress.AppendZstdBytes / compress.AppendUnzstdBytes
	compressed := compress.AppendZstdBytes(nil, payload)
	require.NotEmpty(t, compressed)

	uncompressed, err := compress.AppendUnzstdBytes(nil, compressed)
	require.NoError(t, err)
	assert.Equal(t, payload, uncompressed)

	var out bytes.Buffer

	_, err = compress.WriteUnzstd(&out, compressed)
	require.NoError(t, err)
	assert.Equal(t, payload, out.Bytes())
}

func TestH1Engine_Compression_Deflate(t *testing.T) {
	t.Parallel()

	payload := []byte("Hello Deflate Compression via foundation compress engine! @#$%^&*()")

	// 1. AppendDeflateBytes / AppendInflateBytes
	compressed := compress.AppendDeflateBytes(nil, payload)
	require.NotEmpty(t, compressed)

	var out bytes.Buffer

	_, err := compress.WriteInflate(&out, compressed)
	require.NoError(t, err)
	assert.Equal(t, payload, out.Bytes())
}

func TestH1Engine_Response_BodyUncompressed(t *testing.T) {
	t.Parallel()

	raw := []byte("Response uncompressed payload testing across all algorithms")

	tests := []struct {
		name     string
		encoding string
		compress func(src []byte) []byte
	}{
		{
			name:     "gzip",
			encoding: "gzip",
			compress: func(src []byte) []byte {
				return compress.AppendGzipBytes(nil, src)
			},
		},
		{
			name:     "br",
			encoding: "br",
			compress: func(src []byte) []byte {
				return compress.AppendBrotliBytes(nil, src)
			},
		},
		{
			name:     "zstd",
			encoding: "zstd",
			compress: func(src []byte) []byte {
				return compress.AppendZstdBytes(nil, src)
			},
		},
		{
			name:     "identity",
			encoding: "identity",
			compress: func(src []byte) []byte {
				return src
			},
		},
		{
			name:     "empty",
			encoding: "",
			compress: func(src []byte) []byte {
				return src
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var resp h1.Response
			resp.Header.Set("Content-Encoding", tc.encoding)
			resp.SetBody(tc.compress(raw))

			res, err := resp.BodyUncompressed()
			require.NoError(t, err)
			assert.Equal(t, raw, res)
		})
	}

	t.Run("unsupported_encoding", func(t *testing.T) {
		var resp h1.Response
		resp.Header.Set("Content-Encoding", "unknown-alg-xyz")
		resp.SetBody([]byte("some data"))

		_, err := resp.BodyUncompressed()
		require.Error(t, err)
		assert.ErrorIs(t, err, h1.ErrContentEncodingUnsupported)
	})
}

func TestH1Engine_Request_BodyUncompressed(t *testing.T) {
	t.Parallel()

	raw := []byte("Request uncompressed payload testing")

	var req h1.Request
	req.Header.Set("Content-Encoding", "gzip")
	req.SetBody(compress.AppendGzipBytes(nil, raw))

	res, err := req.BodyUncompressed()
	require.NoError(t, err)
	assert.Equal(t, raw, res)
}

func TestCompression_Append_PreservesDst(t *testing.T) {
	t.Parallel()

	prefix := []byte("PREFIX:")
	payload := []byte("Appended payload content 1234567890")

	// 1. Gzip
	gzComp := compress.AppendGzipBytes(nil, payload)
	gzAppended, err := compress.AppendGunzipBytes(slices.Clone(prefix), gzComp)
	require.NoError(t, err)
	assert.Equal(t, append(slices.Clone(prefix), payload...), gzAppended)

	// 2. Brotli
	brComp := compress.AppendBrotliBytes(nil, payload)
	brAppended, err := compress.AppendUnbrotliBytes(slices.Clone(prefix), brComp)
	require.NoError(t, err)
	assert.Equal(t, append(slices.Clone(prefix), payload...), brAppended)

	// 3. Zstd
	zstdComp := compress.AppendZstdBytes(nil, payload)
	zstdAppended, err := compress.AppendUnzstdBytes(slices.Clone(prefix), zstdComp)
	require.NoError(t, err)
	assert.Equal(t, append(slices.Clone(prefix), payload...), zstdAppended)

	// Also test AppendZstdBytes preserves dst during compression
	zstdCompWithPrefix := compress.AppendZstdBytes(slices.Clone(prefix), payload)
	assert.True(t, bytes.HasPrefix(zstdCompWithPrefix, prefix))
	assert.Equal(t, append(slices.Clone(prefix), zstdComp...), zstdCompWithPrefix)

	// 4. Deflate
	flateComp := compress.AppendDeflateBytes(nil, payload)
	flateAppended, err := compress.AppendInflateBytes(slices.Clone(prefix), flateComp)
	require.NoError(t, err)
	assert.Equal(t, append(slices.Clone(prefix), payload...), flateAppended)
}

func TestCompression_MalformedInput_CleanError(t *testing.T) {
	t.Parallel()

	corrupt := []byte("completely_garbage_not_compressed_data_random_bytes_#@!$%^&*")

	var out bytes.Buffer

	// Gunzip on corrupt input
	_, err := compress.WriteGunzip(&out, corrupt)
	require.Error(t, err)

	_, err = compress.WriteGunzipLimit(&out, corrupt, 1000)
	require.Error(t, err)

	// Unbrotli on corrupt input
	out.Reset()
	_, err = compress.WriteUnbrotli(&out, corrupt)
	require.Error(t, err)

	_, err = compress.WriteUnbrotliLimit(&out, corrupt, 1000)
	require.Error(t, err)

	// Unzstd on corrupt input
	out.Reset()
	_, err = compress.WriteUnzstd(&out, corrupt)
	require.Error(t, err)

	_, err = compress.WriteUnzstdLimit(&out, corrupt, 1000)
	require.Error(t, err)

	// Inflate on corrupt input
	out.Reset()
	_, err = compress.WriteInflate(&out, corrupt)
	require.Error(t, err)

	_, err = compress.WriteInflateLimit(&out, corrupt, 1000)
	require.Error(t, err)

	// Empty slices
	_, err = compress.WriteGunzip(&out, nil)
	require.Error(t, err)
	_, err = compress.WriteInflate(&out, nil)
	require.Error(t, err)
}

func TestCompression_DecompressionBomb_Protection(t *testing.T) {
	t.Parallel()

	// Create highly compressible bomb: 100 KB of zeros
	zeros := make([]byte, 100*1024)

	gz := compress.AppendGzipBytes(nil, zeros)
	br := compress.AppendBrotliBytes(nil, zeros)
	zstdBytes := compress.AppendZstdBytes(nil, zeros)
	deflateBytes := compress.AppendDeflateBytes(nil, zeros)

	var out bytes.Buffer

	// Test limiting to 1024 bytes
	limit := 1024

	_, err := compress.WriteGunzipLimit(&out, gz, limit)
	require.Error(t, err)

	out.Reset()
	_, err = compress.WriteUnbrotliLimit(&out, br, limit)
	require.Error(t, err)

	out.Reset()
	_, err = compress.WriteUnzstdLimit(&out, zstdBytes, limit)
	require.Error(t, err)

	out.Reset()
	_, err = compress.WriteInflateLimit(&out, deflateBytes, limit)
	require.Error(t, err)
}

func TestCompression_AppendLimit_SuccessAndBomb(t *testing.T) {
	t.Parallel()

	payload := []byte("Decompression limit test string for RFC compliance validation: 0123456789")

	// 1. Gzip (RFC 1952 §2.2, §2.3.1)
	gzComp := compress.AppendGzipBytes(nil, payload)
	// Unlimited
	gzDec, err := compress.AppendGunzipBytesLimit(nil, gzComp, 0)
	require.NoError(t, err)
	assert.Equal(t, payload, gzDec)
	// Exact limit
	gzDec, err = compress.AppendGunzipBytesLimit(nil, gzComp, len(payload))
	require.NoError(t, err)
	assert.Equal(t, payload, gzDec)
	// Exceeded limit (bomb protection)
	_, err = compress.AppendGunzipBytesLimit(nil, gzComp, len(payload)-1)
	require.Error(t, err)

	// 2. Deflate / ZLIB (RFC 1950 §2.2, RFC 1951 §3.1.1)
	flateComp := compress.AppendDeflateBytes(nil, payload)
	// Unlimited
	flateDec, err := compress.AppendInflateBytesLimit(nil, flateComp, 0)
	require.NoError(t, err)
	assert.Equal(t, payload, flateDec)
	// Exact limit
	flateDec, err = compress.AppendInflateBytesLimit(nil, flateComp, len(payload))
	require.NoError(t, err)
	assert.Equal(t, payload, flateDec)
	// Exceeded limit
	_, err = compress.AppendInflateBytesLimit(nil, flateComp, len(payload)-1)
	require.Error(t, err)

	// 3. Brotli (RFC 7932 §2, §9.2)
	brComp := compress.AppendBrotliBytes(nil, payload)
	// Unlimited
	brDec, err := compress.AppendUnbrotliBytesLimit(nil, brComp, 0)
	require.NoError(t, err)
	assert.Equal(t, payload, brDec)
	// Exact limit
	brDec, err = compress.AppendUnbrotliBytesLimit(nil, brComp, len(payload))
	require.NoError(t, err)
	assert.Equal(t, payload, brDec)
	// Exceeded limit
	_, err = compress.AppendUnbrotliBytesLimit(nil, brComp, len(payload)-1)
	require.Error(t, err)

	// 4. Zstd (RFC 8878 §3.1.1)
	zstdComp := compress.AppendZstdBytes(nil, payload)
	// Unlimited
	zstdDec, err := compress.AppendUnzstdBytesLimit(nil, zstdComp, 0)
	require.NoError(t, err)
	assert.Equal(t, payload, zstdDec)
	// Exact limit
	zstdDec, err = compress.AppendUnzstdBytesLimit(nil, zstdComp, len(payload))
	require.NoError(t, err)
	assert.Equal(t, payload, zstdDec)
	// Exceeded limit
	_, err = compress.AppendUnzstdBytesLimit(nil, zstdComp, len(payload)-1)
	require.Error(t, err)
}

func TestCompression_ChecksumAndHeader_Validation(t *testing.T) {
	t.Parallel()

	payload := []byte("Validation of checksums and magic headers across all codecs.")

	t.Run("gzip_corrupt_crc32", func(t *testing.T) {
		// RFC 1952 §2.2: The last 8 bytes are CRC32 (4 bytes) and ISIZE (4 bytes)
		gz := compress.AppendGzipBytes(nil, payload)
		require.True(t, len(gz) >= 10)
		corrupt := slices.Clone(gz)
		corrupt[len(corrupt)-8] ^= 0xff // Corrupt CRC32 byte
		_, err := compress.AppendGunzipBytes(nil, corrupt)
		require.Error(t, err)
	})

	t.Run("gzip_corrupt_id_magic", func(t *testing.T) {
		// RFC 1952 §2.3.1: ID1 = 31 (0x1f), ID2 = 139 (0x8b)
		gz := compress.AppendGzipBytes(nil, payload)
		require.True(t, len(gz) >= 2)
		assert.Equal(t, byte(0x1f), gz[0])
		assert.Equal(t, byte(0x8b), gz[1])
		corrupt := slices.Clone(gz)
		corrupt[0] = 0x00
		_, err := compress.AppendGunzipBytes(nil, corrupt)
		require.Error(t, err)
	})

	t.Run("zlib_corrupt_adler32", func(t *testing.T) {
		// RFC 1950 §2.2: The last 4 bytes are Adler-32 checksum
		zl := compress.AppendDeflateBytes(nil, payload)
		require.True(t, len(zl) >= 6)
		corrupt := slices.Clone(zl)
		corrupt[len(corrupt)-1] ^= 0xff // Corrupt Adler-32 byte
		_, err := compress.AppendInflateBytes(nil, corrupt)
		require.Error(t, err)
	})

	t.Run("zlib_corrupt_cmf_flg_header", func(t *testing.T) {
		// RFC 1950 §2.2: CMF and FLG must satisfy (CMF*256 + FLG) % 31 == 0
		zl := compress.AppendDeflateBytes(nil, payload)
		require.True(t, len(zl) >= 2)
		corrupt := slices.Clone(zl)
		corrupt[0] ^= 0x01 // Break CMF/FLG checksum
		_, err := compress.AppendInflateBytes(nil, corrupt)
		require.Error(t, err)
	})

	t.Run("zstd_corrupt_magic", func(t *testing.T) {
		// RFC 8878 §3.1.1: 4 bytes Magic Number: 0xFD2FB528 (little-endian: 0x28, 0xB5, 0x2F, 0xFD)
		zs := compress.AppendZstdBytes(nil, payload)
		require.True(t, len(zs) >= 4)
		corrupt := slices.Clone(zs)
		corrupt[0] ^= 0xff // Invalidate magic
		_, err := compress.AppendUnzstdBytes(nil, corrupt)
		require.Error(t, err)
	})
}
