// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/silicon/randkit"

	"github.com/lemon4ksan/mach/proto/codec/hpack"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

var (
	// http2Preface defines the 24-octet client connection preface string (RFC 9113 §3.4: "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n").
	http2Preface = []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")
	prefaceLen   = len(http2Preface)

	// HTTP/2 Request & Response Pseudo-Headers (RFC 9113 §8.3, §8.3.1 & §8.3.2)
	StringPath          = []byte(":path")
	StringStatus        = []byte(":status")
	StringAuthority     = []byte(":authority")
	StringScheme        = []byte(":scheme")
	StringMethod        = []byte(":method")
	StringContentLength = []byte("content-length")
	StringUserAgent     = []byte("user-agent")
)

// ReadPreface verifies the 24-octet connection initialization preface from r (RFC 9113 §3.4).
func ReadPreface(r io.Reader) bool {
	b := make([]byte, prefaceLen)

	n, err := r.Read(b[:prefaceLen])

	return err == nil && n == prefaceLen && bytes.Equal(b, http2Preface)
}

// WritePreface writes the 24-octet HTTP/2 client connection preface to w (RFC 9113 §3.4).
func WritePreface(w io.Writer) error {
	_, err := w.Write(http2Preface)
	return err
}

// PerformHandshake sends the client connection preface, SETTINGS frame, and initial connection WINDOW_UPDATE (RFC 9113 §3.4, §6.5 & §6.9).
func PerformHandshake(preface bool, bw *bufio.Writer, st *Settings, maxWin int32) error {
	if preface {
		if err := WritePreface(bw); err != nil {
			return err
		}
	}

	fr := AcquireFrameHeader()
	defer ReleaseFrameHeader(fr)

	st2 := &Settings{}
	st.CopyTo(st2)
	fr.SetBody(st2)

	if _, err := fr.WriteTo(bw); err != nil {
		return err
	}

	frWin := AcquireFrameHeader()
	defer ReleaseFrameHeader(frWin)

	wu := AcquireFrame(FrameWindowUpdate).(*WindowUpdate)
	wu.SetIncrement(int(maxWin))
	frWin.SetBody(wu)

	if _, err := frWin.WriteTo(bw); err != nil {
		return err
	}

	return bw.Flush()
}

func uint24ToBytes(b []byte, n uint32) {
	_ = b[2]
	b[0] = byte((n >> 16) & 0xff) //nolint:gosec // 24-bit octet
	b[1] = byte((n >> 8) & 0xff)  //nolint:gosec // 24-bit octet
	b[2] = byte(n & 0xff)         //nolint:gosec // 24-bit octet
}

func bytesToUint24(b []byte) uint32 {
	_ = b[2]
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
}

func appendUint32Bytes(dst []byte, n uint32) []byte {
	return append(
		dst,
		byte(n>>24),
		byte((n>>16)&0xff),
		byte((n>>8)&0xff),
		byte(n&0xff),
	) //nolint:gosec // extracting octets
}

func uint32ToBytes(b []byte, n uint32) {
	_ = b[3]
	b[0] = byte(n >> 24)
	b[1] = byte((n >> 16) & 0xff) //nolint:gosec // extracting octet
	b[2] = byte((n >> 8) & 0xff)  //nolint:gosec // extracting octet
	b[3] = byte(n & 0xff)         //nolint:gosec // extracting octet
}

func bytesToUint32(b []byte) uint32 {
	_ = b[3]
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func resizeSlice(b []byte, neededLen int) []byte {
	b = b[:cap(b)]
	if n := neededLen - len(b); n > 0 {
		b = append(b, make([]byte, n)...)
	}

	return b[:neededLen]
}

func cutPadding(payload []byte, length int) ([]byte, error) {
	if len(payload) == 0 {
		return nil, ErrMissingBytes
	}

	pad := int(payload[0])
	if pad >= len(payload) || length > len(payload) || pad >= length || length-pad < 1 {
		return nil, NewGoAwayError(ProtocolError, "padding out of range (RFC 9113 §6.1)")
	}

	return payload[1 : length-pad], nil
}

func addPadding(b []byte) []byte {
	n := int(randkit.Uint32n(247)) + 9
	nn := len(b)

	b = resizeSlice(b, nn+n)
	b = append(b[:1], b...)
	b[0] = uint8(n) //nolint:gosec

	return b
}

var (
	status200 = []byte("200")
	status204 = []byte("204")
	status206 = []byte("206")
	status301 = []byte("301")
	status302 = []byte("302")
	status304 = []byte("304")
	status400 = []byte("400")
	status401 = []byte("401")
	status403 = []byte("403")
	status404 = []byte("404")
	status500 = []byte("500")
	status502 = []byte("502")
	status503 = []byte("503")
	valZero   = []byte("0")
)

func statusCodeToBytes(code int) []byte {
	switch code {
	case 200:
		return status200
	case 204:
		return status204
	case 206:
		return status206
	case 301:
		return status301
	case 302:
		return status302
	case 304:
		return status304
	case 400:
		return status400
	case 401:
		return status401
	case 403:
		return status403
	case 404:
		return status404
	case 500:
		return status500
	case 502:
		return status502
	case 503:
		return status503
	default:
		return nil
	}
}

// ToLowerCopy converts ASCII header name bytes to lowercase using a stack-allocated buffer to prevent heap allocations.
func ToLowerCopy(b []byte) []byte {
	hasUpper := false
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			hasUpper = true
			break
		}
	}

	if !hasUpper {
		return b
	}

	var (
		stackBuf [64]byte
		out      []byte
	)

	if len(b) <= len(stackBuf) {
		out = stackBuf[:len(b)]
	} else {
		out = make([]byte, len(b))
	}

	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			out[i] = b[i] + 32
		} else {
			out[i] = b[i]
		}
	}

	return out
}

// ResponseHeaders serializes response status and headers from an HTTP/1.x response ([h1.Response]) into hpack.HPACK-encoded header fields.
func ResponseHeaders(dst *Headers, hp *hpack.HPACK, res *h1.Response) {
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	hf.SetKeyBytes(StringStatus)

	code := res.Header.StatusCode()
	if b := statusCodeToBytes(code); b != nil {
		hf.SetValueBytes(b)
	} else {
		var numBuf [16]byte
		hf.SetValueBytes(strconv.AppendInt(numBuf[:0], int64(code), 10))
	}

	dst.AppendHeaderField(hp, hf, true)

	if !res.IsBodyStream() {
		res.Header.SetContentLength(len(res.Body()))
	}

	for k, v := range res.Header.All() {
		if IsConnectionSpecificHeader(k, v) {
			continue
		}

		hf.SetBytes(ToLowerCopy(k), v)
		dst.AppendHeaderField(hp, hf, false)
	}
}

// FasthttpResponseHeaders aliases ResponseHeaders for backwards compatibility.
func FasthttpResponseHeaders(dst *Headers, hp *hpack.HPACK, res *h1.Response) {
	ResponseHeaders(dst, hp, res)
}

// SerializeResponseHeaders serializes response status and headers into hpack.HPACK-encoded header fields.
func SerializeResponseHeaders(dst *Headers, hp *hpack.HPACK, statusCode int, headers http.Header, bodyLen int) {
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	var numBuf [32]byte

	hf.SetKeyBytes(StringStatus)

	if b := statusCodeToBytes(statusCode); b != nil {
		hf.SetValueBytes(b)
	} else {
		hf.SetValueBytes(strconv.AppendInt(numBuf[:0], int64(statusCode), 10))
	}

	dst.AppendHeaderField(hp, hf, false)

	if bodyLen >= 0 {
		hf.SetKeyBytes(StringContentLength)

		if bodyLen == 0 {
			hf.SetValueBytes(valZero)
		} else {
			hf.SetValueBytes(strconv.AppendInt(numBuf[:0], int64(bodyLen), 10))
		}

		dst.AppendHeaderField(hp, hf, false)
	}

	for k, vv := range headers {
		// RFC 9113 §8.2.2: Connection-specific headers MUST NOT be sent in HTTP/2.
		switch strings.ToLower(k) {
		case "connection", "transfer-encoding", "keep-alive", "upgrade":
			continue
		}

		kLower := ToLowerCopy([]byte(k))
		for _, v := range vv {
			hf.SetBytes(kLower, []byte(v))
			dst.AppendHeaderField(hp, hf, false)
		}
	}
}

// ValidateHeaderName validates that a header field name conforms to RFC 9113 §8.2.1.
// A field name MUST NOT contain characters in the ranges 0x00-0x20, 0x41-0x5a ('A'-'Z'), or 0x7f-0xff.
// With the exception of pseudo-headers, field names MUST NOT include a colon (':').
// Messages violating these rules MUST be treated as malformed (stream error of type PROTOCOL_ERROR per RFC 9113 §8.1.1).
func ValidateHeaderName(k []byte, isPseudo bool) error {
	if len(k) == 0 {
		return ProtocolError
	}

	start := 0
	if isPseudo {
		if k[0] != ':' || len(k) == 1 {
			return ProtocolError
		}

		start = 1
	}

	for i := start; i < len(k); i++ {
		c := k[i]
		if c <= 0x20 || (c >= 'A' && c <= 'Z') || c >= 0x7f || c == ':' {
			return ProtocolError
		}
	}

	return nil
}

// ValidateHeaderValue validates that a header field value conforms to RFC 9113 §8.2.1.
// A field value MUST NOT contain NUL (0x00), LF (0x0a), or CR (0x0d) at any position.
// Messages violating these rules MUST be treated as malformed (stream error of type PROTOCOL_ERROR per RFC 9113 §8.1.1).
func ValidateHeaderValue(v []byte) error {
	for i := 0; i < len(v); i++ {
		if v[i] == 0 || v[i] == '\r' || v[i] == '\n' {
			return ProtocolError
		}
	}

	return nil
}

// IsConnectionSpecificHeader reports whether a header field is connection-specific per RFC 9113 §8.2.2.
// Connection-specific header fields (Connection, Keep-Alive, Proxy-Connection, Transfer-Encoding, Upgrade,
// and TE with values other than "trailers") MUST NOT be present in HTTP/2 messages.
// Any message containing them MUST be treated as malformed (stream error of type PROTOCOL_ERROR per RFC 9113 §8.1.1).
func IsConnectionSpecificHeader(k, v []byte) bool {
	switch string(k) {
	case "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade":
		return true
	case "te":
		return string(v) != "trailers"
	default:
		return false
	}
}
