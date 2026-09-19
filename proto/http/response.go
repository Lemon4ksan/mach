package http

import (
	"github.com/lemon4ksan/foundation/net/http/status"
	machcompress "github.com/lemon4ksan/mach/proto/compress"

	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/codec/compress"
	"github.com/lemon4ksan/foundation/net/http/zerocopy"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Response represents HTTP response.
//
// It is forbidden copying Response instances. Create new instances
// and use CopyTo instead.
//
// Response instance MUST NOT be used from concurrently running goroutines.
type Response struct {
	noCopy     zerocopy.NoCopy
	bodyStream io.Reader
	raddr      net.Addr
	laddr      net.Addr
	w          responseBodyWriter
	body       *bytesconv.ByteBuffer
	bodyRaw    []byte

	// Header is the response header.
	//
	// Copying Header by value is forbidden. Use pointer to Header instead.
	Header ResponseHeader

	ImmediateHeaderFlush bool
	StreamBody           bool

	// SkipBody skips reading body if set to true.
	// Use it for reading HEAD responses.
	SkipBody              bool
	KeepBodyBuffer        bool
	SecureErrorLogMessage bool

	// OnInterimResponse is an optional callback that is fired when the client receives
	// an interim 1xx response (such as 103 Early Hints or 100 Continue) before the final response.
	// The provided ResponseHeader is valid only during the callback.
	OnInterimResponse func(statusCode int, header *ResponseHeader)
}

// StatusCode returns response status code.
func (resp *Response) StatusCode() int {
	return resp.Header.StatusCode()
}

// SetStatusCode sets response status code.
func (resp *Response) SetStatusCode(statusCode int) {
	resp.Header.SetStatusCode(statusCode)
}

// ConnectionClose returns true if 'Connection: close' header is set.
func (resp *Response) ConnectionClose() bool {
	return resp.Header.ConnectionClose()
}

// SetConnectionClose sets 'Connection: close' header.
func (resp *Response) SetConnectionClose() {
	resp.Header.SetConnectionClose()
}

// SendFile registers file on the given path to be used as response body
// when Write is called.
//
// Note that SendFile doesn't set Content-Type, so set it yourself
// with Header.SetContentType.
func (resp *Response) SendFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	fileInfo, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	size64 := fileInfo.Size()
	size := int(size64)
	if int64(size) != size64 {
		size = -1
	}
	resp.Header.SetLastModified(fileInfo.ModTime())
	resp.SetBodyStream(f, size)
	return nil
}

// SetBodyStream sets response body stream and, optionally body size.
//
// If bodySize is >= 0, then the bodyStream must provide exactly bodySize bytes
// before returning io.EOF.
//
// If bodySize < 0, then bodyStream is read until io.EOF.
//
// When bodySize < 0 (chunked transfer encoding), fasthttp may frame the body
// using WriteTo instead of Read for *bytes.Reader, *bytes.Buffer, and streams
// implementing BodyWriterTo that return true from SupportsBodyWriteTo.
//
// See BodyWriterTo for controlling whether fasthttp may use WriteTo instead of
// Read when consuming bodyStream.
//
// bodyStream.Close() is called after finishing reading all body data
// if it implements io.Closer.
//
// See also SetBodyStreamWriter.
func (resp *Response) SetBodyStream(bodyStream io.Reader, bodySize int) {
	resp.ResetBody()
	resp.bodyStream = bodyStream
	resp.Header.SetContentLength(bodySize)
}

// IsBodyStream returns true if body is set via SetBodyStream*.
func (resp *Response) IsBodyStream() bool {
	return resp.bodyStream != nil
}

// SetBodyStreamWriter registers the given sw for populating response body.
//
// This function may be used in the following cases:
//
//   - if response body is too big (more than 10MB).
//   - if response body is streamed from slow external sources.
//   - if response body must be streamed to the client in chunks
//     (aka `http server push` or `chunked transfer-encoding`).
//
// See also SetBodyStream.
func (resp *Response) SetBodyStreamWriter(sw StreamWriter) {
	sr := NewStreamReader(sw)
	resp.SetBodyStream(sr, -1)
}

// BodyWriter returns writer for populating response body.
//
// If used inside RequestHandler, the returned writer must not be used
// after returning from RequestHandler. Use RequestCtx.Write
// or SetBodyStreamWriter in this case.
func (resp *Response) BodyWriter() io.Writer {
	resp.w.r = resp
	return &resp.w
}

// BodyStream returns io.Reader.
//
// You must CloseBodyStream or ReleaseResponse after you use it.
func (resp *Response) BodyStream() io.Reader {
	return resp.bodyStream
}

func (resp *Response) CloseBodyStream() error {
	return resp.closeBodyStream(nil)
}

func (resp *Response) ParseNetConn(conn net.Conn) {
	resp.raddr = conn.RemoteAddr()
	resp.laddr = conn.LocalAddr()
}

// RemoteAddr returns the remote network address. The Addr returned is shared
// by all invocations of RemoteAddr, so do not modify it.
func (resp *Response) RemoteAddr() net.Addr {
	return resp.raddr
}

// LocalAddr returns the local network address. The Addr returned is shared
// by all invocations of LocalAddr, so do not modify it.
func (resp *Response) LocalAddr() net.Addr {
	return resp.laddr
}

// Body returns response body.
//
// The returned value is valid until the response is released,
// either though ReleaseResponse or your request handler returning.
// Do not store references to returned value. Make copies instead.
//
// If the body is backed by a stream, Body reads the entire stream into memory.
// Use BodyStream to read it incrementally.
func (resp *Response) Body() []byte {

	if resp.bodyStream != nil {
		bodyBuf := resp.BodyBuffer()
		bodyBuf.Reset()
		_, err := copyBodyStream(bodyBuf, resp.bodyStream)
		resp.closeBodyStream(err)
		if err != nil {
			bodyBuf.SetString(err.Error())
		}
	}
	return resp.bodyBytes()
}

func (resp *Response) bodyBytes() []byte {
	if resp.bodyRaw != nil {
		return resp.bodyRaw
	}
	if resp.body == nil {
		return nil
	}
	return resp.body.B
}

func (resp *Response) BodyBuffer() *bytesconv.ByteBuffer {
	if resp.body == nil {
		resp.body = responseBodyPool.Get()
	}
	resp.bodyRaw = nil
	return resp.body
}

// BodyGunzip returns un-gzipped body data.
//
// This method may be used if the response header contains
// 'Content-Encoding: gzip' for reading un-gzipped body.
// Use Body for reading gzipped response body.
func (resp *Response) BodyGunzip() ([]byte, error) {
	return resp.BodyGunzipWithLimit(0)
}

// BodyGunzipWithLimit returns un-gzipped body data and limits the size
// of uncompressed body data to maxBodySize bytes.
//
// If maxBodySize <= 0, then no limit is applied.
func (resp *Response) BodyGunzipWithLimit(maxBodySize int) ([]byte, error) {
	return gunzipData(resp.Body(), maxBodySize)
}

// BodyUnbrotli returns un-brotlied body data.
//
// This method may be used if the response header contains
// 'Content-Encoding: br' for reading un-brotlied body.
// Use Body for reading brotlied response body.
func (resp *Response) BodyUnbrotli() ([]byte, error) {
	return resp.BodyUnbrotliWithLimit(0)
}

// BodyUnbrotliWithLimit returns un-brotlied body data and limits the size
// of uncompressed body data to maxBodySize bytes.
//
// If maxBodySize <= 0, then no limit is applied.
func (resp *Response) BodyUnbrotliWithLimit(maxBodySize int) ([]byte, error) {
	return unBrotliData(resp.Body(), maxBodySize)
}

// BodyInflate returns inflated body data.
//
// This method may be used if the response header contains
// 'Content-Encoding: deflate' for reading inflated response body.
// Use Body for reading deflated response body.
func (resp *Response) BodyInflate() ([]byte, error) {
	return resp.BodyInflateWithLimit(0)
}

// BodyInflateWithLimit returns inflated body data and limits the size
// of uncompressed body data to maxBodySize bytes.
//
// If maxBodySize <= 0, then no limit is applied.
func (resp *Response) BodyInflateWithLimit(maxBodySize int) ([]byte, error) {
	return inflateData(resp.Body(), maxBodySize)
}

func (resp *Response) BodyUnzstd() ([]byte, error) {
	return resp.BodyUnzstdWithLimit(0)
}

// BodyUnzstdWithLimit returns un-zstd body data and limits the size
// of uncompressed body data to maxBodySize bytes.
//
// If maxBodySize <= 0, then no limit is applied.
func (resp *Response) BodyUnzstdWithLimit(maxBodySize int) ([]byte, error) {
	return unzstdData(resp.Body(), maxBodySize)
}

// BodyUncompressed returns body data and if needed decompresses it from gzip,
// deflate, brotli or zstd.
//
// This method may be used if the response header contains
// 'Content-Encoding' for reading uncompressed response body.
// Use Body for reading the raw response body.
func (resp *Response) BodyUncompressed() ([]byte, error) {
	return resp.BodyUncompressedWithLimit(0)
}

// BodyUncompressedWithLimit returns body data and if needed decompresses it from gzip,
// deflate, brotli or zstd. The size of uncompressed data is limited to maxBodySize bytes.
//
// If maxBodySize <= 0, then no limit is applied.
func (resp *Response) BodyUncompressedWithLimit(maxBodySize int) ([]byte, error) {
	enc := string(resp.Header.ContentEncoding())
	if enc == "" {
		return resp.Body(), nil
	}
	if maxBodySize <= 0 {
		res, err := compress.Decompress(enc, resp.Body(), nil)
		if err != nil {
			if errors.Is(err, compress.ErrUnsupportedEncoding) {
				return nil, ErrContentEncodingUnsupported
			}
			return nil, err
		}
		return res, nil
	}
	switch enc {
	case "deflate":
		return resp.BodyInflateWithLimit(maxBodySize)
	case "gzip":
		return resp.BodyGunzipWithLimit(maxBodySize)
	case "br":
		return resp.BodyUnbrotliWithLimit(maxBodySize)
	case "zstd":
		return resp.BodyUnzstdWithLimit(maxBodySize)
	default:
		return nil, ErrContentEncodingUnsupported
	}
}

// BodyWriteTo writes response body to w.
func (resp *Response) BodyWriteTo(w io.Writer) error {
	if resp.bodyStream != nil {
		_, err := copyBodyStream(w, resp.bodyStream)
		resp.closeBodyStream(err)
		return err
	}
	_, err := w.Write(resp.bodyBytes())
	return err
}

// AppendBody appends p to response body.
//
// It is safe re-using p after the function returns.
func (resp *Response) AppendBody(p []byte) {
	resp.closeBodyStream(nil)
	resp.BodyBuffer().Write(p)
}

// AppendBodyString appends s to response body.
func (resp *Response) AppendBodyString(s string) {
	resp.closeBodyStream(nil)
	resp.BodyBuffer().WriteString(s)
}

// SetBody sets response body.
//
// It is safe re-using body argument after the function returns.
func (resp *Response) SetBody(body []byte) {
	resp.closeBodyStream(nil)
	bodyBuf := resp.BodyBuffer()
	bodyBuf.Reset()
	bodyBuf.Write(body)
}

// SetBodyString sets response body.
func (resp *Response) SetBodyString(body string) {
	resp.closeBodyStream(nil)
	bodyBuf := resp.BodyBuffer()
	bodyBuf.Reset()
	bodyBuf.WriteString(body)
}

// ResetBody resets response body.
func (resp *Response) ResetBody() {
	resp.bodyRaw = nil
	resp.closeBodyStream(nil)
	if resp.body != nil {
		if resp.KeepBodyBuffer {
			resp.body.Reset()
		} else {
			responseBodyPool.Put(resp.body)
			resp.body = nil
		}
	}
}

// SetBodyRaw sets response body, but without copying it.
//
// From this point onward the body argument must not be changed.
func (resp *Response) SetBodyRaw(body []byte) {
	resp.ResetBody()
	resp.bodyRaw = body
}

// ReleaseBody retires the response body if it is greater than "size" bytes.
//
// This permits GC to reclaim the large buffer.  If used, must be before
// ReleaseResponse.
//
// Use this method only if you really understand how it works.
// The majority of workloads don't need this method.
func (resp *Response) ReleaseBody(size int) {
	resp.bodyRaw = nil
	if resp.body == nil {
		return
	}
	if cap(resp.body.B) > size {
		resp.closeBodyStream(nil)
		resp.body = nil
	}
}

// SwapBody swaps response body with the given body and returns
// the previous response body.
//
// It is forbidden to use the body passed to SwapBody after
// the function returns.
func (resp *Response) SwapBody(body []byte) []byte {
	bb := resp.BodyBuffer()
	if resp.bodyStream != nil {
		bb.Reset()
		_, err := copyBodyStream(bb, resp.bodyStream)
		resp.closeBodyStream(err)
		if err != nil {
			bb.Reset()
			bb.SetString(err.Error())
		}
	}
	resp.bodyRaw = nil
	oldBody := bb.B
	bb.B = body
	return oldBody
}

// CopyTo copies resp contents to dst except of body stream.
func (resp *Response) CopyTo(dst *Response) {
	resp.CopyToSkipBody(dst)
	switch {
	case resp.bodyRaw != nil:
		dst.bodyRaw = append(dst.bodyRaw, resp.bodyRaw...)
		if dst.body != nil {
			dst.body.Reset()
		}
	case resp.body != nil:
		dst.BodyBuffer().Set(resp.body.B)
	case dst.body != nil:
		dst.body.Reset()
	}
}

func (resp *Response) CopyToSkipBody(dst *Response) {
	dst.Reset()
	resp.Header.CopyTo(&dst.Header)
	dst.SkipBody = resp.SkipBody
	dst.raddr = resp.raddr
	dst.laddr = resp.laddr
}

// Reset clears response contents.
func (resp *Response) Reset() {
	if bodyPoolSizeLimit := int(responseBodyPoolSizeLimit.Load()); bodyPoolSizeLimit >= 0 && resp.body != nil {
		resp.ReleaseBody(bodyPoolSizeLimit)
	}
	resp.resetSkipHeader()
	resp.Header.Reset()
	resp.SkipBody = false
	resp.raddr = nil
	resp.laddr = nil
	resp.ImmediateHeaderFlush = false
	resp.StreamBody = false
}

func (resp *Response) resetSkipHeader() {
	resp.ResetBody()
}

// Read reads response (including body) from the given r.
//
// Read does not limit the response body size. Use ReadLimitBody with a positive
// maxBodySize when reading responses from untrusted sources.
//
// io.EOF is returned if r is closed before reading the first header byte.
func (resp *Response) Read(r *bufio.Reader) error {
	return resp.ReadLimitBody(r, 0)
}

// ReadLimitBody reads response headers from the given r,
// then reads the body using the ReadBody function and limiting the body size.
//
// Informational responses other than "101 Switching Protocols" are consumed
// before the final response is read.
//
// If resp.SkipBody is true then it skips reading the response body.
//
// If maxBodySize > 0 and the body size exceeds maxBodySize,
// then ErrBodyTooLarge is returned.
// If maxBodySize <= 0, no limit is applied and the response may consume
// unbounded memory.
//
// io.EOF is returned if r is closed before reading the first header byte.
func (resp *Response) ReadLimitBody(r *bufio.Reader, maxBodySize int) error {
	resp.resetSkipHeader()
	err := resp.Header.Read(r)
	if err != nil {
		return err
	}
	for n := 0; ; n++ {
		if resp.Header.statusCode < 100 || resp.Header.statusCode > 199 || resp.Header.statusCode == status.SwitchingProtocols {
			break
		}
		if n >= maxInterimResponses {
			return errTooManyInterimResponses
		}
		if resp.OnInterimResponse != nil {
			resp.OnInterimResponse(resp.Header.statusCode, &resp.Header)
		}
		if err = resp.Header.Read(r); err != nil {
			return err
		}
	}
	if !resp.mustSkipBody() {
		err = resp.ReadBody(r, maxBodySize)
		if err != nil {
			return err
		}
	}
	if resp.Header.ContentLength() == -1 && !resp.StreamBody && !resp.mustSkipBody() {
		err = resp.Header.ReadTrailer(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return ErrBrokenChunk{error: io.ErrUnexpectedEOF}
			}
			return err
		}
	}
	return nil
}

// ReadBody reads response body from the given r, limiting the body size.
//
// If maxBodySize > 0 and the body size exceeds maxBodySize,
// then ErrBodyTooLarge is returned.
func (resp *Response) ReadBody(r *bufio.Reader, maxBodySize int) (err error) {
	bodyBuf := resp.BodyBuffer()
	bodyBuf.Reset()
	contentLength := resp.Header.ContentLength()
	switch {
	case contentLength >= 0:
		bodyBuf.B, err = readBody(r, contentLength, maxBodySize, bodyBuf.B)
		if errors.Is(err, ErrBodyTooLarge) && resp.StreamBody {
			resp.bodyStream = AcquireRequestStream(bodyBuf, r, &resp.Header)
			err = nil
		}
	case contentLength == -1:
		if resp.StreamBody {
			resp.bodyStream = AcquireRequestStream(bodyBuf, r, &resp.Header)
		} else {
			bodyBuf.B, err = readBodyChunked(r, maxBodySize, bodyBuf.B)
		}
	default:
		if resp.StreamBody {
			resp.bodyStream = AcquireRequestStream(bodyBuf, r, &resp.Header)
		} else {
			bodyBuf.B, err = readBodyIdentity(r, maxBodySize, bodyBuf.B)
			resp.Header.SetContentLength(len(bodyBuf.B))
		}
	}
	if err == nil && resp.StreamBody && resp.bodyStream == nil {
		resp.bodyStream = bytes.NewReader(bodyBuf.B)
	}
	return err
}

func (resp *Response) mustSkipBody() bool {
	return resp.SkipBody || resp.Header.mustSkipContentLength()
}

// WriteTo writes response to w. It implements io.WriterTo.
func (resp *Response) WriteTo(w io.Writer) (int64, error) {
	return writeBufio(resp, w)
}

// WriteGzip writes response with gzipped body to w.
//
// The method gzips response body and sets 'Content-Encoding: gzip'
// header before writing response to w.
//
// WriteGzip doesn't flush response to w for performance reasons.
func (resp *Response) WriteGzip(w *bufio.Writer) error {
	return resp.WriteGzipLevel(w, machcompress.CompressDefaultCompression)
}

// WriteGzipLevel writes response with gzipped body to w.
//
// Level is the desired compression level:
//
//   - CompressNoCompression
//   - CompressBestSpeed
//   - CompressBestCompression
//   - machcompress.CompressDefaultCompression
//   - CompressHuffmanOnly
//
// The method gzips response body and sets 'Content-Encoding: gzip'
// header before writing response to w.
//
// WriteGzipLevel doesn't flush response to w for performance reasons.
func (resp *Response) WriteGzipLevel(w *bufio.Writer, level int) error {
	resp.gzipBody(level)
	return resp.Write(w)
}

// WriteDeflate writes response with deflated body to w.
//
// The method deflates response body and sets 'Content-Encoding: deflate'
// header before writing response to w.
//
// WriteDeflate doesn't flush response to w for performance reasons.
func (resp *Response) WriteDeflate(w *bufio.Writer) error {
	return resp.WriteDeflateLevel(w, machcompress.CompressDefaultCompression)
}

// WriteDeflateLevel writes response with deflated body to w.
//
// Level is the desired compression level:
//
//   - CompressNoCompression
//   - CompressBestSpeed
//   - CompressBestCompression
//   - machcompress.CompressDefaultCompression
//   - CompressHuffmanOnly
//
// The method deflates response body and sets 'Content-Encoding: deflate'
// header before writing response to w.
//
// WriteDeflateLevel doesn't flush response to w for performance reasons.
func (resp *Response) WriteDeflateLevel(w *bufio.Writer, level int) error {
	resp.deflateBody(level)
	return resp.Write(w)
}

func (resp *Response) brotliBody(level int) {
	if len(resp.Header.ContentEncoding()) > 0 {
		return
	}
	if !resp.Header.isCompressibleContentType() {
		return
	}
	if resp.bodyStream != nil {
		resp.Header.SetContentLength(-1)
		resp.bodyStream = newCompressedBodyStream(resp.bodyStream, level, compressBrotliBodyStream)
	} else {
		bodyBytes := resp.bodyBytes()
		if len(bodyBytes) < minCompressLen {
			return
		}
		w := responseBodyPool.Get()
		w.B = machcompress.AppendBrotliBytesLevel(w.B, bodyBytes, level)
		if resp.body != nil {
			responseBodyPool.Put(resp.body)
		}
		resp.body = w
		resp.bodyRaw = nil
	}
	resp.Header.SetContentEncodingBytes(zerocopy.StrBr)
	resp.Header.addVaryBytes(zerocopy.StrAcceptEncoding)
} //nolint:unused

func (resp *Response) gzipBody(level int) {
	if len(resp.Header.ContentEncoding()) > 0 {
		return
	}
	if !resp.Header.isCompressibleContentType() {
		return
	}
	if resp.bodyStream != nil {
		resp.Header.SetContentLength(-1)
		resp.bodyStream = newCompressedBodyStream(resp.bodyStream, level, compressGzipBodyStream)
	} else {
		bodyBytes := resp.bodyBytes()
		if len(bodyBytes) < minCompressLen {
			return
		}
		w := responseBodyPool.Get()
		w.B = machcompress.AppendGzipBytesLevel(w.B, bodyBytes, level)
		if resp.body != nil {
			responseBodyPool.Put(resp.body)
		}
		resp.body = w
		resp.bodyRaw = nil
	}
	resp.Header.SetContentEncodingBytes(zerocopy.StrGzip)
	resp.Header.addVaryBytes(zerocopy.StrAcceptEncoding)
}

func (resp *Response) deflateBody(level int) {
	if len(resp.Header.ContentEncoding()) > 0 {
		return
	}
	if !resp.Header.isCompressibleContentType() {
		return
	}
	if resp.bodyStream != nil {
		resp.Header.SetContentLength(-1)
		resp.bodyStream = newCompressedBodyStream(resp.bodyStream, level, compressDeflateBodyStream)
	} else {
		bodyBytes := resp.bodyBytes()
		if len(bodyBytes) < minCompressLen {
			return
		}
		w := responseBodyPool.Get()
		w.B = machcompress.AppendDeflateBytesLevel(w.B, bodyBytes, level)
		if resp.body != nil {
			responseBodyPool.Put(resp.body)
		}
		resp.body = w
		resp.bodyRaw = nil
	}
	resp.Header.SetContentEncodingBytes(zerocopy.StrDeflate)
	resp.Header.addVaryBytes(zerocopy.StrAcceptEncoding)
}

func (resp *Response) zstdBody(level int) {
	if len(resp.Header.ContentEncoding()) > 0 {
		return
	}
	if !resp.Header.isCompressibleContentType() {
		return
	}
	if resp.bodyStream != nil {
		resp.Header.SetContentLength(-1)
		resp.bodyStream = newCompressedBodyStream(resp.bodyStream, level, compressZstdBodyStream)
	} else {
		bodyBytes := resp.bodyBytes()
		if len(bodyBytes) < minCompressLen {
			return
		}
		w := responseBodyPool.Get()
		w.B = machcompress.AppendZstdBytesLevel(w.B, bodyBytes, level)
		if resp.body != nil {
			responseBodyPool.Put(resp.body)
		}
		resp.body = w
		resp.bodyRaw = nil
	}
	resp.Header.SetContentEncodingBytes(zerocopy.StrZstd)
	resp.Header.addVaryBytes(zerocopy.StrAcceptEncoding)
} //nolint:unused

// Write writes response to w.
//
// Write doesn't flush response to w for performance reasons.
//
// See also WriteTo.
func (resp *Response) Write(w *bufio.Writer) error {
	sendBody := !resp.mustSkipBody()
	if resp.bodyStream != nil {
		return resp.writeBodyStream(w, sendBody)
	}
	body := resp.bodyBytes()
	bodyLen := len(body)
	if sendBody || bodyLen > 0 {
		resp.Header.SetContentLength(bodyLen)
	}
	if err := resp.Header.Write(w); err != nil {
		return err
	}
	if sendBody {
		if _, err := w.Write(body); err != nil {
			return err
		}
	}
	return nil
}

func (resp *Response) writeBodyStream(w *bufio.Writer, sendBody bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &ErrBodyStreamWritePanic{error: fmt.Errorf("panic while writing body stream: %+v", r)}
		}
	}()
	contentLength := resp.Header.ContentLength()
	if contentLength < 0 {
		lrSize := limitedReaderSize(resp.bodyStream)
		if lrSize >= 0 {
			contentLength = int(lrSize)
			if int64(contentLength) != lrSize {
				contentLength = -1
			}
			if contentLength >= 0 {
				resp.Header.SetContentLength(contentLength)
			}
		}
	}
	if contentLength >= 0 {
		if err = resp.Header.Write(w); err == nil {
			if resp.ImmediateHeaderFlush {
				err = w.Flush()
			}
			if err == nil && sendBody {
				err = writeBodyFixedSize(w, resp.bodyStream, int64(contentLength))
			}
		}
	} else {
		resp.Header.SetContentLength(-1)
		if err = resp.Header.Write(w); err == nil {
			if resp.ImmediateHeaderFlush {
				err = w.Flush()
			}
			if err == nil && sendBody {
				err = writeBodyChunked(w, resp.bodyStream)
			}
			if err == nil {
				err = resp.Header.writeTrailer(w)
			}
		}
	}
	errc := resp.closeBodyStream(err)
	if err == nil {
		err = errc
	}
	return err
}

func (resp *Response) closeBodyStream(wErr error) error {
	if resp.bodyStream == nil {
		return nil
	}
	err := closeBodyStreamReader(resp.bodyStream, wErr)
	resp.bodyStream = nil
	return err
}

// String returns response representation.
//
// Returns error message instead of response representation on error.
//
// Use Write instead of String for performance-critical code.
func (resp *Response) String() string {
	return getHTTPString(resp)
}

// BodyScoped borrows the response body without memory allocation.
func (resp *Response) BodyScoped(s *borrow.Scope) borrow.Bytes {
	b := resp.Body()
	if len(b) == 0 {
		return borrow.Bytes{}
	}
	return borrow.NewBytes(b, nil)
}

// ReadBodyScoped executes fn with the underlying response body buffer borrowed for the duration of the call.
func (resp *Response) ReadBodyScoped(fn func([]byte) error) error {
	s := borrow.AcquireScope()
	defer s.Release()
	b := resp.Body()
	return fn(b)
}

// ReadStreamScoped reads from the response body stream chunk by chunk, passing each borrowed slice to fn.
func (resp *Response) ReadStreamScoped(s *borrow.Scope, fn func(chunk borrow.Bytes) error) error {
	r := resp.BodyStream()
	if r == nil {
		b := resp.Body()
		if len(b) > 0 {
			return fn(borrow.NewBytes(b, nil))
		}
		return nil
	}
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if callErr := fn(borrow.NewBytes(buf[:n], nil)); callErr != nil {
				return callErr
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	return nil
}
