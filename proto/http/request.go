
package http

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"time"

	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/codec/compress"
	"github.com/lemon4ksan/foundation/net/http/zerocopy"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Request represents HTTP request.
//
// It is forbidden copying Request instances. Create new instances
// and use CopyTo instead.
//
// Request instance MUST NOT be used from concurrently running goroutines.
type Request struct {
	noCopy                zerocopy.NoCopy
	bodyStream            io.Reader
	w                     requestBodyWriter
	body                  *bytesconv.ByteBuffer
	multipartForm         *multipart.Form
	multipartFormBoundary string
	postArgs              zerocopy.Args
	bodyRaw               []byte

	uri zerocopy.URI

	// Header is the request header.
	//
	// Copying Header by value is forbidden. Use pointer to Header instead.
	Header RequestHeader

	Timeout               time.Duration
	SecureErrorLogMessage bool
	parsedURI             bool
	ParsedPostArgs        bool
	uriParseErr           error
	KeepBodyBuffer        bool
	isTLS                 bool
	UseHostHeader         bool

	// DisableRedirectPathNormalizing disables redirect path normalization when used with DoRedirects.
	//
	// By default redirect path values are normalized, i.e.
	// extra slashes are removed, special characters are encoded.
	DisableRedirectPathNormalizing bool
}

// SetHost sets host for the request.
func (req *Request) SetHost(host string) {
	req.URI().SetHost(host)
}

// SetHostBytes sets host for the request.
func (req *Request) SetHostBytes(host []byte) {
	req.URI().SetHostBytes(host)
}

// Host returns the host for the given request.
func (req *Request) Host() []byte {
	return req.URI().Host()
}

// SetRequestURI sets RequestURI.
func (req *Request) SetRequestURI(requestURI string) {
	req.Header.SetRequestURI(requestURI)
	req.parsedURI = false
	req.uriParseErr = nil
}

// SetRequestURIBytes sets RequestURI.
func (req *Request) SetRequestURIBytes(requestURI []byte) {
	req.Header.SetRequestURIBytes(requestURI)
	req.parsedURI = false
	req.uriParseErr = nil
}

// RequestURI returns request's zerocopy.URI.
func (req *Request) RequestURI() []byte {
	if req.parsedURI {
		requestURI := req.uri.RequestURI()
		req.SetRequestURIBytes(requestURI)
	}
	return req.Header.RequestURI()
}

// ConnectionClose returns true if 'Connection: close' header is set.
func (req *Request) ConnectionClose() bool {
	return req.Header.ConnectionClose()
}

// SetConnectionClose sets 'Connection: close' header.
func (req *Request) SetConnectionClose() {
	req.Header.SetConnectionClose()
}

// GetTimeOut retrieves the timeout duration set for the Request.
//
// This method returns a time.Duration that determines how long the request
// can wait before it times out. In the default use case, the timeout applies
// to the entire request lifecycle, including both receiving the response
// headers and the response body.
func (req *Request) GetTimeOut() time.Duration {
	return req.Timeout
}

// SetBodyStream sets request body stream and, optionally body size.
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
// Note that GET and HEAD requests cannot have body.
//
// See also SetBodyStreamWriter.
func (req *Request) SetBodyStream(bodyStream io.Reader, bodySize int) {
	req.ResetBody()
	req.bodyStream = bodyStream
	req.Header.SetContentLength(bodySize)
}

// IsBodyStream returns true if body is set via SetBodyStream*.
func (req *Request) IsBodyStream() bool {
	return req.bodyStream != nil
}

// SetBodyStreamWriter registers the given sw for populating request body.
//
// This function may be used in the following cases:
//
//   - if request body is too big (more than 10MB).
//   - if request body is streamed from slow external sources.
//   - if request body must be streamed to the server in chunks
//     (aka `http client push` or `chunked transfer-encoding`).
//
// Note that GET and HEAD requests cannot have body.
//
// See also SetBodyStream.
func (req *Request) SetBodyStreamWriter(sw StreamWriter) {
	sr := NewStreamReader(sw)
	req.SetBodyStream(sr, -1)
}

// BodyStream returns io.Reader.
//
// You must CloseBodyStream or ReleaseRequest after you use it.
func (req *Request) BodyStream() io.Reader {
	return req.bodyStream
}

func (req *Request) CloseBodyStream() error {
	return req.closeBodyStream()
}

// BodyWriter returns writer for populating request body.
func (req *Request) BodyWriter() io.Writer {
	req.w.r = req
	return &req.w
}

func (req *Request) bodyBytes() []byte {
	if req.bodyRaw != nil {
		return req.bodyRaw
	}
	if req.bodyStream != nil {
		bodyBuf := req.BodyBuffer()
		bodyBuf.Reset()
		_, err := copyBodyStream(bodyBuf, req.bodyStream)
		req.closeBodyStream()
		if err != nil {
			bodyBuf.SetString(err.Error())
		}
	}
	if req.body == nil {
		return nil
	}
	return req.body.B
}

func (req *Request) BodyBuffer() *bytesconv.ByteBuffer {
	if req.body == nil {
		req.body = requestBodyPool.Get()
	}
	req.bodyRaw = nil
	return req.body
}

// BodyGunzip returns un-gzipped body data.
//
// This method may be used if the request header contains
// 'Content-Encoding: gzip' for reading un-gzipped body.
// Use Body for reading gzipped request body.
func (req *Request) BodyGunzip() ([]byte, error) {
	return req.BodyGunzipWithLimit(0)
}

// BodyGunzipWithLimit returns un-gzipped body data and limits the size
// of uncompressed body data to maxBodySize bytes.
//
// If maxBodySize <= 0, then no limit is applied.
func (req *Request) BodyGunzipWithLimit(maxBodySize int) ([]byte, error) {
	return gunzipData(req.Body(), maxBodySize)
}

// BodyUnbrotli returns un-brotlied body data.
//
// This method may be used if the request header contains
// 'Content-Encoding: br' for reading un-brotlied body.
// Use Body for reading brotlied request body.
func (req *Request) BodyUnbrotli() ([]byte, error) {
	return req.BodyUnbrotliWithLimit(0)
}

// BodyUnbrotliWithLimit returns un-brotlied body data and limits the size
// of uncompressed body data to maxBodySize bytes.
//
// If maxBodySize <= 0, then no limit is applied.
func (req *Request) BodyUnbrotliWithLimit(maxBodySize int) ([]byte, error) {
	return unBrotliData(req.Body(), maxBodySize)
}

// BodyInflate returns inflated body data.
//
// This method may be used if the response header contains
// 'Content-Encoding: deflate' for reading inflated request body.
// Use Body for reading deflated request body.
func (req *Request) BodyInflate() ([]byte, error) {
	return req.BodyInflateWithLimit(0)
}

// BodyInflateWithLimit returns inflated body data and limits the size
// of uncompressed body data to maxBodySize bytes.
//
// If maxBodySize <= 0, then no limit is applied.
func (req *Request) BodyInflateWithLimit(maxBodySize int) ([]byte, error) {
	return inflateData(req.Body(), maxBodySize)
}

func (req *Request) RequestBodyStream() io.Reader {
	return req.bodyStream
}

func (req *Request) BodyUnzstd() ([]byte, error) {
	return req.BodyUnzstdWithLimit(0)
}

// BodyUnzstdWithLimit returns un-zstd body data and limits the size
// of uncompressed body data to maxBodySize bytes.
//
// If maxBodySize <= 0, then no limit is applied.
func (req *Request) BodyUnzstdWithLimit(maxBodySize int) ([]byte, error) {
	return unzstdData(req.Body(), maxBodySize)
}

// BodyUncompressed returns body data and if needed decompresses it from gzip,
// deflate, brotli or zstd.
//
// This method may be used if the response header contains
// 'Content-Encoding' for reading uncompressed request body.
// Use Body for reading the raw request body.
func (req *Request) BodyUncompressed() ([]byte, error) {
	return req.BodyUncompressedWithLimit(0)
}

// BodyUncompressedWithLimit returns body data and if needed decompresses it from gzip,
// deflate, brotli or zstd. The size of uncompressed data is limited to maxBodySize bytes.
//
// If maxBodySize <= 0, then no limit is applied.
func (req *Request) BodyUncompressedWithLimit(maxBodySize int) ([]byte, error) {
	enc := string(req.Header.ContentEncoding())
	if enc == "" {
		return req.Body(), nil
	}
	if maxBodySize <= 0 {
		res, err := compress.Decompress(enc, req.Body(), nil)
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
		return req.BodyInflateWithLimit(maxBodySize)
	case "gzip":
		return req.BodyGunzipWithLimit(maxBodySize)
	case "br":
		return req.BodyUnbrotliWithLimit(maxBodySize)
	case "zstd":
		return req.BodyUnzstdWithLimit(maxBodySize)
	default:
		return nil, ErrContentEncodingUnsupported
	}
}

// BodyWriteTo writes request body to w.
func (req *Request) BodyWriteTo(w io.Writer) error {
	if req.bodyStream != nil {
		_, err := copyBodyStream(w, req.bodyStream)
		req.closeBodyStream()
		return err
	}
	if req.onlyMultipartForm() {
		return WriteMultipartForm(w, req.multipartForm, req.multipartFormBoundary)
	}
	_, err := w.Write(req.bodyBytes())
	return err
}

// SetBodyRaw sets response body, but without copying it.
//
// From this point onward the body argument must not be changed.
func (req *Request) SetBodyRaw(body []byte) {
	req.ResetBody()
	req.bodyRaw = body
}

// ReleaseBody retires the request body if it is greater than "size" bytes.
//
// This permits GC to reclaim the large buffer.  If used, must be before
// ReleaseRequest.
//
// Use this method only if you really understand how it works.
// The majority of workloads don't need this method.
func (req *Request) ReleaseBody(size int) {
	req.bodyRaw = nil
	if req.body == nil {
		return
	}
	if cap(req.body.B) > size {
		req.closeBodyStream()
		req.body = nil
	}
}

// SwapBody swaps request body with the given body and returns
// the previous request body.
//
// It is forbidden to use the body passed to SwapBody after
// the function returns.
func (req *Request) SwapBody(body []byte) []byte {
	bb := req.BodyBuffer()
	if req.bodyStream != nil {
		bb.Reset()
		_, err := copyBodyStream(bb, req.bodyStream)
		req.closeBodyStream()
		if err != nil {
			bb.Reset()
			bb.SetString(err.Error())
		}
	}
	req.bodyRaw = nil
	oldBody := bb.B
	bb.B = body
	return oldBody
}

// Body returns request body.
//
// The returned value is valid until the request is released,
// either though ReleaseRequest or your request handler returning.
// Do not store references to returned value. Make copies instead.
//
// If the body is backed by a stream, Body reads the entire stream into memory.
// Use BodyStream to read it incrementally.
func (req *Request) Body() []byte {
	if req.bodyRaw != nil {
		return req.bodyRaw
	} else if req.onlyMultipartForm() {
		body, err := marshalMultipartForm(req.multipartForm, req.multipartFormBoundary)
		if err != nil {
			return []byte(err.Error())
		}
		return body
	}
	return req.bodyBytes()
}

// AppendBody appends p to request body.
//
// It is safe re-using p after the function returns.
func (req *Request) AppendBody(p []byte) {
	req.RemoveMultipartFormFiles()
	req.closeBodyStream()
	req.BodyBuffer().Write(p)
}

// AppendBodyString appends s to request body.
func (req *Request) AppendBodyString(s string) {
	req.RemoveMultipartFormFiles()
	req.closeBodyStream()
	req.BodyBuffer().WriteString(s)
}

// SetBody sets request body.
//
// It is safe re-using body argument after the function returns.
func (req *Request) SetBody(body []byte) {
	req.RemoveMultipartFormFiles()
	req.closeBodyStream()
	req.BodyBuffer().Set(body)
}

// SetBodyString sets request body.
func (req *Request) SetBodyString(body string) {
	req.RemoveMultipartFormFiles()
	req.closeBodyStream()
	req.BodyBuffer().SetString(body)
}

// ResetBody resets request body.
func (req *Request) ResetBody() {
	req.bodyRaw = nil
	req.RemoveMultipartFormFiles()
	req.closeBodyStream()
	if req.body != nil {
		if req.KeepBodyBuffer {
			req.body.Reset()
		} else {
			requestBodyPool.Put(req.body)
			req.body = nil
		}
	}
}

// CopyTo copies req contents to dst except of body stream.
func (req *Request) CopyTo(dst *Request) {
	req.CopyToSkipBody(dst)
	switch {
	case req.bodyRaw != nil:
		dst.bodyRaw = append(dst.bodyRaw[:0], req.bodyRaw...)
		if dst.body != nil {
			dst.body.Reset()
		}
	case req.body != nil:
		dst.BodyBuffer().Set(req.body.B)
	case dst.body != nil:
		dst.body.Reset()
	}
}

func (req *Request) CopyToSkipBody(dst *Request) {
	dst.Reset()
	req.Header.CopyTo(&dst.Header)
	req.uri.CopyTo(&dst.uri)
	dst.parsedURI = req.parsedURI
	dst.uriParseErr = req.uriParseErr
	req.postArgs.CopyTo(&dst.postArgs)
	dst.ParsedPostArgs = req.ParsedPostArgs
	dst.isTLS = req.isTLS
	dst.UseHostHeader = req.UseHostHeader
}

// URI returns request URI.
func (req *Request) URI() *zerocopy.URI {
	req.ParseURI()
	return &req.uri
}

// SetURI initializes request URI.
// Use this method if a single URI may be reused across multiple requests.
// Otherwise, you can just use SetRequestURI() and it will be parsed as new URI.
// The URI is copied and can be safely modified later.
func (req *Request) SetURI(newURI *zerocopy.URI) {
	if newURI != nil {
		newURI.CopyTo(&req.uri)
		req.parsedURI = true
		req.uriParseErr = nil
		return
	}
	req.uri.Reset()
	req.parsedURI = false
	req.uriParseErr = nil
}

func (req *Request) ParseURI() error {
	if req.parsedURI {
		return req.uriParseErr
	}
	req.parsedURI = true
	req.uriParseErr = req.uri.ParseInternal(req.Header.Host(), req.Header.RequestURI(), req.isTLS)
	return req.uriParseErr
}

// PostArgs returns POST arguments.
func (req *Request) PostArgs() *zerocopy.Args {
	req.parsePostArgs()
	return &req.postArgs
}

func (req *Request) parsePostArgs() {
	if req.ParsedPostArgs {
		return
	}
	req.ParsedPostArgs = true
	if !bytes.HasPrefix(req.Header.ContentType(), zerocopy.StrPostArgsContentType) {
		return
	}
	req.postArgs.ParseBytes(req.bodyBytes())
}

// MultipartForm returns request's multipart form.
//
// Returns ErrNoMultipartForm if request's Content-Type
// isn't 'multipart/form-data'.
//
// This method is equivalent to MultipartFormWithLimit(0), i.e. no body size
// limit is applied during multipart parsing.
//
// RemoveMultipartFormFiles must be called after returned multipart form
// is processed.
func (req *Request) MultipartForm() (*multipart.Form, error) {
	return req.MultipartFormWithLimit(0)
}

// MultipartFormWithLimit returns request's multipart form and limits the
// read multipart body size to maxBodySize bytes.
//
// If maxBodySize <= 0, then no limit is applied.
//
// Returns ErrNoMultipartForm if request's Content-Type
// isn't 'multipart/form-data'.
//
// RemoveMultipartFormFiles must be called after returned multipart form
// is processed.
func (req *Request) MultipartFormWithLimit(maxBodySize int) (*multipart.Form, error) {
	if req.multipartForm != nil {
		return req.multipartForm, nil
	}
	req.multipartFormBoundary = string(req.Header.MultipartFormBoundary())
	if req.multipartFormBoundary == "" {
		return nil, ErrNoMultipartForm
	}
	var err error
	ce := req.Header.peek(zerocopy.StrContentEncoding)
	if req.bodyStream != nil {
		bodyStream := req.bodyStream
		var lr *io.LimitedReader
		if bytes.Equal(ce, zerocopy.StrGzip) {
			if bodyStream, err = gzip.NewReader(bodyStream); err != nil {
				return nil, fmt.Errorf("cannot gunzip request body: %w", err)
			}
		} else if len(ce) > 0 {
			return nil, fmt.Errorf("unsupported content-encoding: %q", ce)
		}
		if maxBodySize > 0 {
			lr = &io.LimitedReader{R: bodyStream, N: int64(maxBodySize) + 1}
			bodyStream = lr
		}
		mr := multipart.NewReader(bodyStream, req.multipartFormBoundary)
		req.multipartForm, err = mr.ReadForm(8 * 1024)
		if err != nil {
			if lr != nil && lr.N <= 0 {
				return nil, fmt.Errorf("cannot read multipart/form-data body: %w", ErrBodyTooLarge)
			}
			return nil, fmt.Errorf("cannot read multipart/form-data body: %w", err)
		}
		if lr != nil && lr.N <= 0 {
			req.RemoveMultipartFormFiles()
			return nil, fmt.Errorf("cannot read multipart/form-data body: %w", ErrBodyTooLarge)
		}
	} else {
		body := req.bodyBytes()
		if bytes.Equal(ce, zerocopy.StrGzip) {
			if body, err = gunzipData(body, maxBodySize); err != nil {
				return nil, fmt.Errorf("cannot gunzip request body: %w", err)
			}
		} else if len(ce) > 0 {
			return nil, fmt.Errorf("unsupported content-encoding: %q", ce)
		}
		if maxBodySize > 0 && len(body) > maxBodySize {
			return nil, fmt.Errorf("cannot read multipart/form-data body: %w", ErrBodyTooLarge)
		}
		req.multipartForm, err = readMultipartForm(bytes.NewReader(body), req.multipartFormBoundary, len(body), len(body))
		if err != nil {
			return nil, err
		}
	}
	return req.multipartForm, nil
}

// Reset clears request contents.
func (req *Request) Reset() {
	if bodyPoolSizeLimit := int(requestBodyPoolSizeLimit.Load()); bodyPoolSizeLimit >= 0 && req.body != nil {
		req.ReleaseBody(bodyPoolSizeLimit)
	}
	req.Header.Reset()
	req.resetSkipHeader()
	req.Timeout = 0
	req.UseHostHeader = false
	req.DisableRedirectPathNormalizing = false
}

func (req *Request) resetSkipHeader() {
	req.ResetBody()
	req.uri.Reset()
	req.parsedURI = false
	req.uriParseErr = nil
	req.postArgs.Reset()
	req.ParsedPostArgs = false
	req.isTLS = false
}

// RemoveMultipartFormFiles removes multipart/form-data temporary files
// associated with the request.
func (req *Request) RemoveMultipartFormFiles() {
	if req.multipartForm != nil {
		req.multipartForm.RemoveAll()
		req.multipartForm = nil
	}
	req.multipartFormBoundary = ""
}

// Read reads request (including body) from the given r.
//
// Read does not limit the request body size. Use ReadLimitBody with a positive
// maxBodySize when reading requests from untrusted sources.
//
// RemoveMultipartFormFiles or Reset must be called after
// reading multipart/form-data request in order to delete temporarily
// uploaded files.
//
// If MayContinue returns true, the caller must:
//
//   - Either send StatusExpectationFailed response if request headers don't
//     satisfy the caller.
//   - Or send StatusContinue response before reading request body
//     with ContinueReadBody.
//   - Or close the connection.
//
// io.EOF is returned if r is closed before reading the first header byte.
func (req *Request) Read(r *bufio.Reader) error {
	return req.ReadLimitBody(r, 0)
}

// ReadLimitBody reads request from the given r, limiting the body size.
//
// If maxBodySize > 0 and the body size exceeds maxBodySize,
// then ErrBodyTooLarge is returned.
// If maxBodySize <= 0, no limit is applied and the request may consume
// unbounded memory.
//
// RemoveMultipartFormFiles or Reset must be called after
// reading multipart/form-data request in order to delete temporarily
// uploaded files.
//
// If MayContinue returns true, the caller must:
//
//   - Either send StatusExpectationFailed response if request headers don't
//     satisfy the caller.
//   - Or send StatusContinue response before reading request body
//     with ContinueReadBody.
//   - Or close the connection.
//
// io.EOF is returned if r is closed before reading the first header byte.
func (req *Request) ReadLimitBody(r *bufio.Reader, maxBodySize int) error {
	req.resetSkipHeader()
	if err := req.Header.Read(r); err != nil {
		return err
	}
	return req.readLimitBody(r, maxBodySize, false, true)
}

func (req *Request) readLimitBody(r *bufio.Reader, maxBodySize int, getOnly, preParseMultipartForm bool) error {
	if getOnly && !req.Header.IsGet() && !req.Header.IsHead() {
		return ErrGetOnly
	}
	if req.MayContinue() {
		return nil
	}
	return req.ContinueReadBody(r, maxBodySize, preParseMultipartForm)
}

// MayContinue returns true if the request contains
// 'Expect: 100-continue' header.
//
// The caller must do one of the following actions if MayContinue returns true:
//
//   - Either send StatusExpectationFailed response if request headers don't
//     satisfy the caller.
//   - Or send StatusContinue response before reading request body
//     with ContinueReadBody.
//   - Or close the connection.
func (req *Request) MayContinue() bool {
	return bytes.Equal(req.Header.peek(zerocopy.StrExpect), zerocopy.Str100Continue)
}

// ContinueReadBody reads request body if request header contains
// 'Expect: 100-continue'.
//
// The caller must send StatusContinue response before calling this method.
//
// If maxBodySize > 0 and the body size exceeds maxBodySize,
// then ErrBodyTooLarge is returned.
func (req *Request) ContinueReadBody(r *bufio.Reader, maxBodySize int, preParseMultipartForm ...bool) error {
	var err error
	contentLength := req.Header.ContentLength()
	if contentLength > 0 {
		if maxBodySize > 0 && contentLength > maxBodySize {
			return ErrBodyTooLarge
		}
		if len(preParseMultipartForm) == 0 || preParseMultipartForm[0] {
			req.multipartFormBoundary = string(req.Header.MultipartFormBoundary())
			if req.multipartFormBoundary != "" && len(req.Header.peek(zerocopy.StrContentEncoding)) == 0 {
				req.multipartForm, err = readMultipartForm(r, req.multipartFormBoundary, contentLength, defaultMaxInMemoryFileSize)
				if err != nil {
					req.Reset()
				}
				return err
			}
		}
	}
	if contentLength == -2 {
		if !req.Header.ignoreBody() {
			req.Header.SetContentLength(0)
		}
		return nil
	}
	if err = req.ReadBody(r, contentLength, maxBodySize); err != nil {
		return err
	}
	if contentLength == -1 {
		err = req.Header.ReadTrailer(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return ErrBrokenChunk{error: io.ErrUnexpectedEOF}
			}
			return err
		}
	}
	return nil
}

// ReadBody reads request body from the given r, limiting the body size.
//
// If maxBodySize > 0 and the body size exceeds maxBodySize,
// then ErrBodyTooLarge is returned.
func (req *Request) ReadBody(r *bufio.Reader, contentLength, maxBodySize int) (err error) {
	bodyBuf := req.BodyBuffer()
	bodyBuf.Reset()
	switch {
	case contentLength >= 0:
		bodyBuf.B, err = readBody(r, contentLength, maxBodySize, bodyBuf.B)
	case contentLength == -1:
		bodyBuf.B, err = readBodyChunked(r, maxBodySize, bodyBuf.B)
		if err == nil && len(bodyBuf.B) == 0 {
			req.Header.SetContentLength(0)
		}
	default:
		bodyBuf.B, err = readBodyIdentity(r, maxBodySize, bodyBuf.B)
		req.Header.SetContentLength(len(bodyBuf.B))
	}
	if err != nil {
		req.Reset()
		return err
	}
	return nil
}

// ContinueReadBodyStream reads request body if request header contains
// 'Expect: 100-continue'.
//
// The caller must send StatusContinue response before calling this method.
//
// If maxBodySize > 0 and the body size exceeds maxBodySize,
// then ErrBodyTooLarge is returned.
func (req *Request) ContinueReadBodyStream(r *bufio.Reader, maxBodySize int, preParseMultipartForm ...bool) error {
	var err error
	contentLength := req.Header.ContentLength()
	if contentLength > 0 {
		if len(preParseMultipartForm) == 0 || preParseMultipartForm[0] {
			req.multipartFormBoundary = bytesconv.B2S(req.Header.MultipartFormBoundary())
			if req.multipartFormBoundary != "" && len(req.Header.peek(zerocopy.StrContentEncoding)) == 0 {
				req.multipartForm, err = readMultipartForm(r, req.multipartFormBoundary, contentLength, defaultMaxInMemoryFileSize)
				if err != nil {
					req.Reset()
				}
				return err
			}
		}
	}
	if contentLength == -2 {
		if !req.Header.ignoreBody() {
			req.Header.SetContentLength(0)
		}
		return nil
	}
	bodyBuf := req.BodyBuffer()
	bodyBuf.Reset()
	bodyBuf.B, err = readBodyWithStreaming(r, contentLength, maxBodySize, bodyBuf.B)
	if err != nil {
		if errors.Is(err, ErrBodyTooLarge) {
			req.Header.SetContentLength(contentLength)
			req.body = bodyBuf
			req.bodyStream = AcquireRequestStream(bodyBuf, r, &req.Header)
			return nil
		}
		if errors.Is(err, errChunkedStream) {
			req.body = bodyBuf
			req.bodyStream = AcquireRequestStream(bodyBuf, r, &req.Header)
			return nil
		}
		req.Reset()
		return err
	}
	req.body = bodyBuf
	req.bodyStream = AcquireRequestStream(bodyBuf, r, &req.Header)
	req.Header.SetContentLength(contentLength)
	return nil
}

// WriteTo writes request to w. It implements io.WriterTo.
func (req *Request) WriteTo(w io.Writer) (int64, error) {
	return writeBufio(req, w)
}

func (req *Request) onlyMultipartForm() bool {
	return req.multipartForm != nil && (req.body == nil || len(req.body.B) == 0)
}

// Write writes request to w.
//
// Write doesn't flush request to w for performance reasons.
//
// See also WriteTo.
func (req *Request) Write(w *bufio.Writer) error {
	if len(req.Header.Host()) == 0 || req.parsedURI {
		uri := req.URI()
		host := uri.Host()
		if len(req.Header.Host()) == 0 {
			if len(host) == 0 {
				return errRequestHostRequired
			}
			req.Header.SetHostBytes(host)
		} else if !req.UseHostHeader {
			req.Header.SetHostBytes(host)
		}
		req.Header.SetRequestURIBytes(uri.RequestURI())
		if len(uri.Username()) > 0 {
			nl := len(uri.Username()) + len(uri.Password()) + 1
			nb := nl + len(zerocopy.StrBasicSpace)
			tl := nb + base64.StdEncoding.EncodedLen(nl)
			if tl > cap(req.Header.bufV) {
				req.Header.bufV = make([]byte, 0, tl)
			}
			buf := req.Header.bufV[:0]
			buf = append(buf, uri.Username()...)
			buf = append(buf, zerocopy.StrColon...)
			buf = append(buf, uri.Password()...)
			buf = append(buf, zerocopy.StrBasicSpace...)
			base64.StdEncoding.Encode(buf[nb:tl], buf[:nl])
			req.Header.SetBytesKV(zerocopy.StrAuthorization, buf[nl:tl])
		}
	}
	if req.bodyStream != nil {
		return req.writeBodyStream(w)
	}
	body := req.bodyBytes()
	var err error
	if req.onlyMultipartForm() {
		body, err = marshalMultipartForm(req.multipartForm, req.multipartFormBoundary)
		if err != nil {
			return fmt.Errorf("error when marshaling multipart form: %w", err)
		}
		req.Header.SetMultipartFormBoundary(req.multipartFormBoundary)
	}
	hasBody := false
	if len(body) == 0 {
		body = req.postArgs.QueryString()
	}
	if len(body) != 0 || !req.Header.ignoreBody() {
		hasBody = true
		req.Header.SetContentLength(len(body))
	}
	if err = req.Header.Write(w); err != nil {
		return err
	}
	if hasBody {
		_, err = w.Write(body)
	} else if len(body) > 0 {
		if req.SecureErrorLogMessage {
			return errors.New("non-zero body for non-post request")
		}
		return fmt.Errorf("non-zero body for non-post request: body=%q", body)
	}
	return err
}

// WriteVectored writes the request headers and body directly to conn via net.Buffers (vectored I/O)
// without intermediate copying into a bufio.Writer.
func (req *Request) WriteVectored(conn net.Conn) error {
	if len(req.Header.Host()) == 0 || req.parsedURI {
		uri := req.URI()
		host := uri.Host()
		if len(req.Header.Host()) == 0 {
			if len(host) == 0 {
				return errRequestHostRequired
			}
			req.Header.SetHostBytes(host)
		} else if !req.UseHostHeader {
			req.Header.SetHostBytes(host)
		}
		req.Header.SetRequestURIBytes(uri.RequestURI())
	}
	if req.bodyStream != nil {
		bw := acquireBufioWriter(conn)
		err := req.writeBodyStream(bw)
		if err == nil {
			err = bw.Flush()
		}
		releaseBufioWriter(bw)
		return err
	}
	body := req.bodyBytes()
	var err error
	if req.onlyMultipartForm() {
		body, err = marshalMultipartForm(req.multipartForm, req.multipartFormBoundary)
		if err != nil {
			return fmt.Errorf("error when marshaling multipart form: %w", err)
		}
		req.Header.SetMultipartFormBoundary(req.multipartFormBoundary)
	}
	hasBody := false
	if len(body) == 0 {
		body = req.postArgs.QueryString()
	}
	if len(body) != 0 || !req.Header.ignoreBody() {
		hasBody = true
		req.Header.SetContentLength(len(body))
	}
	headerBytes := req.Header.Header()
	if !hasBody || len(body) == 0 {
		_, err = conn.Write(headerBytes)
		return err
	}
	bufs := net.Buffers{headerBytes, body}
	_, err = bufs.WriteTo(conn)
	return err
}

func (req *Request) writeBodyStream(w *bufio.Writer) error {
	var err error
	contentLength := req.Header.ContentLength()
	if contentLength < 0 {
		lrSize := limitedReaderSize(req.bodyStream)
		if lrSize >= 0 {
			contentLength = int(lrSize)
			if int64(contentLength) != lrSize {
				contentLength = -1
			}
			if contentLength >= 0 {
				req.Header.SetContentLength(contentLength)
			}
		}
	}
	if contentLength >= 0 {
		if err = req.Header.Write(w); err == nil {
			err = writeBodyFixedSize(w, req.bodyStream, int64(contentLength))
		}
	} else {
		req.Header.SetContentLength(-1)
		err = req.Header.Write(w)
		if err == nil {
			err = writeBodyChunked(w, req.bodyStream)
		}
		if err == nil {
			err = req.Header.writeTrailer(w)
		}
	}
	errc := req.closeBodyStream()
	if err == nil {
		err = errc
	}
	return err
}

func (req *Request) closeBodyStream() error {
	if req.bodyStream == nil {
		return nil
	}
	var err error
	if bsc, ok := req.bodyStream.(io.Closer); ok {
		err = bsc.Close()
	}
	if rs, ok := req.bodyStream.(*RequestStream); ok {
		ReleaseRequestStream(rs)
	}
	req.bodyStream = nil
	return err
}

// String returns request representation.
//
// Returns error message instead of request representation on error.
//
// Use Write instead of String for performance-critical code.
func (req *Request) String() string {
	return getHTTPString(req)
}

// SetTimeout sets timeout for the request.
//
// The following code:
//
//	req.SetTimeout(t)
//	c.Do(&req, &resp)
//
// is equivalent to
//
//	c.DoTimeout(&req, &resp, t)
func (req *Request) SetTimeout(t time.Duration) {
	req.Timeout = t
}

// BodyScoped borrows the request body without memory allocation.
func (req *Request) BodyScoped(s *borrow.Scope) borrow.Bytes {
	b := req.Body()
	if len(b) == 0 {
		return borrow.Bytes{}
	}
	return borrow.NewBytes(b, nil)
}

// ReadBodyScoped executes fn with the underlying request body buffer borrowed for the duration of the call.
func (req *Request) ReadBodyScoped(fn func([]byte) error) error {
	s := borrow.AcquireScope()
	defer s.Release()
	b := req.Body()
	return fn(b)
}
