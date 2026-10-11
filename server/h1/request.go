// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	protoh1 "github.com/lemon4ksan/mach/proto/h1"
	"github.com/lemon4ksan/mach/proto/headkit"
)

var (
	// ErrMalformedRequestLine is returned when the request-line fails RFC 9112 §3 syntax rules.
	ErrMalformedRequestLine = errors.New("h1: malformed request line (RFC 9112 §3)")

	// ErrUnsupportedProtocol is returned when the request protocol is not HTTP/1.0 or HTTP/1.1 (RFC 9112 §2.3).
	ErrUnsupportedProtocol = errors.New("h1: unsupported protocol version (RFC 9112 §2.3)")

	// ErrBodyTooLarge is returned when the request body exceeds MaxBodySize (RFC 9110 §15.5.14).
	ErrBodyTooLarge = errors.New("h1: request body exceeds maximum allowed size (RFC 9110 §15.5.14)")

	// ErrMissingHostHeader is returned when an HTTP/1.1 request lacks a Host header (RFC 9112 §3.2).
	ErrMissingHostHeader = errors.New("h1: missing host header in HTTP/1.1 request (RFC 9112 §3.2)")

	// ErrUnsupportedTransferEncoding is returned when Transfer-Encoding does not end with chunked (RFC 9112 §6.3).
	ErrUnsupportedTransferEncoding = errors.New("h1: request transfer-encoding must end with chunked (RFC 9112 §6.3)")

	// ErrHijackNotSupported is returned when connection hijacking is attempted on a non-hijackable socket (RFC 9110 §15.2.2).
	ErrHijackNotSupported = errors.New("h1: hijacking not supported on this connection")

	// ErrWhitespaceBeforeColon is returned when whitespace occurs between header name and colon (RFC 9112 §5.1).
	ErrWhitespaceBeforeColon = errors.New("h1: whitespace between header name and colon prohibited (RFC 9112 §5.1)")

	// ErrDuplicateContentLength is returned when duplicate conflicting Content-Length headers are present (RFC 9112 §6.3).
	ErrDuplicateContentLength = errors.New("h1: duplicate conflicting content-length headers (RFC 9112 §6.3)")

	// ErrMultipleHostHeaders is returned when multiple Host headers are present (RFC 9112 §3.2).
	ErrMultipleHostHeaders = errors.New("h1: multiple host headers prohibited (RFC 9112 §3.2)")

	// ErrInvalidContentLength is returned when Content-Length does not consist of only digits (RFC 9112 §6.3).
	ErrInvalidContentLength = errors.New("h1: invalid content-length (RFC 9112 §6.3)")
)

// Request holds parsed HTTP/1.1 request data (RFC 9112, RFC 9110).
type Request struct {
	Conn            net.Conn
	Method          string
	URI             string
	Path            string
	Query           string
	Proto           string
	Host            string
	Headers         headkit.Headers
	Body            []byte
	RemoteAddr      string
	TLS             *tls.ConnectionState
	HijackFn        func() (net.Conn, *bufio.ReadWriter, error)
	EarlyHintsFn    func(h http.Header) error
	CloseConnection bool // RFC 9112 §6.3 / §11.2 forced connection close indicator

	// Internal zero-allocation state
	headerBuf  []byte
	bw         *bytesconv.ByteBuffer
	br         *bufio.Reader
	isHijacked bool
}

func (r *Request) initClosures() {
	r.EarlyHintsFn = r.writeEarlyHintsImpl
	r.HijackFn = r.hijackImpl
}

func (r *Request) writeEarlyHintsImpl(h http.Header) error {
	if r.Proto != "HTTP/1.1" || len(h) == 0 || r.Conn == nil || r.bw == nil {
		return nil
	}

	_, _ = r.bw.WriteString("HTTP/1.1 103 Early Hints\r\n")
	for k, vv := range h {
		for _, v := range vv {
			_, _ = r.bw.WriteString(k)
			_, _ = r.bw.WriteString(": ")
			_, _ = r.bw.WriteString(v)
			_, _ = r.bw.WriteString("\r\n")
		}
	}

	_, _ = r.bw.WriteString("\r\n")
	if _, err := r.bw.WriteTo(r.Conn); err != nil {
		return err
	}

	r.bw.ResetWriter(r.Conn)

	return nil
}

func (r *Request) hijackImpl() (net.Conn, *bufio.ReadWriter, error) {
	if r.isHijacked {
		return nil, nil, errors.New("h1: connection already hijacked")
	}

	r.isHijacked = true
	rw := bufio.NewReadWriter(r.br, bufio.NewWriter(r.Conn))

	return r.Conn, rw, nil
}

// WriteEarlyHints sends an intermediate 103 Early Hints informational response to the client (RFC 8297).
func (r *Request) WriteEarlyHints(h http.Header) error {
	if r.EarlyHintsFn != nil {
		return r.EarlyHintsFn(h)
	}

	return r.writeEarlyHintsImpl(h)
}

// Reset clears the request structure for reuse.
func (r *Request) Reset() {
	if r.EarlyHintsFn == nil || r.HijackFn == nil {
		r.initClosures()
	}

	r.Method = ""
	r.URI = ""
	r.Path = ""
	r.Query = ""
	r.Proto = ""
	r.Host = ""
	r.Headers.Reset()

	r.Body = r.Body[:0]
	if cap(r.Body) > 64*1024 {
		r.Body = make([]byte, 0, 1024)
	}

	r.headerBuf = r.headerBuf[:0]
	if cap(r.headerBuf) > 64*1024 {
		r.headerBuf = make([]byte, 0, 1024)
	}

	r.RemoteAddr = ""
	r.TLS = nil
	r.CloseConnection = false
	r.bw = nil
	r.br = nil
	r.isHijacked = false
}

// Hijack takes over the raw network connection from the server (RFC 9110 §15.2.2).
func (r *Request) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if r.HijackFn != nil {
		return r.HijackFn()
	}

	if r.Conn == nil {
		return nil, nil, ErrHijackNotSupported
	}

	return r.hijackImpl()
}

// ReadRequest parses an incoming HTTP/1.1 request from the buffered stream using SIMD acceleration.
//
// Standards Conformance:
//   - RFC 9112 §2.2 (Message Parsing & Leading CRLF Robustness)
//   - RFC 9112 §3.2 (Request Target & Host Header Enforcement)
//   - RFC 9112 §6.3 (Message Body Length & Request Smuggling Protection)
//   - RFC 9931 §4 & §8 (Security Considerations for Optimistic Transitions)
func (r *Request) ReadRequest(br *bufio.Reader, bw *bytesconv.ByteBuffer, maxBodySize int64) error {
	// 1. Fast SIMD Path: Check if complete header block (\r\n\r\n) is already in read buffer
	if br.Buffered() < 4 {
		_, _ = br.Peek(4)
	}

	buffered := br.Buffered()
	if buffered >= 4 {
		peekBytes, err := br.Peek(buffered)
		if err == nil || err == io.EOF {
			startIdx := 0
			for startIdx < len(peekBytes) && (peekBytes[startIdx] == '\r' || peekBytes[startIdx] == '\n') {
				startIdx++
			}

			if startIdx < len(peekBytes) {
				headerEnd := protoh1.IndexCRLFCRLF(peekBytes[startIdx:])
				if headerEnd != -1 {
					headerEnd += startIdx
					headerBlock := peekBytes[startIdx : headerEnd-4]
					_, _ = br.Discard(headerEnd)

					if err := r.parseHeaderBlock(headerBlock); err != nil {
						return err
					}

					return r.finishRequestRead(br, bw, maxBodySize)
				}
			}
		}
	}

	// 2. Fallback Streaming Path: Read line by line
	r.headerBuf = r.headerBuf[:0]
	for {
		line, err := br.ReadSlice('\n')
		if err != nil && err != io.EOF {
			return err
		}

		trimmed := bytes.TrimRight(line, "\r\n")
		if len(trimmed) == 0 {
			if err == io.EOF {
				return io.EOF
			}

			continue
		}

		r.headerBuf = append(r.headerBuf, trimmed...)
		reqLine := r.headerBuf[:len(trimmed)]
		if err := r.parseRequestLine(reqLine); err != nil {
			return err
		}

		break
	}

	headerStart := len(r.headerBuf)
	for {
		headerLine, err := br.ReadSlice('\n')
		if err != nil && err != io.EOF {
			return err
		}

		r.headerBuf = append(r.headerBuf, headerLine...)

		trimmed := bytes.TrimRight(headerLine, "\r\n")
		if len(trimmed) == 0 {
			break
		}

		if err == io.EOF {
			break
		}
	}

	r.Headers.ParseHeaderBlockSWAR(r.headerBuf[headerStart:])

	if err := r.Headers.Err(); err != nil {
		return mapHeaderError(err)
	}

	return r.finishRequestRead(br, bw, maxBodySize)
}

func (r *Request) parseHeaderBlock(headerBlock []byte) error {
	crlfIdx := protoh1.IndexByte(headerBlock, '\n')
	if crlfIdx <= 0 {
		return r.parseRequestLine(headerBlock)
	}

	reqLine := headerBlock[:crlfIdx]
	if len(reqLine) > 0 && reqLine[len(reqLine)-1] == '\r' {
		reqLine = reqLine[:len(reqLine)-1]
	}

	if err := r.parseRequestLine(reqLine); err != nil {
		return err
	}

	r.Headers.ParseHeaderBlockSWAR(headerBlock[crlfIdx+1:])

	if err := r.Headers.Err(); err != nil {
		return mapHeaderError(err)
	}

	return nil
}

func mapHeaderError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, headkit.ErrDuplicateContentLength) {
		return ErrDuplicateContentLength
	}
	if errors.Is(err, headkit.ErrMultipleHostHeaders) {
		return ErrMultipleHostHeaders
	}
	if errors.Is(err, headkit.ErrWhitespaceBeforeColon) {
		return ErrWhitespaceBeforeColon
	}
	return err
}

func (r *Request) parseRequestLine(line []byte) error {
	s1 := protoh1.IndexByte(line, ' ')
	if s1 <= 0 {
		return ErrMalformedRequestLine
	}

	rem := line[s1+1:]

	s2Offset := protoh1.IndexByte(rem, ' ')
	if s2Offset <= 0 {
		return ErrMalformedRequestLine
	}

	s2 := s1 + 1 + s2Offset

	if s2+1 > len(line) {
		return ErrMalformedRequestLine
	}

	r.Method = bytesconv.B2S(line[:s1])
	r.URI = bytesconv.B2S(line[s1+1 : s2])
	r.Proto = bytesconv.B2S(line[s2+1:])

	uriBytes := line[s1+1 : s2]
	if qIdx := protoh1.IndexByte(uriBytes, '?'); qIdx != -1 {
		r.Path = bytesconv.B2S(uriBytes[:qIdx])
		r.Query = bytesconv.B2S(uriBytes[qIdx+1:])
	} else {
		r.Path = r.URI
		r.Query = ""
	}

	return nil
}

func (r *Request) finishRequestRead(br *bufio.Reader, bw *bytesconv.ByteBuffer, maxBodySize int64) error {
	r.Host = r.Headers.Get(headkit.Host)

	// RFC 9112 §3.2: HTTP/1.1 requests MUST include a valid Host header
	if r.Proto == "HTTP/1.1" && r.Host == "" {
		return ErrMissingHostHeader
	}

	hasTE := r.Headers.Has(headkit.TransferEncoding)
	hasCL := r.Headers.Has(headkit.ContentLength)

	// Fast Path: no body payload (GET, HEAD, DELETE, OPTIONS)
	if !hasTE && !hasCL {
		return nil
	}

	return r.finishRequestBodyRead(br, bw, maxBodySize, hasTE, hasCL)
}

//go:noinline
func (r *Request) finishRequestBodyRead(
	br *bufio.Reader,
	bw *bytesconv.ByteBuffer,
	maxBodySize int64,
	hasTE, hasCL bool,
) error {
	// RFC 9112 §6.3 Item 3: If both Transfer-Encoding and Content-Length are present,
	// Transfer-Encoding overrides Content-Length to mitigate Request Smuggling (RFC 9112 §11.2).
	// RFC 9112 §11.2 mandates that the server MUST close the connection after the response.
	if hasTE && hasCL {
		r.CloseConnection = true
		r.Headers.Del(headkit.ContentLength)
	}

	// Handle "Expect: 100-continue" (RFC 9110 §10.1.1)
	if r.Proto == "HTTP/1.1" && bytesconv.EqualFoldASCII(r.Headers.Get(headkit.Expect), headkit.Value100Continue) && bw != nil {
		_, _ = bw.WriteString("HTTP/1.1 100 Continue\r\n\r\n")
		if r.Conn != nil {
			_, _ = bw.WriteTo(r.Conn)
			bw.Reset()
		}
	}

	// Read body if present
	if hasTE {
		// RFC 9112 §6.1: Verify all Transfer-Encoding entries
		teCount := 0
		var teVal string
		for k, v := range r.Headers.All() {
			if bytesconv.EqualFoldASCII(k, headkit.TransferEncoding) {
				teCount++
				teVal = v
			}
		}

		if teCount > 1 {
			return ErrUnsupportedTransferEncoding
		}

		teVal = strings.TrimSpace(teVal)
		// RFC 9112 §6.1 & §6.3: Must be exactly "chunked"
		if !strings.EqualFold(teVal, headkit.ValueChunked) {
			return ErrUnsupportedTransferEncoding
		}

		chunkedBody, err := ReadAllChunked(br, maxBodySize)
		if err != nil {
			return err
		}

		r.Body = chunkedBody

		return nil
	}

	clStr := r.Headers.Get(headkit.ContentLength)
	if clStr == "" {
		return nil
	}

	// RFC 9112 §6.3 Item 5 & 6: Validate Content-Length decimal representation
	for i := 0; i < len(clStr); i++ {
		c := clStr[i]
		if c < '0' || c > '9' {
			return fmt.Errorf("%w: %q (RFC 9112 §6.3)", ErrInvalidContentLength, clStr)
		}
	}

	contentLength, err := strconv.ParseInt(clStr, 10, 64)
	if err != nil || contentLength < 0 {
		return fmt.Errorf("%w: %q (RFC 9112 §6.3)", ErrInvalidContentLength, clStr)
	}

	if contentLength > maxBodySize {
		return ErrBodyTooLarge
	}

	if contentLength > 0 {
		if cap(r.Body) < int(contentLength) {
			r.Body = make([]byte, contentLength)
		} else {
			r.Body = r.Body[:contentLength]
		}

		if _, err := io.ReadFull(br, r.Body); err != nil {
			return err
		}
	}

	return nil
}

// ClientIP extracts the client IP address from RemoteAddr, stripping any port number.
func (r *Request) ClientIP() string {
	addr := r.RemoteAddr
	if addr == "" && r.Conn != nil {
		ra := r.Conn.RemoteAddr()
		if ra != nil {
			addr = ra.String()
		}
	}

	if addr != "" {
		host, _, err := net.SplitHostPort(addr)
		if err == nil {
			return host
		}

		return addr
	}

	return ""
}
