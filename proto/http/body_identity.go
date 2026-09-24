// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/lemon4ksan/mach/proto/http/zerocopy"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"
)

// ReadCloserWithError extends io.Reader with an error-propagating CloseWithError method.
//
// It is used by streaming body readers to report underlying stream transport errors
// to upstream consumers and resource managers upon closure.
type ReadCloserWithError interface {
	io.Reader
	CloseWithError(err error) error
}

type closeReader struct {
	io.Reader
	closeFunc func(err error) error
}

// NewCloseReaderWithError wraps r and closeFunc into a ReadCloserWithError implementation.
//
// Panics if r is nil.
func NewCloseReaderWithError(r io.Reader, closeFunc func(err error) error) ReadCloserWithError {
	if r == nil {
		panic(`BUG: reader is nil`)
	}

	return &closeReader{Reader: r, closeFunc: closeFunc}
}

func (c *closeReader) CloseWithError(err error) error {
	if c.closeFunc == nil {
		return nil
	}

	return c.closeFunc(err)
}

type httpWriter interface{ Write(w *bufio.Writer) error }

// BodyWriterTo allows an io.Reader stream to opt in to direct io.WriterTo transfers.
//
// When SupportsBodyWriteTo returns true, the HTTP transmission engine bypasses intermediate
// copy buffer pools and delegates writing directly to WriteTo(w). This permits zero-copy
// kernel transfers (e.g. sendfile, splice) and specialized stream serializers.
type BodyWriterTo interface {
	io.WriterTo
	SupportsBodyWriteTo() bool
}

func limitedReaderSize(r io.Reader) int64 {
	lr, ok := r.(*io.LimitedReader)
	if !ok {
		return -1
	}

	return lr.N
}

func writeBodyFixedSize(w *bufio.Writer, r io.Reader, size int64) error {
	if size > maxSmallFileSize {
		earlyFlush := false
		switch r := r.(type) {
		case *os.File:
			earlyFlush = true
		case *io.LimitedReader:
			_, earlyFlush = r.R.(*os.File)
		}

		if earlyFlush {
			if err := w.Flush(); err != nil {
				return err
			}
		}
	}

	n, err := copyBodyStream(w, r)
	if n != size && err == nil {
		err = fmt.Errorf("copied %d bytes from body stream instead of %d bytes", n, size)
	}

	return err
}

func copyBodyStream(w io.Writer, r io.Reader) (int64, error) {
	if bwt, ok := r.(BodyWriterTo); ok {
		if bwt.SupportsBodyWriteTo() {
			return bwt.WriteTo(w)
		}

		vbuf := zerocopy.CopyBufPool.Get()
		buf := vbuf.([]byte)
		n, err := zerocopy.CopyBuffer(w, r, buf)

		zerocopy.CopyBufPool.Put(vbuf)

		return n, err
	}

	return copyZeroAlloc(w, r)
}

func copyZeroAlloc(w io.Writer, r io.Reader) (int64, error) {
	return bytesconv.CopyZeroAlloc(w, r)
}

// ErrBodyTooLarge is returned if a request or response body exceeds the configured
// maximum body size limit.
var ErrBodyTooLarge = zerocopy.ErrBodyTooLarge

// CopyZeroAllocWithLimit copies up to maxBodySize bytes from r to w without heap allocations,
// using foundation silicon buffer pools.
//
// Returns the number of bytes copied and any error encountered during transfer.
// If the stream contains more than maxBodySize bytes, ErrBodyTooLarge is returned.
func CopyZeroAllocWithLimit(w io.Writer, r io.Reader, maxBodySize int) (int64, error) {
	return bytesconv.CopyZeroAllocWithLimit(w, r, maxBodySize)
}

func readBody(r *bufio.Reader, contentLength, maxBodySize int, dst []byte) ([]byte, error) {
	if maxBodySize > 0 && contentLength > maxBodySize {
		return dst, ErrBodyTooLarge
	}

	return appendBodyFixedSize(r, dst, contentLength)
}

func readBodyWithStreaming(r *bufio.Reader, contentLength, maxBodySize int, dst []byte) (b []byte, err error) {
	if contentLength == -1 {
		return b, errChunkedStream
	}

	dst = dst[:0]
	readN := min(maxBodySize, contentLength)
	readN = min(readN, 8*1024)

	b, err = appendBodyFixedSize(r, dst, readN)
	if err != nil {
		return b, err
	}

	if contentLength > maxBodySize {
		return b, ErrBodyTooLarge
	}

	return b, nil
}

func readBodyIdentity(r *bufio.Reader, maxBodySize int, dst []byte) ([]byte, error) {
	dst = dst[:cap(dst)]
	if len(dst) == 0 {
		dst = make([]byte, 1024)
	}

	offset := 0
	for {
		nn, err := r.Read(dst[offset:])
		if nn <= 0 {
			switch {
			case errors.Is(err, io.EOF):
				return dst[:offset], nil
			case err != nil:
				return dst[:offset], err
			default:
				return dst[:offset], fmt.Errorf("bufio read returned (%d, nil)", nn)
			}
		}

		offset += nn
		if maxBodySize > 0 && offset > maxBodySize {
			return dst[:offset], ErrBodyTooLarge
		}

		if len(dst) == offset {
			n := roundUpForSliceCap(2 * offset)
			if maxBodySize > 0 && n > maxBodySize {
				n = maxBodySize + 1
			}

			b := make([]byte, n)
			copy(b, dst)
			dst = b
		}
	}
}

func appendBodyFixedSize(r *bufio.Reader, dst []byte, n int) ([]byte, error) {
	if n == 0 {
		return dst, nil
	}

	offset := len(dst)

	dstLen := offset + n
	if cap(dst) < dstLen {
		b := make([]byte, roundUpForSliceCap(dstLen))
		copy(b, dst)
		dst = b
	}

	dst = dst[:dstLen]
	for {
		nn, err := r.Read(dst[offset:])
		if nn <= 0 {
			switch {
			case errors.Is(err, io.EOF):
				return dst[:offset], io.ErrUnexpectedEOF
			case err != nil:
				return dst[:offset], err
			default:
				return dst[:offset], fmt.Errorf("bufio read returned (%d, nil)", nn)
			}
		}

		offset += nn
		if offset == dstLen {
			return dst, nil
		}
	}
}

func writeBufio(hw httpWriter, w io.Writer) (int64, error) {
	sw := acquireStatsWriter(w)
	bw := acquireBufioWriter(sw)
	errw := hw.Write(bw)
	errf := bw.Flush()
	releaseBufioWriter(bw)

	n := sw.bytesWritten
	releaseStatsWriter(sw)

	err := errw
	if err == nil {
		err = errf
	}

	return n, err
}

type statsWriter struct {
	w            io.Writer
	bytesWritten int64
}

func (w *statsWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.bytesWritten += int64(n)
	return n, err
}

func (w *statsWriter) WriteString(s string) (int, error) {
	n, err := w.w.Write(bytesconv.S2B(s))
	w.bytesWritten += int64(n)
	return n, err
}

var statsWriterStorage = pool.NewPerPStorage(func() *statsWriter {
	return &statsWriter{}
})

func acquireStatsWriter(w io.Writer) *statsWriter {
	sw := statsWriterStorage.Get()
	sw.w = w
	sw.bytesWritten = 0

	return sw
}

func releaseStatsWriter(sw *statsWriter) {
	if sw != nil {
		sw.w = nil
		sw.bytesWritten = 0
		statsWriterStorage.Put(sw)
	}
}

var bufioWriterStorage = pool.NewPerPStorage(func() *bufio.Writer {
	return bufio.NewWriter(nil)
})

func acquireBufioWriter(w io.Writer) *bufio.Writer {
	bw := bufioWriterStorage.Get()
	bw.Reset(w)
	return bw
}

func releaseBufioWriter(bw *bufio.Writer) {
	if bw != nil {
		bw.Reset(nil)
		bufioWriterStorage.Put(bw)
	}
}

func getHTTPString(hw httpWriter) string {
	w := bytesconv.AcquireByteBuffer()
	defer bytesconv.ReleaseByteBuffer(w)

	bw := bufio.NewWriter(w)
	if err := hw.Write(bw); err != nil {
		return err.Error()
	}

	if err := bw.Flush(); err != nil {
		return err.Error()
	}

	s := string(w.B)

	return s
}
