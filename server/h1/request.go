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

	coreheaders "github.com/lemon4ksan/foundation/net/headkit"
	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/simd"
)

var (
	ErrMalformedRequestLine        = errors.New("h1: malformed request line (RFC 9112 §3)")
	ErrUnsupportedProtocol         = errors.New("h1: unsupported protocol version (RFC 9112 §2.3)")
	ErrBodyTooLarge                = errors.New("h1: request body exceeds maximum allowed size (RFC 9110 §15.5.14)")
	ErrMissingHostHeader           = errors.New("h1: missing host header in HTTP/1.1 request (RFC 9112 §3.2)")
	ErrUnsupportedTransferEncoding = errors.New("h1: request transfer-encoding must end with chunked (RFC 9112 §6.3)")
	ErrHijackNotSupported          = errors.New("h1: hijacking not supported on this connection")
)

// Request holds parsed HTTP/1.1 request data without net/http wrapping.
type Request struct {
	Conn         net.Conn
	Method       string
	URI          string
	Path         string
	Query        string
	Proto        string
	Host         string
	Headers      coreheaders.Headers
	Body         []byte
	RemoteAddr   string
	TLS          *tls.ConnectionState
	HijackFn     func() (net.Conn, *bufio.ReadWriter, error)
	EarlyHintsFn func(h http.Header) error
}

// WriteEarlyHints sends an intermediate 103 Early Hints response to the client.
func (r *Request) WriteEarlyHints(h http.Header) error {
	if r.EarlyHintsFn != nil {
		return r.EarlyHintsFn(h)
	}

	return nil
}

// Reset clears the request structure for pooling.
func (r *Request) Reset() {
	r.EarlyHintsFn = nil
	r.Method = ""
	r.URI = ""
	r.Path = ""
	r.Query = ""
	r.Proto = ""
	r.Host = ""
	r.Headers.Reset()
	r.Body = r.Body[:0]
	r.RemoteAddr = ""
	r.TLS = nil
	r.HijackFn = nil
}

// Hijack takes over the raw network connection from the server.
func (r *Request) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if r.HijackFn == nil {
		return nil, nil, ErrHijackNotSupported
	}

	return r.HijackFn()
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
				headerEnd := simd.IndexCRLFCRLFVector(peekBytes[startIdx:])
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

		if err := r.parseRequestLine(trimmed); err != nil {
			return err
		}

		break
	}

	var fallbackBuf []byte
	for {
		headerLine, err := br.ReadSlice('\n')
		if err != nil && err != io.EOF {
			return err
		}

		fallbackBuf = append(fallbackBuf, headerLine...)

		trimmed := bytes.TrimRight(headerLine, "\r\n")
		if len(trimmed) == 0 {
			break
		}

		if err == io.EOF {
			break
		}
	}

	r.Headers.ParseHeaderBlockSWAR(fallbackBuf)

	return r.finishRequestRead(br, bw, maxBodySize)
}

func (r *Request) parseHeaderBlock(headerBlock []byte) error {
	crlfIdx := simd.ScanByteVector(headerBlock, '\n')
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

	return nil
}

func (r *Request) parseRequestLine(line []byte) error {
	s1, s2 := -1, -1

	_ = line[len(line)-1] // BCE
	for i := range line {
		if line[i] == ' ' {
			if s1 == -1 {
				s1 = i
			} else {
				s2 = i
				break
			}
		}
	}

	if s1 == -1 || s2 == -1 {
		return ErrMalformedRequestLine
	}

	r.Method = bytesconv.B2S(line[:s1])
	r.URI = bytesconv.B2S(line[s1+1 : s2])
	r.Proto = bytesconv.B2S(line[s2+1:])

	if qIdx := strings.IndexByte(r.URI, '?'); qIdx != -1 {
		r.Path = r.URI[:qIdx]
		r.Query = r.URI[qIdx+1:]
	} else {
		r.Path = r.URI
		r.Query = ""
	}

	return nil
}

func (r *Request) finishRequestRead(br *bufio.Reader, bw *bytesconv.ByteBuffer, maxBodySize int64) error {
	r.Host = r.Headers.Get(header.Host)

	// RFC 9112 §3.2: HTTP/1.1 requests MUST include a valid Host header
	if r.Proto == "HTTP/1.1" && r.Host == "" {
		return ErrMissingHostHeader
	}

	hasTE := r.Headers.Has(header.TransferEncoding)
	hasCL := r.Headers.Has(header.ContentLength)

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
	if hasTE && hasCL {
		r.Headers.Del(header.ContentLength)
	}

	// Handle "Expect: 100-continue" (RFC 9110 §10.1.1)
	if bytesconv.EqualFoldASCII(r.Headers.Get(header.Expect), header.Value100Continue) && bw != nil {
		_, _ = bw.WriteString("HTTP/1.1 100 Continue\r\n\r\n")
		if r.Conn != nil {
			_, _ = bw.WriteTo(r.Conn)
			bw.Reset()
		}
	}

	// Read body if present
	if hasTE {
		teVal := r.Headers.Get(header.TransferEncoding)
		// RFC 9112 §6.3 Item 4: In requests, chunked MUST be the final transfer coding
		if !strings.HasSuffix(strings.ToLower(teVal), header.ValueChunked) {
			return ErrUnsupportedTransferEncoding
		}

		chunkedBody, err := ReadAllChunked(br, maxBodySize)
		if err != nil {
			return err
		}

		r.Body = chunkedBody
	} else if clStr := r.Headers.Get(header.ContentLength); clStr != "" {
		// RFC 9112 §6.3 Item 5 & 6: Validate Content-Length decimal representation
		contentLength, err := strconv.ParseInt(clStr, 10, 64)
		if err != nil || contentLength < 0 {
			return fmt.Errorf("h1: invalid content-length %q (RFC 9112 §6.3)", clStr)
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
	}

	return nil
}

// ClientIP extracts the client IP address from remote address.
func (r *Request) ClientIP() string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return r.RemoteAddr
}
