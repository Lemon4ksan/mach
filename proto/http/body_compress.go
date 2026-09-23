// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"bufio"
	"errors"
	"io"
	"sync"

	"github.com/lemon4ksan/foundation/codec/compress"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	machcompress "github.com/lemon4ksan/mach/proto/compress"
)

// ErrContentEncodingUnsupported is returned when an HTTP payload specifies a Content-Encoding
// (RFC 9110 Section 8.4) that is not supported by the decompression engine.
var ErrContentEncodingUnsupported = errors.New("mach: unsupported content-encoding")

// ErrBodyStreamWritePanic is returned when a panic occurs during asynchronous stream
// compression execution.
type ErrBodyStreamWritePanic struct{ error }

const minCompressLen = 200

func gunzipData(p []byte, maxBodySize int) ([]byte, error) {
	if maxBodySize <= 0 {
		return compress.Gunzip(p, nil)
	}

	var bb bytesconv.ByteBuffer

	_, err := machcompress.WriteGunzipLimit(&bb, p, maxBodySize)
	if err != nil {
		return nil, err
	}

	return bb.B, nil
}

func unBrotliData(p []byte, maxBodySize int) ([]byte, error) {
	if maxBodySize <= 0 {
		return compress.Unbrotli(p, nil)
	}

	var bb bytesconv.ByteBuffer

	_, err := machcompress.WriteUnbrotliLimit(&bb, p, maxBodySize)
	if err != nil {
		return nil, err
	}

	return bb.B, nil
}

func unzstdData(p []byte, maxBodySize int) ([]byte, error) {
	if maxBodySize <= 0 {
		return compress.Unzstd(p, nil)
	}

	var bb bytesconv.ByteBuffer

	_, err := machcompress.WriteUnzstdLimit(&bb, p, maxBodySize)
	if err != nil {
		return nil, err
	}

	return bb.B, nil
}

func inflateData(p []byte, maxBodySize int) ([]byte, error) {
	if maxBodySize <= 0 {
		return compress.Inflate(p, nil)
	}

	var bb bytesconv.ByteBuffer

	_, err := machcompress.WriteInflateLimit(&bb, p, maxBodySize)
	if err != nil {
		return nil, err
	}

	return bb.B, nil
}

type compressedBodyStream struct {
	io.ReadCloser
	bodyStream     io.Reader
	level          int
	compress       compressBodyStream
	done           chan struct{}
	closeReadOnce  sync.Once
	closeReadErr   error
	originalLock   sync.Mutex
	originalClosed bool
	closeErr       error
}

func (s *compressedBodyStream) Close() error {
	s.closeReadOnce.Do(func() {
		s.closeReadErr = s.ReadCloser.Close()
		if err := s.closeOriginalForDiscard(); s.closeReadErr == nil {
			s.closeReadErr = err
		}
	})

	err := s.closeReadErr
	select {
	case <-s.done:
		if err == nil {
			err = s.closeErr
		}
	default:
	}

	return err
}

func (s *compressedBodyStream) write(sw *bufio.Writer) {
	s.closeErr = s.closeOriginal(s.compress(sw, s.bodyStream, s.level))
	close(s.done)
}

func (s *compressedBodyStream) closeOriginal(wErr error) error {
	s.originalLock.Lock()
	defer s.originalLock.Unlock()

	var err error
	if !s.originalClosed {
		if bsc, ok := s.bodyStream.(io.Closer); ok {
			err = bsc.Close()
		}

		s.originalClosed = true
	}

	if bsc, ok := s.bodyStream.(ReadCloserWithError); ok {
		if errc := bsc.CloseWithError(wErr); err == nil {
			err = errc
		}
	}

	if bsr, ok := s.bodyStream.(*RequestStream); ok {
		ReleaseRequestStream(bsr)
	}

	return err
}

func (s *compressedBodyStream) closeOriginalForDiscard() error {
	s.originalLock.Lock()
	defer s.originalLock.Unlock()

	if s.originalClosed {
		return nil
	}

	bsc, ok := s.bodyStream.(io.Closer)
	if !ok {
		return nil
	}

	s.originalClosed = true

	return bsc.Close()
}

type compressBodyStream func(sw *bufio.Writer, bodyStream io.Reader, level int) error

func newCompressedBodyStream(bodyStream io.Reader, level int, compress compressBodyStream) io.ReadCloser {
	s := &compressedBodyStream{bodyStream: bodyStream, level: level, compress: compress, done: make(chan struct{})}
	s.ReadCloser = NewStreamReader(s.write)
	return s
}

func compressBrotliBodyStream(sw *bufio.Writer, bodyStream io.Reader, _ int) error {
	_, wErr := copyBodyStream(sw, bodyStream)
	return wErr
}

func compressGzipBodyStream(sw *bufio.Writer, bodyStream io.Reader, level int) error {
	zw := machcompress.AcquireStacklessGzipWriter(sw, level)
	fw := &flushWriter{wf: zw, bw: sw}
	_, wErr := copyBodyStream(fw, bodyStream)

	machcompress.ReleaseStacklessGzipWriter(zw, level)

	return wErr
}

func compressDeflateBodyStream(sw *bufio.Writer, bodyStream io.Reader, level int) error {
	zw := machcompress.AcquireStacklessDeflateWriter(sw, level)
	fw := &flushWriter{wf: zw, bw: sw}
	_, wErr := copyBodyStream(fw, bodyStream)

	machcompress.ReleaseStacklessDeflateWriter(zw, level)

	return wErr
}

func compressZstdBodyStream(sw *bufio.Writer, bodyStream io.Reader, _ int) error {
	_, wErr := copyBodyStream(sw, bodyStream)
	return wErr
}

func closeBodyStreamReader(bodyStream io.Reader, wErr error) error {
	var err error
	if bsc, ok := bodyStream.(io.Closer); ok {
		err = bsc.Close()
	}

	if bsc, ok := bodyStream.(ReadCloserWithError); ok {
		if errc := bsc.CloseWithError(wErr); err == nil {
			err = errc
		}
	}

	if bsr, ok := bodyStream.(*RequestStream); ok {
		ReleaseRequestStream(bsr)
	}

	return err
}

type writeFlusher interface {
	io.Writer
	Flush() error
}

type flushWriter struct {
	wf writeFlusher
	bw *bufio.Writer
}

func (w *flushWriter) Write(p []byte) (int, error) {
	n, err := w.wf.Write(p)
	if err != nil {
		return 0, err
	}

	if err = w.wf.Flush(); err != nil {
		return 0, err
	}

	if err = w.bw.Flush(); err != nil {
		return 0, err
	}

	return n, nil
}

func (w *flushWriter) WriteString(s string) (int, error) {
	return w.Write(bytesconv.S2B(s))
}
