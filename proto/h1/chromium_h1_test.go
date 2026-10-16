// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/lemon4ksan/mach/proto/h1"
)

// oneByteReader reads exactly 1 byte at a time to test incremental parsing and buffer fragmentation.
type oneByteReader struct {
	r io.Reader
}

func (o *oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return o.r.Read(p[:1])
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: Basic
func TestChromium_Chunked_Basic(t *testing.T) {
	raw := "B\r\nhello hello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	data, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello hello" {
		t.Fatalf("expected 'hello hello', got %q", string(data))
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: Typical
func TestChromium_Chunked_Typical(t *testing.T) {
	raw := "5\r\nhello\r\n" +
		"1\r\n \r\n" +
		"5\r\nworld\r\n" +
		"0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	data, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(data))
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: Incremental (1-byte fragmentation)
func TestChromium_Chunked_Incremental1Byte(t *testing.T) {
	raw := "5\r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(&oneByteReader{r: strings.NewReader(raw)}))

	// Also read out 1 byte at a time
	var out []byte
	buf := make([]byte, 1)
	for {
		n, err := cr.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("unexpected error during 1-byte read: %v", err)
		}
	}
	if string(out) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(out))
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: Extensions
func TestChromium_Chunked_Extensions(t *testing.T) {
	raw := "5;x=0\r\nhello\r\n" +
		"0;y=\"2 \"\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	data, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(data))
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: Trailers
func TestChromium_Chunked_Trailers(t *testing.T) {
	raw := "5\r\nhello\r\n" +
		"0\r\n" +
		"Foo: 1\r\n" +
		"Bar: 2\r\n" +
		"\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	data, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(data))
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: InvalidChunkSize_TooBig
func TestChromium_Chunked_InvalidChunkSize_TooBig(t *testing.T) {
	raw := "48469410265455838241\r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected error on chunk size too big (uint64 overflow)")
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: ExcessiveChunkLen
func TestChromium_Chunked_ExcessiveChunkLen(t *testing.T) {
	// 0x8000000000000000 cannot be represented as a signed int64
	raw := "8000000000000000\r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected error on excessive chunk len (signed int64 overflow)")
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: ExcessiveChunkLen2
func TestChromium_Chunked_ExcessiveChunkLen2(t *testing.T) {
	// Smallest number that can't be represented as an unsigned int64
	raw := "10000000000000000\r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected error on excessive chunk len (unsigned int64 overflow)")
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: InvalidChunkSize_0X
func TestChromium_Chunked_InvalidChunkSize_0X(t *testing.T) {
	raw := "0x5\r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected error on 0x prefix in chunk size")
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: InvalidChunkSize_Negative
func TestChromium_Chunked_InvalidChunkSize_Negative(t *testing.T) {
	raw := "-5\r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected error on negative chunk size")
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: InvalidChunkSize_Plus
func TestChromium_Chunked_InvalidChunkSize_Plus(t *testing.T) {
	raw := "+5\r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected error on plus chunk size")
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: InvalidChunkSize_LeadingSpace
func TestChromium_Chunked_InvalidChunkSize_LeadingSpace(t *testing.T) {
	raw := " 5\r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected error on leading space in chunk size")
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: InvalidChunkSize_TrailingTab
func TestChromium_Chunked_InvalidChunkSize_TrailingTab(t *testing.T) {
	raw := "5\t\r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected error on trailing tab in chunk size")
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: InvalidChunkSize_TrailingNonHexDigit
func TestChromium_Chunked_InvalidChunkSize_TrailingNonHexDigit(t *testing.T) {
	raw := "5H\r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected error on trailing non-hex digit")
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: InvalidLeadingSeparator
func TestChromium_Chunked_InvalidLeadingSeparator(t *testing.T) {
	raw := "\r\n5\r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected error on leading CRLF before chunk size")
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: InvalidConsecutiveCRLFs
func TestChromium_Chunked_InvalidConsecutiveCRLFs(t *testing.T) {
	raw := "5\r\nhello\r\n\r\n\r\n\r\n\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected error on consecutive CRLFs between chunks")
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: MissingCRLFAtChunkBoundary
func TestChromium_Chunked_MissingCRLFAtChunkBoundary(t *testing.T) {
	// Chunk data 'hello' followed by 'XX' instead of '\r\n'
	raw := "5\r\nhelloXX0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected boundary error on missing CRLF after chunk data")
	}
	if !errors.Is(err, h1.ErrChunkBoundaryError) {
		t.Fatalf("expected ErrChunkBoundaryError, got: %v", err)
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: LongChunkLengthLine
// Chromium rejects chunk header lines that exceed kMaxLineBufLen (16384 bytes) to prevent DoS/OOM.
func TestChromium_Chunked_LongChunkLengthLine(t *testing.T) {
	// 20000 '0' characters before 5\r\n
	longZeros := strings.Repeat("0", 20000) + "5\r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(longZeros)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected error on excessively long chunk length line (> 16384 bytes)")
	}
}

// Port of Chromium net/http/http_chunked_decoder_unittest.cc: LongLengthLengthLine
// Chromium rejects chunk extension lines that exceed kMaxLineBufLen (16384 bytes).
func TestChromium_Chunked_LongLengthLengthLine(t *testing.T) {
	longExt := "5;" + strings.Repeat("x", 20000) + "\r\nhello\r\n0\r\n\r\n"
	cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(longExt)))
	_, err := io.ReadAll(cr)
	if err == nil {
		t.Fatal("expected error on excessively long chunk extension (> 16384 bytes)")
	}
}

// Port of Chromium net/http/http_stream_parser_unittest.cc: ReceivedBytesChunkedTransferExcludesNextResponse
// Test that chunked decoding stops cleanly at terminal chunk, leaving subsequent pipelined data in the buffer.
func TestChromium_Chunked_PipelinedDataIntact(t *testing.T) {
	chunkedResp := "5\r\nhello\r\n0\r\n\r\n"
	nextPipelinedData := "GET /next HTTP/1.1\r\nHost: example.com\r\n\r\n"
	combined := chunkedResp + nextPipelinedData

	br := bufio.NewReader(strings.NewReader(combined))
	cr := h1.NewChunkedReader(br)

	body, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("unexpected error reading chunked body: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(body))
	}

	// Verify next pipelined data is intact and unconsumed in br
	rem, err := io.ReadAll(br)
	if err != nil {
		t.Fatalf("unexpected error reading remaining pipelined bytes: %v", err)
	}
	if string(rem) != nextPipelinedData {
		t.Fatalf("expected remaining pipelined data %q, got %q", nextPipelinedData, string(rem))
	}
}

// Zero allocation verification on chunked reader hot path.
func TestChromium_Chunked_ZeroAllocs(t *testing.T) {
	chunkData := []byte("B\r\nhello world\r\n0\r\n\r\n")
	r := bytes.NewReader(chunkData)
	br := bufio.NewReader(r)
	cr := h1.NewChunkedReader(br)
	buf := make([]byte, 32)

	allocs := testing.AllocsPerRun(10, func() {
		r.Reset(chunkData)
		br.Reset(r)
		cr.Reset(br)
		for {
			_, err := cr.Read(buf)
			if err != nil {
				break
			}
		}
	})

	if allocs != 0 {
		t.Fatalf("expected 0 allocs on chunked read hot path, got %v", allocs)
	}
}
