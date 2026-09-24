// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"
)

func TestNoCopyLockUnlock(t *testing.T) {
	var nc NoCopy
	nc.Lock()
	nc.Unlock()
}

type shortWriter struct{}

func (s *shortWriter) Write(p []byte) (int, error) {
	return len(p) / 2, nil
}

type errWriter struct {
	err error
}

func (e *errWriter) Write(p []byte) (int, error) {
	return 0, e.err
}

type errReader struct {
	err error
}

func (e *errReader) Read(p []byte) (int, error) {
	return 0, e.err
}

type badLenWriter struct{}

func (b *badLenWriter) Write(p []byte) (int, error) {
	return len(p) + 10, nil // invalid write result: nr < nw
}

type plainReader struct {
	r io.Reader
}

func (p *plainReader) Read(buf []byte) (int, error) {
	return p.r.Read(buf)
}

type plainWriter struct {
	w io.Writer
}

func (p *plainWriter) Write(buf []byte) (int, error) {
	return p.w.Write(buf)
}

func TestCopyZeroAlloc(t *testing.T) {
	content := []byte("Hello Zero-Copy Buffer World!")

	// 1. WriterTo path (e.g. bytes.Reader implements io.WriterTo)
	var buf bytes.Buffer
	n, err := CopyZeroAlloc(&buf, bytes.NewReader(content))
	if err != nil || n != int64(len(content)) || !bytes.Equal(buf.Bytes(), content) {
		t.Fatalf("CopyZeroAlloc with WriterTo failed: %d, %v", n, err)
	}

	// 2. ReaderFrom path (plain reader without WriterTo, bytes.Buffer implements ReaderFrom)
	buf.Reset()
	pr := &plainReader{r: bytes.NewReader(content)}
	n2, err := CopyZeroAlloc(&buf, pr)
	if err != nil || n2 != int64(len(content)) || !bytes.Equal(buf.Bytes(), content) {
		t.Fatalf("CopyZeroAlloc with ReaderFrom failed: %d, %v", n2, err)
	}

	// 3. Neither WriterTo nor ReaderFrom (uses CopyBufPool)
	var plainBuf bytes.Buffer
	pw := &plainWriter{w: &plainBuf}
	pr2 := &plainReader{r: bytes.NewReader(content)}
	n3, err := CopyZeroAlloc(pw, pr2)
	if err != nil || n3 != int64(len(content)) || !bytes.Equal(plainBuf.Bytes(), content) {
		t.Fatalf("CopyZeroAlloc with pooled buffer failed: %d, %v", n3, err)
	}

	// 4. Test with temporary file
	tmpFile, err := os.CreateTemp("", "zerocopy_test")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()

	nFile, err := CopyZeroAlloc(tmpFile, bytes.NewReader(content))
	if err != nil || nFile != int64(len(content)) {
		t.Fatalf("CopyZeroAlloc to file failed: %d, %v", nFile, err)
	}
}

func TestCopyBufferEdgeCases(t *testing.T) {
	buf := make([]byte, 32)
	data := []byte("0123456789")

	// Short write
	var sw shortWriter
	_, err := CopyBuffer(&sw, bytes.NewReader(data), buf)
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("expected io.ErrShortWrite, got %v", err)
	}

	// Writer error
	customErr := errors.New("write failure")
	ew := &errWriter{err: customErr}
	_, err = CopyBuffer(ew, bytes.NewReader(data), buf)
	if !errors.Is(err, customErr) {
		t.Fatalf("expected customErr, got %v", err)
	}

	// Reader error
	er := &errReader{err: customErr}
	var dst bytes.Buffer
	_, err = CopyBuffer(&dst, er, buf)
	if !errors.Is(err, customErr) {
		t.Fatalf("expected customErr from reader, got %v", err)
	}

	// Bad write result (nr < nw)
	var blw badLenWriter
	_, err = CopyBuffer(&blw, bytes.NewReader(data), buf)
	if err == nil || err.Error() != "invalid write result" {
		t.Fatalf("expected 'invalid write result', got %v", err)
	}
}

func TestCopyZeroAllocWithLimit(t *testing.T) {
	data := []byte("The quick brown fox jumps over the lazy dog")

	// 1. Within limit
	var dst1 bytes.Buffer
	n1, err := CopyZeroAllocWithLimit(&dst1, bytes.NewReader(data), len(data))
	if err != nil || n1 != int64(len(data)) || !bytes.Equal(dst1.Bytes(), data) {
		t.Fatalf("CopyZeroAllocWithLimit within limit failed: %d, %v", n1, err)
	}

	// 2. Exceeding limit
	var dst2 bytes.Buffer
	_, err = CopyZeroAllocWithLimit(&dst2, bytes.NewReader(data), 10)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("expected ErrBodyTooLarge, got %v", err)
	}

	// 3. Non-positive limit (unlimited)
	var dst3 bytes.Buffer
	n3, err := CopyZeroAllocWithLimit(&dst3, bytes.NewReader(data), 0)
	if err != nil || n3 != int64(len(data)) {
		t.Fatalf("CopyZeroAllocWithLimit with limit <= 0 failed: %d, %v", n3, err)
	}
}
