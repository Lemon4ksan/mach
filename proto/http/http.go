// Code automatically split by refactoring script

package http

import (
	machcompress "github.com/lemon4ksan/mach/proto/compress"

	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"sync"
	"sync/atomic"

	"github.com/lemon4ksan/foundation/codec/compress"
	"github.com/lemon4ksan/foundation/net/http/zerocopy"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"
)

var (
	requestBodyPoolSizeLimit  atomic.Int64
	responseBodyPoolSizeLimit atomic.Int64
)

func SetBodySizePoolLimit(reqBodyLimit, respBodyLimit int) {
	requestBodyPoolSizeLimit.Store(int64(reqBodyLimit))
	responseBodyPoolSizeLimit.Store(int64(respBodyLimit))
} // SetBodySizePoolLimit set the max body size for bodies to be returned to the pool.
// If the body size is larger it will be released instead of put back into the pool for reuse.

type ReadCloserWithError interface {
	io.Reader
	CloseWithError(err error) error
}

type closeReader struct {
	io.Reader
	closeFunc func(err error) error
}

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

type responseBodyWriter struct{ r *Response }

func (w *responseBodyWriter) Write(p []byte) (int, error) {
	w.r.AppendBody(p)
	return len(p), nil
}

func (w *responseBodyWriter) WriteString(s string) (int, error) {
	w.r.AppendBodyString(s)
	return len(s), nil
}

type requestBodyWriter struct{ r *Request }

func (w *requestBodyWriter) Write(p []byte) (int, error) {
	w.r.AppendBody(p)
	return len(p), nil
}

func (w *requestBodyWriter) WriteString(s string) (int, error) {
	w.r.AppendBodyString(s)
	return len(s), nil
}

var (
	responseBodyPool bytesconv.ByteBufferPool
	requestBodyPool  bytesconv.ByteBufferPool
)

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

var ErrContentEncodingUnsupported = errors.New("mach: unsupported content-encoding")

func SwapRequestBody(a, b *Request) {
	a.body, b.body = b.body, a.body
	a.bodyRaw, b.bodyRaw = b.bodyRaw, a.bodyRaw
	a.bodyStream, b.bodyStream = b.bodyStream, a.bodyStream
	if rs, ok := a.bodyStream.(*RequestStream); ok {
		rs.header = &a.Header
	}
	if rs, ok := b.bodyStream.(*RequestStream); ok {
		rs.header = &b.Header
	}
}

func SwapResponseBody(a, b *Response) {
	a.body, b.body = b.body, a.body
	a.bodyRaw, b.bodyRaw = b.bodyRaw, a.bodyRaw
	a.bodyStream, b.bodyStream = b.bodyStream, a.bodyStream
}

var ErrNoMultipartForm = errors.New("mach: request content-type has bad boundary or is not multipart/form-data") // ErrNoMultipartForm means that the request's Content-Type
// isn't 'multipart/form-data'.

func marshalMultipartForm(f *multipart.Form, boundary string) ([]byte, error) {
	var buf bytesconv.ByteBuffer
	if err := WriteMultipartForm(&buf, f, boundary); err != nil {
		return nil, err
	}
	return buf.B, nil
}

func WriteMultipartForm(w io.Writer, f *multipart.Form, boundary string) error {
	if boundary == "" {
		return errors.New("form boundary cannot be empty")
	}
	mw := multipart.NewWriter(w)
	if err := mw.SetBoundary(boundary); err != nil {
		return fmt.Errorf("cannot use form boundary %q: %w", boundary, err)
	}
	for k, vv := range // WriteMultipartForm writes the given multipart form f with the given
	// boundary to w.
	f.Value {
		for _, v := range vv {
			if err := mw.WriteField(k, v); err != nil {
				return fmt.Errorf("cannot write form field %q value %q: %w", k, v, err)
			}
		}
	}
	for k, fvv := range f.File {
		for _, fv := range fvv {
			vw, err := mw.CreatePart(fv.Header)
			if err != nil {
				return fmt.Errorf("cannot create form file %q (%q): %w", k, fv.Filename, err)
			}
			fh, err := fv.Open()
			if err != nil {
				return fmt.Errorf("cannot open form file %q (%q): %w", k, fv.Filename, err)
			}
			if _, err = copyZeroAlloc(vw, fh); err != nil {
				_ = fh.Close()
				return fmt.Errorf("error when copying form file %q (%q): %w", k, fv.Filename, err)
			}
			if err = fh.Close(); err != nil {
				return fmt.Errorf("cannot close form file %q (%q): %w", k, fv.Filename, err)
			}
		}
	}
	if err := mw.Close(); err != nil {
		return fmt.Errorf("error when closing multipart form writer: %w", err)
	}
	return nil
}

func readMultipartForm(r io.Reader, boundary string, size, maxInMemoryFileSize int) (*multipart.Form, error) {
	if size <= 0 {
		return nil, fmt.Errorf("form size must be greater than 0: given %d", size)
	}
	lr := io.LimitReader(r, int64(size))
	mr := multipart.NewReader(lr, boundary)
	f, err := mr.ReadForm(int64(maxInMemoryFileSize))
	if err != nil {
		return nil, fmt.Errorf("cannot read multipart/form-data body: %w", err)
	}
	return f, nil
}

const defaultMaxInMemoryFileSize = 16 * 1024 * 1024

var ErrGetOnly = errors.New("mach: non-get request received") // ErrGetOnly is returned when server expects only GET requests,
// but some other type of request came (Server.GetOnly option is true).

const maxInterimResponses = 100 // maxInterimResponses limits the number of consecutive informational responses
// accepted before ReadLimitBody returns errTooManyInterimResponses.

var errTooManyInterimResponses = errors.New("mach: too many 1xx informational responses received")

var errRequestHostRequired = errors.New("missing required host header in request")

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
} //nolint:unused

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
} //nolint:unused

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

const minCompressLen = 200 // Bodies with sizes smaller than minCompressLen aren't compressed at all.

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

type ErrBodyStreamWritePanic struct{ error } // ErrBodyStreamWritePanic is returned when panic happens during writing body stream.

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

type httpWriter interface{ Write(w *bufio.Writer) error }

type BodyWriterTo interface {
	io.WriterTo
	SupportsBodyWriteTo() bool
} // BodyWriterTo lets a body stream control whether fasthttp may use WriteTo
// instead of Read when consuming the stream.
//
// Returning false from SupportsBodyWriteTo forces fasthttp to use Read.
// Returning true permits fasthttp to use WriteTo. Existing body-copy paths
// retain their historical io.WriterTo behavior for streams that do not
// implement BodyWriterTo, while direct unknown-size chunked framing uses Read
// for unmarked streams.
//
// SupportsBodyWriteTo must return true only when WriteTo can safely replace
// Read, including any pacing, accounting, transformations, or other observable
// side effects Read performs — emitting the same eventual bytes is not enough.
// A bare io.WriterTo check is avoided for direct chunked framing because a
// WriteTo promoted from an embedded reader would opt in by accident and bypass
// an overridden Read; the bool also lets an embedding type opt back out.

type chunkedBodyWriter struct {
	w   *bufio.Writer
	err error
}

func (cw *chunkedBodyWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := writeChunk(cw.w, p); err != nil {
		cw.err = err
		return 0, err
	}
	return len(p), nil
}

func writeBodyChunked(w *bufio.Writer, r io.Reader) error {
	var wt io.WriterTo
	switch v := r.(type // Frame WriteTo output directly, skipping zerocopy.CopyBufPool, for bodies whose
	// WriteTo is known to match reading.
	) {
	case *bytes.Reader:
		wt = v
	case *bytes.Buffer:
		wt = v
	default:
		if bwt, ok := r.(BodyWriterTo); ok && bwt.SupportsBodyWriteTo() {
			wt = bwt
		}
	}
	if wt != nil {
		cw := chunkedBodyWriter{w: w}
		if _, err := wt.WriteTo(&cw); err != nil {
			return err
		}
		if cw.err != nil {
			return cw.err
		}
		return writeChunk(w, nil)
	}
	vbuf := zerocopy.CopyBufPool.Get()
	buf := vbuf.([]byte)
	var (
		err error
		n   int
	)
	for {
		n, err = r.Read(buf)
		if n == 0 {
			if err == nil {
				continue
			}
			if errors.Is(err, io.EOF) {
				if err = writeChunk(w, buf[:0]); err != nil {
					break
				}
				err = nil
			}
			break
		}
		if err = writeChunk(w, buf[:n]); err != nil {
			break
		}
	}
	zerocopy.CopyBufPool.Put(vbuf)
	return err
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

func writeChunk(w *bufio.Writer, b []byte) error {
	n := len(b)
	if err := zerocopy.WriteHexInt(w, n); err != nil {
		return err
	}
	if _, err := w.Write(zerocopy.StrCRLF); err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	if n > 0 {
		if _, err := w.Write(zerocopy.StrCRLF); err != nil {
			return err
		}
	}
	return w.Flush()
}

var ErrBodyTooLarge = zerocopy.ErrBodyTooLarge // ErrBodyTooLarge is returned if either request or response body exceeds
// the given limit.

func copyZeroAllocWithLimit(w io.Writer, r io.Reader, maxBodySize int) (int64, error) {
	return bytesconv.CopyZeroAllocWithLimit(w, r, maxBodySize)
} //nolint:unused

func readBody(r *bufio.Reader, contentLength, maxBodySize int, dst []byte) ([]byte, error) {
	if maxBodySize > 0 && contentLength > maxBodySize {
		return dst, ErrBodyTooLarge
	}
	return appendBodyFixedSize(r, dst, contentLength)
}

var errChunkedStream = errors.New("chunked stream")

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

type ErrBrokenChunk struct{ error } // ErrBrokenChunk is returned when server receives a broken chunked body (Transfer-Encoding: chunked).

func readBodyChunked(r *bufio.Reader, maxBodySize int, dst []byte) ([]byte, error) {
	if len(dst) > 0 {
		panic("BUG: expected zero-length buffer")
	}
	for {
		chunkSize, err := parseChunkSize(r)
		if err != nil {
			return dst, err
		}
		if chunkSize == 0 {
			return dst, err
		}
		if maxBodySize > 0 && len(dst)+chunkSize > maxBodySize {
			return dst, ErrBodyTooLarge
		}
		dst, err = appendBodyFixedSize(r, dst, chunkSize+len(zerocopy.StrCRLF))
		if err != nil {
			return dst, err
		}
		if !bytes.Equal(dst[len(dst)-len(zerocopy.StrCRLF):], zerocopy.StrCRLF) {
			return dst, ErrBrokenChunk{error: errors.New("cannot find crlf at the end of chunk")}
		}
		dst = dst[:len(dst)-len(zerocopy.StrCRLF)]
	}
}

func parseChunkSize(r *bufio.Reader) (int, error) {
	n, err := zerocopy.ReadHexInt(r)
	if err != nil {
		return -1, err
	}
	inExt := false
	afterSizeOWS := false
	for {
		c, err := r.ReadByte()
		if err != nil {
			return -1, ErrBrokenChunk{error: fmt.Errorf("cannot read '\\r' char at the end of chunk size: %w", err)}
		}
		if c == '\r' {
			if err := r.UnreadByte(); err != nil {
				return -1, ErrBrokenChunk{error: fmt.Errorf("cannot unread '\\r' char at the end of chunk size: %w", err)}
			}
			break
		}
		if c == '\n' {
			return -1, ErrBrokenChunk{error: errors.New("invalid character '\\n' after chunk size")}
		}
		if inExt {
			continue
		}
		switch c {
		case ' ', '\t':
			afterSizeOWS = true
			continue
		case ';':
			if afterSizeOWS {
				return -1, ErrBrokenChunk{error: fmt.Errorf("invalid character %q after chunk size", c)}
			}
			inExt = true
			continue
		default:
			return -1, ErrBrokenChunk{error: fmt.Errorf("invalid character %q after chunk size", c)}
		}
	}
	err = readCrLf(r)
	if err != nil {
		return -1, err
	}
	return n, nil
}

func readCrLf(r *bufio.Reader) error {
	for _, exp := range []byte{'\r', '\n'} {
		c, err := r.ReadByte()
		if err != nil {
			return ErrBrokenChunk{error: fmt.Errorf("cannot read %q char at the end of chunk size: %w", exp, err)}
		}
		if c != exp {
			return ErrBrokenChunk{error: fmt.Errorf("unexpected char %q at the end of chunk size: expected %q", c, exp)}
		}
	}
	return nil
}

func init() {
	requestBodyPoolSizeLimit.Store(-1)
	responseBodyPoolSizeLimit.Store(-1)
}
